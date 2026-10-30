package auth

import (
	"context"
	"errors"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// ErrNotFound はメールアドレス・セッショントークンに該当するレコードが存在しないことを表す。
var ErrNotFound = errors.New("not found")

// ErrEmailTaken はサインアップ時にメールアドレスが既に登録済みであることを表す。
var ErrEmailTaken = errors.New("email already registered")

// UserWithHash はログイン検証時にのみ使う内部表現（password_hash を含む）。
type UserWithHash struct {
	domain.User
	PasswordHash string
}

// UserRepository は Service がユーザー情報の永続化に必要とする操作を定義する。
// 実装は internal/infrastructure/postgres が提供する。
type UserRepository interface {
	// Create はユーザーを作成する。email が既に登録済みなら ErrEmailTaken を返す。
	Create(ctx context.Context, email, passwordHash string) (*domain.User, error)
	// FindByEmail はログイン検証のためパスワードハッシュ込みで返す。存在しなければ ErrNotFound を返す。
	FindByEmail(ctx context.Context, email string) (*UserWithHash, error)
	// PasswordHash はパスワード変更の確認のため、userID のパスワードハッシュを返す。存在しなければ ErrNotFound を返す。
	PasswordHash(ctx context.Context, userID int64) (string, error)
	// UpdatePassword は userID のパスワードハッシュを差し替え、keepTokenHash 以外の userID のセッションを消す。
	// 2 つを 1 つのトランザクションで行う。別々だと、差し替えだけ成功して古いセッションが残る失敗が起きうる。
	// 存在しなければ ErrNotFound を返す。
	UpdatePassword(ctx context.Context, userID int64, passwordHash, keepTokenHash string) error
}

// SessionRepository は Service がセッションの永続化に必要とする操作を定義する。
// 実装は internal/infrastructure/postgres が提供する。
//
// 受け取るのはトークンのハッシュで、平文は渡さない（hashToken を参照）。
type SessionRepository interface {
	Create(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error
	// FindUser は有効期限内のセッションからユーザーを引く。期限切れ・不存在なら ErrNotFound を返す。
	FindUser(ctx context.Context, tokenHash string) (*domain.User, error)
	Delete(ctx context.Context, tokenHash string) error
}
