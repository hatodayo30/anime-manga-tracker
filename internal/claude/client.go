// Package claude は Anthropic の Claude API を呼び出すクライアント。usecase が定義した
// インターフェース（translate.AIGateway など）を実装する。
//
// Claude は作品データの取得元には使わない。事実（作品・話数・放送日・ID）は AniList / Jikan から取り、
// Claude は受け取った文章の加工だけを担う（BACKLOG 9 の前提）。
//
// 用途ごとのプロンプトとスキーマは用途ごとのファイル（translate.go など）に置き、呼び出しは
// すべて generate を通す。日次上限・stop_reason の扱い・プロンプトのタグ名の漏れの検出を
// 用途ごとに書き分けると、どれかで抜けたときに請求か配信される出力のどちらかが壊れるため。
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// model は Haiku 5.5。どの用途も「渡した文章の加工」で高い推論力は要らず、
// 単価は Opus 5.5 の 1/40（BACKLOG 9「決めたこと」）。
const model = "claude-haiku-5-5"

// defaultMaxOutputTokens は request.maxTokens を指定しなかったときの出力の上限。
// 翻訳の見積もり（translate.go の maxTranslateTokens）に合わせた値で、どの用途も出力は数千トークンに収まる。
const defaultMaxOutputTokens = 4096

// requestTimeout と maxRetries は、サーバーの WriteTimeout（60 秒、cmd/server/main.go）より
// 十分短く終わるように決めた。SDK の既定（10 分・再試行 2 回）のままだと、Anthropic が遅いときに
// 従来の経路（MyMemory など）へのフォールバックが間に合わず、利用者には空の応答が返る。
const (
	requestTimeout = 20 * time.Second
	maxRetries     = 1
)

// ErrBudgetExceeded は今日の日次上限を使い切ったことを表す。usecase はこれを他の失敗と同じく
// フォールバックの合図として扱う。
var ErrBudgetExceeded = errors.New("claude: daily token budget exceeded")

// Client は全用途で 1 つを共有する。Budget はプロセス全体の上限なので、用途ごとに Client を
// 作ると上限がその数だけ緩む。
type Client struct {
	api    anthropic.Client
	budget *Budget
}

// NewClient の opts は既定の設定の後に適用される。テストが option.WithBaseURL で
// 偽のサーバーに向けるためのもので、本番では渡さない。
func NewClient(apiKey string, budget *Budget, opts ...option.RequestOption) *Client {
	base := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithRequestTimeout(requestTimeout),
		option.WithMaxRetries(maxRetries),
	}
	return &Client{
		api:    anthropic.NewClient(append(base, opts...)...),
		budget: budget,
	}
}

// request は構造化出力で Haiku を 1 回呼ぶための材料。
type request struct {
	// name はエラーメッセージに出す用途名（"translate" など）。
	name   string
	system string
	// user は利用者や外部 API から来た信用できない入力を含むメッセージ。入力は promptTags の
	// タグで囲み、system でタグの中身に従わないよう指示しておく。
	user string
	// schema は応答の JSON スキーマ。自由なテキストで返させると Haiku は前置きや規則への
	// 対応を書き出すので（9-a の比較）、どの用途も出力を入れる欄だけを持つスキーマで縛る。
	schema map[string]any
	// promptTags は user で入力を囲んだタグ名（"title" など、山括弧なし）。
	// 出力の文字列にこれが出ていたら、出力ではなくプロンプトへの言及を書いた印としてエラーにする。
	promptTags []string
	// maxTokens は 0 なら defaultMaxOutputTokens。
	maxTokens int64
}

// generate は req で Haiku を呼び、応答の JSON を out にデコードする。
//
// 次の場合はエラーを返す。どれも呼び出し側は「従来の経路に落とす」合図として扱う前提で、
// 部分的に使える応答を救うことはしない。
//   - 日次上限を使い切っている（ErrBudgetExceeded。API は呼ばない）
//   - stop_reason が end_turn 以外。max_tokens で切れた JSON は壊れているか途中までで、
//     Haiku 5.5 にはサーバー側の refusal フォールバックが無い（BACKLOG 9「決めたこと」）
//   - 出力のどこかの文字列にプロンプトのタグ名が含まれる
func (c *Client) generate(ctx context.Context, req request, out any) error {
	if !c.budget.Allow() {
		return ErrBudgetExceeded
	}

	maxTokens := req.maxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxOutputTokens
	}
	resp, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: req.system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.user)),
		},
		// どの用途も渡したデータの加工で推論は要らないので思考を切り、出力トークン（＝費用の大半）を
		// 答えだけにする。Haiku 5.5 で思考を切れるのは effort が high 以下のときだけなので low と組にする。
		// 9-a の比較では構造化出力にしたあと思考のオン・オフで質に差は無かった。
		Thinking: anthropic.ThinkingConfigParamUnion{OfDisabled: &anthropic.ThinkingConfigDisabledParam{}},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: req.schema},
		},
	})
	if err != nil {
		return fmt.Errorf("claude: %s: %w", req.name, err)
	}

	// 失敗した応答にも課金されるので、中身を見る前に使用量を数える。プロンプトキャッシュの
	// 読み書きは input_tokens と別に数えられるので足しておく（単価は違うが、上限の目安としては十分）
	u := resp.Usage
	c.budget.Add(u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens + u.OutputTokens)

	switch resp.StopReason {
	case anthropic.StopReasonEndTurn:
	case anthropic.StopReasonRefusal:
		return fmt.Errorf("claude: %s refused (category %q)", req.name, resp.StopDetails.Category)
	default:
		// max_tokens で切れた出力を返すと、翻訳ならそのまま保存されて全員に配られ続ける
		return fmt.Errorf("claude: %s stopped with %q", req.name, resp.StopReason)
	}

	var sb strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	raw := []byte(sb.String())

	// タグ名の検出は、out の型を知らなくても全部の文字列を見られるよう汎用の形で一度デコードして行う
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return fmt.Errorf("claude: decode %s: %w", req.name, err)
	}
	if s, ok := findPromptTag(generic, req.promptTags); ok {
		return fmt.Errorf("claude: %s output contains prompt tags: %.80q", req.name, s)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("claude: decode %s: %w", req.name, err)
	}
	return nil
}

// findPromptTag は v に含まれる文字列のうち、"<tag>" を含む最初のものを返す。
//
// タグ名が出力に出ているのは、出力ではなく「<title> の作品名は〜として扱います」のような
// 前置きを書いた印（9-a で実際に出た）。構造化出力にしてからは出ていないが、翻訳は保存して
// 全員に配り、おすすめの理由も画面に出るので、どの用途でも弾いておく。
func findPromptTag(v any, tags []string) (string, bool) {
	switch v := v.(type) {
	case string:
		for _, tag := range tags {
			if strings.Contains(v, "<"+tag+">") {
				return v, true
			}
		}
	case []any:
		for _, e := range v {
			if s, ok := findPromptTag(e, tags); ok {
				return s, true
			}
		}
	case map[string]any:
		for _, e := range v {
			if s, ok := findPromptTag(e, tags); ok {
				return s, true
			}
		}
	}
	return "", false
}
