// Package volumes は漫画の既刊の巻数を楽天ブックスから数える（BACKLOG 11-c）。
//
// AniList は連載中の漫画の巻数（volumes）をほとんど登録しておらず、作品モーダルの進捗が「5巻 / ?巻」になる。
// MyAnimeList（Jikan）も同じ傾向なので、書籍の発売情報を持つ楽天ブックスで作品名を検索し、
// 発売済みの巻の番号の最大を既刊とする。
//
// 結果は表示にだけ使い、records.total には保存しない。楽天の規約が取得した情報の保存と改変を制限しているため
// （第 10 条 1 項(7)(9)、BACKLOG 11-b「規約で確かめたこと」）。
package volumes

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/hatodayo30/anime-manga-tracker/internal/rakuten"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/newreleases"
)

// Gateway は書名で漫画を探す楽天ブックス側の操作。実装は internal/rakuten が提供する。
type Gateway interface {
	// ComicsByTitle は書名に title を含む漫画を、発売日の新しい順に 1 ページ（30 件）返す。page は 1 始まり。
	ComicsByTitle(ctx context.Context, title string, page int) (*rakuten.Page, error)
}

// maxPages は 1 つの作品で読むページ数の上限（30 件 × 3 = 90 冊）。
//
// 「発売日の新しい順」の先頭には、本編より新しい再編集版が並ぶことがある。2026-10-25 に「呪術廻戦」で確かめたところ、
// 1 ページ目は「集英社リミックス」（呪術廻戦 13 人外魔境新宿決戦…）と外伝ばかりで、本編（30 巻）が入らなかった。
// 1 ページだけだと既刊を 13 と答える。楽天は 1.5 秒に 1 本なので、3 ページで 4〜5 秒になる。
const maxPages = 3

// cacheTTL / cacheMaxEntries は既刊のキャッシュ。巻は月に 1 冊も増えないので 1 日で十分。
// キーは利用者が開いた作品の題名で際限なく増えうるので、上限を設ける。
const (
	cacheTTL        = 24 * time.Hour
	cacheMaxEntries = 512
)

// jst は「発売済み」の「今日」の区切り（usecase/newreleases と同じ）。
var jst = time.FixedZone("JST", 9*60*60)

// Result は既刊の巻数。
type Result struct {
	// Configured は楽天のキーが設定されているか。false なら他の項目は空
	Configured bool `json:"configured"`
	// Found は題名が合う巻が見つかったか。題名が合わないものは数えないので、見つからないことも多い
	Found bool `json:"found"`
	// LatestVolume は発売済みの巻の番号の最大
	LatestVolume int `json:"latestVolume,omitempty"`
	// SalesDate はその巻の発売日（YYYY-MM-DD）
	SalesDate string `json:"salesDate,omitempty"`
}

type Usecase struct {
	gateway Gateway
	now     func() time.Time
	cache   *cache.TTLCache[Result]
}

// NewUsecase の gateway は楽天のキーが無い環境では nil にする。そのとき Latest は Configured=false を返す。
func NewUsecase(gateway Gateway, now func() time.Time) *Usecase {
	return &Usecase{
		gateway: gateway,
		now:     now,
		cache:   cache.NewBounded[Result](cacheTTL, cacheMaxEntries),
	}
}

// Latest は作品名 title の既刊の巻数を返す（キャッシュあり）。
func (u *Usecase) Latest(ctx context.Context, title string) (Result, error) {
	if u.gateway == nil {
		return Result{Configured: false}, nil
	}
	key := normalize(title)
	if key == "" {
		return Result{Configured: true}, nil
	}
	return u.cache.Get(ctx, key, func(ctx context.Context) (Result, error) {
		return u.fetch(ctx, title, key)
	})
}

func (u *Usecase) fetch(ctx context.Context, title, key string) (Result, error) {
	now := u.now().In(jst)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, jst)

	result := Result{Configured: true}
	for page := 1; page <= maxPages; page++ {
		pg, err := u.gateway.ComicsByTitle(ctx, strings.TrimSpace(title), page)
		if err != nil {
			return Result{}, err
		}
		for _, b := range pg.Items {
			vol, ok := volumeOf(key, b.Title)
			if !ok {
				continue
			}
			// 予約（未発売）は既刊に数えない。日が分からないもの（「2026年12月」など）も予約とみなす
			d, _, ok := newreleases.ParseSalesDate(b.SalesDate)
			if !ok || d.After(today) {
				continue
			}
			if vol > result.LatestVolume {
				result.Found = true
				result.LatestVolume = vol
				result.SalesDate = d.Format(time.DateOnly)
			}
		}
		if page >= pg.PageCount {
			break
		}
	}
	return result, nil
}

// volumePattern は題名の直後に続く巻の番号。題名と番号の間の空白・句読点（「〜されています。6」）、
// 括弧（「（14）」）、「第」「巻」を許す。番号の後ろは何が続いてもよい（「呪術廻戦 13 人外魔境新宿決戦…」の再編集版）。
var volumePattern = regexp.MustCompile(`^[\s。．、！？!?・:：]*[（(]?第?(\d+)巻?[）)]?(\s|$|[^\d])`)

// volumeOf は書名 bookTitle が、正規化した作品名 key の直後に巻の番号が続く形なら、その番号を返す。
//
// 作品名の直後が番号でないもの（「呪術廻戦≡ 3」の外伝、「呪術廻戦 公式ファンブック」）は別の本として数えない。
// 楽天の書名には作品名の後ろに版の名前が付くこともあるが（「【特装版】」は前に付くことが多い）、
// 前に付いた括弧書きは normalize で落とす。
func volumeOf(key, bookTitle string) (int, bool) {
	t := normalize(bookTitle)
	if !strings.HasPrefix(t, key) {
		return 0, false
	}
	m := volumePattern.FindStringSubmatch(t[len(key):] + " ")
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

var leadingBracket = regexp.MustCompile(`^[【［\[][^】］\]]*[】］\]]`)

// normalize は題名を照合しやすい形にする。全角の英数字・記号を半角に（NFKC）、英字を小文字にし、
// 先頭の【特装版】のような括弧書きを落とし、空白を 1 つにまとめる。題名の中の空白は落とさない。
// 「ONE PIECE 110」の空白は作品名と巻の番号の区切りで、落とすと「ONEPIECE110」から番号を切り出せない
func normalize(s string) string {
	s = norm.NFKC.String(s)
	s = strings.ToLower(s)
	s = strings.TrimSpace(leadingBracket.ReplaceAllString(strings.TrimSpace(s), ""))
	return strings.Join(strings.Fields(s), " ")
}
