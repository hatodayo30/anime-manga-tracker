package record_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/record"
)

// fakeRepository は record.Repository のテスト用実装。呼び出された引数と、返す戻り値を記録する。
type fakeRepository struct {
	listFunc   func(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status) ([]*domain.Record, error)
	upsertFunc func(ctx context.Context, userID int64, in domain.NewRecordInput) (*domain.Record, error)
	updateFunc func(ctx context.Context, userID, id int64, in domain.UpdateRecordInput) (*domain.Record, error)
	deleteFunc func(ctx context.Context, userID, id int64) error

	called bool
}

func (f *fakeRepository) List(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status) ([]*domain.Record, error) {
	f.called = true
	return f.listFunc(ctx, userID, mediaType, status)
}

func (f *fakeRepository) Upsert(ctx context.Context, userID int64, in domain.NewRecordInput) (*domain.Record, error) {
	f.called = true
	return f.upsertFunc(ctx, userID, in)
}

func (f *fakeRepository) Update(ctx context.Context, userID, id int64, in domain.UpdateRecordInput) (*domain.Record, error) {
	f.called = true
	return f.updateFunc(ctx, userID, id, in)
}

func (f *fakeRepository) Delete(ctx context.Context, userID, id int64) error {
	f.called = true
	return f.deleteFunc(ctx, userID, id)
}

func TestUsecase_List_InvalidMediaType(t *testing.T) {
	repo := &fakeRepository{}
	u := record.NewUsecase(repo)

	_, err := u.List(context.Background(), 1, "movie", "")
	if err == nil {
		t.Fatal("expected error for invalid media type, got nil")
	}
	if repo.called {
		t.Error("repository should not be called when validation fails")
	}
}

func TestUsecase_List_InvalidStatus(t *testing.T) {
	repo := &fakeRepository{}
	u := record.NewUsecase(repo)

	_, err := u.List(context.Background(), 1, "", "paused")
	if err == nil {
		t.Fatal("expected error for invalid status, got nil")
	}
	if repo.called {
		t.Error("repository should not be called when validation fails")
	}
}

func TestUsecase_List_DelegatesToRepository(t *testing.T) {
	want := []*domain.Record{{ID: 1, Title: "Frieren"}}
	repo := &fakeRepository{
		listFunc: func(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status) ([]*domain.Record, error) {
			if userID != 42 || mediaType != domain.MediaTypeAnime || status != domain.StatusActive {
				t.Errorf("unexpected args: userID=%d mediaType=%s status=%s", userID, mediaType, status)
			}
			return want, nil
		},
	}
	u := record.NewUsecase(repo)

	got, err := u.List(context.Background(), 42, domain.MediaTypeAnime, domain.StatusActive)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUsecase_AddOrUpdateStatus_Validation(t *testing.T) {
	tests := []struct {
		name string
		in   domain.NewRecordInput
	}{
		{"invalid media type", domain.NewRecordInput{MediaType: "movie", Status: domain.StatusWant, Title: "X"}},
		{"invalid status", domain.NewRecordInput{MediaType: domain.MediaTypeAnime, Status: "paused", Title: "X"}},
		{"missing title", domain.NewRecordInput{MediaType: domain.MediaTypeAnime, Status: domain.StatusWant, Title: ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			u := record.NewUsecase(repo)

			_, err := u.AddOrUpdateStatus(context.Background(), 1, tt.in)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if repo.called {
				t.Error("repository should not be called when validation fails")
			}
		})
	}
}

func TestUsecase_AddOrUpdateStatus_DelegatesToRepository(t *testing.T) {
	in := domain.NewRecordInput{
		AniListID: 100,
		MediaType: domain.MediaTypeManga,
		Title:     "Vinland Saga",
		Status:    domain.StatusActive,
	}
	want := &domain.Record{ID: 7, Title: in.Title}
	repo := &fakeRepository{
		upsertFunc: func(ctx context.Context, userID int64, got domain.NewRecordInput) (*domain.Record, error) {
			if userID != 1 || got.AniListID != in.AniListID || got.MediaType != in.MediaType || got.Title != in.Title || got.Status != in.Status {
				t.Errorf("unexpected args: userID=%d in=%+v", userID, got)
			}
			return want, nil
		},
	}
	u := record.NewUsecase(repo)

	got, err := u.AddOrUpdateStatus(context.Background(), 1, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUsecase_UpdateProgressOrStatus_Validation(t *testing.T) {
	invalidStatus := domain.Status("paused")
	negativeProgress := -1

	tests := []struct {
		name string
		in   domain.UpdateRecordInput
	}{
		{"invalid status", domain.UpdateRecordInput{Status: &invalidStatus}},
		{"negative progress", domain.UpdateRecordInput{Progress: &negativeProgress}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{}
			u := record.NewUsecase(repo)

			_, err := u.UpdateProgressOrStatus(context.Background(), 1, 2, tt.in)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if repo.called {
				t.Error("repository should not be called when validation fails")
			}
		})
	}
}

func TestUsecase_UpdateProgressOrStatus_NotFound(t *testing.T) {
	repo := &fakeRepository{
		updateFunc: func(ctx context.Context, userID, id int64, in domain.UpdateRecordInput) (*domain.Record, error) {
			return nil, record.ErrNotFound
		},
	}
	u := record.NewUsecase(repo)

	progress := 5
	_, err := u.UpdateProgressOrStatus(context.Background(), 1, 2, domain.UpdateRecordInput{Progress: &progress})
	if !errors.Is(err, record.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUsecase_Delete_DelegatesToRepository(t *testing.T) {
	repo := &fakeRepository{
		deleteFunc: func(ctx context.Context, userID, id int64) error {
			if userID != 1 || id != 2 {
				t.Errorf("unexpected args: userID=%d id=%d", userID, id)
			}
			return nil
		},
	}
	u := record.NewUsecase(repo)

	if err := u.Delete(context.Background(), 1, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUsecase_Delete_NotFound(t *testing.T) {
	repo := &fakeRepository{
		deleteFunc: func(ctx context.Context, userID, id int64) error {
			return record.ErrNotFound
		},
	}
	u := record.NewUsecase(repo)

	err := u.Delete(context.Background(), 1, 2)
	if !errors.Is(err, record.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
