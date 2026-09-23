package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/jikan"
	"golang.org/x/sync/errgroup"
)

// homeCacheTTL は外部API呼び出し結果をホーム画面向けにキャッシュしておく時間。
// タブ切り替えのたびに毎回AniList/Jikanへ問い合わせるのを防ぐ。
const homeCacheTTL = 10 * time.Minute

// homeCache はJSONで返せる任意の型を、TTL付きでメモリ上に1件だけ保持する簡易キャッシュ。
// ホーム画面のデータはユーザーに依存しないため、ハンドラー単位でグローバルに共有してよい。
type homeCache[T any] struct {
	mu        sync.Mutex
	data      T
	fetchedAt time.Time
}

func (c *homeCache[T]) get(fetch func() (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.fetchedAt.IsZero() && time.Since(c.fetchedAt) < homeCacheTTL {
		return c.data, nil
	}

	data, err := fetch()
	if err != nil {
		var zero T
		return zero, err
	}
	c.data = data
	c.fetchedAt = time.Now()
	return c.data, nil
}

// keyedCacheTTL はkeyedCacheのTTL。おすすめはユーザーのライブラリ変化に追従してほしいので
// homeCacheTTLより短くしておく。
const keyedCacheTTL = 3 * time.Minute

// keyedCache はhomeCacheのキー付き版。パラメータ（ジャンルの組み合わせ等）ごとに結果を
// TTL付きでメモリ上に保持する。同じキーへの短時間の連打（例: モーダルでの記録変更のたびに
// 呼ばれるonChange）が毎回外部APIを叩くのを防ぐ。
type keyedCache[T any] struct {
	mu    sync.Mutex
	items map[string]keyedCacheEntry[T]
}

type keyedCacheEntry[T any] struct {
	data      T
	fetchedAt time.Time
}

func (c *keyedCache[T]) get(key string, fetch func() (T, error)) (T, error) {
	c.mu.Lock()
	if entry, ok := c.items[key]; ok && time.Since(entry.fetchedAt) < keyedCacheTTL {
		c.mu.Unlock()
		return entry.data, nil
	}
	c.mu.Unlock()

	data, err := fetch()
	if err != nil {
		var zero T
		return zero, err
	}

	c.mu.Lock()
	if c.items == nil {
		c.items = make(map[string]keyedCacheEntry[T])
	}
	c.items[key] = keyedCacheEntry[T]{data: data, fetchedAt: time.Now()}
	c.mu.Unlock()
	return data, nil
}

// HomeHandler はログイン不要のホーム画面向けデータ（今季アニメ・人気ランキング）を提供する。
type HomeHandler struct {
	client      *anilist.Client
	jikanClient *jikan.Client

	seasonAnimeCache   homeCache[[]anilist.SearchResult]
	trendingCache      homeCache[[]anilist.SearchResult]
	trendingMangaCache homeCache[[]MangaRankingItem]
}

func NewHomeHandler(client *anilist.Client, jikanClient *jikan.Client) *HomeHandler {
	return &HomeHandler{client: client, jikanClient: jikanClient}
}

// SeasonAnime handles GET /api/home/season-anime[?season=WINTER&year=2026]
// season/yearを省略すると現在のシーズンを返す（ホーム画面向け）。
// 両方指定するとシーズンブラウジングページ向けに任意のシーズンを返す。
func (h *HomeHandler) SeasonAnime(w http.ResponseWriter, r *http.Request) {
	seasonParam := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("season")))
	yearParam := strings.TrimSpace(r.URL.Query().Get("year"))

	if seasonParam == "" && yearParam == "" {
		results, err := h.seasonAnimeCache.get(func() ([]anilist.SearchResult, error) {
			return h.client.SeasonAnime(r.Context())
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)
		return
	}

	if !anilist.ValidSeasons[seasonParam] {
		writeError(w, http.StatusBadRequest, "season must be one of WINTER, SPRING, SUMMER, FALL")
		return
	}
	year, err := strconv.Atoi(yearParam)
	if err != nil {
		writeError(w, http.StatusBadRequest, "year must be a number")
		return
	}

	results, err := h.client.SeasonAnimeFor(r.Context(), seasonParam, year)
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// Trending handles GET /api/home/trending
func (h *HomeHandler) Trending(w http.ResponseWriter, r *http.Request) {
	results, err := h.trendingCache.get(func() ([]anilist.SearchResult, error) {
		return h.client.TrendingAnime(r.Context())
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// MangaRankingItem は「話題の漫画 TOP10」「掲載誌で探す」向けに、AniListの検索結果へ
// Jikan（MyAnimeList）由来のランキング順位・会員数・掲載誌を足したもの。
type MangaRankingItem struct {
	anilist.SearchResult
	MalRank   *int     `json:"malRank,omitempty"`
	Members   *int     `json:"members,omitempty"`
	Magazines []string `json:"magazines"`
}

// jikanEnrichTimeout は1件あたりのJikan呼び出しに許すタイムアウト。
// Jikanはレート制限（3req/秒程度）があるパブリックAPIのため、1件失敗しても全体は継続する。
const jikanEnrichTimeout = 4 * time.Second

// TrendingManga handles GET /api/home/trending-manga
// AniListの人気順TOP10を取得し、各作品のMAL IDを使ってJikanからランキング順位・会員数・掲載誌を補う。
func (h *HomeHandler) TrendingManga(w http.ResponseWriter, r *http.Request) {
	items, err := h.trendingMangaCache.get(func() ([]MangaRankingItem, error) {
		return h.fetchTrendingManga(r.Context())
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, items)
}

// fetchTrendingManga はAniListの人気順TOP10を取得し、各作品のJikanエンリッチを並列に行う。
// jikan.Clientのthrottleがレート制限を守るための待ち合わせを担うので、並列化しても
// Jikanへのリクエスト間隔は守られたまま、待機時間とHTTP応答時間が重なり全体のレイテンシが縮む。
func (h *HomeHandler) fetchTrendingManga(ctx context.Context) ([]MangaRankingItem, error) {
	results, err := h.client.TrendingManga(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]MangaRankingItem, len(results))
	g, gctx := errgroup.WithContext(ctx)
	for i, res := range results {
		items[i] = MangaRankingItem{SearchResult: res, Magazines: []string{}}
		if res.MalID == nil {
			continue
		}

		i, res := i, res
		g.Go(func() error {
			enrichCtx, cancel := context.WithTimeout(gctx, jikanEnrichTimeout)
			defer cancel()
			stats, err := h.jikanClient.GetManga(enrichCtx, *res.MalID)
			if err != nil {
				return nil // Jikan側の失敗はランキング自体を止めない。この作品の補足情報だけ空になる
			}

			items[i].MalRank = stats.Rank
			items[i].Members = stats.Members
			items[i].Magazines = stats.Magazines
			return nil
		})
	}
	_ = g.Wait() // 各goroutineは自身のエラーをnilとして飲み込むので、ここでのエラーは発生しない

	return items, nil
}
