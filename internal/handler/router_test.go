package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/handler"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

// signup と login は 1 IP あたりの枠（loginRateLimit）を共有する（BACKLOG 4-c）。
// signup は既存メールに 409 を返すのでメールアドレスの存在確認に使える。枠が別々だと
// 交互に叩くだけで確認の速さが倍になり、login 側だけ絞っている意味が薄れる。
func TestSignUpAndLoginShareRateLimit(t *testing.T) {
	// ここで叩くのは入力検証で落ちる signup と、レート制限で落ちる login だけなので、
	// リポジトリには届かない。ルート登録に要るミドルウェアは素通しにしておく
	passThrough := func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	e := handler.NewRouter(handler.Handlers{
		Auth:        handler.NewAuthHandler(auth.NewService(nil, nil), false),
		RequireUser: passThrough,
	})

	post := func(path string) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"email":"invalid","password":"password123"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.1:12345"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	// バースト（5 回）を signup で使い切る。入力検証で落ちても 1 回として数えられる
	for i := range 5 {
		if code := post("/api/auth/signup"); code != http.StatusBadRequest {
			t.Fatalf("signup #%d: status = %d, want %d", i+1, code, http.StatusBadRequest)
		}
	}

	if code := post("/api/auth/login"); code != http.StatusTooManyRequests {
		t.Errorf("login after 5 signups: status = %d, want %d", code, http.StatusTooManyRequests)
	}
	if code := post("/api/auth/signup"); code != http.StatusTooManyRequests {
		t.Errorf("6th signup: status = %d, want %d", code, http.StatusTooManyRequests)
	}
}

// 外部 API を呼ぶ /api/* はすべて RequireUser の内側にあることを保証する（BACKLOG 4-a の 4）。
// 未ログインで叩けると、AniList のプロセス全体の枠（24 本/分）や Claude の費用を誰でもただで使い潰せる。
// ルートを足すときに RequireUser を渡し忘れると、ここで落ちる。
func TestExternalAPIRoutesRequireLogin(t *testing.T) {
	deny := func(echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error { return c.NoContent(http.StatusUnauthorized) }
	}
	// ハンドラは呼ばれる前に deny で止まるので、中身は空でよい
	e := handler.NewRouter(handler.Handlers{RequireUser: deny})

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/search?type=anime&q=x"},
		{http.MethodGet, "/api/search/natural?type=anime&q=x"},
		{http.MethodGet, "/api/anilist/media?type=anime&ids=1"},
		{http.MethodGet, "/api/anilist/relations?id=1"},
		{http.MethodGet, "/api/recommendations?type=anime&genres=Action"},
		{http.MethodGet, "/api/recommendations/ai?type=anime"},
		{http.MethodGet, "/api/home/season-anime"},
		{http.MethodGet, "/api/home/trending"},
		{http.MethodGet, "/api/home/trending-manga"},
		{http.MethodGet, "/api/home/new-series"},
		{http.MethodPost, "/api/translate"},
		{http.MethodGet, "/api/new-releases/comics"},
		{http.MethodGet, "/api/books/latest-volume?title=x"},
		{http.MethodGet, "/api/records"},
		// 外部 API は呼ばないが、ログイン中の本人のパスワードを変える操作なので同じく塞がっていること
		{http.MethodPost, "/api/auth/password"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.path, nil)
			req.RemoteAddr = "203.0.113.2:12345"
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d (route is reachable without login)", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}
