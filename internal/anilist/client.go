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

// AniListの公開レート制限（2023年以降 目安 30req/分）は「直近1分あたりの本数」という窓で
// 数えられており、一定間隔での送信を強制されているわけではない。そのため固定間隔ではなく
// トークンバケットで制御し、平均レートを制限内に抑えつつ、短時間に数本が重なるケース
// （画面の初期表示や開発時の二重リクエスト）は待たせずに通す。
//
// 最悪ケースでも burstCapacity + 60s/tokenInterval = 5 + 24 = 29本/分 に収まり、
// 目安の30req/分を超えない。
const (
	// tokenInterval はトークンが1個回復するまでの時間（= 定常状態でのリクエスト間隔）。
	tokenInterval = 2500 * time.Millisecond
	// burstCapacity はアイドル時に貯めておけるトークン数（= 待たずに連続送信できる本数）。
	burstCapacity = 5
)

// maxRetries は429（Too Many Requests）応答時にリトライする最大回数。
const maxRetries = 3

// Client は AniList GraphQL API へのHTTPクライアント。
type Client struct {
	httpClient *http.Client

	mu sync.Mutex
	// allowAt はバケットを使い切る時刻。now からどれだけ手前にあるかが残トークン量を表す。
	// 全呼び出しで共有し、送信枠を先に予約することで並行呼び出しどうしも間隔を守る。
	allowAt time.Time
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// throttle はリクエストを送る前に呼び出し、トークンバケットから1枠を消費する。
// トークンが残っていれば即座に返り、尽きていれば次に回復するまで待つ。
func (c *Client) throttle(ctx context.Context) error {
	now := time.Now()

	c.mu.Lock()
	// allowAt が now から burstCapacity 個ぶん以上手前にある場合、貯まったトークンは
	// 上限で頭打ちにする（長時間アイドルでも溜め込みすぎない）。
	if earliest := now.Add(-burstCapacity * tokenInterval); c.allowAt.Before(earliest) {
		c.allowAt = earliest
	}
	c.allowAt = c.allowAt.Add(tokenInterval)
	wait := c.allowAt.Sub(now)
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

// penalize は429を受けたときに、共有バケットの再開時刻を d だけ先送りする。リトライ中の
// ゴルーチンだけでなく同時に走る他の呼び出しもまとめて待たせ、429の直後にさらにリクエストを
// 重ねてしまうのを防ぐ。
func (c *Client) penalize(d time.Duration) {
	resumeAt := time.Now().Add(d)
	c.mu.Lock()
	if c.allowAt.Before(resumeAt) {
		c.allowAt = resumeAt
	}
	c.mu.Unlock()
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
			c.penalize(wait)
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
