package airing

import (
	"context"
	"time"
)

// NextAiring は1作品ぶんの次話放送日時の更新内容。At が nil なら「次話なし」（NULL を書く）。
type NextAiring struct {
	AniListID int64
	At        *time.Time
}

// Repository は Usecase が next_airing_at の更新に必要とする操作を定義する。
// 実装は internal/infrastructure/postgres が提供する。
//
// 記録の参照・更新を担う record.Repository と違い、こちらは特定の利用者のためではなく
// 保守のために全ユーザーのレコードを横断する。そのため userID でスコープされない。
type Repository interface {
	// StaleAiringAnimeIDs は next_airing_at が過去の日時のまま残っているアニメの AniList ID を、
	// 古い順に最大 limit 件返す。同じ作品を複数の利用者が登録していても1件にまとめる。
	StaleAiringAnimeIDs(ctx context.Context, limit int) ([]int64, error)
	// UpdateNextAiringAt は AniList ID が一致するアニメレコードの next_airing_at を
	// 全ユーザーぶんまとめて書き換え、実際に値が変わった行数を返す。
	UpdateNextAiringAt(ctx context.Context, updates []NextAiring) (int64, error)
}
