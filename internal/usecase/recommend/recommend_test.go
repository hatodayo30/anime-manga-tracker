package recommend_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/recommend"
)

type fakeRecords struct {
	records []*domain.Record
	userIDs []int64
}

func (f *fakeRecords) List(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status, sort domain.SortKey) ([]*domain.Record, error) {
	f.userIDs = append(f.userIDs, userID)
	return f.records, nil
}

type fakeTaxonomy struct{}

func (fakeTaxonomy) Get(ctx context.Context) (anilist.Taxonomy, error) {
	return anilist.Taxonomy{
		Genres: []string{"Action", "Fantasy", "Romance", "Horror"},
		Tags: []anilist.MediaTag{
			{Name: "Isekai"}, {Name: "Cooking"}, {Name: "Gore"},
			{Name: "Time Loop", IsGeneralSpoiler: true},
		},
	}, nil
}

type fakeAniList struct {
	filters      []anilist.Filter
	filterFunc   func(f anilist.Filter) ([]anilist.SearchResult, error)
	byGenresArgs [][]string
}

func (f *fakeAniList) SearchByFilter(ctx context.Context, filter anilist.Filter) ([]anilist.SearchResult, error) {
	f.filters = append(f.filters, filter)
	return f.filterFunc(filter)
}

func (f *fakeAniList) ByGenres(ctx context.Context, genres []string, mediaType domain.MediaType) ([]anilist.SearchResult, error) {
	f.byGenresArgs = append(f.byGenresArgs, genres)
	return []anilist.SearchResult{{AniListID: 900, Title: "ジャンルの作品"}, {AniListID: 1, Title: "記録済み"}}, nil
}

type fakeAI struct {
	prefs     recommend.Preferences
	prefsErr  error
	picksFunc func(in recommend.RankInput) []recommend.Pick
	rankErr   error

	prefInputs []recommend.PreferenceInput
	rankInputs []recommend.RankInput
}

func (f *fakeAI) TastePreferences(ctx context.Context, in recommend.PreferenceInput) (recommend.Preferences, error) {
	f.prefInputs = append(f.prefInputs, in)
	return f.prefs, f.prefsErr
}

func (f *fakeAI) RankRecommendations(ctx context.Context, in recommend.RankInput) ([]recommend.Pick, error) {
	f.rankInputs = append(f.rankInputs, in)
	if f.rankErr != nil {
		return nil, f.rankErr
	}
	return f.picksFunc(in), nil
}

func (f *fakeAI) calls() int { return len(f.prefInputs) }

// clock はテストから進められる時刻。
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func rating(n int) *int { return &n }

// tasteRecords は AI を使うのに足りる数の「見た」記録。AniList ID は 1〜3。
func tasteRecords() []*domain.Record {
	return []*domain.Record{
		{AniListID: 1, Title: "作品A", Status: domain.StatusDone, Rating: rating(5), Genres: []string{"Fantasy"}, Memo: "最高"},
		{AniListID: 2, Title: "作品B", Status: domain.StatusDone, Rating: rating(4), Genres: []string{"Fantasy", "Action"}},
		{AniListID: 3, Title: "作品C", Status: domain.StatusActive, Genres: []string{"Action"}},
	}
}

// candidatesBy はタグ・ジャンルごとに決まった候補を返す filterFunc。
func candidatesBy(f anilist.Filter) ([]anilist.SearchResult, error) {
	if len(f.Tags) > 0 {
		return []anilist.SearchResult{{AniListID: 10, Title: "タグの作品"}, {AniListID: 1, Title: "記録済み"}}, nil
	}
	return []anilist.SearchResult{{AniListID: 20, Title: "ジャンルの作品1"}, {AniListID: 10, Title: "重複"}}, nil
}

func pickAll(in recommend.RankInput) []recommend.Pick {
	picks := make([]recommend.Pick, 0, len(in.Candidates))
	for i := len(in.Candidates) - 1; i >= 0; i-- { // 候補と逆順に選ぶ
		picks = append(picks, recommend.Pick{AniListID: in.Candidates[i].AniListID, Reason: "作品Aが好きならこれ"})
	}
	return picks
}

