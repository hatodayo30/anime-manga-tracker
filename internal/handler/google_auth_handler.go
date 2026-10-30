package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
)

// Google ログイン（BACKLOG 8-b）。認可コードフローのうち、ブラウザとのやりとり（リダイレクトと state の Cookie）を持つ。
// Google との通信は internal/google、ユーザーの決め方とセッションの発行は usecase/auth。

const (
	// googleStateCookieName は state を入れておく短命 Cookie。
	googleStateCookieName = "google_oauth_state"
	// googleStateCookiePath は state の Cookie を送る範囲。コールバックにだけ届けばよく、
	// 他の API のリクエストに毎回付けて回る理由が無い。
	googleStateCookiePath = "/api/auth/google"
	// googleStateMaxAge は state の寿命（秒）。Google の同意画面で迷っても足りる長さにして、
	// 置き去りにされた state がいつまでも使えないようにする。
	googleStateMaxAge = 10 * 60
)

// コールバックで失敗したときにログイン画面へ渡す理由（?google_error=）。文言はフロントが持つ。
const (
	googleErrCancelled  = "cancelled"   // 同意画面で利用者が断った
	googleErrState      = "state"       // state が無い・一致しない（期限切れも含む）
	googleErrUnverified = "unverified"  // 初回で Google のメールアドレスが未確認
	googleErrLinked     = "linked"      // 同じメールアドレスのアカウントに別の Google アカウントが紐付いている
	googleErrFailed     = "failed"      // Google との通信や DB の失敗
	googleErrDisabled   = "unavailable" // 設定が無い
)

// Providers handles GET /api/auth/providers — ログイン画面がボタンを出し分けるための、使えるログインの手段。
// 未ログインで叩く。
func (h *AuthHandler) Providers(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]bool{"google": h.service.GoogleEnabled()})
}

// GoogleStart handles GET /api/auth/google/start — state を Cookie に入れて Google の同意画面へ送る。
// ?next= はログイン後に戻る画面で、state と一緒に Cookie に入れておく（Google を経由すると URL の情報は消える）。
//
// fetch ではなくブラウザの画面遷移で来るので、エラーも JSON ではなくログイン画面へのリダイレクトで返す。
func (h *AuthHandler) GoogleStart(c echo.Context) error {
	state := rand.Text() // crypto/rand の 128 ビット
	authURL, err := h.service.GoogleAuthURL(state)
	if err != nil {
		return redirectToLoginWithError(c, googleErrDisabled)
	}

	value := state
	if next := safeRedirectPath(c.QueryParam("next")); next != "" {
		value += "." + base64.RawURLEncoding.EncodeToString([]byte(next))
	}
	h.setGoogleStateCookie(c, value, googleStateMaxAge)
	return c.Redirect(http.StatusFound, authURL)
}

// GoogleCallback handles GET /api/auth/google/callback — Google から戻ってきたところ。
// Google Cloud Console に登録するリダイレクト URI はこのパスで、完全一致が要る。
func (h *AuthHandler) GoogleCallback(c echo.Context) error {
	// state は 1 回きり。成否にかかわらず消して、同じ state を使い回せないようにする
	cookie, cookieErr := c.Cookie(googleStateCookieName)
	h.setGoogleStateCookie(c, "", -1)

	if c.QueryParam("error") != "" {
		return redirectToLoginWithError(c, googleErrCancelled)
	}

	// state を照合しないと OAuth の CSRF が成立する。攻撃者が自分の Google アカウントで取った認可コードを
	// 踏ませると、被害者が攻撃者のアカウントにログインした状態になり、記録が攻撃者の側に溜まる
	if cookieErr != nil {
		return redirectToLoginWithError(c, googleErrState)
	}
	state, encodedNext, _ := strings.Cut(cookie.Value, ".")
	got := c.QueryParam("state")
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(got)) != 1 {
		return redirectToLoginWithError(c, googleErrState)
	}

	code := c.QueryParam("code")
	if code == "" {
		return redirectToLoginWithError(c, googleErrFailed)
	}

	_, token, expiresAt, err := h.service.LoginWithGoogle(c.Request().Context(), code)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrGoogleDisabled):
			return redirectToLoginWithError(c, googleErrDisabled)
		case errors.Is(err, auth.ErrGoogleEmailUnverified):
			return redirectToLoginWithError(c, googleErrUnverified)
		case errors.Is(err, auth.ErrIdentityTaken):
			return redirectToLoginWithError(c, googleErrLinked)
		}
		c.Logger().Errorf("auth: google login: %v", err)
		return redirectToLoginWithError(c, googleErrFailed)
	}

	h.setSessionCookie(c, token, expiresAt)
	next := "/"
	if b, err := base64.RawURLEncoding.DecodeString(encodedNext); err == nil {
		// Cookie は利用者が書き換えられるので、入れるときに確かめていても出すときにもう一度確かめる
		if p := safeRedirectPath(string(b)); p != "" {
			next = p
		}
	}
	return c.Redirect(http.StatusFound, next)
}

// setGoogleStateCookie は state の Cookie を書く（maxAge < 0 で消す）。
// SameSite は Lax でなければならない。Google からのコールバックは別サイトからの画面遷移で、
// Strict だと Cookie が送られず必ず state の不一致になる（Lax ならトップレベルの GET には付く）。
func (h *AuthHandler) setGoogleStateCookie(c echo.Context, value string, maxAge int) {
	c.SetCookie(&http.Cookie{
		Name:     googleStateCookieName,
		Value:    value,
		Path:     googleStateCookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func redirectToLoginWithError(c echo.Context, reason string) error {
	return c.Redirect(http.StatusFound, "/login?"+url.Values{"google_error": {reason}}.Encode())
}

// safeRedirectPath は同じサイト内のパスだけを返し、それ以外は空文字にする（オープンリダイレクト対策）。
// フロントの safeNextPath と同じ条件に加えて、"/\" で始まるもの（ブラウザが "//" と同じに扱う）と
// 制御文字も弾く。フロントは React Router の画面遷移なので外へは出ないが、こちらは Location ヘッダで
// ブラウザに解釈させるので条件を厳しくする。
func safeRedirectPath(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") {
		return ""
	}
	if strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	return p
}
