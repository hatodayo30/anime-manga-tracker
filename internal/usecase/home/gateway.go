package home

import (
	"context"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/jikan"
)

// AniListGateway は Usecase がホーム画面向けデータの取得に必要とする AniList 側の操作を定義する。
// 実装は internal/anilist が提供する。
type AniListGateway interface {
	SeasonAnime(ctx context.Context) ([]anilist.SearchResult, error)
	SeasonAnimeFor(ctx context.Context, season string, year int) ([]anilist.SearchResult, error)
	TrendingAnime(ctx context.Context) ([]anilist.SearchResult, error)
	TrendingManga(ctx context.Context) ([]anilist.SearchResult, error)
}

// JikanGateway は Usecase が話題の漫画ランキングの補足情報取得に必要とする
// Jikan 側の操作を定義する。実装は internal/jikan が提供する。
type JikanGateway interface {
	GetManga(ctx context.Context, malID int64) (*jikan.MangaStats, error)
}
