// Package home はログイン不要のホーム画面向けデータ（今季アニメ・人気ランキング）を担う。
package home

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
)

// cacheTTL は外部API呼び出し結果をホーム画面向けにキャッシュしておく時間。
// タブ切り替えのたびに毎回AniList/Jikanへ問い合わせるのを防ぐ。
const cacheTTL = 10 * time.Minute

// seasonBrowseCacheTTL はシーズンブラウジング画面（任意のseason/year指定）向けキャッシュの保持時間。
// 過去シーズンのラインナップは変わらず、放送中シーズンでも変動するのはスコア程度なので、
// ホーム画面向けのcacheTTLより長めに取って画面の行き来を軽くする。
const seasonBrowseCacheTTL = 30 * time.Minute

// seasonBrowseCacheMaxEntries はシーズンブラウジング用キャッシュが保持するseason/yearの数。
// 1年4シーズンなので十数年ぶんの行き来をカバーできる。yearはクエリパラメータ由来で
// 際限なく増えうるため、上限を設けてメモリ使用量を一定に保つ。
const seasonBrowseCacheMaxEntries = 64

// WarmInterval はホーム画面向けキャッシュを定期リフレッシュする間隔。cacheTTL より短くして、
// TTLが切れて利用者が古い値を踏む前に、裏で中身が入れ替わっているようにする。
// AniList/Jikan への負荷は Warm 1回あたり AniList 3本 + Jikan 10本で、
// AniListの平均レート上限（30req/分）に対して十分に小さい。
const WarmInterval = 8 * time.Minute

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
	seasonBrowseCache  *cache.TTLCache[[]anilist.SearchResult]
	trendingCache      *cache.TTLCache[[]anilist.SearchResult]
	trendingMangaCache *cache.TTLCache[[]MangaRankingItem]
}

func NewUsecase(anilistGateway AniListGateway, jikanGateway JikanGateway) *Usecase {
	return &Usecase{
		anilist:            anilistGateway,
		jikan:              jikanGateway,
		seasonAnimeCache:   cache.New[[]anilist.SearchResult](cacheTTL),
		seasonBrowseCache:  cache.NewBounded[[]anilist.SearchResult](seasonBrowseCacheTTL, seasonBrowseCacheMaxEntries),
		trendingCache:      cache.New[[]anilist.SearchResult](cacheTTL),
		trendingMangaCache: cache.New[[]MangaRankingItem](cacheTTL),
	}
}

// CurrentSeasonAnime は現在のシーズンのアニメ一覧を返す（ホーム画面向け、キャッシュあり）。
func (u *Usecase) CurrentSeasonAnime(ctx context.Context) ([]anilist.SearchResult, error) {
	return u.seasonAnimeCache.Get(ctx, "", u.anilist.SeasonAnime)
}

// SeasonAnimeFor はシーズンブラウジングページ向けに、任意のシーズンのアニメ一覧を返す
// （season+yearごとにキャッシュあり）。
func (u *Usecase) SeasonAnimeFor(ctx context.Context, season string, year int) ([]anilist.SearchResult, error) {
	key := season + ":" + strconv.Itoa(year)
	return u.seasonBrowseCache.Get(ctx, key, func(ctx context.Context) ([]anilist.SearchResult, error) {
		return u.anilist.SeasonAnimeFor(ctx, season, year)
	})
}

// TrendingAnime は人気アニメ一覧を返す（キャッシュあり）。
func (u *Usecase) TrendingAnime(ctx context.Context) ([]anilist.SearchResult, error) {
	return u.trendingCache.Get(ctx, "", u.anilist.TrendingAnime)
}

// TrendingManga はAniListの人気順TOP10を取得し、各作品のMAL IDを使ってJikanから
// ランキング順位・会員数・掲載誌を補って返す（キャッシュあり）。
func (u *Usecase) TrendingManga(ctx context.Context) ([]MangaRankingItem, error) {
	return u.trendingMangaCache.Get(ctx, "", u.fetchTrendingManga)
}

// Warm はホーム画面向けキャッシュ（今季アニメ・人気アニメ・話題の漫画）をTTLの残りに関わらず
// 取り直す。起動直後と、その後 WarmInterval ごとに internal/scheduler から呼ばれる。
// キャッシュミス時の取得には数秒かかる（AniList 1本 + Jikan 10本で3秒以上）ので、
// 利用者がその待ち時間を肩代わりする前に裏で埋めておくのが狙い。
//
// シーズンブラウジング用キャッシュ（任意のseason/year指定）は温めない。キーがクエリパラメータ
// 由来で際限なく増えうるうえ、ホーム画面の表示には使われないため。
//
// 1つ失敗しても残りは続行し、失敗したぶんをまとめて返す。AniListは全呼び出しで共通の
// スロットルを持つため、3つは直列に実行して利用者のリクエストと競合する時間を短く保つ。
func (u *Usecase) Warm(ctx context.Context) error {
	targets := []struct {
		name    string
		refresh func(context.Context) error
	}{
		{"current season anime", func(ctx context.Context) error {
			return u.seasonAnimeCache.Refresh(ctx, "", u.anilist.SeasonAnime)
		}},
		{"trending anime", func(ctx context.Context) error {
			return u.trendingCache.Refresh(ctx, "", u.anilist.TrendingAnime)
		}},
		{"trending manga", func(ctx context.Context) error {
			return u.trendingMangaCache.Refresh(ctx, "", u.fetchTrendingManga)
		}},
	}

	var errs []error
	for _, t := range targets {
		if err := t.refresh(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.name, err))
		}
	}
	return errors.Join(errs...)
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
