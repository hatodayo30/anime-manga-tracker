package home_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/jikan"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/home"
)

type fakeAniList struct {
	seasonAnimeCalls int32
	trendingManga    []anilist.SearchResult
}

func (f *fakeAniList) SeasonAnime(ctx context.Context) ([]anilist.SearchResult, error) {
	atomic.AddInt32(&f.seasonAnimeCalls, 1)
	return []anilist.SearchResult{{Title: "Current Season Anime"}}, nil
}

func (f *fakeAniList) SeasonAnimeFor(ctx context.Context, season string, year int) ([]anilist.SearchResult, error) {
	return []anilist.SearchResult{{Title: season}}, nil
}

func (f *fakeAniList) TrendingAnime(ctx context.Context) ([]anilist.SearchResult, error) {
	return []anilist.SearchResult{{Title: "Trending"}}, nil
}

func (f *fakeAniList) TrendingManga(ctx context.Context) ([]anilist.SearchResult, error) {
	return f.trendingManga, nil
}

type fakeJikan struct {
	getMangaFunc func(ctx context.Context, malID int64) (*jikan.MangaStats, error)
}

func (f *fakeJikan) GetManga(ctx context.Context, malID int64) (*jikan.MangaStats, error) {
	return f.getMangaFunc(ctx, malID)
}

func TestUsecase_CurrentSeasonAnime_Caches(t *testing.T) {
	anilistGW := &fakeAniList{}
	u := home.NewUsecase(anilistGW, &fakeJikan{})

	if _, err := u.CurrentSeasonAnime(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := u.CurrentSeasonAnime(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if anilistGW.seasonAnimeCalls != 1 {
		t.Errorf("expected gateway to be called once (cached on second call), got %d calls", anilistGW.seasonAnimeCalls)
	}
}

func TestUsecase_TrendingManga_EnrichesWithJikanStats(t *testing.T) {
	malID := int64(42)
	anilistGW := &fakeAniList{
		trendingManga: []anilist.SearchResult{
			{Title: "Vinland Saga", MalID: &malID},
			{Title: "No MAL ID", MalID: nil},
		},
	}
	rank := 3
	members := 1000
	jikanGW := &fakeJikan{
		getMangaFunc: func(ctx context.Context, id int64) (*jikan.MangaStats, error) {
			if id != malID {
				t.Errorf("unexpected malID: %d", id)
			}
			return &jikan.MangaStats{Rank: &rank, Members: &members, Magazines: []string{"Weekly Shonen"}}, nil
		},
	}
	u := home.NewUsecase(anilistGW, jikanGW)

	items, err := u.TrendingManga(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].MalRank == nil || *items[0].MalRank != rank {
		t.Errorf("expected MalRank %d, got %+v", rank, items[0].MalRank)
	}
	if items[1].MalRank != nil {
		t.Errorf("expected no enrichment for item without MalID, got %+v", items[1])
	}
}

func TestUsecase_TrendingManga_ToleratesJikanFailure(t *testing.T) {
	malID := int64(1)
	anilistGW := &fakeAniList{
		trendingManga: []anilist.SearchResult{{Title: "X", MalID: &malID}},
	}
	jikanGW := &fakeJikan{
		getMangaFunc: func(ctx context.Context, id int64) (*jikan.MangaStats, error) {
			return nil, context.DeadlineExceeded
		},
	}
	u := home.NewUsecase(anilistGW, jikanGW)

	items, err := u.TrendingManga(context.Background())
	if err != nil {
		t.Fatalf("expected jikan failure to be tolerated, got error: %v", err)
	}
	if len(items) != 1 || items[0].MalRank != nil {
		t.Errorf("expected item without enrichment, got %+v", items)
	}
}
