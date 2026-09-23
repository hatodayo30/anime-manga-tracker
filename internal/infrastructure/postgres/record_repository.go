package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/record"
)

// RecordRepository は records テーブルへのアクセスを提供する。全操作はユーザーIDでスコープされる。
// usecase/record.Repository interface の実装。
type RecordRepository struct {
	pool *pgxpool.Pool
}

func NewRecordRepository(pool *pgxpool.Pool) *RecordRepository {
	return &RecordRepository{pool: pool}
}

var _ record.Repository = (*RecordRepository)(nil)

const recordColumns = `
	id, anilist_id, media_type, title, cover_image_url, genres,
	status, progress, total, next_airing_at, created_at, updated_at
`

func scanRecord(row pgx.Row) (*domain.Record, error) {
	var r domain.Record
	err := row.Scan(
		&r.ID, &r.AniListID, &r.MediaType, &r.Title, &r.CoverImageURL, &r.Genres,
		&r.Status, &r.Progress, &r.Total, &r.NextAiringAt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// List は種別・ステータスで記録を絞り込んで返す。どちらも空文字なら絞り込まない。
func (r *RecordRepository) List(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status) ([]*domain.Record, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM records
		WHERE user_id = $1
		  AND ($2 = '' OR media_type = $2)
		  AND ($3 = '' OR status = $3)
		ORDER BY
		  CASE WHEN next_airing_at IS NULL THEN 1 ELSE 0 END, next_airing_at ASC,
		  created_at DESC
	`, recordColumns)

	rows, err := r.pool.Query(ctx, query, userID, string(mediaType), string(status))
	if err != nil {
		return nil, fmt.Errorf("query records: %w", err)
	}
	defer rows.Close()

	var records []*domain.Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

// Upsert は AniList ID + 種別が一致する記録があれば更新、なければ新規作成する。
func (r *RecordRepository) Upsert(ctx context.Context, userID int64, in domain.NewRecordInput) (*domain.Record, error) {
	query := fmt.Sprintf(`
		INSERT INTO records (user_id, anilist_id, media_type, title, cover_image_url, genres, status, progress, total, next_airing_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, $8, $9)
		ON CONFLICT (user_id, anilist_id, media_type)
		DO UPDATE SET status = EXCLUDED.status, next_airing_at = EXCLUDED.next_airing_at, updated_at = now()
		RETURNING %s
	`, recordColumns)

	var nextAiringAt *time.Time
	if in.NextAiringAt != nil {
		t := time.Unix(*in.NextAiringAt, 0)
		nextAiringAt = &t
	}

	row := r.pool.QueryRow(ctx, query,
		userID, in.AniListID, in.MediaType, in.Title, in.CoverImageURL, in.Genres, in.Status, in.Total, nextAiringAt,
	)
	rec, err := scanRecord(row)
	if err != nil {
		return nil, fmt.Errorf("upsert record: %w", err)
	}
	return rec, nil
}

// Update は指定IDの記録のステータス/進捗を部分更新する。他ユーザーの記録は更新できない。
func (r *RecordRepository) Update(ctx context.Context, userID, id int64, in domain.UpdateRecordInput) (*domain.Record, error) {
	query := fmt.Sprintf(`
		UPDATE records SET
			status = COALESCE($3, status),
			progress = COALESCE($4, progress),
			updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING %s
	`, recordColumns)

	row := r.pool.QueryRow(ctx, query, id, userID, in.Status, in.Progress)
	rec, err := scanRecord(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, record.ErrNotFound
		}
		return nil, fmt.Errorf("update record: %w", err)
	}
	return rec, nil
}

// Delete は指定IDの記録をライブラリから削除する。他ユーザーの記録は削除できない。
func (r *RecordRepository) Delete(ctx context.Context, userID, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM records WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return record.ErrNotFound
	}
	return nil
}
