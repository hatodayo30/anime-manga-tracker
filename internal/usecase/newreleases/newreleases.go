// Package newreleases は「新着」タブの新刊（漫画の単行本の発売）を担う（BACKLOG 11-b）。
//
// AniList / Jikan には巻ごとの発売日が無いので楽天ブックスから取る。AniList の作品との結び付け
// （タイトル照合）はまだしていない。
package newreleases

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/rakuten"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
)

// Gateway は Usecase が新刊の取得に必要とする楽天ブックス側の操作。実装は internal/rakuten が提供する。
type Gateway interface {
	// LatestComics はジャンル genreID の本を発売日の新しい順に並べた結果の page ページ目（1 始まり）を返す。
	LatestComics(ctx context.Context, genreID string, page int) (*rakuten.Page, error)
}

// genre は新刊を引くジャンル。漫画（コミック）の下のジャンルを全部、この順で引く。
type genre struct {
	id   string
	name string
}

// genres を 1 本にまとめて引かないのは、「発売日の新しい順」の先頭に予約が数千件並び、page の上限（100）までに
// 直近の発売日に届かなくなりかけたため（2026-10-25、BACKLOG 11-b）。ジャンルで分ければ 1 本あたりの予約が減る。
var genres = []genre{
	{rakuten.GenreShonen, "少年"},
	{rakuten.GenreShojo, "少女"},
	{rakuten.GenreSeinen, "青年"},
	{rakuten.GenreLadies, "レディース"},
	{rakuten.GenreBunko, "文庫"},
	{rakuten.GenreOtherCm, "その他"},
}

// windowDays は「新刊」とする日数（今日を含む直近 7 日）。
//
// 漫画の単行本は出版社ごとに決まった日にまとめて出る（月の上旬・中旬に固まる）ので、3 日程度だと
// 大手の発売日が無い期間に欄がほぼ空になる。逆に 1 か月にすると数千件になり、楽天を数十ページ
// 読むことになるうえ「新しく出た」という感覚から外れる。1 週間なら毎回何かしら並び、
// 件数も数百件に収まる（コミックの新刊は年 1 万点余りで、1 週間で 200〜300 点ほど）。
// 未発売（予約）は含めない。「新着」タブの新連載（11-a）と同じく、出たものを見せる欄にする。
const windowDays = 7

// maxScanPages は 1 つのジャンルで期間内の結果を読み進めるページ数の上限（30 件 × 5 = 150 件）。
// 1 週間ぶんの見込み（全ジャンルで 200〜300 点）を、いちばん多いジャンルでも読み切れる数にしてある。
// 足りずに打ち切ったときは Result.Truncated を立てる。
const maxScanPages = 5

// cacheTTL は新刊の一覧を保持する時間。
//
// 発売日は日単位でしか変わらず、楽天は上限が非公開で超えると 429 になるので、利用者が開くたびには
// 叩かない。1 回の取り直しは十数本（二分探索 7 本前後 + 読み進め 10 本まで）で、1 秒に 1 本の
// 間隔だと 20 秒近くかかる。6 時間なら日付が変わってから最悪でも 6 時間で期間が切り替わる。
const cacheTTL = 6 * time.Hour

// WarmInterval は新刊の一覧を定期的に取り直す間隔。home.WarmInterval と同じく cacheTTL より短くして、
// 利用者が冷えたキャッシュ（20 秒近い待ち）を踏まないようにする。
const WarmInterval = 3 * time.Hour

// jst は発売日の「今日」の区切り。本番イメージに tzdata が無いので固定オフセット（usecase/recommend と同じ）。
var jst = time.FixedZone("JST", 9*60*60)

