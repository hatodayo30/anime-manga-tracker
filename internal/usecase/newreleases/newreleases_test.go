package newreleases_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/rakuten"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/newreleases"
)

var jst = time.FixedZone("JST", 9*60*60)

// fakeGateway は各ジャンルの本を「発売日の新しい順」に並んだ全件とみなし、30 件ずつのページで返す。
// pageCount は楽天と同じく 100 で頭打ちにする。呼ばれたページを記録する。
//
// books は少年のジャンルの本（多くのテストは 1 ジャンルで足りる）。byGenre を使えばジャンルごとに決められる。
type fakeGateway struct {
	books   []rakuten.Book
	byGenre map[string][]rakuten.Book
	err     error

	pages      []int // 少年のジャンルで呼ばれたページ
	totalCalls int
}

func (f *fakeGateway) LatestComics(_ context.Context, genreID string, page int) (*rakuten.Page, error) {
	f.totalCalls++
	if genreID == rakuten.GenreShonen {
		f.pages = append(f.pages, page)
	}
	if f.err != nil {
		return nil, f.err
	}
	books := f.byGenre[genreID]
	if f.byGenre == nil && genreID == rakuten.GenreShonen {
		books = f.books
	}
	pageCount := min((len(books)+rakuten.HitsPerPage-1)/rakuten.HitsPerPage, rakuten.MaxPage)
	start := (page - 1) * rakuten.HitsPerPage
	if start >= len(books) {
		return &rakuten.Page{PageCount: pageCount, Items: []rakuten.Book{}}, nil
	}
	end := min(start+rakuten.HitsPerPage, len(books))
	return &rakuten.Page{PageCount: pageCount, Items: books[start:end]}, nil
}

// book は ISBN を連番で振った本を作る。
func book(n int, salesDate string) rakuten.Book {
	return rakuten.Book{Title: fmt.Sprintf("本 %d", n), ISBN: fmt.Sprintf("978%010d", n), SalesDate: salesDate}
}

// daily は from から 1 日ずつさかのぼり、各日に perDay 冊ずつ並べる（発売日の新しい順）。
func daily(next *int, from time.Time, days, perDay int) []rakuten.Book {
	var out []rakuten.Book
	for d := range days {
		date := from.AddDate(0, 0, -d)
		for range perDay {
			*next++
			out = append(out, book(*next, fmt.Sprintf("%d年%02d月%02d日", date.Year(), date.Month(), date.Day())))
		}
	}
	return out
}

func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

// load は Warm で一覧を取り終えてから Comics を返す。Comics は値が無いと取得を待たずに Loading を返すので
// （WriteTimeout を超えないため）、取得の結果を見るテストはこれを通す。
func load(t *testing.T, u *newreleases.Usecase) (*newreleases.Result, error) {
	t.Helper()
	if err := u.Warm(context.Background()); err != nil {
		return nil, err
	}
	return u.Comics(context.Background())
}

// 2026-10-25 10:00 JST
var now = time.Date(2026, 10, 25, 10, 0, 0, 0, jst)

