package record

import (
	"context"
	"errors"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// ErrNotFound は指定されたIDの記録が存在しない（または他ユーザーの記録である）ことを表す。
var ErrNotFound = errors.New("record not found")

// Repository は Usecase が記録の永続化に必要とする操作を定義する。
// 実装は internal/infrastructure/postgres が提供する。
type Repository interface {
	// List は種別・ステータスで記録を絞り込んで返す。どちらも空文字なら絞り込まない。
	List(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status) ([]*domain.Record, error)
	// Upsert は AniList ID + 種別が一致する記録があれば更新、なければ新規作成する。
	Upsert(ctx context.Context, userID int64, in domain.NewRecordInput) (*domain.Record, error)
	// Update は指定IDの記録のステータス/進捗を部分更新する。存在しなければ ErrNotFound を返す。
	Update(ctx context.Context, userID, id int64, in domain.UpdateRecordInput) (*domain.Record, error)
	// Delete は指定IDの記録を削除する。存在しなければ ErrNotFound を返す。
	Delete(ctx context.Context, userID, id int64) error
}
