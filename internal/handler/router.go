package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Handlers はルーティングに必要なハンドラ・ミドルウェア一式をまとめたもの。
type Handlers struct {
	Record      *RecordHandler
	Auth        *AuthHandler
	Search      *SearchHandler
	Home        *HomeHandler
	Translate   *TranslateHandler
	RequireUser echo.MiddlewareFunc
	WebDir      string
}

// NewRouter はEchoのルーティングを構築する。
//
// records はEcho化されたハンドラをそのまま登録する。auth/search/home/translate は
// net/http ベースのハンドラのまま（次段階で移行予定）なので echo.WrapHandler で接続する。
func NewRouter(h Handlers) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	records := e.Group("/api/records", h.RequireUser)
	records.GET("", h.Record.List)
	records.POST("", h.Record.Create)
	records.PATCH("/:id", h.Record.Update)
	records.DELETE("/:id", h.Record.Delete)

	e.GET("/api/search", echo.WrapHandler(http.HandlerFunc(h.Search.Search)))
	e.GET("/api/anilist/media", echo.WrapHandler(http.HandlerFunc(h.Search.ByIDs)))
	e.GET("/api/anilist/relations", echo.WrapHandler(http.HandlerFunc(h.Search.Relations)))
	e.GET("/api/recommendations", echo.WrapHandler(http.HandlerFunc(h.Search.Recommendations)))
	e.GET("/api/home/season-anime", echo.WrapHandler(http.HandlerFunc(h.Home.SeasonAnime)))
	e.GET("/api/home/trending", echo.WrapHandler(http.HandlerFunc(h.Home.Trending)))
	e.GET("/api/home/trending-manga", echo.WrapHandler(http.HandlerFunc(h.Home.TrendingManga)))
	e.POST("/api/translate", echo.WrapHandler(http.HandlerFunc(h.Translate.Translate)))

	e.POST("/api/auth/signup", echo.WrapHandler(http.HandlerFunc(h.Auth.SignUp)))
	e.POST("/api/auth/login", echo.WrapHandler(http.HandlerFunc(h.Auth.Login)))
	e.POST("/api/auth/logout", echo.WrapHandler(http.HandlerFunc(h.Auth.Logout)))
	e.GET("/api/auth/me", echo.WrapHandler(http.HandlerFunc(h.Auth.Me)))

	e.Any("/*", echo.WrapHandler(NewStaticHandler(h.WebDir)))

	return e
}
