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
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultEndpoint = "https://openapi.rakuten.co.jp/services/api/BooksBook/Search/20170404"

// 漫画（コミック）のジャンルの下のジャンル。2026-10-25 に Books Genre Search API（booksGenreId=001001）で確かめた。
//
// ジャンルごとに分けて引くのは、「発売日の新しい順」の先頭に予約が数千件並び、page の上限（100）までに
// 直近の発売日に届かなくなりかけたため（BACKLOG 11-b）。ジャンルで絞れば 1 本あたりの予約が減る。
// 判型（size=9 がコミック）で絞るのはやめた。「文庫」のジャンルの漫画は判型が文庫なので落ちる。
const (
	GenreShonen  = "001001001" // 少年
	GenreShojo   = "001001002" // 少女
	GenreSeinen  = "001001003" // 青年
	GenreLadies  = "001001004" // レディース
	GenreBunko   = "001001006" // 文庫
	GenreOtherCm = "001001012" // その他
)

// HitsPerPage は 1 ページの件数。API の上限（30）にして、ページ数（＝リクエスト数）を減らす。
const HitsPerPage = 30

// MaxPage は API が受け付ける page の上限。それより後ろの結果は取れない。
const MaxPage = 100

// minRequestInterval はリクエストの最小間隔。楽天は上限を公開しておらず、超えると 429 になる。
//
// 2026-10-25 に実際のキーで確かめたこと: ちょうど 1 秒間隔で送ると 10 本目あたりで 429 になった。
// 1.1 秒の sleep を挟んだ（応答時間を含めて 1.3〜1.5 秒間隔の）15 本は全部通った。楽天側は秒単位で数えていて、
// ちょうど 1 秒だと境目で同じ 1 秒に 2 本入るのだとみている（確かめてはいない）。余裕を見て 1.5 秒にした。
// 呼び出すのは新刊のキャッシュの取り直しだけで、1 回あたり十数本（30 秒弱）なので、待ち時間は裏で吸収できる。
const minRequestInterval = 1500 * time.Millisecond

// Book は書籍検索の 1 件のうち、新刊の欄で使う項目。
type Book struct {
	Title         string `json:"title"`
	SeriesName    string `json:"seriesName"`
	Author        string `json:"author"`
	PublisherName string `json:"publisherName"`
	ISBN          string `json:"isbn"`
	// SalesDate は「2026年10月24日」「2026年11月上旬」「2026年12月頃」のような表記のまま返す。
	// 曖昧な表記をどう扱うかは usecase の判断なので、ここでは解釈しない。
	SalesDate string `json:"salesDate"`
	ItemPrice int    `json:"itemPrice"`
	ItemURL   string `json:"itemUrl"`
	// AffiliateURL は affiliateId を渡したときだけ入る、楽天アフィリエイト経由の商品ページの URL。
	AffiliateURL  string `json:"affiliateUrl"`
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

// errorResponse は失敗時の本文。文書の形（error / error_description）と、2026-10-25 に実際に返ってきた形
// （{"errors": {"errorCode": 403, "errorMessage": "REQUEST_CONTEXT_BODY_HTTP_REFERRER_MISSING"}}）の両方を読む。
// 理由がログに出ないと、403 の原因（Referer の欠落など）が分からない。
type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Errors           struct {
		ErrorMessage string `json:"errorMessage"`
	} `json:"errors"`
}

func (e errorResponse) reason() string {
	switch {
	case e.Errors.ErrorMessage != "":
		return e.Errors.ErrorMessage
	case e.Error != "":
		return e.Error + ": " + e.ErrorDescription
	default:
		return ""
	}
}

// Client は楽天ブックス書籍検索 API への HTTP クライアント。
type Client struct {
	httpClient    *http.Client
	endpoint      string
	applicationID string
	accessKey     string
	referer       string
	origin        string
	affiliateID   string

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

// WithAffiliateID は楽天アフィリエイト ID を付ける。付けると応答の各商品に AffiliateURL が入る。
//
// 規約（第 3 条）は、楽天アフィリエイトの対象の商品ページへリンクするときはアフィリエイト経由で作るよう定めている
// （英訳で確認。BACKLOG 11-b）。本番で使うときは付けること。
func WithAffiliateID(id string) Option {
	return func(c *Client) { c.affiliateID = id }
}

// WithRequestInterval はリクエストの最小間隔を差し替える（テストで 1 秒待たないため）。
func WithRequestInterval(d time.Duration) Option {
	return func(c *Client) { c.interval = d }
}

// NewClient の referer は、楽天のアプリ設定の「許可されたウェブサイト」に登録したサイトの URL
// （"https://mirokuapp.com/"）。Referer と Origin のヘッダで送る。2026-10-25 に確かめたところ、Referer だけでは
// 同じ 403 になり、Origin も付けると通った。
//
// アプリの種類を「API/バックエンドサービス」にすると、許可は IP アドレスで登録することになるが、ECS のタスクの
// IP はデプロイや Spot の中断のたびに変わる。なので「ウェブアプリケーション」で登録し、サーバーからも
// ヘッダで送っている。2026-10-25 に確かめたところ、ヘッダが無いと 403
// （REQUEST_CONTEXT_BODY_HTTP_REFERRER_MISSING）、あれば手元のマシンからでも 200 だった。
func NewClient(applicationID, accessKey, referer string, opts ...Option) *Client {
	c := &Client{
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		endpoint:      defaultEndpoint,
		applicationID: applicationID,
		accessKey:     accessKey,
		referer:       referer,
		origin:        originOf(referer),
		interval:      minRequestInterval,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// originOf は URL から Origin ヘッダの値（scheme://host）を取り出す。読めなければ空にし、Origin は送らない。
func originOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
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

// LatestComics はジャンル genreID（Genre* の定数）の本を発売日の新しい順（-releaseDate）に並べた結果の
// page ページ目を返す。先頭には未発売の予約が来るので、日付の範囲は呼び出し側で絞る。
//
// 該当なし（404 not_found）はエラーにせず空のページとして返す。総ページ数より後ろを読んだときにも
// 起こりうるため。
func (c *Client) LatestComics(ctx context.Context, genreID string, page int) (*Page, error) {
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
		"booksGenreId":  {genreID},
		"sort":          {"-releaseDate"},
		"hits":          {strconv.Itoa(HitsPerPage)},
		"page":          {strconv.Itoa(page)},
		// 既定（0）は在庫切れを除く。発売直後に売り切れた巻こそ新刊の欄に出したいので含める
		"outOfStockFlag": {"1"},
		// elements で項目を絞ると応答は軽くなるが、pageCount のような最上位の項目まで落ちるのかが
		// 文書から読み取れない。総ページ数は二分探索に要るので、絞らずに受け取る
	}
	if c.affiliateID != "" {
		q.Set("affiliateId", c.affiliateID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("rakuten: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	// accessKey はクエリでもヘッダでも渡せる。クエリに入れると、失敗時の *url.Error や
	// アクセスログに URL ごと残るので、ヘッダで渡す
	req.Header.Set("accessKey", c.accessKey)
	req.Header.Set("Referer", c.referer)
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}

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
		// どちらの形でも読めなかったとき（2026-10-25 の 429 がそうだった）に理由が空にならないよう、本文の先頭を残す。
		// 失敗の本文に applicationId や accessKey は入らない
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		var e errorResponse
		_ = json.Unmarshal(raw, &e)
		reason := e.reason()
		if reason == "" {
			reason = strings.TrimSpace(string(raw))
		}
		return nil, fmt.Errorf("rakuten: books search failed: %s (%s)", resp.Status, reason)
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
