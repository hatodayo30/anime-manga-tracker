// Package record は記録（ライブラリ）まわりのユースケースを担う。
package record

import (
	"context"
	"fmt"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// Usecase は記録の参照・追加・更新・削除のビジネスロジックを提供する。
type Usecase struct {
	repo Repository
}

func NewUsecase(repo Repository) *Usecase {
	return &Usecase{repo: repo}
}

// List は種別・ステータスで記録を絞り込む。空文字は「絞り込みなし」を意味する。
func (u *Usecase) List(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status) ([]*domain.Record, error) {
	if mediaType != "" && !mediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}
	if status != "" && !status.Valid() {
		return nil, fmt.Errorf("invalid status: %s", status)
	}
	return u.repo.List(ctx, userID, mediaType, status)
}

// AddOrUpdateStatus は作品をライブラリに追加する。既に登録済みならステータスのみ更新する
// （検索画面でステータスボタンを押し直すケースに対応）。
func (u *Usecase) AddOrUpdateStatus(ctx context.Context, userID int64, in domain.NewRecordInput) (*domain.Record, error) {
	if !in.MediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", in.MediaType)
	}
	if !in.Status.Valid() {
		return nil, fmt.Errorf("invalid status: %s", in.Status)
	}
	if in.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	return u.repo.Upsert(ctx, userID, in)
}

// UpdateProgressOrStatus は既存レコードのステータス/進捗を部分更新する。
func (u *Usecase) UpdateProgressOrStatus(ctx context.Context, userID, id int64, in domain.UpdateRecordInput) (*domain.Record, error) {
	if in.Status != nil && !in.Status.Valid() {
		return nil, fmt.Errorf("invalid status: %s", *in.Status)
	}
	if in.Progress != nil && *in.Progress < 0 {
		return nil, fmt.Errorf("progress must be >= 0")
	}
	return u.repo.Update(ctx, userID, id, in)
}

// Delete は記録をライブラリから削除する（詳細モーダルの「記録から外す」）。
func (u *Usecase) Delete(ctx context.Context, userID, id int64) error {
	return u.repo.Delete(ctx, userID, id)
}
