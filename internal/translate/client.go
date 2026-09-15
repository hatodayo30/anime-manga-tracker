// Package translate は作品モーダルのあらすじ表示用に、テキストを日本語へ機械翻訳する。
// AniListのdescriptionは英語のみで、日本語版を持たないため。
package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// MyMemory Translation API。APIキー不要で小規模な利用に使える。
// （以前はGoogle翻訳の非公式エンドポイントを使っていたが、自動アクセスとして
// ブロックされ安定して動かなかったため切り替えた）
// 匿名利用は1リクエストあたり500バイト程度の制限があるため、長いあらすじは
// 文単位でチャンクに分割してから翻訳し、結果を結合する。
const endpoint = "https://api.mymemory.translated.net/get"

// chunkByteLimit はMyMemoryの匿名利用時の制限に収まるよう、余裕を持たせた1リクエストあたりの上限。
const chunkByteLimit = 450

var sentenceSplitPattern = regexp.MustCompile(`(?:[.!?]\s+|\n+)`)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 8 * time.Second}}
}

type mymemoryResponse struct {
	ResponseData struct {
		TranslatedText string `json:"translatedText"`
	} `json:"responseData"`
	ResponseStatus int `json:"responseStatus"`
}

// Translate は text を targetLang（例: "ja"）に翻訳する。
func (c *Client) Translate(ctx context.Context, text, targetLang string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	var sb strings.Builder
	for _, chunk := range splitIntoChunks(text, chunkByteLimit) {
		translated, err := c.translateChunk(ctx, chunk, targetLang)
		if err != nil {
			return "", err
		}
		sb.WriteString(translated)
		sb.WriteString(" ")
	}

	return strings.TrimSpace(sb.String()), nil
}

func (c *Client) translateChunk(ctx context.Context, text, targetLang string) (string, error) {
	q := url.Values{
		// AniListのdescriptionは常に英語のため、原文言語を固定する。
		"langpair": {"en|" + targetLang},
		"q":        {text},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call translate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("translate returned status %d", resp.StatusCode)
	}

	var wrapper mymemoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if wrapper.ResponseStatus != http.StatusOK {
		return "", fmt.Errorf("translate returned status %d", wrapper.ResponseStatus)
	}

	return wrapper.ResponseData.TranslatedText, nil
}

// splitIntoChunks は text を文単位で、maxBytes以下のチャンクにまとめる。
// 1文だけでmaxBytesを超える場合は、そのまま1チャンクとして送る（MyMemory側で切り詰められる可能性はあるが、
// 文の途中で機械的に切ると翻訳品質が落ちるため避ける）。
func splitIntoChunks(text string, maxBytes int) []string {
	sentences := sentenceSplitPattern.Split(text, -1)

	var chunks []string
	var current strings.Builder
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if current.Len() > 0 && current.Len()+len(s)+1 > maxBytes {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(s)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}
