package nlsearch_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/nlsearch"
)

type fakeAI struct {
	result nlsearch.Conditions
	err    error

	calls       int
	gotInput    string
	gotType     domain.MediaType
	gotTaxonomy anilist.Taxonomy
}

func (f *fakeAI) ParseSearch(ctx context.Context, input string, defaultType domain.MediaType, tax anilist.Taxonomy) (nlsearch.Conditions, error) {
	f.calls++
	f.gotInput = input
	f.gotType = defaultType
	f.gotTaxonomy = tax
	return f.result, f.err
}

type fakeTaxonomy struct {
	result anilist.Taxonomy
	err    error
}

func (f *fakeTaxonomy) Get(ctx context.Context) (anilist.Taxonomy, error) {
	return f.result, f.err
}

type fakeAniList struct {
	result []anilist.SearchResult
	err    error

	calls     int
	gotFilter anilist.Filter

	characters   []anilist.CharacterMatch
	characterErr error

	characterCalls     int
	gotCharacterNames  []string
	gotCharacterFilter anilist.Filter
}

func (f *fakeAniList) SearchByFilter(ctx context.Context, filter anilist.Filter) ([]anilist.SearchResult, error) {
	f.calls++
	f.gotFilter = filter
	return f.result, f.err
}

func (f *fakeAniList) SearchByCharacter(ctx context.Context, names []string, filter anilist.Filter) ([]anilist.CharacterMatch, error) {
	f.characterCalls++
	f.gotCharacterNames = names
	f.gotCharacterFilter = filter
	return f.characters, f.characterErr
}

type fakeKeyword struct {
	result []anilist.SearchResult
	err    error

	calls    int
	gotType  domain.MediaType
	gotQuery string
}

func (f *fakeKeyword) Search(ctx context.Context, mediaType domain.MediaType, query string) ([]anilist.SearchResult, error) {
	f.calls++
	f.gotType = mediaType
	f.gotQuery = query
	return f.result, f.err
}

var testTaxonomy = anilist.Taxonomy{
	Genres: []string{"Action", "Comedy", "Fantasy", "Horror", "Slice of Life"},
	Tags:   []anilist.MediaTag{{Name: "Isekai"}, {Name: "Cooking"}, {Name: "Gore"}, {Name: "Food"}, {Name: "Magic"}},
}

type fakes struct {
	ai      *fakeAI
	anilist *fakeAniList
	keyword *fakeKeyword
}

func newUsecase(cond nlsearch.Conditions) (*nlsearch.Usecase, fakes) {
	f := fakes{
		ai:      &fakeAI{result: cond},
		anilist: &fakeAniList{result: []anilist.SearchResult{{AniListID: 1, Title: "AI の結果"}}},
		keyword: &fakeKeyword{result: []anilist.SearchResult{{AniListID: 2, Title: "従来の結果"}}},
	}
	return nlsearch.NewUsecase(f.ai, &fakeTaxonomy{result: testTaxonomy}, f.anilist, f.keyword), f
}

// AI が作った条件で AniList の条件検索を呼び、その結果と条件を返すことを保証する。
func TestUsecase_Search_UsesAIConditions(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Genres: []string{"Fantasy"}, Tags: []string{"Isekai"}})

	got, err := u.Search(context.Background(), domain.MediaTypeManga, "  異世界で料理する漫画 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Fallback != nlsearch.FallbackNone || got.Conditions == nil {
		t.Fatalf("expected AI result, got %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].AniListID != 1 {
		t.Errorf("unexpected items: %+v", got.Items)
	}
	if f.ai.gotInput != "異世界で料理する漫画" || f.ai.gotType != domain.MediaTypeManga {
		t.Errorf("unexpected AI input: %q %q", f.ai.gotInput, f.ai.gotType)
	}
	if len(f.ai.gotTaxonomy.Tags) != len(testTaxonomy.Tags) {
		t.Errorf("expected taxonomy to be passed to AI")
	}
	want := anilist.Filter{MediaType: domain.MediaTypeManga, Genres: []string{"Fantasy"}, Tags: []string{"Isekai"}, ExcludedGenres: []string{}, ExcludedTags: []string{}}
	if !filterEqual(f.anilist.gotFilter, want) {
		t.Errorf("got filter %+v, want %+v", f.anilist.gotFilter, want)
	}
	if f.keyword.calls != 0 {
		t.Errorf("expected no keyword search, got %d", f.keyword.calls)
	}
}

