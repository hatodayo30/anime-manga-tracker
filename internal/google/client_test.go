package google_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/google"
)

// newServer は偽の Google（トークンと userinfo）に向けた Client を返す。
func newServer(t *testing.T, userinfo http.HandlerFunc) *google.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token request: %v", err)
		}
		if got := r.Form.Get("code"); got != "the-code" {
			t.Errorf("code = %q, want %q", got, "the-code")
		}
		if got := r.Form.Get("redirect_uri"); got != "http://localhost/callback" {
			t.Errorf("redirect_uri = %q", got)
		}
		// PKCE: 同意画面で code_challenge を載せたのと同じ verifier を、引き換えのときに送る
		if got := r.Form.Get("code_verifier"); got != "the-verifier" {
			t.Errorf("code_verifier = %q, want %q", got, "the-verifier")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access-token", "token_type": "Bearer", "expires_in": 3600,
			// 検証しない id_token。読まずに捨てることを確かめるため、中身は壊れた値にしておく
			"id_token": "not-a-jwt",
		})
	})
	mux.HandleFunc("/userinfo", userinfo)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return google.NewClient("client-id", "client-secret", "http://localhost/callback",
		google.WithEndpoints(srv.URL+"/auth", srv.URL+"/token", srv.URL+"/userinfo"))
}

// 認可コードをトークンに引き換え、そのアクセストークンで userinfo を叩いて sub / email / email_verified を返す。
func TestProfile(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Errorf("Authorization = %q, want Bearer access-token", got)
		}
		_, _ = w.Write([]byte(`{"sub": "1234", "email": "user@example.com", "email_verified": true, "picture": "x"}`))
	})

	p, err := c.Profile(context.Background(), "the-code", "the-verifier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Subject != "1234" || p.Email != "user@example.com" || !p.EmailVerified {
		t.Errorf("profile = %+v", p)
	}
}

// sub が無い応答は失敗にする。空の subject で通すと、全員が同じアカウントに入ってしまう。
func TestProfile_MissingSub(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"email": "user@example.com", "email_verified": true}`))
	})

	if _, err := c.Profile(context.Background(), "the-code", "the-verifier"); err == nil {
		t.Fatal("expected error for userinfo without sub")
	}
}

// userinfo が 200 以外なら失敗にする。
func TestProfile_UserinfoError(t *testing.T) {
	c := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error": "invalid_token"}`, http.StatusUnauthorized)
	})

	if _, err := c.Profile(context.Background(), "the-code", "the-verifier"); err == nil {
		t.Fatal("expected error for non-200 userinfo")
	}
}

// 同意画面の URL に state・リダイレクト URI・最小のスコープ（openid email）が載る。
func TestAuthCodeURL(t *testing.T) {
	c := google.NewClient("client-id", "client-secret", "http://localhost/callback")

	u, err := url.Parse(c.AuthCodeURL("the-state", "the-verifier"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("state") != "the-state" || q.Get("client_id") != "client-id" || q.Get("redirect_uri") != "http://localhost/callback" {
		t.Errorf("query = %v", q)
	}
	// PKCE: verifier そのものは載せず、SHA-256 を base64url にした code_challenge を S256 で載せる（RFC 7636）
	sum := sha256.Sum256([]byte("the-verifier"))
	if q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(sum[:]) || q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge = %q, method = %q", q.Get("code_challenge"), q.Get("code_challenge_method"))
	}
	if strings.Contains(u.String(), "the-verifier") {
		t.Errorf("verifier leaked into the URL: %s", u)
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want code", q.Get("response_type"))
	}
	if scopes := strings.Fields(q.Get("scope")); len(scopes) != 2 || scopes[0] != "openid" || scopes[1] != "email" {
		t.Errorf("scope = %q, want \"openid email\"", q.Get("scope"))
	}
}
