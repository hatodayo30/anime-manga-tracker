// Package home はログイン不要のホーム画面向けデータ（今季アニメ・人気ランキング）を担う。
package home

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
)

// cacheTTL は外部API呼び出し結果をホーム画面向けにキャッシュしておく時間。
// タブ切り替えのたびに毎回AniList/Jikanへ問い合わせるのを防ぐ。
const cacheTTL = 10 * time.Minute

// jikanEnrichTimeout は1件あたりのJikan呼び出しに許すタイムアウト。
// Jikanはレート制限（3req/秒程度）があるパブリックAPIのため、1件失敗しても全体は継続する。
const jikanEnrichTimeout = 4 * time.Second

// MangaRankingItem は「話題の漫画 TOP10」「掲載誌で探す」向けに、AniListの検索結果へ
// Jikan（MyAnimeList）由来のランキング順位・会員数・掲載誌を足したもの。
type MangaRankingItem struct {
	anilist.SearchResult
	MalRank   *int     `json:"malRank,omitempty"`
	Members   *int     `json:"members,omitempty"`
	Magazines []string `json:"magazines"`
}

// ホーム画面のデータはユーザーに依存しないため、Usecase単位でキャッシュを共有してよい。
type Usecase struct {
	anilist AniListGateway
	jikan   JikanGateway

	seasonAnimeCache   *cache.TTLCache[[]anilist.SearchResult]
	trendingCache      *cache.TTLCache[[]anilist.SearchResult]
	trendingMangaCache *cache.TTLCache[[]MangaRankingItem]
}

func NewUsecase(anilistGateway AniListGateway, jikanGateway JikanGateway) *Usecase {
	return &Usecase{
		anilist:            anilistGateway,
		jikan:              jikanGateway,
		seasonAnimeCache:   cache.New[[]anilist.SearchResult](cacheTTL),
		trendingCache:      cache.New[[]anilist.SearchResult](cacheTTL),
		trendingMangaCache: cache.New[[]MangaRankingItem](cacheTTL),
	}
}

// CurrentSeasonAnime は現在のシーズンのアニメ一覧を返す（ホーム画面向け、キャッシュあり）。
func (u *Usecase) CurrentSeasonAnime(ctx context.Context) ([]anilist.SearchResult, error) {
	return u.seasonAnimeCache.Get("", func() ([]anilist.SearchResult, error) {
		return u.anilist.SeasonAnime(ctx)
	})
}

// SeasonAnimeFor はシーズンブラウジングページ向けに、任意のシーズンのアニメ一覧を返す。
func (u *Usecase) SeasonAnimeFor(ctx context.Context, season string, year int) ([]anilist.SearchResult, error) {
	return u.anilist.SeasonAnimeFor(ctx, season, year)
}

// TrendingAnime は人気アニメ一覧を返す（キャッシュあり）。
func (u *Usecase) TrendingAnime(ctx context.Context) ([]anilist.SearchResult, error) {
	return u.trendingCache.Get("", func() ([]anilist.SearchResult, error) {
		return u.anilist.TrendingAnime(ctx)
	})
}

// TrendingManga はAniListの人気順TOP10を取得し、各作品のMAL IDを使ってJikanから
// ランキング順位・会員数・掲載誌を補って返す（キャッシュあり）。
func (u *Usecase) TrendingManga(ctx context.Context) ([]MangaRankingItem, error) {
	return u.trendingMangaCache.Get("", func() ([]MangaRankingItem, error) {
		return u.fetchTrendingManga(ctx)
	})
}

// fetchTrendingManga はAniListの人気順TOP10を取得し、各作品のJikanエンリッチを並列に行う。
// jikan.Clientのthrottleがレート制限を守るための待ち合わせを担うので、並列化しても
// Jikanへのリクエスト間隔は守られたまま、待機時間とHTTP応答時間が重なり全体のレイテンシが縮む。
func (u *Usecase) fetchTrendingManga(ctx context.Context) ([]MangaRankingItem, error) {
	results, err := u.anilist.TrendingManga(ctx)
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
			stats, err := u.jikan.GetManga(enrichCtx, *res.MalID)
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
