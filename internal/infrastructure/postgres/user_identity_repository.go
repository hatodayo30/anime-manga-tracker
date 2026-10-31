package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

// UserRepository のうち、パスワード以外の手段（Google）でのログインに使う操作（BACKLOG 8-b）。
// user_identities の形は migrations/0007_user_identities.up.sql を参照。

// identityUserProviderKey は UNIQUE (user_id, provider) の制約名（PostgreSQL が付ける既定の名前）。
// 一意制約違反のうち、これだけを「別の識別子が紐付き済み」として区別する。
const identityUserProviderKey = "user_identities_user_id_provider_key"

func (r *UserRepository) FindByIdentity(ctx context.Context, provider, subject string) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.created_at
		FROM user_identities i
		JOIN users u ON u.id = i.user_id
		WHERE i.provider = $1 AND i.subject = $2
	`, provider, subject).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrNotFound
		}
		return nil, fmt.Errorf("find user by identity: %w", err)
	}
	return &u, nil
}

// LinkIdentityByEmail は users を引いてから user_identities に足す 2 文だが、トランザクションにはしない。
// 間にユーザーが消えれば外部キー違反で失敗するだけで、中途半端な行は残らない。
func (r *UserRepository) LinkIdentityByEmail(ctx context.Context, email, provider, subject string) (*domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, `SELECT id, email, created_at FROM users WHERE email = $1`, email).
		Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, auth.ErrNotFound
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}

	if _, err := r.pool.Exec(ctx, `
		INSERT INTO user_identities (user_id, provider, subject) VALUES ($1, $2, $3)
	`, u.ID, provider, subject); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == identityUserProviderKey {
			return nil, auth.ErrIdentityTaken
		}
		// (provider, subject) の違反は、同じ Google アカウントの初回ログインが並行して先に紐付けたとき。
		// まれなのでそのまま失敗させ、もう一度押せば FindByIdentity で入れる
		return nil, fmt.Errorf("link identity: %w", err)
	}
	return &u, nil
}

// CreateWithIdentity は users と user_identities の 2 つの表に書くので、1 つのトランザクションで行う。
// 別々だと、手段の無い（どの方法でもログインできない）ユーザーが残る失敗が起きうる。
func (r *UserRepository) CreateWithIdentity(ctx context.Context, email, provider, subject string) (*domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create user with identity: %w", err)
	}
	defer tx.Rollback(ctx) // Commit が済んでいれば何もしない

	u, err := insertUser(ctx, tx, email)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_identities (user_id, provider, subject) VALUES ($1, $2, $3)
	`, u.ID, provider, subject); err != nil {
		return nil, fmt.Errorf("create identity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create user with identity: %w", err)
	}
	return u, nil
}

// Providers は user_identities の provider を名前順で返す。アカウント画面が、パスワードを持たない
// ユーザーにパスワード変更の欄を出さないために使う。
func (r *UserRepository) Providers(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT provider FROM user_identities WHERE user_id = $1 ORDER BY provider
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	providers, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	return providers, nil
}