// Item は新刊の 1 冊。
type Item struct {
	Title         string `json:"title"`
	SeriesName    string `json:"seriesName"`
	Author        string `json:"author"`
	PublisherName string `json:"publisherName"`
	ISBN          string `json:"isbn"`
	// SalesDate は発売日（YYYY-MM-DD、JST）。
	SalesDate string `json:"salesDate"`
	// SalesDateApproximate は楽天の表記が「YYYY年MM月DD日頃」だったこと。日付は確定していないが
	// 日まで分かっているので期間の判定には使う。画面で「頃」を添えるための印。
	SalesDateApproximate bool `json:"salesDateApproximate"`
	Price                int  `json:"price"`
	// URL は商品ページ。アフィリエイト ID を設定していればアフィリエイト経由の URL（規約第 3 条、BACKLOG 11-b）。
	URL      string `json:"url"`
	ImageURL string `json:"imageUrl"`
	// Genre は楽天のジャンル名（少年・少女・青年・レディース・文庫・その他）。画面の絞り込みに使う
	Genre string `json:"genre"`
}

// Result は新刊の一覧。
type Result struct {
	// Configured は楽天のキーが設定されているか。false のときは他の項目は空で、画面は「未設定」を出す。
	// キーが無いのは利用者のリクエストの誤りでも上流の失敗でもないので、エラーにせず 200 で返す。
	Configured bool `json:"configured"`
	// From / To は対象期間（YYYY-MM-DD、JST、両端を含む）。
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// FetchedAt は楽天から取った時刻（RFC 3339）。キャッシュが最長 cacheTTL 古いので、画面で出せるように返す。
	FetchedAt string `json:"fetchedAt,omitempty"`
	// Truncated は期間内の結果を読み切れなかったこと（どれかのジャンルで maxScanPages か、API の page 上限に届いた）。
	Truncated bool `json:"truncated"`
	// Loading は一覧をまだ取り終えていないこと（起動直後など）。全ジャンルを取るのに 1 分を超え、
	// サーバーの WriteTimeout（60 秒）より長いので、待たせずにこれを返す。画面は「取得中」を出す
	Loading bool   `json:"loading,omitempty"`
	Items   []Item `json:"items"`
}

type Usecase struct {
	gateway Gateway
	now     func() time.Time
	cache   *cache.TTLCache[*Result]
}

// NewUsecase の gateway は楽天のキーが無い環境では nil にする。そのとき Comics は Configured=false を返す。
func NewUsecase(gateway Gateway, now func() time.Time) *Usecase {
	return &Usecase{
		gateway: gateway,
		now:     now,
		cache:   cache.New[*Result](cacheTTL),
	}
}

// Comics は直近 windowDays 日に発売されたコミックを発売日の新しい順に返す（キャッシュあり）。
// 新刊はユーザーに依存しないので、キャッシュは全員で共有する。
func (u *Usecase) Comics(ctx context.Context) (*Result, error) {
	if u.gateway == nil {
		return &Result{Configured: false, Items: []Item{}}, nil
	}
	if _, ok := u.cache.Peek(""); ok {
		// 取得済みの値がある。TTL 切れなら Get が古い値を返しつつ裏で取り直す
		return u.cache.Get(ctx, "", u.fetch)
	}
	// まだ値が無い（起動直後で Warm が走っている、または失敗が続いている）。取得を待たせると
	// WriteTimeout を超えるので、裏で取得を始めて「取得中」を返す。取得中のキーの Refresh は何もしない
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), backgroundFetchTimeout)
		defer cancel()
		_ = u.cache.Refresh(ctx, "", u.fetch)
	}()
	return &Result{Configured: true, Loading: true, Items: []Item{}}, nil
}

// backgroundFetchTimeout は利用者のリクエストから裏で始めた取得の打ち切り時間。全ジャンルで
// 60 本前後 × 1.5 秒の見込みに余裕を持たせた
const backgroundFetchTimeout = 5 * time.Minute

// Warm は新刊の一覧を TTL の残りに関わらず取り直す。起動直後と WarmInterval ごとに
// internal/scheduler から呼ばれる。キーが無ければ何もしない。
func (u *Usecase) Warm(ctx context.Context) error {
	if u.gateway == nil {
		return nil
	}
	return u.cache.Refresh(ctx, "", u.fetch)
}

