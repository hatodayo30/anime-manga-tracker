package claude_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/claude"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// recordingAnthropic は送られてきたリクエストの本文を記録して、決めた本文を返すサーバー。
// スキーマの enum を検証するため fakeAnthropic と分けた。
type recordingAnthropic struct {
	text string

	body map[string]any
}

func (f *recordingAnthropic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = json.NewDecoder(r.Body).Decode(&f.body)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-haiku-5-5",
		"content":     []map[string]any{{"type": "text", "text": f.text}},
		"stop_reason": "end_turn", "stop_sequence": nil,
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 10},
	})
}

var nlTaxonomy = anilist.Taxonomy{
	Genres: []string{"Fantasy", "Comedy"},
	Tags:   []anilist.MediaTag{{Name: "Isekai"}, {Name: "Cooking"}},
}

func newRecordingClient(t *testing.T, fake *recordingAnthropic) *claude.Client {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return claude.NewClient("test-key", claude.NewBudget(100000, time.Now), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
}

// ジャンル・タグを一覧の値の enum で縛ったスキーマを送ることを保証する（BACKLOG 9-b）。
func TestParseSearch_ConstrainsGenresAndTagsWithEnum(t *testing.T) {
	fake := &recordingAnthropic{text: `{"mediaType":"unspecified","genres":[],"tags":["Cooking"],"excludedGenres":[],"excludedTags":[],"query":""}`}
	c := newRecordingClient(t, fake)

	if _, err := c.ParseSearch(context.Background(), "料理", domain.MediaTypeAnime, nlTaxonomy); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	schema := dig(t, fake.body, "output_config", "format", "schema", "properties")
	for field, want := range map[string][]string{
		"genres": {"Fantasy", "Comedy"}, "excludedGenres": {"Fantasy", "Comedy"},
		"tags": {"Isekai", "Cooking"}, "excludedTags": {"Isekai", "Cooking"},
	} {
		got := toStrings(dig(t, schema, field, "items", "enum"))
		if !slices.Equal(got, want) {
			t.Errorf("%s enum = %v, want %v", field, got, want)
		}
	}
}

// 構造化出力の欄を条件に移し、"unspecified" の種別は空（画面の種別を使う）にすることを保証する。
func TestParseSearch_MapsOutput(t *testing.T) {
	cases := map[string]domain.MediaType{"unspecified": "", "manga": domain.MediaTypeManga}
	for mt, want := range cases {
		t.Run(mt, func(t *testing.T) {
			fake := &recordingAnthropic{text: `{"mediaType":"` + mt + `","genres":["Fantasy"],"tags":["Isekai"],"excludedGenres":["Comedy"],"excludedTags":[],"query":"ダンジョン飯"}`}
			c := newRecordingClient(t, fake)

			got, err := c.ParseSearch(context.Background(), "x", domain.MediaTypeAnime, nlTaxonomy)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.MediaType != want || got.Query != "ダンジョン飯" ||
				!slices.Equal(got.Genres, []string{"Fantasy"}) || !slices.Equal(got.Tags, []string{"Isekai"}) ||
				!slices.Equal(got.ExcludedGenres, []string{"Comedy"}) {
				t.Errorf("unexpected conditions: %+v", got)
			}
		})
	}
}

// キーワード欄にプロンプトのタグ名が出ていたら、条件として使わずエラーにすることを保証する。
func TestParseSearch_RejectsPromptTags(t *testing.T) {
	fake := &recordingAnthropic{text: `{"mediaType":"unspecified","genres":[],"tags":[],"excludedGenres":[],"excludedTags":[],"query":"<request> の内容"}`}
	c := newRecordingClient(t, fake)

	if _, err := c.ParseSearch(context.Background(), "x", domain.MediaTypeAnime, nlTaxonomy); err == nil {
		t.Fatal("expected error")
	}
}

func dig(t *testing.T, v any, keys ...string) any {
	t.Helper()
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("expected object at %q, got %T", k, v)
		}
		v = m[k]
	}
	return v
}

func toStrings(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		s, _ := e.(string)
		out = append(out, s)
	}
	return out
}

// キャラクター名とそのローマ字を、作品名のキーワード（query）とは別の欄として受け取ることを保証する。
// query は作品検索に渡るので、キャラクター名が入ると 0 件になる。
func TestParseSearch_MapsCharacter(t *testing.T) {
	fake := &recordingAnthropic{text: `{"mediaType":"unspecified","genres":[],"tags":[],"excludedGenres":[],"excludedTags":[],"query":"","character":"薫子","characterRomaji":"Kaoruko"}`}
	c := newRecordingClient(t, fake)

	got, err := c.ParseSearch(context.Background(), "薫子っていうキャラが出る", domain.MediaTypeAnime, nlTaxonomy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Character != "薫子" || got.CharacterRomaji != "Kaoruko" || got.Query != "" {
		t.Errorf("unexpected conditions: %+v", got)
	}

	required := toStrings(dig(t, fake.body, "output_config", "format", "schema", "required"))
	for _, field := range []string{"character", "characterRomaji"} {
		if !slices.Contains(required, field) {
			t.Errorf("expected %q to be required, got %v", field, required)
		}
	}
}