// 文で種別が指定されていれば、画面で選んでいる種別ではなくそちらで検索し、結果にも返すことを保証する。
// フロントはこの値で作品モーダルとライブラリの照合を行う。
func TestUsecase_Search_MediaTypeFromAIOverridesSelected(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{MediaType: domain.MediaTypeManga, Tags: []string{"Cooking"}})

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "料理の漫画")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.anilist.gotFilter.MediaType != domain.MediaTypeManga || got.MediaType != domain.MediaTypeManga {
		t.Errorf("expected manga, got filter=%q result=%q", f.anilist.gotFilter.MediaType, got.MediaType)
	}
}

// 一覧に無い値・重複・絞り込みと除外の衝突を落とすことを保証する。
func TestUsecase_Search_SanitizesConditions(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{
		Genres:         []string{"NotAGenre", "Fantasy", "Fantasy"},
		Tags:           []string{"Unknown"},
		ExcludedGenres: []string{"Fantasy", "Horror"},
		ExcludedTags:   []string{"Gore", "Gore", "Unknown"},
		Query:          "  ",
	})

	if _, err := u.Search(context.Background(), domain.MediaTypeAnime, "x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := anilist.Filter{
		MediaType:      domain.MediaTypeAnime,
		Genres:         []string{"Fantasy"},
		Tags:           []string{},
		ExcludedGenres: []string{"Horror"},
		ExcludedTags:   []string{"Gore"},
	}
	if !filterEqual(f.anilist.gotFilter, want) {
		t.Errorf("got filter %+v, want %+v", f.anilist.gotFilter, want)
	}
}

// 絞り込みはジャンルとタグを合わせて 2 つまでにし、枠はタグから先に使うことを保証する。
// 2026-10-24 の実機確認で、「異世界で料理する漫画」に AI がジャンル 2・タグ 3 を選んで
// 1 件しか出なかった（genre_in / tag_in は AND で効いているとみられる。BACKLOG 9-b）。
func TestUsecase_Search_LimitsIncludesToTwoPreferringTags(t *testing.T) {
	cases := map[string]struct {
		in         nlsearch.Conditions
		wantGenres []string
		wantTags   []string
	}{
		"実機で出た条件": {
			in:         nlsearch.Conditions{Genres: []string{"Fantasy", "Comedy"}, Tags: []string{"Isekai", "Food", "Magic"}},
			wantGenres: []string{},
			wantTags:   []string{"Isekai", "Food"},
		},
		"タグ 1 つならジャンルで埋める": {
			in:         nlsearch.Conditions{Genres: []string{"Slice of Life", "Comedy"}, Tags: []string{"Food"}},
			wantGenres: []string{"Slice of Life"},
			wantTags:   []string{"Food"},
		},
		"ジャンルだけ": {
			in:         nlsearch.Conditions{Genres: []string{"Horror", "Action", "Comedy"}},
			wantGenres: []string{"Horror", "Action"},
			wantTags:   []string{},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			u, f := newUsecase(tc.in)

			got, err := u.Search(context.Background(), domain.MediaTypeManga, "x")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(f.anilist.gotFilter.Genres, tc.wantGenres) || !slices.Equal(f.anilist.gotFilter.Tags, tc.wantTags) {
				t.Errorf("got genres=%v tags=%v, want genres=%v tags=%v", f.anilist.gotFilter.Genres, f.anilist.gotFilter.Tags, tc.wantGenres, tc.wantTags)
			}
			// 画面のチップも実際に検索した条件と同じものを出す
			if !slices.Equal(got.Conditions.Tags, tc.wantTags) || !slices.Equal(got.Conditions.Genres, tc.wantGenres) {
				t.Errorf("returned conditions differ from searched filter: %+v", got.Conditions)
			}
		})
	}
}

