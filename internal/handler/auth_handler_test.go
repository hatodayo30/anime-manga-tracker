package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
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

// failingUsers は Create が DB の障害で失敗する UserRepository。
type failingUsers struct{ auth.UserRepository }

func (failingUsers) Create(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	return nil, errors.New("connection refused: internal detail")
}

// サインアップの失敗のうち、入力の誤りだけを 400 で文言ごと返し、DB の障害などは 500 と定型文にする（BACKLOG 4-d）。
// 以前は全部が 400 と err.Error() で、内部のエラーメッセージが画面に出ていた。
func TestSignUp_ErrorStatus(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		users    auth.UserRepository
		wantCode int
		wantMsg  string
	}{
		{"invalid email", `{"email":"no-at-mark","password":"password123"}`, failingUsers{}, http.StatusBadRequest, "invalid email"},
		{"short password", `{"email":"user@example.com","password":"short"}`, failingUsers{}, http.StatusBadRequest, "password must be at least 8 characters"},
		{"database failure", `{"email":"user@example.com","password":"password123"}`, failingUsers{}, http.StatusInternalServerError, "failed to sign up"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewAuthHandler(auth.NewService(tt.users, &googleSessions{}), false)
			req := httptest.NewRequest(http.MethodPost, "/api/auth/signup", strings.NewReader(tt.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			if err := h.SignUp(echo.New().NewContext(req, rec)); err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if !strings.Contains(rec.Body.String(), tt.wantMsg) {
				t.Errorf("body = %s, want it to contain %q", rec.Body.String(), tt.wantMsg)
			}
			if strings.Contains(rec.Body.String(), "internal detail") {
				t.Errorf("body leaks the internal error: %s", rec.Body.String())
			}
		})
	}
}

// meSessions はどのトークンでもユーザー 1 を返す SessionRepository。
type meSessions struct{ auth.SessionRepository }

func (meSessions) FindUser(ctx context.Context, tokenHash string) (*domain.User, error) {
	return &domain.User{ID: 1, Email: "user@example.com"}, nil
}

// providerUsers は Providers だけを満たす UserRepository。err があればそれを返す。
type providerUsers struct {
	auth.UserRepository
	providers []string
	err       error
}

func (u providerUsers) Providers(ctx context.Context, userID int64) ([]string, error) {
	return u.providers, u.err
}

// /me はログインの手段を返し、アカウント画面はこれでパスワード変更の欄を出し分ける。
// 手段が引けなくても /me 自体は 200 で返す（ログイン状態の確認まで止めない）。そのときは providers を省く。
func TestMe_Providers(t *testing.T) {
	tests := []struct {
		name  string
		users providerUsers
		want  string
	}{
		{"google only", providerUsers{providers: []string{"google"}}, `"providers":["google"]`},
		{"password and google", providerUsers{providers: []string{"google", "password"}}, `"providers":["google","password"]`},
		{"lookup failed", providerUsers{err: errors.New("db down")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewAuthHandler(auth.NewService(tt.users, meSessions{}), false)
			rec := serve(t, h.Me, "/api/auth/me", &http.Cookie{Name: middleware.SessionCookieName, Value: "a-token"})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, `"email":"user@example.com"`) {
				t.Errorf("body = %s, want the user", body)
			}
			if tt.want == "" {
				if strings.Contains(body, "providers") {
					t.Errorf("body = %s, want providers omitted", body)
				}
			} else if !strings.Contains(body, tt.want) {
				t.Errorf("body = %s, want %s", body, tt.want)
			}
		})
	}
}
