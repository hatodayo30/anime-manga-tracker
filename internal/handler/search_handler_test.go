package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/handler"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/search"
)

// 未認証で叩ける AniList プロキシの入力上限が効いていること。上限を超えた入力は
// 上流（AniList）へ 1 本も投げずに 400 で弾く、が保証したい内容。
func TestByIDsRejectsTooManyIDs(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		wantCode int
		wantCall bool
	}{
		{name: "上限ちょうどは通す", count: 100, wantCode: http.StatusOK, wantCall: true},
		{name: "上限超過は弾く", count: 101, wantCode: http.StatusBadRequest, wantCall: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeAniList{}
			h := handler.NewSearchHandler(search.NewUsecase(gw, &fakeJikan{}))

			rec := call(t, h.ByIDs, "/api/anilist/media?type=anime&ids="+joinIDs(tt.count))

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if gw.mediaByIDsCalls != boolToInt(tt.wantCall) {
				t.Errorf("MediaByIDs calls = %d, want %d", gw.mediaByIDsCalls, boolToInt(tt.wantCall))
			}
		})
	}
}

// q の長さ上限。バイトではなく文字数で数えるので、日本語 100 文字は通る。
func TestSearchRejectsTooLongQuery(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantCode int
		wantCall bool
	}{
		{name: "日本語100文字は通す", query: strings.Repeat("あ", 100), wantCode: http.StatusOK, wantCall: true},
		{name: "101文字は弾く", query: strings.Repeat("a", 101), wantCode: http.StatusBadRequest, wantCall: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeAniList{}
			h := handler.NewSearchHandler(search.NewUsecase(gw, &fakeJikan{}))

			rec := call(t, h.Search, "/api/search?type=anime&q="+tt.query)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tt.wantCode, rec.Body.String())
			}
			if gw.searchCalls != boolToInt(tt.wantCall) {
				t.Errorf("Search calls = %d, want %d", gw.searchCalls, boolToInt(tt.wantCall))
			}
		})
	}
}

func call(t *testing.T, h echo.HandlerFunc, target string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, target, nil), rec)
	if err := h(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return rec
}

func joinIDs(n int) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = strconv.Itoa(i + 1)
	}
	return strings.Join(ids, ",")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// fakeAniList は呼び出し回数だけを記録する。上限で弾いたとき上流に届いていないことを見る。
type fakeAniList struct {
	searchCalls     int
	mediaByIDsCalls int
}

func (f *fakeAniList) Search(context.Context, string, domain.MediaType) ([]anilist.SearchResult, error) {
	f.searchCalls++
	// Jikan フォールバックまで走らせたくないので、しきい値より多い件数を返す。
	return make([]anilist.SearchResult, 10), nil
}

func (f *fakeAniList) MediaByIDs(context.Context, []int64, domain.MediaType) ([]anilist.SearchResult, error) {
	f.mediaByIDsCalls++
	return nil, nil
}

func (f *fakeAniList) Relations(context.Context, int64) ([]anilist.RelatedWork, error) {
	return nil, nil
}

func (f *fakeAniList) ByGenres(context.Context, []string, domain.MediaType) ([]anilist.SearchResult, error) {
	return nil, nil
}

type fakeJikan struct{}

func (f *fakeJikan) SearchAnime(context.Context, string) ([]anilist.SearchResult, error) {
	return nil, nil
}

func (f *fakeJikan) SearchManga(context.Context, string) ([]anilist.SearchResult, error) {
	return nil, nil
}
