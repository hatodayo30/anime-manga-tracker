package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/handler"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/newreleases"
)

// 楽天のキーが無い環境でも新刊の API は 200 で {"configured": false, "items": []} を返すことを保証する。
// 画面はこれを見て「未設定」を出す（BACKLOG 11-b）。
func TestNewReleasesComics_NotConfigured(t *testing.T) {
	passThrough := func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	e := handler.NewRouter(handler.Handlers{
		NewReleases: handler.NewNewReleasesHandler(newreleases.NewUsecase(nil, time.Now)),
		RequireUser: passThrough,
	})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/new-releases/comics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items, ok := body["items"].([]any)
	if body["configured"] != false || !ok || len(items) != 0 {
		t.Errorf("body = %s, want configured=false and items=[]", rec.Body.String())
	}
}
