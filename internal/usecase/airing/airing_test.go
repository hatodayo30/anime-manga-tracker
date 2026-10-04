package airing_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/airing"
)

type fakeRepo struct {
	staleIDs    []int64
	staleErr    error
	staleLimit  int
	updateErr   error
	updateCalls int
	updates     []airing.NextAiring
}

func (f *fakeRepo) StaleAiringAnimeIDs(ctx context.Context, limit int) ([]int64, error) {
	f.staleLimit = limit
	return f.staleIDs, f.staleErr
}

func (f *fakeRepo) UpdateNextAiringAt(ctx context.Context, updates []airing.NextAiring) (int64, error) {
	f.updateCalls++
	f.updates = updates
	return int64(len(updates)), f.updateErr
}

type fakeAniList struct {
	calls     int
	gotIDs    []int64
	gotType   domain.MediaType
	results   []anilist.SearchResult
	returnErr error
}

func (f *fakeAniList) MediaByIDs(ctx context.Context, ids []int64, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
	f.calls++
	f.gotIDs = ids
	f.gotType = mediaType
	return f.results, f.returnErr
}

func result(id int64, airingAt *int64) anilist.SearchResult {
	return anilist.SearchResult{AniListID: id, NextAiringAt: airingAt}
}

func ptr[T any](v T) *T { return &v }

// 問い合わせたID全件ぶんの更新内容が書き戻る。AniListが次話を返したものは その日時、
// 次話を返さなかったもの（放送終了）と、そもそも返ってこなかったもの（存在しないID）は NULL。
// 後者を NULL にしないと過去の日時が残り続け、毎回の対象に居座ってバッチ枠を食い潰す。
func TestSync_WritesOneUpdatePerRequestedID(t *testing.T) {
	airingAt := int64(1759500000)
	repo := &fakeRepo{staleIDs: []int64{1, 2, 3}}
	gw := &fakeAniList{results: []anilist.SearchResult{
		result(1, &airingAt),
		result(2, nil), // 放送終了
		// 3 は返ってこない（削除された等）
	}}

	if err := airing.NewUsecase(repo, gw).Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if gw.gotType != domain.MediaTypeAnime {
		t.Errorf("fetched media type = %q, want %q", gw.gotType, domain.MediaTypeAnime)
	}
	want := []airing.NextAiring{
		{AniListID: 1, At: ptr(time.Unix(airingAt, 0))},
		{AniListID: 2, At: nil},
		{AniListID: 3, At: nil},
	}
	if len(repo.updates) != len(want) {
		t.Fatalf("got %d updates, want %d: %+v", len(repo.updates), len(want), repo.updates)
	}
	for i, w := range want {
		got := repo.updates[i]
		if got.AniListID != w.AniListID {
			t.Errorf("updates[%d].AniListID = %d, want %d", i, got.AniListID, w.AniListID)
		}
		switch {
		case w.At == nil && got.At != nil:
			t.Errorf("updates[%d].At = %v, want nil", i, *got.At)
		case w.At != nil && got.At == nil:
			t.Errorf("updates[%d].At = nil, want %v", i, *w.At)
		case w.At != nil && !got.At.Equal(*w.At):
			t.Errorf("updates[%d].At = %v, want %v", i, *got.At, *w.At)
		}
	}
}

// 腐ったレコードが無ければ AniList を叩かない。スロットルは利用者のリクエストと共有なので、
// 用がないときに枠を消費しないこと自体が要件。
func TestSync_NoStaleRecordsSkipsAniList(t *testing.T) {
	repo := &fakeRepo{staleIDs: nil}
	gw := &fakeAniList{}

	if err := airing.NewUsecase(repo, gw).Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if gw.calls != 0 {
		t.Errorf("anilist called %d times, want 0", gw.calls)
	}
	if repo.updateCalls != 0 {
		t.Errorf("update called %d times, want 0", repo.updateCalls)
	}
}

// AniList 側が失敗したら書き戻さない。取れなかったぶんを NULL で潰すと、
// 一時的な障害で全ライブラリの次話表示が消える。
func TestSync_AniListErrorDoesNotWrite(t *testing.T) {
	repo := &fakeRepo{staleIDs: []int64{1}}
	gw := &fakeAniList{returnErr: errors.New("anilist down")}

	err := airing.NewUsecase(repo, gw).Sync(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if repo.updateCalls != 0 {
		t.Errorf("update called %d times, want 0", repo.updateCalls)
	}
}

func TestSync_RepositoryErrorIsReturned(t *testing.T) {
	repo := &fakeRepo{staleErr: errors.New("db down")}
	gw := &fakeAniList{}

	if err := airing.NewUsecase(repo, gw).Sync(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
	if gw.calls != 0 {
		t.Errorf("anilist called %d times, want 0", gw.calls)
	}
}

// 1回の実行で取り直す件数には上限があり、AniListの1リクエスト上限（50件）の倍数に収まっている。
func TestSync_RequestsBoundedBatch(t *testing.T) {
	repo := &fakeRepo{staleIDs: []int64{1}}
	if err := airing.NewUsecase(repo, &fakeAniList{}).Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if repo.staleLimit <= 0 || repo.staleLimit > 150 {
		t.Errorf("batch limit = %d, want a small positive bound", repo.staleLimit)
	}
}
