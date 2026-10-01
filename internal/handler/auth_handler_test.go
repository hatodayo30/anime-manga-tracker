package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

// Secure 属性は COOKIE_SECURE で切り替わる。Phase 1 の平文HTTPアクセスでは
// 立ててはならない（ブラウザがCookieを送らずログインできない）。
func TestSessionCookieSecureAttribute(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "insecure", true: "secure"}[secure], func(t *testing.T) {
			h := &AuthHandler{cookieSecure: secure}

			set := issueCookie(t, func(c echo.Context) {
				h.setSessionCookie(c, "token-value", time.Now().Add(time.Hour))
			})
			if set.Secure != secure {
				t.Errorf("setSessionCookie: Secure = %v, want %v", set.Secure, secure)
			}
			if !set.HttpOnly {
				t.Error("setSessionCookie: HttpOnly = false, want true")
			}

			// 属性が食い違うとブラウザが別のCookieとみなし、ログアウトで消えない。
			cleared := issueCookie(t, h.clearSessionCookie)
			if cleared.Secure != set.Secure {
				t.Errorf("clearSessionCookie: Secure = %v, want %v", cleared.Secure, set.Secure)
			}
			if cleared.MaxAge >= 0 {
				t.Errorf("clearSessionCookie: MaxAge = %d, want negative", cleared.MaxAge)
			}
		})
	}
}

// issueCookie は set が書き込んだ Set-Cookie ヘッダを1つ読み戻す。
func issueCookie(t *testing.T, set func(echo.Context)) *http.Cookie {
	t.Helper()

	rec := httptest.NewRecorder()
	set(echo.New().NewContext(httptest.NewRequest(http.MethodPost, "/api/auth/login", nil), rec))

	cookies := (&http.Response{Header: rec.Header()}).Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies, want 1: %v", len(cookies), rec.Header().Values("Set-Cookie"))
	}
	return cookies[0]
}
