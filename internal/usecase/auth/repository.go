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

// ErrIdentityTaken は手段を紐付けようとしたユーザーが、同じ手段の別の識別子を既に持っていることを表す
// （別の Google アカウントが紐付いている）。
var ErrIdentityTaken = errors.New("identity already linked")

// ログインの手段（user_identities.provider）。
const (
	ProviderPassword = "password"
	ProviderGoogle   = "google"
)

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
	// パスワードを持たないユーザー（Google だけで登録した）も ErrNotFound にする。
	FindByEmail(ctx context.Context, email string) (*UserWithHash, error)
	// PasswordHash はパスワード変更の確認のため、userID のパスワードハッシュを返す。存在しなければ ErrNotFound を返す。
	// ユーザーはいるがパスワードを持たないときは ErrNoPassword を返す。
	PasswordHash(ctx context.Context, userID int64) (string, error)
	// UpdatePassword は userID のパスワードハッシュを差し替え、keepTokenHash 以外の userID のセッションを消す。
	// 2 つを 1 つのトランザクションで行う。別々だと、差し替えだけ成功して古いセッションが残る失敗が起きうる。
	// 存在しなければ ErrNotFound を返す。
	UpdatePassword(ctx context.Context, userID int64, passwordHash, keepTokenHash string) error

	// FindByIdentity は手段と識別子（Google なら sub）からユーザーを引く。存在しなければ ErrNotFound を返す。
	FindByIdentity(ctx context.Context, provider, subject string) (*domain.User, error)
	// LinkIdentityByEmail は email のユーザーに手段を紐付けて返す。email のユーザーがいなければ ErrNotFound、
	// そのユーザーが同じ手段の別の識別子を既に持っていれば ErrIdentityTaken を返す。
	LinkIdentityByEmail(ctx context.Context, email, provider, subject string) (*domain.User, error)
	// CreateWithIdentity はパスワードを持たないユーザーを、手段を紐付けた状態で作る。
	// 2 つの表への書き込みを 1 つのトランザクションで行う。email が既に登録済みなら ErrEmailTaken を返す。
	CreateWithIdentity(ctx context.Context, email, provider, subject string) (*domain.User, error)
	// Providers は userID がログインに使える手段（ProviderPassword など）を名前順で返す。
	// ユーザーがいなければ空のスライスを返す（エラーにはしない）。
	Providers(ctx context.Context, userID int64) ([]string, error)
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
