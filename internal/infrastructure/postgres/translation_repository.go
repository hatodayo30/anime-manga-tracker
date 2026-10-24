package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/translate"
)

// TranslationRepository は AI 翻訳の保存先。翻訳は原文に対して 1 つの共有データで、
// 利用者に紐付かないため user_id の条件を持たない（translate.Repository のコメントを参照）。
type TranslationRepository struct {
	pool *pgxpool.Pool
}

func NewTranslationRepository(pool *pgxpool.Pool) *TranslationRepository {
	return &TranslationRepository{pool: pool}
}

var _ translate.Repository = (*TranslationRepository)(nil)

func (r *TranslationRepository) Find(ctx context.Context, sourceSHA256 []byte, targetLang string) (string, error) {
	var translated string
	err := r.pool.QueryRow(ctx, `
		SELECT translated FROM translations WHERE source_sha256 = $1 AND target_lang = $2
	`, sourceSHA256, targetLang).Scan(&translated)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", translate.ErrNotFound
		}
		return "", fmt.Errorf("find translation: %w", err)
	}
	return translated, nil
}

// Save は既に保存済みなら何もしない。同じ原文を 2 人が同時に開くと AI が 2 回走って両方が保存しに来るが、
// どちらの訳でも構わないので先着を残す。
func (r *TranslationRepository) Save(ctx context.Context, sourceSHA256 []byte, targetLang, translated string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO translations (source_sha256, target_lang, translated) VALUES ($1, $2, $3)
		ON CONFLICT (source_sha256, target_lang) DO NOTHING
	`, sourceSHA256, targetLang, translated)
	if err != nil {
		return fmt.Errorf("save translation: %w", err)
	}
	return nil
}
