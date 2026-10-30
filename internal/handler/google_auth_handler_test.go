package handler

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

// googleUsers は Google ログインの経路だけを満たす UserRepository。sub-1 だけが登録済み。
type googleUsers struct{ auth.UserRepository }

func (googleUsers) FindByIdentity(ctx context.Context, provider, subject string) (*domain.User, error) {
	if provider == auth.ProviderGoogle && subject == "sub-1" {
		return &domain.User{ID: 1, Email: "user@example.com"}, nil
	}
	return nil, auth.ErrNotFound
}

type googleSessions struct {
	auth.SessionRepository
	created int
}

func (s *googleSessions) Create(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
	s.created++
	return nil
}

type stubGoogle struct{ profileCalls int }

func (g *stubGoogle) AuthCodeURL(state string) string {
	return "https://accounts.example/auth?" + url.Values{"state": {state}}.Encode()
}

func (g *stubGoogle) Profile(ctx context.Context, code string) (*auth.GoogleProfile, error) {
	g.profileCalls++
	return &auth.GoogleProfile{Subject: "sub-1", Email: "user@example.com", EmailVerified: true}, nil
}

func newGoogleHandler(secure bool) (*AuthHandler, *stubGoogle, *googleSessions) {
	g := &stubGoogle{}
	sessions := &googleSessions{}
	return NewAuthHandler(auth.NewService(googleUsers{}, sessions, auth.WithGoogle(g)), secure), g, sessions
}

// serve は h のハンドラに target のリクエストを 1 本流し、応答を返す。cookies はリクエストに付ける。
func serve(t *testing.T, handle func(echo.Context) error, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	if err := handle(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range (&http.Response{Header: rec.Header()}).Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// start は state を短命 Cookie に入れ、同じ state を付けて Google へ送る。Cookie はコールバックのパスにだけ届き、
// HttpOnly で、Google からの別サイトの画面遷移でも送られるよう SameSite=Lax にする（Strict だと必ず不一致になる）。
func TestGoogleStart_SetsStateCookieAndRedirects(t *testing.T) {
	h, _, _ := newGoogleHandler(true)

	rec := serve(t, h.GoogleStart, "/api/auth/google/start")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	cookie := cookieNamed(rec, googleStateCookieName)
	if cookie == nil {
		t.Fatal("state cookie is not set")
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Host != "accounts.example" || loc.Query().Get("state") != cookie.Value || len(cookie.Value) < 20 {
		t.Errorf("Location = %s, cookie = %q; want the same random state in both", loc, cookie.Value)
	}
	if cookie.Path != googleStateCookiePath || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie attributes = %+v", cookie)
	}
	if cookie.MaxAge <= 0 || cookie.MaxAge > 600 {
		t.Errorf("MaxAge = %d, want short-lived", cookie.MaxAge)
	}
}

// state が Cookie に無い・一致しないときは、Google に問い合わせずセッションも作らずにログイン画面へ返す（OAuth の CSRF 対策）。
// どの場合も state の Cookie は消す（1 回きり）。
func TestGoogleCallback_RejectsBadState(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		cookies []*http.Cookie
	}{
		{"no cookie", "/api/auth/google/callback?state=abc&code=c", nil},
		{"mismatch", "/api/auth/google/callback?state=abc&code=c", []*http.Cookie{{Name: googleStateCookieName, Value: "xyz"}}},
		{"empty state in both", "/api/auth/google/callback?state=&code=c", []*http.Cookie{{Name: googleStateCookieName, Value: ""}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, g, sessions := newGoogleHandler(false)

			rec := serve(t, h.GoogleCallback, tt.target, tt.cookies...)
			if loc := rec.Header().Get("Location"); loc != "/login?google_error=state" {
				t.Errorf("Location = %q, want /login?google_error=state", loc)
			}
			if g.profileCalls != 0 || sessions.created != 0 {
				t.Errorf("Google called %d times, sessions created %d; want 0", g.profileCalls, sessions.created)
			}
			if c := cookieNamed(rec, googleStateCookieName); c == nil || c.MaxAge >= 0 {
				t.Error("state cookie should be cleared")
			}
			if cookieNamed(rec, middleware.SessionCookieName) != nil {
				t.Error("session cookie should not be set")
			}
		})
	}
}

// 同意画面で断られたら（?error=access_denied）Google に問い合わせずに戻す。
func TestGoogleCallback_Cancelled(t *testing.T) {
	h, g, _ := newGoogleHandler(false)

	rec := serve(t, h.GoogleCallback, "/api/auth/google/callback?error=access_denied&state=abc",
		&http.Cookie{Name: googleStateCookieName, Value: "abc"})
	if loc := rec.Header().Get("Location"); loc != "/login?google_error=cancelled" {
		t.Errorf("Location = %q", loc)
	}
	if g.profileCalls != 0 {
		t.Error("Google should not be called")
	}
}

// state が一致すれば、パスワードのログインと同じセッション Cookie を発行し、start で預けた next へ戻す。
func TestGoogleCallback_SuccessRedirectsToNext(t *testing.T) {
	h, _, sessions := newGoogleHandler(false)
	state := "abc" + "." + base64.RawURLEncoding.EncodeToString([]byte("/library?type=manga"))

	rec := serve(t, h.GoogleCallback, "/api/auth/google/callback?state=abc&code=c",
		&http.Cookie{Name: googleStateCookieName, Value: state})
	if loc := rec.Header().Get("Location"); loc != "/library?type=manga" {
		t.Errorf("Location = %q, want /library?type=manga", loc)
	}
	if sessions.created != 1 {
		t.Errorf("sessions created = %d, want 1", sessions.created)
	}
	if c := cookieNamed(rec, middleware.SessionCookieName); c == nil || c.Value == "" || !c.HttpOnly {
		t.Errorf("session cookie = %+v", c)
	}
}

// Cookie に入った next が外へ出るパスなら使わずに "/" へ戻す。Cookie は利用者が書き換えられるので、
// start で確かめていてもコールバックでもう一度確かめる。
func TestGoogleCallback_IgnoresUnsafeNext(t *testing.T) {
	for _, next := range []string{"//evil.example", "/\\evil.example", "https://evil.example", "/a\r\nSet-Cookie: x=y"} {
		h, _, _ := newGoogleHandler(false)
		value := "abc." + base64.RawURLEncoding.EncodeToString([]byte(next))

		rec := serve(t, h.GoogleCallback, "/api/auth/google/callback?state=abc&code=c",
			&http.Cookie{Name: googleStateCookieName, Value: value})
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("next %q: Location = %q, want /", next, loc)
		}
	}
}

// 設定が無ければ start はログイン画面へ戻し、state の Cookie も作らない。
func TestGoogleStart_Disabled(t *testing.T) {
	h := NewAuthHandler(auth.NewService(googleUsers{}, &googleSessions{}), false)

	rec := serve(t, h.GoogleStart, "/api/auth/google/start")
	if loc := rec.Header().Get("Location"); loc != "/login?google_error=unavailable" {
		t.Errorf("Location = %q", loc)
	}
	if cookieNamed(rec, googleStateCookieName) != nil {
		t.Error("state cookie should not be set")
	}
}
