// Package anilist は AniList GraphQL API (https://graphql.anilist.co) への
// 最小限のクライアントを提供する。パブリックデータのみを扱うため API キーは不要。
package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const endpoint = "https://graphql.anilist.co"

// minRequestInterval はAniListの公開レート制限（2023年以降 目安 30req/分）を守るための、
// リクエスト間の最小間隔。Client経由の全呼び出しで共有する。
const minRequestInterval = 2100 * time.Millisecond

// maxRetries は429（Too Many Requests）応答時にリトライする最大回数。
const maxRetries = 3

// Client は AniList GraphQL API へのHTTPクライアント。
type Client struct {
	httpClient *http.Client

	mu       sync.Mutex
	lastCall time.Time
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// throttle はリクエストを送る前に呼び出し、直前の呼び出しからminRequestInterval経つまで待つ。
func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := max(minRequestInterval-time.Since(c.lastCall), 0)
	c.lastCall = time.Now().Add(wait)
	c.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// sleep はctxのキャンセルを尊重しつつdだけ待つ。
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryAfter はレスポンスのRetry-Afterヘッダー（秒数）を読む。無ければfallbackを返す。
func retryAfter(header http.Header, fallback time.Duration) time.Duration {
	if s := header.Get("Retry-After"); s != "" {
		if secs, err := strconv.Atoi(s); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return fallback
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type graphQLError struct {
	Message string `json:"message"`
}

func (c *Client) do(ctx context.Context, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(graphQLRequest{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	var wait time.Duration
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, wait); err != nil {
				return err
			}
		}

		if err := c.throttle(ctx); err != nil {
			return err
		}

		respBody, status, header, err := c.post(ctx, body)
		if err != nil {
			return err
		}

		if status == http.StatusTooManyRequests {
			wait = retryAfter(header, (2<<attempt)*time.Second)
			continue
		}

		var wrapper struct {
			Data   json.RawMessage `json:"data"`
			Errors []graphQLError  `json:"errors"`
		}
		if err := json.Unmarshal(respBody, &wrapper); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if len(wrapper.Errors) > 0 {
			return fmt.Errorf("anilist error: %s", wrapper.Errors[0].Message)
		}
		if err := json.Unmarshal(wrapper.Data, out); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
		return nil
	}

	return fmt.Errorf("anilist error: rate limited after %d retries", maxRetries)
}

// post は1回分のHTTPリクエストを送り、レスポンスボディ・ステータス・ヘッダーを返す。
func (c *Client) post(ctx context.Context, body []byte) ([]byte, int, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("call anilist: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("read response: %w", err)
	}
	return respBody, resp.StatusCode, resp.Header, nil
}
