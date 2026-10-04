package airing

import (
	"context"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// AniListGateway は Usecase が次話放送日時の取得に必要とする AniList 側の操作を定義する。
// 実装は internal/anilist が提供する。
type AniListGateway interface {
	// MediaByIDs は AniList の作品IDリストから最新情報をまとめて取得する。
	// 存在しないIDは結果に含まれない。
	MediaByIDs(ctx context.Context, ids []int64, mediaType domain.MediaType) ([]anilist.SearchResult, error)
}