// キーが無い（gateway が nil）ときはエラーにせず Configured=false と空の items を返すことを保証する。
// 画面は「未設定」を出すので、items は null でなく空配列にする。
func TestComics_NotConfigured(t *testing.T) {
	u := newreleases.NewUsecase(nil, fixedNow(now))

	got, err := u.Comics(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Configured || got.Items == nil || len(got.Items) != 0 {
		t.Errorf("got %+v, want Configured=false and empty items", got)
	}
	if err := u.Warm(context.Background()); err != nil {
		t.Errorf("Warm should be a no-op without a gateway, got %v", err)
	}
}

// 今日を含む直近 7 日に発売されたものだけを返し、予約（未来）と 8 日以上前のものは除くことを保証する。
// 曖昧な発売日のうち「日頃」は使って印を付け、日の無いものや「以降」は捨てる。
func TestComics_FiltersWindowAndAmbiguousDates(t *testing.T) {
	n := 0
	var books []rakuten.Book
	// 先頭は日の決まっていない予約と、来月の予約
	books = append(books, book(1001, "2027年01月"), book(1002, "2026年12月上旬"))
	books = append(books, daily(&n, time.Date(2026, 11, 30, 0, 0, 0, 0, jst), 30, 3)...)
	books = append(books, daily(&n, time.Date(2026, 10, 31, 0, 0, 0, 0, jst), 6, 2)...) // 10/26〜10/31（未来）
	books = append(books, daily(&n, now, 7, 2)...)                                      // 10/19〜10/25（期間内）
	books = append(books,
		book(2001, "2026年10月19日頃"),  // 期間内・おおよそ
		book(2002, "2026年10月20日以降"), // 下限だけなので捨てる
		book(2003, "2026年10月中旬"),    // 日が無いので捨てる
		book(2004, "2026年02月30日"),   // 存在しない日付なので捨てる
	)
	books = append(books, daily(&n, time.Date(2026, 10, 18, 0, 0, 0, 0, jst), 20, 3)...) // 期間より前

	u := newreleases.NewUsecase(&fakeGateway{books: books}, fixedNow(now))
	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !got.Configured || got.From != "2026-10-19" || got.To != "2026-10-25" || got.Truncated {
		t.Errorf("got Configured=%v From=%q To=%q Truncated=%v", got.Configured, got.From, got.To, got.Truncated)
	}
	if len(got.Items) != 7*2+1 {
		t.Fatalf("got %d items, want %d: %+v", len(got.Items), 7*2+1, got.Items)
	}
	if got.Items[0].SalesDate != "2026-10-25" {
		t.Errorf("first item date = %q, want newest first", got.Items[0].SalesDate)
	}
	last := got.Items[len(got.Items)-1]
	if last.ISBN != book(2001, "").ISBN || last.SalesDate != "2026-10-19" || !last.SalesDateApproximate {
		t.Errorf("last item = %+v, want the approximate 10/19 book", last)
	}
	for _, it := range got.Items[:len(got.Items)-1] {
		if it.SalesDateApproximate {
			t.Errorf("item %+v should not be approximate", it)
		}
	}
}

// 予約が何十ページも並んでいても、先頭から順に読まず二分探索で期間に届くことを保証する。
// 楽天は上限が非公開で 429 を返すので、1 回の取り直しのリクエスト数を十数本に抑える。
func TestComics_BinarySearchKeepsRequestsSmall(t *testing.T) {
	n := 0
	var books []rakuten.Book
	// 未来の予約だけで 80 ページ（2,400 冊）
	books = append(books, daily(&n, time.Date(2027, 2, 28, 0, 0, 0, 0, jst), 120, 20)...)
	books = append(books, daily(&n, now, 7, 5)...)
	books = append(books, daily(&n, now.AddDate(0, 0, -7), 30, 30)...)
	gw := &fakeGateway{books: books}

	u := newreleases.NewUsecase(gw, fixedNow(now))
	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 7*5 || got.Truncated {
		t.Errorf("got %d items (truncated=%v), want %d", len(got.Items), got.Truncated, 7*5)
	}
	if len(gw.pages) > 12 {
		t.Errorf("made %d requests (%v), want at most 12", len(gw.pages), gw.pages)
	}
	seen := map[int]bool{}
	for _, p := range gw.pages {
		if seen[p] {
			t.Errorf("page %d was requested twice (%v)", p, gw.pages)
		}
		seen[p] = true
	}
}

// 期間内の件数が読み進めの上限（1 ジャンル 5 ページ）を超えたら、そこで止めて Truncated を立てることを保証する。
func TestComics_TruncatesWhenWindowIsTooLarge(t *testing.T) {
	n := 0
	books := daily(&n, now, 7, 60) // 期間内だけで 420 冊 = 14 ページ
	books = append(books, daily(&n, now.AddDate(0, 0, -7), 3, 30)...)

	u := newreleases.NewUsecase(&fakeGateway{books: books}, fixedNow(now))
	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Truncated || len(got.Items) != 150 {
		t.Errorf("got %d items (truncated=%v), want 150 and truncated", len(got.Items), got.Truncated)
	}
}

// 予約だけで page の上限（100 ページ）を埋めて期間に届かないときは、空の一覧に Truncated を立てる
// ことを保証する。「新刊は 0 冊」と区別できないと、画面が黙って空になる。
func TestComics_WindowBeyondPageLimit(t *testing.T) {
	n := 0
	books := daily(&n, time.Date(2027, 6, 30, 0, 0, 0, 0, jst), 200, 20) // 4,000 冊 > 3,000 冊

	u := newreleases.NewUsecase(&fakeGateway{books: books}, fixedNow(now))
	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Truncated || len(got.Items) != 0 {
		t.Errorf("got %d items (truncated=%v), want empty and truncated", len(got.Items), got.Truncated)
	}
}

// 結果の末尾まで読み切って期間内が 0 冊なら、Truncated は立てないことを保証する。
func TestComics_EmptyResultIsNotTruncated(t *testing.T) {
	u := newreleases.NewUsecase(&fakeGateway{}, fixedNow(now))
	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Truncated || len(got.Items) != 0 || got.Items == nil {
		t.Errorf("got %+v, want empty non-truncated", got)
	}
}

// 「今日」は JST で決めることを保証する。UTC の 10/24 15:30 は JST の 10/25 00:30。
func TestComics_TodayIsJST(t *testing.T) {
	u := newreleases.NewUsecase(&fakeGateway{}, fixedNow(time.Date(2026, 10, 24, 15, 30, 0, 0, time.UTC)))
	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.To != "2026-10-25" || got.From != "2026-10-19" {
		t.Errorf("From=%q To=%q, want 2026-10-19 .. 2026-10-25", got.From, got.To)
	}
}

// 読んでいる間に予約が増えてページの境目がずれ、同じ本が 2 ページに出ても 1 冊にまとめることを保証する。
func TestComics_DeduplicatesAcrossPages(t *testing.T) {
	n := 0
	books := daily(&n, now, 1, 30)
	books = append(books, books[29]) // 2 ページ目の先頭に 1 ページ目の末尾が再び出る
	books = append(books, daily(&n, now.AddDate(0, 0, -7), 1, 5)...)

	u := newreleases.NewUsecase(&fakeGateway{books: books}, fixedNow(now))
	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 30 {
		t.Errorf("got %d items, want 30", len(got.Items))
	}
}

// 2 回目以降はキャッシュから返し、利用者が開くたびに楽天を叩かないことを保証する。
func TestComics_Cached(t *testing.T) {
	n := 0
	gw := &fakeGateway{books: daily(&n, now, 10, 2)}
	u := newreleases.NewUsecase(gw, fixedNow(now))

	if _, err := load(t, u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	calls := len(gw.pages)
	if _, err := u.Comics(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gw.pages) != calls {
		t.Errorf("second call made %d more requests, want 0", len(gw.pages)-calls)
	}
}

// 楽天の失敗はエラーとして返すことを保証する（途中までの一覧を「新刊はこれだけ」として配らない）。
func TestComics_GatewayError(t *testing.T) {
	wantErr := errors.New("429")
	u := newreleases.NewUsecase(&fakeGateway{err: wantErr}, fixedNow(now))

	if _, err := load(t, u); !errors.Is(err, wantErr) {
		t.Errorf("got error %v, want %v", err, wantErr)
	}
}

// ジャンルごとに引いた本を発売日の新しい順にまとめ、ジャンル名を付け、ジャンルをまたいだ重複を除くことを保証する。
// 1 本にまとめて引くと、予約だけで page の上限（100）を埋めて直近の発売日に届かなくなりかけた（2026-10-25）。
func TestComics_MergesGenres(t *testing.T) {
	shared := book(9001, "2026年10月24日")
	gw := &fakeGateway{byGenre: map[string][]rakuten.Book{
		rakuten.GenreShonen: {book(1, "2026年10月23日"), shared},
		rakuten.GenreShojo:  {book(2, "2026年10月25日"), shared},
		rakuten.GenreBunko:  {book(3, "2026年10月19日")},
	}}
	u := newreleases.NewUsecase(gw, fixedNow(now))

	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var gotDates, gotGenres []string
	for _, it := range got.Items {
		gotDates = append(gotDates, it.SalesDate)
		gotGenres = append(gotGenres, it.Genre)
	}
	wantDates := []string{"2026-10-25", "2026-10-24", "2026-10-23", "2026-10-19"}
	wantGenres := []string{"少女", "少年", "少年", "文庫"}
	if fmt.Sprint(gotDates) != fmt.Sprint(wantDates) || fmt.Sprint(gotGenres) != fmt.Sprint(wantGenres) {
		t.Errorf("got dates %v genres %v, want %v %v", gotDates, gotGenres, wantDates, wantGenres)
	}
}

// アフィリエイトの URL があればそちらを商品ページの URL にすることを保証する（規約第 3 条）。
func TestComics_PrefersAffiliateURL(t *testing.T) {
	b := book(1, "2026年10月24日")
	b.ItemURL = "https://books.rakuten.co.jp/rb/1/"
	b.AffiliateURL = "https://hb.afl.rakuten.co.jp/hgc/xxx/"
	u := newreleases.NewUsecase(&fakeGateway{books: []rakuten.Book{b}}, fixedNow(now))

	got, err := load(t, u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].URL != b.AffiliateURL {
		t.Errorf("got %+v, want the affiliate URL", got.Items)
	}
}

// blockingGateway は release が閉じるまで応答しない。
type blockingGateway struct {
	release chan struct{}
}

func (g *blockingGateway) LatestComics(ctx context.Context, _ string, _ int) (*rakuten.Page, error) {
	select {
	case <-g.release:
		return &rakuten.Page{PageCount: 1, Items: []rakuten.Book{}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// 値がまだ無いときは、取得を待たずに Loading を返すことを保証する。全ジャンルの取得は 1 分を超え、
// 待たせるとサーバーの WriteTimeout（60 秒）を超える。
func TestComics_ColdCacheReturnsLoading(t *testing.T) {
	gw := &blockingGateway{release: make(chan struct{})}
	defer close(gw.release)
	u := newreleases.NewUsecase(gw, fixedNow(now))

	done := make(chan *newreleases.Result, 1)
	go func() {
		got, _ := u.Comics(context.Background())
		done <- got
	}()
	select {
	case got := <-done:
		if !got.Configured || !got.Loading || got.Items == nil {
			t.Errorf("got %+v, want Configured and Loading with empty items", got)
		}
	case <-time.After(time.Second):
		t.Fatal("Comics blocked on a cold cache")
	}
}
