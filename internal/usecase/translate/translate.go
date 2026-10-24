// Package translate は作品モーダルのあらすじを表示言語に翻訳するユースケースを担う。
package translate

import (
	"context"
	"crypto/sha256"
	"errors"
	"log"
	"strings"
)

// Gateway はフォールバックの機械翻訳を定義する。実装は internal/translate（MyMemory）。
type Gateway interface {
	Translate(ctx context.Context, text, targetLang string) (string, error)
}

// AIGateway は AI 翻訳を定義する。実装は internal/claude。
//
// Gateway と分けて title を受け取るのは、あらすじだけを渡すと AI が作品名を知らずに訳すため。
// AniList のあらすじは作品名をローマ字で書くことが多く、"Ao no Hako" を「青の箱」と直訳したり、
// "Tensei shitara Ken deshita" を似た名前の別作品に取り違えたりした（BACKLOG 9-a）。
// MyMemory は文脈を受け取れないので title を渡さない。
type AIGateway interface {
	Translate(ctx context.Context, text, title, targetLang string) (string, error)
}

// ErrNotFound は保存済みの翻訳が無いことを表す。
var ErrNotFound = errors.New("translation not found")

// Repository は AI 翻訳の保存先を定義する。実装は internal/infrastructure/postgres が提供する。
// 翻訳は利用者に紐付かない共有データなので、ユーザースコープは持たない。
type Repository interface {
	// Find は原文と作品名から作ったキー（sourceKey）と翻訳先言語で保存済みの翻訳を引く。
	// 無ければ ErrNotFound を返す。
	Find(ctx context.Context, sourceSHA256 []byte, targetLang string) (string, error)
	Save(ctx context.Context, sourceSHA256 []byte, targetLang, translated string) error
}

const defaultTargetLang = "ja"

type Usecase struct {
	repo     Repository
	ai       AIGateway
	fallback Gateway
}

// NewUsecase は ai に nil を許す。ANTHROPIC_API_KEY を設定していない環境（ローカル開発・
// CI・キーを SSM に入れる前の本番）では、従来どおり MyMemory だけで動く。
func NewUsecase(repo Repository, ai AIGateway, fallback Gateway) *Usecase {
	return &Usecase{repo: repo, ai: ai, fallback: fallback}
}

// Translate は text を targetLang に翻訳する。targetLang が空なら日本語（ja）を既定とする。
// title は text が説明している作品の表示名で、AI に作品名の表記を合わせさせるために使う。
//
// 保存済みの AI 翻訳があれば、ログインの有無にかかわらずそれを返す（費用がかからないため）。
// 無いときに AI を呼ぶのは allowAI（呼び出し元がログイン済み）の場合だけ。未ログインで AI を
// 呼べるようにすると、/api/translate を連打されたぶんだけ請求が積み上がる（BACKLOG 9）。
//
// 保存先と AI の失敗はどちらも翻訳を止めず、MyMemory に落とす（CLAUDE.md「失敗の扱い」）。
func (u *Usecase) Translate(ctx context.Context, text, title, targetLang string, allowAI bool) (string, error) {
	if targetLang == "" {
		targetLang = defaultTargetLang
	}
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	key := sourceKey(text, title)

	saved, err := u.repo.Find(ctx, key, targetLang)
	if err == nil {
		return saved, nil
	}
	if !errors.Is(err, ErrNotFound) {
		// DB 障害は未保存と同じ扱いで先へ進む。止めるほどの理由が無い
		log.Printf("translate: find saved translation: %v", err)
	}

	if allowAI && u.ai != nil {
		translated, err := u.ai.Translate(ctx, text, title, targetLang)
		if err == nil {
			// 保存に失敗しても翻訳は手元にあるので返す。次に同じ原文が来たとき AI をもう一度呼ぶだけ
			if err := u.repo.Save(ctx, key, targetLang, translated); err != nil {
				log.Printf("translate: save translation: %v", err)
			}
			return translated, nil
		}
		// 日次上限の超過・refusal・Anthropic 側の障害はいずれも MyMemory に落とす
		log.Printf("translate: ai translate failed, falling back: %v", err)
	}

	return u.fallback.Translate(ctx, text, targetLang)
}

// sourceKey は保存先のキー。原文だけでなく title も含める。
//
// title もクライアントが送ってくる値なので、原文だけをキーにすると、本物のあらすじに偽の作品名を
// 付けて送るだけで、その誤訳が同じあらすじを開く全員に配られてしまう。title を含めれば、汚せるのは
// 「同じ原文と同じ title を送った人」の結果だけになり、原文ハッシュにした理由（BACKLOG 9-a）が保たれる。
// title を固定長のハッシュにしてから連結するのは、title と text の境目をずらして
// 同じキーを作られないようにするため（区切り文字だと、その文字を title に含めればずらせる）。
func sourceKey(text, title string) []byte {
	titleSum := sha256.Sum256([]byte(title))
	h := sha256.New()
	h.Write(titleSum[:])
	h.Write([]byte(text))
	return h.Sum(nil)
}
