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
		Auth:         handler.NewAuthHandler(auth.NewService(nil, nil), false),
		RequireUser:  passThrough,
		OptionalUser: passThrough,
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
