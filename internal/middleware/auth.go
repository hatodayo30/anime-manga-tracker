// Package middleware は echo.HandlerFunc を横断する共通処理を提供する。
package middleware

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

const SessionCookieName = "session_token"

const userContextKey = "user"

// AuthService は Auth がログイン中ユーザーの解決に必要とする操作を定義する。
// usecase/auth.Service がこれを満たす。
type AuthService interface {
	CurrentUser(ctx context.Context, token string) (*domain.User, error)
}

// Auth はセッションCookieの検証を行うミドルウェアファクトリ。
type Auth struct {
	authService AuthService
}

func NewAuth(authService AuthService) *Auth {
	return &Auth{authService: authService}
}

// RequireUser はログイン必須のハンドラに被せる。未ログインなら401を返す。
func (a *Auth) RequireUser(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(SessionCookieName)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
		}

		user, err := a.authService.CurrentUser(c.Request().Context(), cookie.Value)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "login required"})
		}

		c.Set(userContextKey, user)
		return next(c)
	}
}

// UserFromContext は RequireUser を通ったリクエストの echo.Context からユーザーを取り出す。
func UserFromContext(c echo.Context) *domain.User {
	u, _ := c.Get(userContextKey).(*domain.User)
	return u
}
