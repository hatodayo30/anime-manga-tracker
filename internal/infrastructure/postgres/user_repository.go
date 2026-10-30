package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

var _ auth.UserRepository = (*UserRepository)(nil)

// Create はパスワードで登録するユーザーを作成する。email は UNIQUE 制約により重複登録を防ぐ。
// users と user_identities の 2 つの表に書くので、1 つのトランザクションで行う。
// パスワードの行の subject には users.id を入れる（0007 のマイグレーションのコメントを参照）。
func (r *UserRepository) Create(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create user: %w", err)
	}
	defer tx.Rollback(ctx) // Commit が済んでいれば何もしない

	u, err := insertUser(ctx, tx, email)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_identities (user_id, provider, subject, password_hash) VALUES ($1, $2, $3, $4)
	`, u.ID, auth.ProviderPassword, strconv.FormatInt(u.ID, 10), passwordHash); err != nil {
		return nil, fmt.Errorf("create password identity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create user: %w", err)
	}
	return u, nil
}

// insertUser は users に 1 行足す。email が登録済みなら auth.ErrEmailTaken を返す。
func insertUser(ctx context.Context, tx pgx.Tx, email string) (*domain.User, error) {
	var u domain.User
	err := tx.QueryRow(ctx, `
		INSERT INTO users (email) VALUES ($1)
		RETURNING id, email, created_at
	`, email).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, auth.ErrEmailTaken
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

// FindByEmail はログイン時にパスワード検証するためハッシュ込みで返す。
// パスワードの行を INNER JOIN するので、Google だけで登録したユーザーは見つからない扱いになり、
// Login は存在しないメールアドレスと同じ経路（ダミーハッシュとの比較）を通る。
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*auth.UserWithHash, error) {
	var u auth.UserWithHash
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.email, i.password_hash, u.created_at
		FROM users u
		JOIN user_identities i ON i.user_id = u.id AND i.provider = $2
		WHERE u.email = $1
	`, email, auth.ProviderPassword).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrNotFound
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return &u, nil
}

// PasswordHash は users から LEFT JOIN し、「ユーザーがいない」（ErrNotFound）と
// 「パスワードを持たない」（ErrNoPassword）を区別する。
func (r *UserRepository) PasswordHash(ctx context.Context, userID int64) (string, error) {
	var hash *string
	err := r.pool.QueryRow(ctx, `
		SELECT i.password_hash
		FROM users u
		LEFT JOIN user_identities i ON i.user_id = u.id AND i.provider = $2
		WHERE u.id = $1
	`, userID, auth.ProviderPassword).Scan(&hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", auth.ErrNotFound
		}
		return "", fmt.Errorf("find password hash: %w", err)
	}
	if hash == nil {
		return "", auth.ErrNoPassword
	}
	return *hash, nil
}

// UpdatePassword は users と sessions の 2 つの表を触るが、SessionRepository ではなくここに置く。
// パスワードの差し替えと古いセッションの削除は 1 つのトランザクションでなければならず、
// リポジトリをまたぐとトランザクションを usecase に持ち出すことになるため。
func (r *UserRepository) UpdatePassword(ctx context.Context, userID int64, passwordHash, keepTokenHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin update password: %w", err)
	}
	defer tx.Rollback(ctx) // Commit が済んでいれば何もしない

	tag, err := tx.Exec(ctx, `UPDATE user_identities SET password_hash = $2 WHERE user_id = $1 AND provider = $3`,
		userID, passwordHash, auth.ProviderPassword)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return auth.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, userID, keepTokenHash); err != nil {
		return fmt.Errorf("delete other sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit update password: %w", err)
	}
	return nil
}
