package home_test

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/jikan"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/home"
)

type fakeAniList struct {
	seasonAnimeCalls    int32
	seasonAnimeForCalls int32
	trendingAnimeCalls  int32
	trendingMangaCalls  int32
	trendingManga       []anilist.SearchResult
	err                 error // 非nilなら全メソッドがこれを返す
}

func (f *fakeAniList) SeasonAnime(ctx context.Context) ([]anilist.SearchResult, error) {
	atomic.AddInt32(&f.seasonAnimeCalls, 1)
	return []anilist.SearchResult{{Title: "Current Season Anime"}}, f.err
}

func (f *fakeAniList) SeasonAnimeFor(ctx context.Context, season string, year int) ([]anilist.SearchResult, error) {
	atomic.AddInt32(&f.seasonAnimeForCalls, 1)
	return []anilist.SearchResult{{Title: season + ":" + strconv.Itoa(year)}}, nil
}

func (f *fakeAniList) TrendingAnime(ctx context.Context) ([]anilist.SearchResult, error) {
	atomic.AddInt32(&f.trendingAnimeCalls, 1)
	return []anilist.SearchResult{{Title: "Trending"}}, f.err
}

func (f *fakeAniList) TrendingManga(ctx context.Context) ([]anilist.SearchResult, error) {
	atomic.AddInt32(&f.trendingMangaCalls, 1)
	return f.trendingManga, f.err
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

// シーズンブラウジング（season/year指定）も、同じ組み合わせなら2回目以降はキャッシュから返す。
// 組み合わせが違えばそれぞれ取得する。
func TestUsecase_SeasonAnimeFor_CachesPerSeasonAndYear(t *testing.T) {
	anilistGW := &fakeAniList{}
	u := home.NewUsecase(anilistGW, &fakeJikan{})

	first, err := u.SeasonAnimeFor(context.Background(), "SPRING", 2025)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := u.SeasonAnimeFor(context.Background(), "SPRING", 2025)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anilistGW.seasonAnimeForCalls != 1 {
		t.Errorf("expected the same season/year to be cached, got %d calls", anilistGW.seasonAnimeForCalls)
	}
	if len(second) != 1 || second[0].Title != first[0].Title {
		t.Errorf("cached result differs from the first: %+v vs %+v", second, first)
	}

	// シーズンが違えば別キー
	if _, err := u.SeasonAnimeFor(context.Background(), "SUMMER", 2025); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 年が違っても別キー（season単体をキーにしていないことの確認）
	if _, err := u.SeasonAnimeFor(context.Background(), "SPRING", 2024); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if anilistGW.seasonAnimeForCalls != 3 {
		t.Errorf("expected distinct season/year combinations to fetch separately, got %d calls", anilistGW.seasonAnimeForCalls)
	}
}

// ホーム画面向けの現在シーズンとシーズンブラウジングはキャッシュが独立している。
func TestUsecase_SeasonAnimeFor_DoesNotShareCacheWithCurrentSeason(t *testing.T) {
	anilistGW := &fakeAniList{}
	u := home.NewUsecase(anilistGW, &fakeJikan{})

	current, err := u.CurrentSeasonAnime(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	browsed, err := u.SeasonAnimeFor(context.Background(), "WINTER", 2021)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if anilistGW.seasonAnimeCalls != 1 || anilistGW.seasonAnimeForCalls != 1 {
		t.Errorf("expected one call to each gateway method, got %d and %d",
			anilistGW.seasonAnimeCalls, anilistGW.seasonAnimeForCalls)
	}
	if current[0].Title == browsed[0].Title {
		t.Errorf("expected the browsing cache to be independent of the home cache, both returned %q", current[0].Title)
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

// Warm はホーム画面向けの3つのキャッシュを埋める。埋めたあとは外部APIを叩かずに返る。
func TestUsecase_Warm_FillsHomeCaches(t *testing.T) {
	anilistGW := &fakeAniList{}
	u := home.NewUsecase(anilistGW, &fakeJikan{})

	if err := u.Warm(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := u.CurrentSeasonAnime(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := u.TrendingAnime(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := u.TrendingManga(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if anilistGW.seasonAnimeCalls != 1 || anilistGW.trendingAnimeCalls != 1 || anilistGW.trendingMangaCalls != 1 {
		t.Errorf("expected the warmed caches to serve the requests, got %d/%d/%d calls",
			anilistGW.seasonAnimeCalls, anilistGW.trendingAnimeCalls, anilistGW.trendingMangaCalls)
	}
}

// Warm はTTLが残っていても取り直す。TTLが切れてから動くのでは、利用者が古い値を踏む前に
// 入れ替えるという目的を果たせない。
func TestUsecase_Warm_RefreshesWithinTTL(t *testing.T) {
	anilistGW := &fakeAniList{}
	u := home.NewUsecase(anilistGW, &fakeJikan{})

	if _, err := u.CurrentSeasonAnime(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := u.Warm(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if anilistGW.seasonAnimeCalls != 2 {
		t.Errorf("expected Warm to refetch inside the TTL, got %d calls", anilistGW.seasonAnimeCalls)
	}
}

// シーズンブラウジング用キャッシュは温めない（キーが際限なく増えうるため）。
func TestUsecase_Warm_SkipsSeasonBrowseCache(t *testing.T) {
	anilistGW := &fakeAniList{}
	u := home.NewUsecase(anilistGW, &fakeJikan{})

	if err := u.Warm(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if anilistGW.seasonAnimeForCalls != 0 {
		t.Errorf("expected the season browsing cache to be left alone, got %d calls", anilistGW.seasonAnimeForCalls)
	}
}

// 1つ失敗しても残りは取りに行き、失敗はまとめて返す。
func TestUsecase_Warm_ReportsFailures(t *testing.T) {
	wantErr := errors.New("anilist down")
	anilistGW := &fakeAniList{err: wantErr}
	u := home.NewUsecase(anilistGW, &fakeJikan{})

	err := u.Warm(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want it to wrap %v", err, wantErr)
	}

	if anilistGW.seasonAnimeCalls != 1 || anilistGW.trendingAnimeCalls != 1 || anilistGW.trendingMangaCalls != 1 {
		t.Errorf("expected every target to be attempted despite failures, got %d/%d/%d calls",
			anilistGW.seasonAnimeCalls, anilistGW.trendingAnimeCalls, anilistGW.trendingMangaCalls)
	}
}