// fetch は全ジャンルの期間内の新刊を取り、発売日の新しい順にまとめる。
//
// 失敗はどのジャンル・どのページでも全体の失敗にする。途中までの一覧を 6 時間「新刊はこれだけ」として配るより、
// キャッシュの古い値を返し続ける方がよい（cache.TTLCache は失敗を保持せず、古い値を残す）。
func (u *Usecase) fetch(ctx context.Context) (*Result, error) {
	now := u.now().In(jst)
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, jst)
	from := to.AddDate(0, 0, -(windowDays - 1))

	result := &Result{
		Configured: true,
		From:       from.Format(time.DateOnly),
		To:         to.Format(time.DateOnly),
		FetchedAt:  now.Format(time.RFC3339),
		Items:      []Item{},
	}
	// 同じ本が 2 つのジャンルに入っていることもありうるので、ジャンルをまたいで重複を除く
	seen := map[string]bool{}
	for _, g := range genres {
		items, finished, err := u.fetchGenre(ctx, g, from, to, seen)
		if err != nil {
			return nil, fmt.Errorf("genre %s: %w", g.name, err)
		}
		result.Items = append(result.Items, items...)
		if !finished {
			result.Truncated = true
		}
	}
	// ジャンルごとに発売日の新しい順なので、まとめたあと発売日で並べ直す（同じ日の中はジャンルの順のまま）
	slices.SortStableFunc(result.Items, func(a, b Item) int { return strings.Compare(b.SalesDate, a.SalesDate) })
	return result, nil
}

// fetchGenre は 1 つのジャンルの「発売日の新しい順」から、期間内の本を取り出す。finished は期間内を読み切ったか。
//
// 楽天の書籍検索には日付で絞る引数が無く、-releaseDate の先頭には未発売の予約が並ぶ。予約は
// 1〜2 か月先まであり、ジャンルで分けても千件単位になるので、先頭から順に読むと期間に届くまでに
// 数十ページかかる（page は 100 までなので、届かないこともある）。そこで発売日の並びを使って
// 「今日以前の発売日が初めて現れるページ」を二分探索で見つけ、そこから期間を抜けるまで読み進める。
// 100 ページでも二分探索は 7 本で済む。
func (u *Usecase) fetchGenre(ctx context.Context, g genre, from, to time.Time, seen map[string]bool) ([]Item, bool, error) {
	f := &pageFetcher{gateway: u.gateway, genreID: g.id, pages: map[int]*rakuten.Page{}}
	first, err := f.get(ctx, 1)
	if err != nil {
		return nil, false, err
	}
	lastPage := min(max(first.PageCount, 1), rakuten.MaxPage)

	// start は「今日以前の発売日を含む」最初のページ。見つからなければ lastPage+1
	start, end := 1, lastPage+1
	for start < end {
		mid := (start + end) / 2
		pg, err := f.get(ctx, mid)
		if err != nil {
			return nil, false, err
		}
		if reachedOnOrBefore(pg, to) {
			end = mid
		} else {
			start = mid + 1
		}
	}

	items := []Item{}
	finished := false
	page := start
	for scanned := 0; page <= lastPage && scanned < maxScanPages; scanned++ {
		pg, err := f.get(ctx, page)
		if err != nil {
			return nil, false, err
		}
		for _, b := range pg.Items {
			d, approx, ok := parseSalesDate(b.SalesDate)
			if !ok || d.Before(from) || d.After(to) {
				continue
			}
			// 読んでいる間に予約が増えるとページの境目がずれ、同じ本が 2 ページに出る
			key := b.ISBN
			if key == "" {
				key = b.ItemURL
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, toItem(b, d, approx, g.name))
		}
		page++
		if passedBefore(pg, from) {
			finished = true
			break
		}
	}
	// 結果の最後のページまで読んだなら、期間内のものは全部見ている。ただし pageCount は API の仕様で
	// 100 が頭打ちなので、100 ページ目まで読んだときは後ろがまだあるかもしれず、読み切ったとは言えない
	if page > lastPage && lastPage < rakuten.MaxPage {
		finished = true
	}
	return items, finished, nil
}

// pageFetcher は 1 回の fetch の中で同じページを 2 度取らないためのもの。二分探索の最後に見たページは
// 読み進めの最初のページでもある。
type pageFetcher struct {
	gateway Gateway
	genreID string
	pages   map[int]*rakuten.Page
}

