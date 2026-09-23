package search

import (
	"context"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// AniListGateway は Usecase が作品検索・取得に必要とする AniList 側の操作を定義する。
// 実装は internal/anilist が提供する。
type AniListGateway interface {
	Search(ctx context.Context, query string, mediaType domain.MediaType) ([]anilist.SearchResult, error)
	MediaByIDs(ctx context.Context, ids []int64, mediaType domain.MediaType) ([]anilist.SearchResult, error)
	Relations(ctx context.Context, id int64) ([]anilist.RelatedWork, error)
	ByGenres(ctx context.Context, genres []string, mediaType domain.MediaType) ([]anilist.SearchResult, error)
}

// JikanGateway は Usecase がAniListの検索結果が少ない場合のフォールバックに使う
// Jikan 側の操作を定義する。実装は internal/jikan が提供する。
type JikanGateway interface {
	SearchAnime(ctx context.Context, query string) ([]anilist.SearchResult, error)
	SearchManga(ctx context.Context, query string) ([]anilist.SearchResult, error)
}