// AI・タグ一覧・条件検索のどれが失敗しても、入力をそのままキーワードにして従来の検索に落とすことを保証する
// （CLAUDE.md「失敗の扱い」）。
func TestUsecase_Search_FallsBackOnFailure(t *testing.T) {
	cases := map[string]func(u **nlsearch.Usecase, f *fakes){
		"ai": func(u **nlsearch.Usecase, f *fakes) { f.ai.err = errors.New("budget exceeded") },
		"taxonomy": func(u **nlsearch.Usecase, f *fakes) {
			*u = nlsearch.NewUsecase(f.ai, &fakeTaxonomy{err: errors.New("anilist down")}, f.anilist, f.keyword)
		},
		"empty taxonomy": func(u **nlsearch.Usecase, f *fakes) {
			*u = nlsearch.NewUsecase(f.ai, &fakeTaxonomy{}, f.anilist, f.keyword)
		},
		"filter search": func(u **nlsearch.Usecase, f *fakes) { f.anilist.err = errors.New("429") },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			u, f := newUsecase(nlsearch.Conditions{Tags: []string{"Cooking"}})
			setup(&u, &f)

			got, err := u.Search(context.Background(), domain.MediaTypeManga, "料理 漫画")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Fallback != nlsearch.FallbackFailed || got.Conditions != nil {
				t.Errorf("expected failed fallback, got %+v", got)
			}
			if f.keyword.calls != 1 || f.keyword.gotQuery != "料理 漫画" || f.keyword.gotType != domain.MediaTypeManga {
				t.Errorf("unexpected keyword search: calls=%d q=%q type=%q", f.keyword.calls, f.keyword.gotQuery, f.keyword.gotType)
			}
			if len(got.Items) != 1 || got.Items[0].AniListID != 2 {
				t.Errorf("unexpected items: %+v", got.Items)
			}
		})
	}
}

// AI を組み込んでいない環境では AI を呼ばずに従来の検索を返し、理由を区別できることを保証する。
func TestUsecase_Search_WithoutAI(t *testing.T) {
	keyword := &fakeKeyword{}
	u := nlsearch.NewUsecase(nil, &fakeTaxonomy{result: testTaxonomy}, &fakeAniList{}, keyword)

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "泣ける話")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Fallback != nlsearch.FallbackUnavailable || keyword.calls != 1 {
		t.Errorf("expected unavailable fallback, got %+v (keyword calls %d)", got, keyword.calls)
	}
	if got.Items == nil {
		t.Error("items must not be nil (JSON null を返さない)")
	}
}

// AI が条件を 1 つも作れなかったときは、条件なしの人気順一覧を「検索結果」として出さず従来の検索に落とすことを保証する。
func TestUsecase_Search_NoConditionsFallsBack(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Genres: []string{"NotAGenre"}})

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "こんにちは")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Fallback != nlsearch.FallbackNoConditions || f.anilist.calls != 0 || f.keyword.calls != 1 {
		t.Errorf("expected no_conditions fallback, got %+v (anilist %d, keyword %d)", got, f.anilist.calls, f.keyword.calls)
	}
}

// 空の文では AI を呼ばない（課金しない）ことを保証する。
func TestUsecase_Search_BlankInputDoesNotCallAI(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Tags: []string{"Cooking"}})

	if _, err := u.Search(context.Background(), domain.MediaTypeAnime, "  "); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.ai.calls != 0 {
		t.Errorf("expected AI not to be called, got %d", f.ai.calls)
	}
}

// 同じ文の検索し直しでは AI を呼び直さず、AniList の検索は毎回行うことを保証する。
// 結果をキャッシュしないのは、人気順や放送状況が AniList 側で変わるため。
func TestUsecase_Search_CachesConditions(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Tags: []string{"Cooking"}})

	for range 3 {
		if _, err := u.Search(context.Background(), domain.MediaTypeAnime, "料理アニメ"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if f.ai.calls != 1 {
		t.Errorf("expected AI to be called once, got %d", f.ai.calls)
	}
	if f.anilist.calls != 3 {
		t.Errorf("expected AniList to be called 3 times, got %d", f.anilist.calls)
	}
}

// AI の失敗はキャッシュせず、次の検索で呼び直すことを保証する。
func TestUsecase_Search_DoesNotCacheAIErrors(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Tags: []string{"Cooking"}})
	f.ai.err = errors.New("timeout")

	if _, err := u.Search(context.Background(), domain.MediaTypeAnime, "料理アニメ"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f.ai.err = nil
	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "料理アニメ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.ai.calls != 2 || got.Fallback != nlsearch.FallbackNone {
		t.Errorf("expected AI retry to succeed, calls=%d result=%+v", f.ai.calls, got)
	}
}

// 落とした先の従来の検索まで失敗したときだけエラーを返すことを保証する。
func TestUsecase_Search_ReturnsErrorWhenKeywordSearchFails(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{})
	f.ai.err = errors.New("ai down")
	f.keyword.err = errors.New("anilist down")

	if _, err := u.Search(context.Background(), domain.MediaTypeAnime, "x"); err == nil {
		t.Fatal("expected error")
	}
}

