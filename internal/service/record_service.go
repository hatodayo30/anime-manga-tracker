// Package service はハンドラとリポジトリの間のビジネスロジックを担う。
package service

import (
	"context"
	"fmt"

	"github.com/hatodayo30/anime-manga-tracker/internal/model"
	"github.com/hatodayo30/anime-manga-tracker/internal/repository"
)

type RecordService struct {
	repo *repository.RecordRepository
}

func NewRecordService(repo *repository.RecordRepository) *RecordService {
	return &RecordService{repo: repo}
}

// List は種別・ステータスで記録を絞り込む。空文字は「絞り込みなし」を意味する。
func (s *RecordService) List(ctx context.Context, mediaType model.MediaType, status model.Status) ([]*model.Record, error) {
	if mediaType != "" && !mediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}
	if status != "" && !status.Valid() {
		return nil, fmt.Errorf("invalid status: %s", status)
	}
	return s.repo.List(ctx, mediaType, status)
}

// AddOrUpdateStatus は作品をライブラリに追加する。既に登録済みならステータスのみ更新する
// （検索画面でステータスボタンを押し直すケースに対応）。
func (s *RecordService) AddOrUpdateStatus(ctx context.Context, in model.NewRecordInput) (*model.Record, error) {
	if !in.MediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", in.MediaType)
	}
	if !in.Status.Valid() {
		return nil, fmt.Errorf("invalid status: %s", in.Status)
	}
	if in.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	return s.repo.Upsert(ctx, in)
}

// UpdateProgressOrStatus は既存レコードのステータス/進捗を部分更新する。
func (s *RecordService) UpdateProgressOrStatus(ctx context.Context, id int64, in model.UpdateRecordInput) (*model.Record, error) {
	if in.Status != nil && !in.Status.Valid() {
		return nil, fmt.Errorf("invalid status: %s", *in.Status)
	}
	if in.Progress != nil && *in.Progress < 0 {
		return nil, fmt.Errorf("progress must be >= 0")
	}
	return s.repo.Update(ctx, id, in)
}
