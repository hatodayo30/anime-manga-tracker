package housekeeping_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/housekeeping"
)

type fakeRepo struct {
	sessionCalls int
	deleted      int64
	sessionErr   error
}

func (f *fakeRepo) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	f.sessionCalls++
	return f.deleted, f.sessionErr
}

// 1 回の実行で期限切れセッションの削除を 1 回だけ呼ぶ。消すものが無い回（0 件）も成功として扱う。
func TestRun_DeletesExpiredSessions(t *testing.T) {
	for _, deleted := range []int64{0, 3} {
		repo := &fakeRepo{deleted: deleted}

		if err := housekeeping.NewUsecase(repo).Run(context.Background()); err != nil {
			t.Fatalf("deleted=%d: unexpected error: %v", deleted, err)
		}
		if repo.sessionCalls != 1 {
			t.Errorf("deleted=%d: DeleteExpiredSessions called %d times, want 1", deleted, repo.sessionCalls)
		}
	}
}

// 削除の失敗は握りつぶさずに返す。scheduler がそれをログに出すので、黙って溜まり続けることがない。
// 元のエラーは errors.Is で辿れる。
func TestRun_ReturnsDeleteError(t *testing.T) {
	dbErr := errors.New("connection refused")
	repo := &fakeRepo{sessionErr: dbErr}

	err := housekeeping.NewUsecase(repo).Run(context.Background())
	if !errors.Is(err, dbErr) {
		t.Fatalf("expected error wrapping %v, got %v", dbErr, err)
	}
}
