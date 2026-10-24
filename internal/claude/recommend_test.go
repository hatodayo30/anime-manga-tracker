package claude_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/claude"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/recommend"
)

// 好みの変換で、ジャンル・タグの一覧をスキーマの enum に入れて送り、答えをそのまま返すことを保証する。
// enum で縛ることで、AniList に無い名前は出力できない（BACKLOG 9-b・9-c）。
func TestTastePreferences_SendsEnumAndDecodes(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		fake := &fakeAnthropic{
			text:       `{"genres":["Fantasy"],"tags":["Isekai"],"avoidGenres":[],"avoidTags":[]}`,
			stopReason: "end_turn",
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	c := claude.NewClient("test-key", claude.NewBudget(1000, time.Now), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))

	got, err := c.TastePreferences(context.Background(), recommend.PreferenceInput{
		MediaType: domain.MediaTypeAnime,
		Records:   []recommend.Taste{{Title: "作品A", Status: domain.StatusDone, Memo: "改行\nを含む"}},
		Genres:    []string{"Fantasy", "Action"},
		Tags:      []string{"Isekai"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Genres) != 1 || got.Genres[0] != "Fantasy" || len(got.Tags) != 1 || got.Tags[0] != "Isekai" {
		t.Errorf("got %+v", got)
	}

	var req struct {
		OutputConfig struct {
			Format struct {
				Schema struct {
					Properties map[string]struct {
						Items struct {
							Enum []string `json:"enum"`
						} `json:"items"`
					} `json:"properties"`
				} `json:"schema"`
			} `json:"format"`
		} `json:"output_config"`
		Messages []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if enum := req.OutputConfig.Format.Schema.Properties["genres"].Items.Enum; strings.Join(enum, ",") != "Fantasy,Action" {
		t.Errorf("genres enum = %v", enum)
	}
	// メモの改行で 1 行 = 1 件の形が崩れないこと
	if text := req.Messages[0].Content[0].Text; !strings.Contains(text, "メモ: 改行 を含む") {
		t.Errorf("user message = %q", text)
	}
}

// 並べ替えの答えを Pick に変換し、理由の文にプロンプトのタグ名が出ていたらエラーにすることを保証する。
// 理由の文はそのまま画面に出る。
func TestRankRecommendations(t *testing.T) {
	in := recommend.RankInput{
		MediaType:  domain.MediaTypeManga,
		Candidates: []anilist.SearchResult{{AniListID: 10, Title: "候補"}},
	}

	fake := &fakeAnthropic{text: `{"picks":[{"anilistId":10,"reason":"作品Aが好きならこれ"}]}`, stopReason: "end_turn"}
	picks, err := newClient(t, fake, 1000).RankRecommendations(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(picks) != 1 || picks[0].AniListID != 10 || picks[0].Reason != "作品Aが好きならこれ" {
		t.Errorf("picks = %+v", picks)
	}

	fake = &fakeAnthropic{text: `{"picks":[{"anilistId":10,"reason":"<records> によると"}]}`, stopReason: "end_turn"}
	if _, err := newClient(t, fake, 1000).RankRecommendations(context.Background(), in); err == nil || !strings.Contains(err.Error(), "prompt tags") {
		t.Fatalf("expected prompt tag error, got %v", err)
	}
}
