package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Handlers はルーティングに必要なハンドラ・ミドルウェア一式をまとめたもの。
type Handlers struct {
	Record      *RecordHandler
	Auth        *AuthHandler
	Search      *SearchHandler
	Home        *HomeHandler
	Translate   *TranslateHandler
	Health      *HealthHandler
	RequireUser echo.MiddlewareFunc
	WebDir      string
}

// NewRouter はEchoのルーティングを構築する。静的ファイル配信のみ、既存の
// net/http.Handler実装（NewStaticHandler）をecho.WrapHandlerで接続する。
func NewRouter(h Handlers) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	// 未認証で叩ける POST（/api/translate・/api/auth/*）が上流 API への増幅器になるため、
	// ボディサイズに上限を置く。64K の根拠は「実際に必要な最大 × 十分な余裕」:
	//   - 最大のボディは /api/translate のあらすじ本文。AniList の description は長くても
	//     数 KB（UTF-8）で、これが実利用の上限。
	//   - 次に大きいのが /api/records の memo と genres で、いずれも桁が 1 つ小さい。
	// つまり 64K は正当な利用を一切壊さずに、1MB・10MB のボディを入口で落とせる値。
	//
	// ただしこれは「上限」であって対策そのものではない。64K でも translate は
	// chunkByteLimit=450 で分割されるので MyMemory 約 145 本に化ける（1MB なら約 2,300 本）。
	// 桁を 1 つ落とすだけの措置で、BACKLOG 4-a-1 の text 長の上限とは別に必要。
	// 同様に、GET のクエリ文字列（/api/anilist/media?ids=）にはこのミドルウェアは効かない。
	e.Use(middleware.BodyLimit("64K"))

	// コンテナ/ALBのヘルスチェック用。静的ファイルのワイルドカードより先に登録する。
	e.GET("/healthz", h.Health.Healthz)

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
