package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

type AuthHandler struct {
	service *auth.Service
}

func NewAuthHandler(s *auth.Service) *AuthHandler {
	return &AuthHandler{service: s}
}

type credentialsInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// SignUp handles POST /api/auth/signup
func (h *AuthHandler) SignUp(c echo.Context) error {
	var in credentialsInput
	if err := c.Bind(&in); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body")
	}

	user, token, expiresAt, err := h.service.SignUp(c.Request().Context(), in.Email, in.Password)
	if err != nil {
		if errors.Is(err, auth.ErrEmailTaken) {
			return jsonError(c, http.StatusConflict, "email already registered")
		}
		return jsonError(c, http.StatusBadRequest, err.Error())
	}

	setSessionCookie(c, token, expiresAt)
	return c.JSON(http.StatusCreated, user)
}

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(c echo.Context) error {
	var in credentialsInput
	if err := c.Bind(&in); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body")
	}

	user, token, expiresAt, err := h.service.Login(c.Request().Context(), in.Email, in.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return jsonError(c, http.StatusUnauthorized, "invalid email or password")
		}
		return jsonError(c, http.StatusBadRequest, err.Error())
	}

	setSessionCookie(c, token, expiresAt)
	return c.JSON(http.StatusOK, user)
}

// Logout handles POST /api/auth/logout
func (h *AuthHandler) Logout(c echo.Context) error {
	if cookie, err := c.Cookie(middleware.SessionCookieName); err == nil {
		_ = h.service.Logout(c.Request().Context(), cookie.Value)
	}
	clearSessionCookie(c)
	return c.NoContent(http.StatusNoContent)
}

// Me handles GET /api/auth/me — ログイン中なら現在のユーザー、未ログインなら401。
func (h *AuthHandler) Me(c echo.Context) error {
	cookie, err := c.Cookie(middleware.SessionCookieName)
	if err != nil {
		return jsonError(c, http.StatusUnauthorized, "not logged in")
	}

	user, err := h.service.CurrentUser(c.Request().Context(), cookie.Value)
	if err != nil {
		return jsonError(c, http.StatusUnauthorized, "not logged in")
	}
	return c.JSON(http.StatusOK, user)
}

func setSessionCookie(c echo.Context, token string, expiresAt time.Time) {
	c.SetCookie(&http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