func filterEqual(a, b anilist.Filter) bool {
	return a.MediaType == b.MediaType && a.Query == b.Query &&
		slices.Equal(a.Genres, b.Genres) && slices.Equal(a.Tags, b.Tags) &&
		slices.Equal(a.ExcludedGenres, b.ExcludedGenres) && slices.Equal(a.ExcludedTags, b.ExcludedTags)
}

// character は AniList のキャラクター検索の結果 1 人分を作る。
func character(native, full string, favourites int, works ...anilist.SearchResult) anilist.CharacterMatch {
	return anilist.CharacterMatch{
		Name:       anilist.CharacterName{Native: native, Full: full},
		Favourites: favourites,
		Works:      works,
	}
}

func work(id int64, popularity int) anilist.SearchResult {
	return anilist.SearchResult{AniListID: id, Popularity: popularity}
}

func ids(items []anilist.SearchResult) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.AniListID
	}
	return out
}

// キャラクター名があれば名前の候補でキャラクター検索を呼び、ジャンル・タグの条件も一緒に渡すことを保証する。
// 条件を渡さないと「薫子が出る恋愛もの」の恋愛が効かない。作品の条件検索は呼ばない（AniList の枠を 1 本で済ませる）。
func TestUsecase_Search_CharacterUsesCharacterSearch(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "薫子", CharacterNames: []string{"Kaoruko"}, Genres: []string{"Comedy"}})
	f.anilist.characters = []anilist.CharacterMatch{character("和栗薫子", "Kaoruko Waguri", 5955, work(10, 100))}

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "薫子っていうキャラが出るコメディ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Fallback != nlsearch.FallbackNone || got.Conditions == nil || got.Conditions.Character != "薫子" {
		t.Fatalf("expected character result, got %+v", got)
	}
	if !slices.Equal(ids(got.Items), []int64{10}) {
		t.Errorf("unexpected items: %v", ids(got.Items))
	}
	gf := f.anilist.gotCharacterFilter
	if !slices.Equal(f.anilist.gotCharacterNames, []string{"Kaoruko"}) || gf.Query != "" || gf.MediaType != domain.MediaTypeAnime || !slices.Equal(gf.Genres, []string{"Comedy"}) {
		t.Errorf("unexpected character filter: %+v", gf)
	}
	if f.anilist.calls != 0 {
		t.Errorf("expected no filter search, got %d", f.anilist.calls)
	}
}

// 名前が合わないキャラクターを捨て、書かれた漢字が日本語名に含まれるキャラクターを優先することを保証する。
// AniList は名前と関係の無い人気キャラクターも返し、ローマ字では読みが同じ別の漢字（香子）も当たる。
func TestUsecase_Search_CharacterPicksByName(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "薫子", CharacterNames: []string{"Kaoruko"}})
	f.anilist.characters = []anilist.CharacterMatch{
		character("和栗薫子", "Kaoruko Waguri", 5955, work(10, 100)),
		character("五条悟", "Satoru Gojou", 39398, work(20, 900)),   // 名前と関係の無い人気キャラクター
		character("花柳香子", "Kaoruko Hanayagi", 251, work(30, 50)), // 読みは同じだが漢字が違う
		character("萌田薫子", "Kaoruko Moeta", 540, work(40, 300), work(10, 100)),
	}

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "薫子が出る")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 薫子の 2 人の作品だけを、重複を除いて人気順に
	if want := []int64{40, 10}; !slices.Equal(ids(got.Items), want) {
		t.Errorf("got %v, want %v", ids(got.Items), want)
	}
}

// 書かれた名前が日本語名に無ければ（ひらがなで書かれたなど）、ローマ字で合うキャラクターを採ることを保証する。
func TestUsecase_Search_CharacterFallsBackToRomajiMatch(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "かおるこ", CharacterNames: []string{"Kaoruko"}})
	f.anilist.characters = []anilist.CharacterMatch{
		character("五条悟", "Satoru Gojou", 39398, work(20, 900)),
		character("花柳香子", "Kaoruko Hanayagi", 251, work(30, 50)),
	}

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "かおるこが出る")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int64{30}; !slices.Equal(ids(got.Items), want) {
		t.Errorf("got %v, want %v", ids(got.Items), want)
	}
}

