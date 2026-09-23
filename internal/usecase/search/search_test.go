package search_test

import (
	"context"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/search"
)

type fakeAniList struct {
	searchFunc     func(ctx context.Context, query string, mediaType domain.MediaType) ([]anilist.SearchResult, error)
	mediaByIDsFunc func(ctx context.Context, ids []int64, mediaType domain.MediaType) ([]anilist.SearchResult, error)
	relationsFunc  func(ctx context.Context, id int64) ([]anilist.RelatedWork, error)
	byGenresFunc   func(ctx context.Context, genres []string, mediaType domain.MediaType) ([]anilist.SearchResult, error)
	byGenresCalls  int
}

func (f *fakeAniList) Search(ctx context.Context, query string, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
	return f.searchFunc(ctx, query, mediaType)
}

func (f *fakeAniList) MediaByIDs(ctx context.Context, ids []int64, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
	return f.mediaByIDsFunc(ctx, ids, mediaType)
}

func (f *fakeAniList) Relations(ctx context.Context, id int64) ([]anilist.RelatedWork, error) {
	return f.relationsFunc(ctx, id)
}

func (f *fakeAniList) ByGenres(ctx context.Context, genres []string, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
	f.byGenresCalls++
	return f.byGenresFunc(ctx, genres, mediaType)
}

type fakeJikan struct {
	searchAnimeFunc func(ctx context.Context, query string) ([]anilist.SearchResult, error)
	searchMangaFunc func(ctx context.Context, query string) ([]anilist.SearchResult, error)
	called          bool
}

func (f *fakeJikan) SearchAnime(ctx context.Context, query string) ([]anilist.SearchResult, error) {
	f.called = true
	return f.searchAnimeFunc(ctx, query)
}

func (f *fakeJikan) SearchManga(ctx context.Context, query string) ([]anilist.SearchResult, error) {
	f.called = true
	return f.searchMangaFunc(ctx, query)
}

func TestUsecase_Search_NoFallbackWhenEnoughResults(t *testing.T) {
	many := make([]anilist.SearchResult, 5)
	anilistGW := &fakeAniList{
		searchFunc: func(ctx context.Context, query string, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
			return many, nil
		},
	}
	jikanGW := &fakeJikan{}
	u := search.NewUsecase(anilistGW, jikanGW)

	results, err := u.Search(context.Background(), domain.MediaTypeAnime, "frieren")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("got %d results, want 5", len(results))
	}
	if jikanGW.called {
		t.Error("jikan should not be called when anilist already has enough results")
	}
}

func TestUsecase_Search_NoFallbackWhenQueryEmpty(t *testing.T) {
	anilistGW := &fakeAniList{
		searchFunc: func(ctx context.Context, query string, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
			return nil, nil
		},
	}
	jikanGW := &fakeJikan{}
	u := search.NewUsecase(anilistGW, jikanGW)

	if _, err := u.Search(context.Background(), domain.MediaTypeAnime, "  "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jikanGW.called {
		t.Error("jikan should not be called for an empty query")
	}
}

func TestUsecase_Search_FallbackMergesAndDedupes(t *testing.T) {
	anilistGW := &fakeAniList{
		searchFunc: func(ctx context.Context, query string, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
			return []anilist.SearchResult{
				{Title: "Frieren", Popularity: 50},
			}, nil
		},
	}
	jikanGW := &fakeJikan{
		searchAnimeFunc: func(ctx context.Context, query string) ([]anilist.SearchResult, error) {
			return []anilist.SearchResult{
				{Title: "frieren", Popularity: 10},             // duplicate (case-insensitive) — should be dropped
				{Title: "Sousou no Frieren 2", Popularity: 90}, // unique, higher popularity
			}, nil
		},
	}
	u := search.NewUsecase(anilistGW, jikanGW)

	results, err := u.Search(context.Background(), domain.MediaTypeAnime, "frieren")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !jikanGW.called {
		t.Fatal("expected jikan fallback to be triggered")
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (deduped)", len(results))
	}
	if results[0].Title != "Sousou no Frieren 2" {
		t.Errorf("expected results sorted by popularity desc, got %+v", results)
	}
}

func TestUsecase_Recommendations_CachesByKey(t *testing.T) {
	anilistGW := &fakeAniList{
		byGenresFunc: func(ctx context.Context, genres []string, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
			return []anilist.SearchResult{{Title: "X"}}, nil
		},
	}
	u := search.NewUsecase(anilistGW, &fakeJikan{})

	if _, err := u.Recommendations(context.Background(), domain.MediaTypeAnime, []string{"Action"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := u.Recommendations(context.Background(), domain.MediaTypeAnime, []string{"Action"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anilistGW.byGenresCalls != 1 {
		t.Errorf("expected gateway to be called once (cached on second call), got %d calls", anilistGW.byGenresCalls)
	}

	if _, err := u.Recommendations(context.Background(), domain.MediaTypeManga, []string{"Action"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anilistGW.byGenresCalls != 2 {
		t.Errorf("expected a different media type to bypass the cache, got %d calls", anilistGW.byGenresCalls)
	}
}
