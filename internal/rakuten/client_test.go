package rakuten_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/rakuten"
)

// newServer は handler を返す偽の書籍検索 API に向けた Client を返す。
func newServer(t *testing.T, handler http.HandlerFunc) *rakuten.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return rakuten.NewClient("app-id", "secret-key", "https://mirokuapp.com/", rakuten.WithEndpoint(srv.URL), rakuten.WithRequestInterval(0))
}

// コミック・発売日の新しい順・在庫切れを含む・formatVersion=2 で問い合わせ、accessKey はクエリでなく
// ヘッダで渡すことを保証する。クエリに入れるとエラーやログに URL ごと残る。
func TestLatestComics_SendsQueryAndHeader(t *testing.T) {
	var got *http.Request
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = w.Write([]byte(`{"pageCount": 1, "items": []}`))
	})

	if _, err := c.LatestComics(context.Background(), 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	q := got.URL.Query()
	want := map[string]string{
		"applicationId":  "app-id",
		"size":           "9",
		"sort":           "-releaseDate",
		"hits":           "30",
		"page":           "3",
		"formatVersion":  "2",
		"outOfStockFlag": "1",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("query %s = %q, want %q", k, q.Get(k), v)
		}
	}
	if q.Has("accessKey") {
		t.Error("accessKey must not be sent as a query parameter")
	}
	if h := got.Header.Get("accessKey"); h != "secret-key" {
		t.Errorf("accessKey header = %q, want %q", h, "secret-key")
	}
	// Referer が無いと楽天は 403（REQUEST_CONTEXT_BODY_HTTP_REFERRER_MISSING）を返す（2026-10-25 に確認）
	if h := got.Header.Get("Referer"); h != "https://mirokuapp.com/" {
		t.Errorf("Referer header = %q, want %q", h, "https://mirokuapp.com/")
	}
	if h := got.Header.Get("Origin"); h != "https://mirokuapp.com" {
		t.Errorf("Origin header = %q, want %q", h, "https://mirokuapp.com")
	}
}

// formatVersion=2 の応答から本と総ページ数を読み取ることを保証する。文書の例は "items"、XML の説明は
// "Items" と揺れているので、どちらの綴りでも読めること。
func TestLatestComics_DecodesItems(t *testing.T) {
	for _, key := range []string{"items", "Items"} {
		t.Run(key, func(t *testing.T) {
			c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"count": 2, "pageCount": 2, "` + key + `": [
					{"title": "作品 3", "seriesName": "シリーズ", "author": "作者", "publisherName": "出版社",
					 "isbn": "9784000000001", "salesDate": "2026年10月24日", "itemPrice": 594,
					 "itemUrl": "https://books.rakuten.co.jp/rb/1/", "largeImageUrl": "https://thumbnail.image.rakuten.co.jp/1.jpg"}
				]}`))
			})

			pg, err := c.LatestComics(context.Background(), 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pg.PageCount != 2 {
				t.Errorf("PageCount = %d, want 2", pg.PageCount)
			}
			want := rakuten.Book{
				Title: "作品 3", SeriesName: "シリーズ", Author: "作者", PublisherName: "出版社",
				ISBN: "9784000000001", SalesDate: "2026年10月24日", ItemPrice: 594,
				ItemURL: "https://books.rakuten.co.jp/rb/1/", LargeImageURL: "https://thumbnail.image.rakuten.co.jp/1.jpg",
			}
			if len(pg.Items) != 1 || pg.Items[0] != want {
				t.Errorf("Items = %+v, want [%+v]", pg.Items, want)
			}
		})
	}
}

// 該当なし（404 not_found）はエラーにせず空のページにすることを保証する。総ページ数より後ろを
// 読んだときにも返りうるので、エラーにすると新刊の取得全体が失敗する。
func TestLatestComics_NotFoundIsEmpty(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "not_found", "error_description": "not found"}`))
	})

	pg, err := c.LatestComics(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pg.Items == nil || len(pg.Items) != 0 {
		t.Errorf("Items = %#v, want empty non-nil slice", pg.Items)
	}
}

// 429 などの失敗はエラーにし、メッセージに楽天のエラーコードを含め、キーは含めないことを保証する。
func TestLatestComics_ErrorStatus(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": "too_many_requests", "error_description": "number of allowed requests has been exceeded"}`))
	})

	_, err := c.LatestComics(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "too_many_requests") {
		t.Errorf("error %q should contain the rakuten error code", err)
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Errorf("error %q must not contain the access key", err)
	}
}

// 接続の失敗のエラーに URL（applicationId を含む）を載せないことを保証する。
// *url.Error をそのまま包むと、ログに applicationId が残る。
func TestLatestComics_TransportErrorOmitsURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := srv.URL
	srv.Close() // 閉じたサーバーに向けて接続を失敗させる

	c := rakuten.NewClient("app-id", "secret-key", "https://mirokuapp.com/", rakuten.WithEndpoint(endpoint), rakuten.WithRequestInterval(0))
	_, err := c.LatestComics(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "app-id") {
		t.Errorf("error %q must not contain the application id", err)
	}
}

// API が受け付けない page は送らずにエラーにすることを保証する。
func TestLatestComics_RejectsOutOfRangePage(t *testing.T) {
	calls := 0
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) { calls++ })

	for _, page := range []int{0, rakuten.MaxPage + 1} {
		if _, err := c.LatestComics(context.Background(), page); err == nil {
			t.Errorf("page %d: expected error", page)
		}
	}
	if calls != 0 {
		t.Errorf("expected no requests, got %d", calls)
	}
}

// 実際の楽天が返す形（{"errors": {"errorMessage": ...}}）の失敗でも、理由をエラーに含めることを保証する。
// 含めないと、403 の原因（Referer の欠落など）がログから分からない。
func TestLatestComics_ErrorStatusNewShape(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":{"errorCode":403,"errorMessage":"REQUEST_CONTEXT_BODY_HTTP_REFERRER_MISSING"}}`))
	})

	_, err := c.LatestComics(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "REQUEST_CONTEXT_BODY_HTTP_REFERRER_MISSING") {
		t.Fatalf("expected error with the rakuten reason, got %v", err)
	}
}

// どちらの形でも読めない失敗の本文は、先頭をそのまま理由に入れることを保証する。2026-10-25 の 429 では
// 理由が空（"(: )"）になり、原因が分からなかった。
func TestLatestComics_ErrorStatusUnknownShape(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`Too many requests`))
	})

	_, err := c.LatestComics(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "Too many requests") {
		t.Fatalf("expected error with the raw body, got %v", err)
	}
}
