package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

var _ auth.SessionRepository = (*SessionRepository)(nil)

func (r *SessionRepository) Create(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)
	`, token, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// FindUser は有効期限内のセッションからユーザーを引く。期限切れ・不存在なら ErrNotFound。
func (r *SessionRepository) FindUser(ctx context.Context, token string) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = $1 AND s.expires_at > now()
	`, token).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrNotFound
		}
		return nil, fmt.Errorf("find session user: %w", err)
	}
	return &u, nil
}

func (r *SessionRepository) Delete(ctx context.Context, token string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
