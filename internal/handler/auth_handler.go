package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/service"
)

type AuthHandler struct {
	service *service.AuthService
}

func NewAuthHandler(s *service.AuthService) *AuthHandler {
	return &AuthHandler{service: s}
}

type credentialsInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// SignUp handles POST /api/auth/signup
func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	var in credentialsInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, token, expiresAt, err := h.service.SignUp(r.Context(), in.Email, in.Password)
	if err != nil {
		if errors.Is(err, service.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	setSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusCreated, user)
}

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in credentialsInput
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, token, expiresAt, err := h.service.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	setSessionCookie(w, token, expiresAt)
	writeJSON(w, http.StatusOK, user)
}

// Logout handles POST /api/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.SessionCookieName); err == nil {
		_ = h.service.Logout(r.Context(), cookie.Value)
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// Me handles GET /api/auth/me — ログイン中なら現在のユーザー、未ログインなら401。
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(middleware.SessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}

	user, err := h.service.CurrentUser(r.Context(), cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
