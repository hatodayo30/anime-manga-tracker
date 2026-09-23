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
}

// SessionRepository は Service がセッションの永続化に必要とする操作を定義する。
// 実装は internal/infrastructure/postgres が提供する。
type SessionRepository interface {
	Create(ctx context.Context, token string, userID int64, expiresAt time.Time) error
	// FindUser は有効期限内のセッションからユーザーを引く。期限切れ・不存在なら ErrNotFound を返す。
	FindUser(ctx context.Context, token string) (*domain.User, error)
	Delete(ctx context.Context, token string) error
}
