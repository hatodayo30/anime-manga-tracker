package postgres

import (
	"context"
	"errors"
	"fmt"

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

// Create はユーザーを作成する。email は UNIQUE 制約により重複登録を防ぐ。
func (r *UserRepository) Create(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash) VALUES ($1, $2)
		RETURNING id, email, created_at
	`, email, passwordHash).Scan(&u.ID, &u.Email, &u.CreatedAt)
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
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*auth.UserWithHash, error) {
	var u auth.UserWithHash
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, password_hash, created_at FROM users WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrNotFound
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return &u, nil
}
