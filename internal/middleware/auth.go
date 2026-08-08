// Package middleware は net/http ハンドラを横断する共通処理を提供する。
package middleware

import (
	"context"
	"net/http"

	"github.com/hatodayo30/anime-manga-tracker/internal/model"
	"github.com/hatodayo30/anime-manga-tracker/internal/service"
)

const SessionCookieName = "session_token"

type contextKey int

const userContextKey contextKey = iota

// Auth はセッションCookieの検証を行うミドルウェアファクトリ。
type Auth struct {
	authService *service.AuthService
}

func NewAuth(authService *service.AuthService) *Auth {
	return &Auth{authService: authService}
}

// RequireUser はログイン必須のハンドラに被せる。未ログインなら401を返す。
func (a *Auth) RequireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.userFromRequest(r)
		if err != nil {
			http.Error(w, `{"error":"login required"}`, http.StatusUnauthorized)
			return
		}
		next(w, r.WithContext(withUser(r.Context(), user)))
	}
}

func (a *Auth) userFromRequest(r *http.Request) (*model.User, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil, err
	}
	return a.authService.CurrentUser(r.Context(), cookie.Value)
}

func withUser(ctx context.Context, u *model.User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// UserFromContext は RequireUser を通ったリクエストのコンテキストからユーザーを取り出す。
func UserFromContext(ctx context.Context) *model.User {
	u, _ := ctx.Value(userContextKey).(*model.User)
	return u
}