// 名前が合うキャラクターがいなければ、名前を作品名のキーワードにして条件検索をやり直し、
// 返す条件からキャラクター名を消すことを保証する。AI が作品名をキャラクター名と取り違えたときの救済で、
// 画面に「〇〇が出る作品」と出したまま作品名の一致の結果を並べないため。
func TestUsecase_Search_CharacterNotFoundRetriesAsTitle(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "ワンピース", CharacterNames: []string{"Wan Piisu"}})
	f.anilist.characters = []anilist.CharacterMatch{character("五条悟", "Satoru Gojou", 39398, work(20, 900))}

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "ワンピースっていうキャラ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.anilist.gotFilter.Query != "ワンピース" {
		t.Errorf("expected retry with title keyword, got filter %+v", f.anilist.gotFilter)
	}
	if got.Conditions == nil || got.Conditions.Character != "" || got.Conditions.Query != "ワンピース" {
		t.Errorf("unexpected conditions: %+v", got.Conditions)
	}
	if !slices.Equal(ids(got.Items), []int64{1}) {
		t.Errorf("unexpected items: %v", ids(got.Items))
	}
}

// キャラクター検索の失敗は、他の失敗と同じく従来の検索に落とすことを保証する（失敗の扱いの方針）。
func TestUsecase_Search_CharacterSearchFailureFallsBack(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "薫子", CharacterNames: []string{"Kaoruko"}})
	f.anilist.characterErr = errors.New("anilist down")

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "薫子が出る")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Fallback != nlsearch.FallbackFailed || f.keyword.calls != 1 {
		t.Errorf("expected keyword fallback, got %+v (keyword calls %d)", got, f.keyword.calls)
	}
}

// 名前の候補の無いキャラクター名は、キャラクター検索に使えないので作品名のキーワードに回すことを保証する。
func TestUsecase_Search_CharacterWithoutRomajiBecomesQuery(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "薫子"})

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "薫子")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.anilist.characterCalls != 0 || f.anilist.gotFilter.Query != "薫子" {
		t.Errorf("expected filter search with query, got character calls %d, filter %+v", f.anilist.characterCalls, f.anilist.gotFilter)
	}
	if got.Conditions == nil || got.Conditions.Character != "" {
		t.Errorf("unexpected conditions: %+v", got.Conditions)
	}
}

// 名前の候補のどれか 1 つと合えば採ることを保証する。1 つ目の読みが外れても、フルネームや英語名で当たれば見つかる。
func TestUsecase_Search_CharacterMatchesAnyName(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "るふぃ", CharacterNames: []string{"Rufi", "Monkey D. Luffy"}})
	f.anilist.characters = []anilist.CharacterMatch{
		character("モンキー・D・ルフィ", "Luffy Monkey", 35999, work(50, 999)),
		character("五条悟", "Satoru Gojou", 39398, work(20, 900)),
	}

	got, err := u.Search(context.Background(), domain.MediaTypeAnime, "るふぃが出る")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []int64{50}; !slices.Equal(ids(got.Items), want) {
		t.Errorf("got %v, want %v", ids(got.Items), want)
	}
}

// 文に作品名もあれば、キャラクター検索にその作品名を渡して出演作品を絞ることを保証する（「呪術廻戦の五条」）。
func TestUsecase_Search_CharacterWithTitlePassesQuery(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "五条", CharacterNames: []string{"Gojou"}, Query: "呪術廻戦"})
	f.anilist.characters = []anilist.CharacterMatch{character("五条悟", "Satoru Gojou", 39398, work(20, 900))}

	if _, err := u.Search(context.Background(), domain.MediaTypeAnime, "呪術廻戦の五条"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.anilist.gotCharacterFilter.Query != "呪術廻戦" {
		t.Errorf("expected title query to be passed, got %+v", f.anilist.gotCharacterFilter)
	}
}

// 名前の候補は空白を落とし、重複を除いて 3 つまでにすることを保証する。AniList に 1 本で送れるのは 3 つまでと確かめてある。
func TestUsecase_Search_LimitsCharacterNames(t *testing.T) {
	u, f := newUsecase(nlsearch.Conditions{Character: "薫子", CharacterNames: []string{" Kaoruko ", "Kaoruko", "", "Waguri Kaoruko", "Kaoru", "Kao"}})
	f.anilist.characters = []anilist.CharacterMatch{character("和栗薫子", "Kaoruko Waguri", 5955, work(10, 100))}

	if _, err := u.Search(context.Background(), domain.MediaTypeAnime, "薫子"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"Kaoruko", "Waguri Kaoruko", "Kaoru"}; !slices.Equal(f.anilist.gotCharacterNames, want) {
		t.Errorf("got %v, want %v", f.anilist.gotCharacterNames, want)
	}
}
