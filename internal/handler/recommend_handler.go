package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/recommend"
)

type RecommendHandler struct {
	usecase *recommend.Usecase
}

func NewRecommendHandler(u *recommend.Usecase) *RecommendHandler {
	return &RecommendHandler{usecase: u}
}

// AI handles GET /api/recommendations/ai?type=anime
// ホームの「AI のおすすめ」向けに、ログイン中の利用者の記録から理由付きのおすすめを返す（BACKLOG 9-c）。
//
// /api/recommendations と違ってジャンルをクライアントから受け取らない。記録はサーバーで読むので、
// ルーターで RequireUser を被せている（AI を呼ぶ経路はログイン必須、BACKLOG 9「決めたこと」）。
func (h *RecommendHandler) AI(c echo.Context) error {
	mediaType := domain.MediaType(c.QueryParam("type"))
	if !mediaType.Valid() {
		return writeError(c, http.StatusBadRequest, "type must be 'anime' or 'manga'")
	}

	result, err := h.usecase.For(c.Request().Context(), middleware.UserFromContext(c).ID, mediaType)
	if err != nil {
		// 詳細は外部に出さずログにだけ残す（BACKLOG 4-d）。AI の失敗はここまで来ない（ジャンルベースに落ちる）
		c.Logger().Errorf("recommend: %v", err)
		return writeError(c, http.StatusBadGateway, "recommendations failed")
	}
	return c.JSON(http.StatusOK, result)
}
