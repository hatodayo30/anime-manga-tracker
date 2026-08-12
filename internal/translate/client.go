// Package translate は作品モーダルのあらすじ表示用に、テキストを日本語へ機械翻訳する。
// AniListのdescriptionは英語のみで、日本語版を持たないため。
package translate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Google翻訳の非公式エンドポイント。APIキー不要で小規模な利用に使える。
// 公式にサポートされたAPIではないため、失敗時は呼び出し元で原文にフォールバックすること。
const endpoint = "https://translate.googleapis.com/translate_a/single"

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 8 * time.Second}}
}

// Translate は text を targetLang（例: "ja"）に翻訳する。
func (c *Client) Translate(ctx context.Context, text, targetLang string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	q := url.Values{
		"client": {"gtx"},
		// AniListのdescriptionは常に英語。sl:autoだと "Mushoku Tensei" のようなローマ字の
		// 作品名を含むテキストを日本語と誤検出し、翻訳がスキップされることがあるため固定する。
		"sl": {"en"},
		"tl": {targetLang},
		"dt": {"t"},
		"q":  {text},
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

	// レスポンスは [[[訳文, 原文, ...], [訳文, 原文, ...], ...], ...] という
	// ネストした配列（型がまちまち）なので、先頭要素の各セグメントの訳文だけを拾う。
	var raw []any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(raw) == 0 {
		return "", fmt.Errorf("unexpected translate response")
	}
	segments, ok := raw[0].([]any)
	if !ok {
		return "", fmt.Errorf("unexpected translate response shape")
	}

	var sb strings.Builder
	for _, seg := range segments {
		parts, ok := seg.([]any)
		if !ok || len(parts) == 0 {
			continue
		}
		translated, ok := parts[0].(string)
		if !ok {
			continue
		}
		sb.WriteString(translated)
	}

	return sb.String(), nil
}
