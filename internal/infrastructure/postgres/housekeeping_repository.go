package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/housekeeping"
)

// HousekeepingRepository は定期的な後始末のための操作を提供する。usecase/housekeeping.Repository の実装。
//
// 他のリポジトリと違い、ここの操作はユーザーIDでスコープされない（全ユーザーを横断して消す）。
// SessionRepository に足さずに型ごと分けているのは、利用者のリクエストから呼ばれる経路に
// スコープされない DELETE が混ざらないようにするため。
type HousekeepingRepository struct {
	pool *pgxpool.Pool
}

func NewHousekeepingRepository(pool *pgxpool.Pool) *HousekeepingRepository {
	return &HousekeepingRepository{pool: pool}
}

var _ housekeeping.Repository = (*HousekeepingRepository)(nil)

// DeleteExpiredSessions は期限切れのセッションを消す。期限の判定は SessionRepository.FindUser と
// 同じく DB の now() で行う。アプリ側の時計を渡すと、両者の時計がずれたときに
// 「FindUser はまだ有効とみなすのに消される」セッションが生まれうる。
//
// sessions.expires_at に索引は無いので毎回全件を走査する。セッションは利用者 × 端末数の行数しかなく、
// 1 時間に 1 回なら問題にならない（索引を足すのはマイグレーションを伴うので見送り。BACKLOG 4-d）。
func (r *HousekeepingRepository) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	if err != nil {
		return 0, fmt.Errorf("delete expired sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}
