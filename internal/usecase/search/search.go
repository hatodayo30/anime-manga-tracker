// Package search は作品検索・関連作品・おすすめ表示まわりのユースケースを担う。
package search

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
)

// jikanFallbackThreshold 以下の件数しかAniListの検索結果が無いとき、Jikan（MyAnimeList）でも
// 検索してマージする。AniListには無い/ヒットしにくい作品を拾うためのフォールバック。
const jikanFallbackThreshold = 3

// recommendCacheTTL はおすすめ結果のキャッシュ保持時間。ユーザーのライブラリ変化に追従してほしいので
// 短めにしておく。
const recommendCacheTTL = 3 * time.Minute

type Usecase struct {
	anilist AniListGateway
	jikan   JikanGateway

	recommendCache *cache.TTLCache[[]anilist.SearchResult]
}

func NewUsecase(anilistGateway AniListGateway, jikanGateway JikanGateway) *Usecase {
	return &Usecase{
		anilist:        anilistGateway,
		jikan:          jikanGateway,
		recommendCache: cache.New[[]anilist.SearchResult](recommendCacheTTL),
	}
}

// Search は作品名でAniListを検索する。ヒット件数が少ない場合はJikanの結果もマージする。
func (u *Usecase) Search(ctx context.Context, mediaType domain.MediaType, query string) ([]anilist.SearchResult, error) {
	results, err := u.anilist.Search(ctx, query, mediaType)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(query) != "" && len(results) <= jikanFallbackThreshold {
		results = u.withJikanFallback(ctx, mediaType, query, results)
	}
	return results, nil
}

// withJikanFallback はAniListの検索結果にJikanの検索結果を足し、タイトルの重複を除いて
// 人気順にまとめる。Jikan側が失敗してもAniListの結果はそのまま返す。
func (u *Usecase) withJikanFallback(ctx context.Context, mediaType domain.MediaType, query string, aniListResults []anilist.SearchResult) []anilist.SearchResult {
	var jikanResults []anilist.SearchResult
	var err error
	if mediaType == domain.MediaTypeManga {
		jikanResults, err = u.jikan.SearchManga(ctx, query)
	} else {
		jikanResults, err = u.jikan.SearchAnime(ctx, query)
	}
	if err != nil || len(jikanResults) == 0 {
		return aniListResults
	}

	seenTitles := make(map[string]bool, len(aniListResults))
	for _, res := range aniListResults {
		seenTitles[strings.ToLower(res.Title)] = true
	}

	merged := append([]anilist.SearchResult{}, aniListResults...)
	for _, res := range jikanResults {
		key := strings.ToLower(res.Title)
		if seenTitles[key] {
			continue
		}
		seenTitles[key] = true
		merged = append(merged, res)
	}

	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Popularity > merged[j].Popularity })
	return merged
}

// ByIDs はマイライブラリ画面で、保存済み作品の総話数・放送状況・現在の話数を最新化するために使う。
func (u *Usecase) ByIDs(ctx context.Context, mediaType domain.MediaType, ids []int64) ([]anilist.SearchResult, error) {
	return u.anilist.MediaByIDs(ctx, ids, mediaType)
}

// Relations は作品モーダルの「関連作品」セクション向けに、続編・前日譚・スピンオフ等を返す。
func (u *Usecase) Relations(ctx context.Context, id int64) ([]anilist.RelatedWork, error) {
	return u.anilist.Relations(ctx, id)
}

// Recommendations はおすすめ画面向けに、指定ジャンルのいずれかに合致する作品を人気順で返す。
func (u *Usecase) Recommendations(ctx context.Context, mediaType domain.MediaType, genres []string) ([]anilist.SearchResult, error) {
	sortedGenres := append([]string{}, genres...)
	sort.Strings(sortedGenres)
	cacheKey := string(mediaType) + "|" + strings.Join(sortedGenres, ",")

	return u.recommendCache.Get(cacheKey, func() ([]anilist.SearchResult, error) {
		return u.anilist.ByGenres(ctx, genres, mediaType)
	})
}
