package housekeeping

import "context"

// Repository は Usecase が後始末に必要とする操作を定義する。
// 実装は internal/infrastructure/postgres が提供する。
//
// airing.Repository と同じく、特定の利用者のためではなく保守のために全ユーザーを横断する。
// userID でスコープされない操作なので、auth.SessionRepository には混ぜずにここで分けている。
type Repository interface {
	// DeleteExpiredSessions は有効期限を過ぎたセッションを全ユーザーぶん削除し、削除した行数を返す。
	DeleteExpiredSessions(ctx context.Context) (int64, error)
}
