// Package rakuten は楽天ブックス書籍検索 API
// （https://webservice.rakuten.co.jp/documentation/books-book-search）への最小限のクライアント。
// usecase/newreleases が定義した Gateway を実装する。
//
// AniList / Jikan には漫画の巻ごとの発売日が無いので、「新着」タブの新刊はここから取る（BACKLOG 11-b）。
package rakuten

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const defaultEndpoint = "https://openapi.rakuten.co.jp/services/api/BooksBook/Search/20170404"

// sizeComic は判型「コミック」。漫画の単行本だけに絞る。
const sizeComic = "9"

// HitsPerPage は 1 ページの件数。API の上限（30）にして、ページ数（＝リクエスト数）を減らす。
const HitsPerPage = 30

// MaxPage は API が受け付ける page の上限。それより後ろの結果は取れない。
const MaxPage = 100

// minRequestInterval はリクエストの最小間隔。楽天は上限を公開しておらず、超えると 429 になる。
// 楽天ウェブサービスで以前から目安とされている「1 アプリ ID あたり 1 秒に 1 本」に合わせる。
// 呼び出すのは新刊のキャッシュの取り直しだけで、1 回あたり十数本なので、待ち時間は裏で吸収できる。
const minRequestInterval = time.Second

// Book は書籍検索の 1 件のうち、新刊の欄で使う項目。
type Book struct {
	Title         string `json:"title"`
	SeriesName    string `json:"seriesName"`
	Author        string `json:"author"`
	PublisherName string `json:"publisherName"`
	ISBN          string `json:"isbn"`
	// SalesDate は「2026年10月24日」「2026年11月上旬」「2026年12月頃」のような表記のまま返す。
	// 曖昧な表記をどう扱うかは usecase の判断なので、ここでは解釈しない。
	SalesDate     string `json:"salesDate"`
	ItemPrice     int    `json:"itemPrice"`
	ItemURL       string `json:"itemUrl"`
	LargeImageURL string `json:"largeImageUrl"`
}

// Page は検索結果の 1 ページ。
type Page struct {
	// PageCount は総ページ数。API の仕様上 100 で頭打ちになる。
	PageCount int
	Items     []Book
}

// searchResponse は formatVersion=2（items を {"item": {...}} で包まない形）の応答。
// 文書の JSON の例は "items"、XML の説明は "Items" と大文字小文字が揺れているが、
// encoding/json のフィールド名の照合は大文字小文字を区別しないので、どちらでも読める。
type searchResponse struct {
	PageCount int    `json:"pageCount"`
	Items     []Book `json:"items"`
}

type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Client は楽天ブックス書籍検索 API への HTTP クライアント。
type Client struct {
	httpClient    *http.Client
	endpoint      string
	applicationID string
	accessKey     string

	mu       sync.Mutex
	lastCall time.Time
	interval time.Duration
}

// Option は NewClient の既定値を変える。テストが偽のサーバーに向けるためのもので、本番では渡さない。
type Option func(*Client)

// WithEndpoint は呼び出し先の URL を差し替える。
func WithEndpoint(endpoint string) Option {
	return func(c *Client) { c.endpoint = endpoint }
}

// WithRequestInterval はリクエストの最小間隔を差し替える（テストで 1 秒待たないため）。
func WithRequestInterval(d time.Duration) Option {
	return func(c *Client) { c.interval = d }
}

func NewClient(applicationID, accessKey string, opts ...Option) *Client {
	c := &Client{
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		endpoint:      defaultEndpoint,
		applicationID: applicationID,
		accessKey:     accessKey,
		interval:      minRequestInterval,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// throttle は直前の呼び出しから interval 経つまで待つ（jikan.Client と同じ形）。
func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := max(c.interval-time.Since(c.lastCall), 0)
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

// LatestComics はコミックを発売日の新しい順（-releaseDate）に並べた結果の page ページ目を返す。
// 先頭には未発売の予約が来るので、日付の範囲は呼び出し側で絞る。
//
// 該当なし（404 not_found）はエラーにせず空のページとして返す。総ページ数より後ろを読んだときにも
// 起こりうるため。
func (c *Client) LatestComics(ctx context.Context, page int) (*Page, error) {
	if page < 1 || page > MaxPage {
		return nil, fmt.Errorf("rakuten: page must be between 1 and %d (got %d)", MaxPage, page)
	}
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	q := url.Values{
		"applicationId": {c.applicationID},
		"format":        {"json"},
		"formatVersion": {"2"},
		"size":          {sizeComic},
		"sort":          {"-releaseDate"},
		"hits":          {strconv.Itoa(HitsPerPage)},
		"page":          {strconv.Itoa(page)},
		// 既定（0）は在庫切れを除く。発売直後に売り切れた巻こそ新刊の欄に出したいので含める
		"outOfStockFlag": {"1"},
		// elements で項目を絞ると応答は軽くなるが、pageCount のような最上位の項目まで落ちるのかが
		// 文書から読み取れない。総ページ数は二分探索に要るので、絞らずに受け取る
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("rakuten: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	// accessKey はクエリでもヘッダでも渡せる。クエリに入れると、失敗時の *url.Error や
	// アクセスログに URL ごと残るので、ヘッダで渡す
	req.Header.Set("accessKey", c.accessKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// *url.Error のメッセージには URL（applicationId を含む）が入るので、中身のエラーだけを返す
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("rakuten: call books search: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return &Page{Items: []Book{}}, nil
	default:
		var e errorResponse
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return nil, fmt.Errorf("rakuten: books search failed: %s (%s: %s)", resp.Status, e.Error, e.ErrorDescription)
	}

	var body searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("rakuten: decode response: %w", err)
	}
	if body.Items == nil {
		body.Items = []Book{}
	}
	return &Page{PageCount: min(body.PageCount, MaxPage), Items: body.Items}, nil
}
