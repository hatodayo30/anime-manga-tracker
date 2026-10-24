package taxonomy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/taxonomy"
)

type fakeAniList struct {
	result anilist.Taxonomy
	err    error

	calls int
}

func (f *fakeAniList) Taxonomy(ctx context.Context) (anilist.Taxonomy, error) {
	f.calls++
	return f.result, f.err
}

// 2 回目以降は AniList を呼ばずにキャッシュから返すことを保証する。
// 呼ぶたびに取ると、自然文検索 1 回で AniList を 2 本使ってしまう。
func TestUsecase_Get_CachesResult(t *testing.T) {
	fake := &fakeAniList{result: anilist.Taxonomy{
		Genres: []string{"Action"},
		Tags:   []anilist.MediaTag{{Name: "Isekai"}},
	}}
	u := taxonomy.NewUsecase(fake)

	for range 3 {
		got, err := u.Get(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Genres) != 1 || got.Genres[0] != "Action" || len(got.Tags) != 1 || got.Tags[0].Name != "Isekai" {
			t.Fatalf("unexpected taxonomy: %+v", got)
		}
	}
	if fake.calls != 1 {
		t.Errorf("expected AniList to be called once, got %d", fake.calls)
	}
}

// 取得の失敗はキャッシュせず、次の呼び出しで取り直すことを保証する。
// 失敗を覚えると、AniList が戻っても AI の機能が TTL のあいだ止まったままになる。
func TestUsecase_Get_DoesNotCacheErrors(t *testing.T) {
	fake := &fakeAniList{err: errors.New("anilist down")}
	u := taxonomy.NewUsecase(fake)

	if _, err := u.Get(context.Background()); err == nil {
		t.Fatal("expected error")
	}

	fake.err = nil
	fake.result = anilist.Taxonomy{Genres: []string{"Drama"}}
	got, err := u.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Genres) != 1 || got.Genres[0] != "Drama" {
		t.Errorf("unexpected taxonomy: %+v", got)
	}
	if fake.calls != 2 {
		t.Errorf("expected AniList to be called twice, got %d", fake.calls)
	}
}
