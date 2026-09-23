package handler

import (
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

// NewRouter はEchoのルーティングを構築する。静的ファイル配信のみ、既存の
// net/http.Handler実装（NewStaticHandler）をecho.WrapHandlerで接続する。
func NewRouter(h Handlers) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	records := e.Group("/api/records", h.RequireUser)
	records.GET("", h.Record.List)
	records.POST("", h.Record.Create)
	records.PATCH("/:id", h.Record.Update)
	records.DELETE("/:id", h.Record.Delete)

	e.GET("/api/search", h.Search.Search)
	e.GET("/api/anilist/media", h.Search.ByIDs)
	e.GET("/api/anilist/relations", h.Search.Relations)
	e.GET("/api/recommendations", h.Search.Recommendations)
	e.GET("/api/home/season-anime", h.Home.SeasonAnime)
	e.GET("/api/home/trending", h.Home.Trending)
	e.GET("/api/home/trending-manga", h.Home.TrendingManga)
	e.POST("/api/translate", h.Translate.Translate)

	e.POST("/api/auth/signup", h.Auth.SignUp)
	e.POST("/api/auth/login", h.Auth.Login)
	e.POST("/api/auth/logout", h.Auth.Logout)
	e.GET("/api/auth/me", h.Auth.Me)

	e.Any("/*", echo.WrapHandler(NewStaticHandler(h.WebDir)))

	return e
}
