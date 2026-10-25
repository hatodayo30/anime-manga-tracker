package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/newreleases"
)

// NewReleasesHandler は「新着」タブの新刊（BACKLOG 11-b）を提供する。
type NewReleasesHandler struct {
	usecase *newreleases.Usecase
}

func NewNewReleasesHandler(u *newreleases.Usecase) *NewReleasesHandler {
	return &NewReleasesHandler{usecase: u}
}

// Comics handles GET /api/new-releases/comics
// 直近 1 週間に発売されたコミックを返す。楽天のキーが無い環境では {"configured": false, "items": []} を 200 で返す
// （応答の形は BACKLOG 11-b）。
func (h *NewReleasesHandler) Comics(c echo.Context) error {
	result, err := h.usecase.Comics(c.Request().Context())
	if err != nil {
		// 詳細は外部に出さずログにだけ残す（BACKLOG 4-d）
		c.Logger().Errorf("new releases: comics: %v", err)
		return writeError(c, http.StatusBadGateway, "rakuten request failed")
	}
	return c.JSON(http.StatusOK, result)
}