func (f *pageFetcher) get(ctx context.Context, page int) (*rakuten.Page, error) {
	if pg, ok := f.pages[page]; ok {
		return pg, nil
	}
	pg, err := f.gateway.LatestComics(ctx, f.genreID, page)
	if err != nil {
		return nil, err
	}
	f.pages[page] = pg
	return pg, nil
}

// reachedOnOrBefore は、ページの末尾（発売日の最も古い側）が day 以前に届いているかを返す。
// 二分探索の判定で、ページを後ろへ進むほど false → true と変わる前提。
//
// 曖昧な発売日（「YYYY年MM月」など）が「発売日の新しい順」のどこに並ぶかは文書に無いので、
// 判定には日まで分かる発売日だけを使う。日まで分かる発売日が 1 つも無いページは、予約の
// 「YYYY年MM月」ばかりが並ぶ未来側とみなす。空のページは結果の末尾より後ろなので true。
func reachedOnOrBefore(pg *rakuten.Page, day time.Time) bool {
	if len(pg.Items) == 0 {
		return true
	}
	d, ok := lastDefiniteDate(pg)
	return ok && !d.After(day)
}

// passedBefore は、ページの末尾が day より前に出ていて、これ以降のページに期間内のものが無いことを返す。
func passedBefore(pg *rakuten.Page, day time.Time) bool {
	if len(pg.Items) == 0 {
		return true
	}
	d, ok := lastDefiniteDate(pg)
	return ok && d.Before(day)
}

func lastDefiniteDate(pg *rakuten.Page) (time.Time, bool) {
	for i := len(pg.Items) - 1; i >= 0; i-- {
		if d, _, ok := parseSalesDate(pg.Items[i].SalesDate); ok {
			return d, true
		}
	}
	return time.Time{}, false
}

var salesDatePattern = regexp.MustCompile(`^(\d{4})年(\d{1,2})月(\d{1,2})日(.*)$`)

// parseSalesDate は楽天の salesDate を日付にする。
//
// 扱いを決めたこと:
//   - 「YYYY年MM月DD日」は確定した発売日として使う
//   - 「YYYY年MM月DD日頃」は日まで分かっているので使い、approximate を立てる
//   - 「YYYY年MM月」「YYYY年MM月上旬」「YYYY年」など日が無いものは捨てる。直近 7 日に入るかが
//     決められず、実際にはほとんどが発売日の決まっていない予約。月の初日などに寄せると、
//     まだ出ていない本が新刊の欄に並ぶ
//   - 「YYYY年MM月DD日以降」のように日の後ろに「頃」以外が付くものも捨てる。「以降」は下限でしかなく、
//     その日に出たとは言えない
func parseSalesDate(s string) (date time.Time, approximate bool, ok bool) {
	m := salesDatePattern.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, false, false
	}
	switch strings.TrimSpace(m[4]) {
	case "":
	case "頃":
		approximate = true
	default:
		return time.Time{}, false, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	date = time.Date(y, time.Month(mo), d, 0, 0, 0, 0, jst)
	// time.Date は 2 月 30 日を 3 月 2 日に繰り上げるので、繰り上がったものは不正な表記として捨てる
	if date.Year() != y || int(date.Month()) != mo || date.Day() != d {
		return time.Time{}, false, false
	}
	return date, approximate, true
}

func toItem(b rakuten.Book, date time.Time, approximate bool, genreName string) Item {
	url := b.ItemURL
	if b.AffiliateURL != "" {
		url = b.AffiliateURL
	}
	return Item{
		Title:                b.Title,
		SeriesName:           b.SeriesName,
		Author:               b.Author,
		PublisherName:        b.PublisherName,
		ISBN:                 b.ISBN,
		SalesDate:            date.Format(time.DateOnly),
		SalesDateApproximate: approximate,
		Price:                b.ItemPrice,
		URL:                  url,
		ImageURL:             b.LargeImageURL,
		Genre:                genreName,
	}
}
