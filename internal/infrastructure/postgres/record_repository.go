package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/airing"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/record"
)

// RecordRepository は records テーブルへのアクセスを提供する。利用者からの操作は全て
// ユーザーIDでスコープされる（定期ジョブ向けの airing.Repository だけが例外で、
// 保守のために全ユーザーのレコードを横断する）。
// usecase/record.Repository と usecase/airing.Repository interface の実装。
type RecordRepository struct {
	pool *pgxpool.Pool
}

func NewRecordRepository(pool *pgxpool.Pool) *RecordRepository {
	return &RecordRepository{pool: pool}
}

var (
	_ record.Repository = (*RecordRepository)(nil)
	_ airing.Repository = (*RecordRepository)(nil)
)

const recordColumns = `
	id, anilist_id, media_type, title, cover_image_url, genres,
	status, progress, total, next_airing_at, rating, memo, created_at, updated_at
`

func scanRecord(row pgx.Row) (*domain.Record, error) {
	var r domain.Record
	err := row.Scan(
		&r.ID, &r.AniListID, &r.MediaType, &r.Title, &r.CoverImageURL, &r.Genres,
		&r.Status, &r.Progress, &r.Total, &r.NextAiringAt, &r.Rating, &r.Memo, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// orderByClauses は domain.SortKey ごとの ORDER BY 句。SQLに直接埋め込むため、
// ここに定義済みのキー以外は絶対にクエリへ渡さない（List の先頭でフォールバックする）。
// タイトル順は ICU の日本語コレーションを使う。読み仮名を持たないため漢字タイトルは
// 厳密な五十音順にはならないが、コードポイント順よりは日本語として自然な並びになる。
// 同値時は id で決着を付け、ページ再読み込みで順序が揺れないようにする。
var orderByClauses = map[domain.SortKey]string{
	domain.SortDefault: `CASE WHEN next_airing_at IS NULL THEN 1 ELSE 0 END, next_airing_at ASC, created_at DESC, id DESC`,
	domain.SortTitle:   `title COLLATE "ja-x-icu" ASC, id ASC`,
	domain.SortScore:   `rating DESC NULLS LAST, updated_at DESC, id DESC`,
	domain.SortUpdated: `updated_at DESC, id DESC`,
	domain.SortAdded:   `created_at DESC, id DESC`,
}

// List は種別・ステータスで記録を絞り込み、sort の順で返す。種別・ステータスはどちらも空文字なら絞り込まない。
func (r *RecordRepository) List(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status, sort domain.SortKey) ([]*domain.Record, error) {
	orderBy, ok := orderByClauses[sort]
	if !ok {
		orderBy = orderByClauses[domain.SortDefault]
	}

	query := fmt.Sprintf(`
		SELECT %s FROM records
		WHERE user_id = $1
		  AND ($2 = '' OR media_type = $2)
		  AND ($3 = '' OR status = $3)
		ORDER BY %s
	`, recordColumns, orderBy)

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

// Update は指定IDの記録のステータス/進捗/総数/評価/メモを部分更新する。他ユーザーの記録は更新できない。
// rating は 0 を渡すと NULL（未評価）に戻す特別扱い。
func (r *RecordRepository) Update(ctx context.Context, userID, id int64, in domain.UpdateRecordInput) (*domain.Record, error) {
	query := fmt.Sprintf(`
		UPDATE records SET
			status = COALESCE($3, status),
			progress = COALESCE($4, progress),
			total = COALESCE($5, total),
			rating = CASE WHEN $6::smallint IS NULL THEN rating WHEN $6::smallint = 0 THEN NULL ELSE $6::smallint END,
			memo = COALESCE($7, memo),
			updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING %s
	`, recordColumns)

	row := r.pool.QueryRow(ctx, query, id, userID, in.Status, in.Progress, in.Total, in.Rating, in.Memo)
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

// StaleAiringAnimeIDs は next_airing_at が過去の日時のまま残っているアニメの AniList ID を、
// 古い順に最大 limit 件返す。定期ジョブ（usecase/airing）が AniList から引き直す対象を選ぶために使う。
//
// 同じ作品を複数の利用者が登録していても AniList への問い合わせは1回で足りるので、
// anilist_id で重複を除く。並び順は「最も長く腐っているものから」。
func (r *RecordRepository) StaleAiringAnimeIDs(ctx context.Context, limit int) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT anilist_id FROM records
		WHERE media_type = 'anime'
		  AND next_airing_at IS NOT NULL
		  AND next_airing_at <= now()
		GROUP BY anilist_id
		ORDER BY min(next_airing_at) ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query stale airing anime ids: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan anilist id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpdateNextAiringAt は AniList ID が一致するアニメレコードの next_airing_at を、
// 全ユーザーぶんまとめて書き換える。戻り値は実際に値が変わった行数。
//
// updated_at は触らない。これは利用者の操作ではなく裏での同期なので、更新すると
// ライブラリの「最近更新した順」（domain.SortUpdated）が定期ジョブで塗り替わってしまう。
// 同じ理由で、値が変わらない行は WHERE で弾いて空更新を避ける。
func (r *RecordRepository) UpdateNextAiringAt(ctx context.Context, updates []airing.NextAiring) (int64, error) {
	if len(updates) == 0 {
		return 0, nil
	}

	ids := make([]int64, len(updates))
	times := make([]*time.Time, len(updates))
	for i, u := range updates {
		ids[i] = u.AniListID
		times[i] = u.At
	}

	// 1作品ずつ UPDATE を投げると往復が件数ぶん増えるので、配列を渡して1文にまとめる。
	tag, err := r.pool.Exec(ctx, `
		UPDATE records AS r SET next_airing_at = v.next_airing_at
		FROM (
			SELECT unnest($1::bigint[]) AS anilist_id, unnest($2::timestamptz[]) AS next_airing_at
		) AS v
		WHERE r.media_type = 'anime'
		  AND r.anilist_id = v.anilist_id
		  AND r.next_airing_at IS DISTINCT FROM v.next_airing_at
	`, ids, times)
	if err != nil {
		return 0, fmt.Errorf("update next airing at: %w", err)
	}
	return tag.RowsAffected(), nil
}