func newUsecase(records *fakeRecords, al *fakeAniList, ai recommend.AIGateway, c *clock) *recommend.Usecase {
	return recommend.NewUsecase(records, fakeTaxonomy{}, al, ai, c.now)
}

// 3 段を通したとき、AI が選んだ順に候補を並べ、理由を付けて返すことを保証する。
// 候補から記録済み・重複が除かれていること、AI に作品を挙げさせる口が無く候補の中からだけ選ばれることも確かめる。
func TestFor_AIPipeline(t *testing.T) {
	records := &fakeRecords{records: tasteRecords()}
	al := &fakeAniList{filterFunc: candidatesBy}
	ai := &fakeAI{
		prefs:     recommend.Preferences{Genres: []string{"Fantasy"}, Tags: []string{"Isekai"}},
		picksFunc: pickAll,
	}
	u := newUsecase(records, al, ai, &clock{t: time.Now()})

	got, err := u.For(context.Background(), 7, domain.MediaTypeAnime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != recommend.SourceAI {
		t.Fatalf("source = %q, want ai", got.Source)
	}
	if len(got.Items) != 2 || got.Items[0].AniListID != 20 || got.Items[1].AniListID != 10 {
		t.Fatalf("items = %+v, want [20, 10] in the AI's order", got.Items)
	}
	if got.Items[0].Reason != "作品Aが好きならこれ" {
		t.Errorf("reason = %q", got.Items[0].Reason)
	}
	if records.userIDs[0] != 7 {
		t.Errorf("records listed for user %d, want 7", records.userIDs[0])
	}
	if len(ai.rankInputs[0].Candidates) != 2 {
		t.Errorf("candidates = %+v, want recorded and duplicate removed", ai.rankInputs[0].Candidates)
	}
}

// 検索 1 本に渡すジャンル・タグが 1 つずつで、本数が 2 本までであることを保証する。
// genre_in / tag_in に複数渡したときの意味（AND か OR か）を確かめていないので、どちらでも同じ結果になる形に限っている。
func TestFor_FiltersUseSingleGenreOrTag(t *testing.T) {
	al := &fakeAniList{filterFunc: candidatesBy}
	ai := &fakeAI{
		prefs: recommend.Preferences{
			Genres:      []string{"Fantasy", "Action"},
			Tags:        []string{"Isekai", "Cooking"},
			AvoidGenres: []string{"Horror"},
			AvoidTags:   []string{"Gore"},
		},
		picksFunc: pickAll,
	}
	u := newUsecase(&fakeRecords{records: tasteRecords()}, al, ai, &clock{t: time.Now()})

	if _, err := u.For(context.Background(), 1, domain.MediaTypeAnime); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(al.filters) != 2 {
		t.Fatalf("got %d AniList queries, want 2", len(al.filters))
	}
	for _, f := range al.filters {
		if len(f.Genres)+len(f.Tags) != 1 {
			t.Errorf("filter %+v should have exactly one genre or tag", f)
		}
		if len(f.ExcludedGenres) != 1 || f.ExcludedGenres[0] != "Horror" || len(f.ExcludedTags) != 1 || f.ExcludedTags[0] != "Gore" {
			t.Errorf("filter %+v should carry the avoided genres and tags", f)
		}
	}
}

// AI が一覧に無い名前や、好きと避けたいの両方に入れた名前を返しても、AniList には渡さないことを保証する。
// AniList は知らない名前でもエラーにせず 0 件を返すので、渡すと気付かないまま候補が空になる。
func TestFor_SanitizesPreferences(t *testing.T) {
	al := &fakeAniList{filterFunc: candidatesBy}
	ai := &fakeAI{
		prefs: recommend.Preferences{
			Genres:      []string{"NotAGenre", "Action"},
			Tags:        []string{"Fantasy"}, // ジャンル名をタグとして返した
			AvoidGenres: []string{"Action"},
		},
		picksFunc: pickAll,
	}
	u := newUsecase(&fakeRecords{records: tasteRecords()}, al, ai, &clock{t: time.Now()})

	got, err := u.For(context.Background(), 1, domain.MediaTypeAnime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 好みの条件が全部消えるので AniList は呼ばれず、候補なしとしてジャンルベースに落ちる
	if len(al.filters) != 0 {
		t.Errorf("filters = %+v, want none", al.filters)
	}
	if got.Source != recommend.SourceGenre {
		t.Errorf("source = %q, want genre", got.Source)
	}
}

// AI にはネタバレのタグを選ばせないことを保証する。理由の文に書かれるとネタバレになる。
func TestFor_ExcludesSpoilerTags(t *testing.T) {
	ai := &fakeAI{prefs: recommend.Preferences{Tags: []string{"Isekai"}}, picksFunc: pickAll}
	u := newUsecase(&fakeRecords{records: tasteRecords()}, &fakeAniList{filterFunc: candidatesBy}, ai, &clock{t: time.Now()})

	if _, err := u.For(context.Background(), 1, domain.MediaTypeAnime); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, tag := range ai.prefInputs[0].Tags {
		if tag == "Time Loop" {
			t.Fatal("spoiler tag should not be offered to the AI")
		}
	}
}

// 1 人 1 日 1 回だけ作り、同じ日はキャッシュから返し、JST の日付が変わったら作り直すことを保証する。
// 利用者ごと・種別ごとに別のキーであることも確かめる。
func TestFor_GeneratesOncePerUserPerDay(t *testing.T) {
	ai := &fakeAI{prefs: recommend.Preferences{Tags: []string{"Isekai"}}, picksFunc: pickAll}
	jst := time.FixedZone("JST", 9*60*60)
	c := &clock{t: time.Date(2026, 10, 24, 10, 0, 0, 0, jst)}
	u := newUsecase(&fakeRecords{records: tasteRecords()}, &fakeAniList{filterFunc: candidatesBy}, ai, c)
	ctx := context.Background()

	for range 3 {
		if _, err := u.For(ctx, 1, domain.MediaTypeAnime); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ai.calls() != 1 {
		t.Fatalf("AI called %d times on the same day, want 1", ai.calls())
	}

	_, _ = u.For(ctx, 2, domain.MediaTypeAnime)
	_, _ = u.For(ctx, 1, domain.MediaTypeManga)
	if ai.calls() != 3 {
		t.Fatalf("AI called %d times, want another user and another media type to generate separately", ai.calls())
	}

	c.t = time.Date(2026, 10, 25, 0, 1, 0, 0, jst)
	_, _ = u.For(ctx, 1, domain.MediaTypeAnime)
	if ai.calls() != 4 {
		t.Errorf("AI called %d times, want regeneration after the JST date changes", ai.calls())
	}
}

// 作った後に記録した作品は、キャッシュから返すときにも棚から外すことを保証する。
func TestFor_HidesWorksRecordedAfterGeneration(t *testing.T) {
	records := &fakeRecords{records: tasteRecords()}
	ai := &fakeAI{prefs: recommend.Preferences{Tags: []string{"Isekai"}}, picksFunc: pickAll}
	u := newUsecase(records, &fakeAniList{filterFunc: candidatesBy}, ai, &clock{t: time.Now()})
	ctx := context.Background()

	if _, err := u.For(ctx, 1, domain.MediaTypeAnime); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	records.records = append(records.records, &domain.Record{AniListID: 20, Status: domain.StatusWant})

	got, _ := u.For(ctx, 1, domain.MediaTypeAnime)
	for _, it := range got.Items {
		if it.AniListID == 20 {
			t.Fatal("work recorded after generation should be hidden")
		}
	}
}

// 記録が少ない人には AI を呼ばず、ジャンルベースのおすすめ（記録済みを除く）を返すことを保証する。
func TestFor_FewRecordsFallsBackToGenres(t *testing.T) {
	al := &fakeAniList{filterFunc: candidatesBy}
	ai := &fakeAI{}
	records := &fakeRecords{records: tasteRecords()[:2]}
	u := newUsecase(records, al, ai, &clock{t: time.Now()})

	got, err := u.For(context.Background(), 1, domain.MediaTypeAnime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ai.calls() != 0 {
		t.Error("AI should not be called with too few records")
	}
	if got.Source != recommend.SourceGenre || len(got.Items) != 1 || got.Items[0].AniListID != 900 {
		t.Errorf("got %+v, want genre-based items without recorded works", got)
	}
	if strings.Join(al.byGenresArgs[0], ",") != "Fantasy,Action" {
		t.Errorf("genres = %v, want the most frequent genres first", al.byGenresArgs[0])
	}
}

// AI が無い環境（API キーなし）ではジャンルベースで動くことを保証する。
func TestFor_NoAIFallsBackToGenres(t *testing.T) {
	u := newUsecase(&fakeRecords{records: tasteRecords()}, &fakeAniList{filterFunc: candidatesBy}, nil, &clock{t: time.Now()})

	got, err := u.For(context.Background(), 1, domain.MediaTypeAnime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != recommend.SourceGenre {
		t.Errorf("source = %q, want genre", got.Source)
	}
}

// AI が失敗したらジャンルベースに落とし、しばらくは AI を呼び直さないことを保証する。
// 失敗した応答にも課金されるので、ホームを開くたびに呼び直すと請求が積み上がる。
func TestFor_AIFailureFallsBackAndCoolsDown(t *testing.T) {
	ai := &fakeAI{prefs: recommend.Preferences{Tags: []string{"Isekai"}}, rankErr: errors.New("refused")}
	c := &clock{t: time.Now()}
	u := newUsecase(&fakeRecords{records: tasteRecords()}, &fakeAniList{filterFunc: candidatesBy}, ai, c)
	ctx := context.Background()

	got, err := u.For(ctx, 1, domain.MediaTypeAnime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != recommend.SourceGenre {
		t.Fatalf("source = %q, want genre", got.Source)
	}

	_, _ = u.For(ctx, 1, domain.MediaTypeAnime)
	if ai.calls() != 1 {
		t.Fatalf("AI called %d times during the cooldown, want 1", ai.calls())
	}

	c.t = c.t.Add(31 * time.Minute)
	_, _ = u.For(ctx, 1, domain.MediaTypeAnime)
	if ai.calls() != 2 {
		t.Errorf("AI called %d times, want a retry after the cooldown", ai.calls())
	}
}

// AniList の検索が 1 本失敗しても、もう 1 本の結果でおすすめを作ることを保証する（「失敗の扱い」）。
func TestFor_OneFilterQueryFailing(t *testing.T) {
	al := &fakeAniList{filterFunc: func(f anilist.Filter) ([]anilist.SearchResult, error) {
		if len(f.Tags) > 0 {
			return nil, errors.New("429")
		}
		return candidatesBy(f)
	}}
	ai := &fakeAI{prefs: recommend.Preferences{Genres: []string{"Fantasy"}, Tags: []string{"Isekai"}}, picksFunc: pickAll}
	u := newUsecase(&fakeRecords{records: tasteRecords()}, al, ai, &clock{t: time.Now()})

	got, err := u.For(context.Background(), 1, domain.MediaTypeAnime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Source != recommend.SourceAI || len(got.Items) != 2 {
		t.Errorf("got %+v, want AI items from the genre query", got)
	}
}

// AI が候補に無い ID・重複・理由の無いものを返しても棚に載せず、長すぎる理由は切ることを保証する。
func TestFor_DropsInvalidPicks(t *testing.T) {
	ai := &fakeAI{
		prefs: recommend.Preferences{Tags: []string{"Isekai"}},
		picksFunc: func(in recommend.RankInput) []recommend.Pick {
			return []recommend.Pick{
				{AniListID: 999, Reason: "候補に無い"},
				{AniListID: 10, Reason: strings.Repeat("長", 200)},
				{AniListID: 10, Reason: "重複"},
				{AniListID: 1, Reason: "記録済み（候補に無い）"},
			}
		},
	}
	u := newUsecase(&fakeRecords{records: tasteRecords()}, &fakeAniList{filterFunc: candidatesBy}, ai, &clock{t: time.Now()})

	got, err := u.For(context.Background(), 1, domain.MediaTypeAnime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].AniListID != 10 {
		t.Fatalf("items = %+v, want only id 10", got.Items)
	}
	if n := len([]rune(got.Items[0].Reason)); n > 81 {
		t.Errorf("reason has %d runes, want it truncated", n)
	}
}
