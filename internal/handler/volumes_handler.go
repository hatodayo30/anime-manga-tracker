package handler

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/volumes"
)

// maxVolumeTitleRunes は /api/books/latest-volume の title の上限（文字数）。作品名なので /api/search の q と同じにした。
// 長い題名を通すと、楽天への問い合わせとキャッシュのキーの大きさを呼び出し側が決められてしまう
const maxVolumeTitleRunes = 100

// VolumesHandler は漫画の既刊の巻数（BACKLOG 11-c）を提供する。
type VolumesHandler struct {
	usecase *volumes.Usecase
}

func NewVolumesHandler(u *volumes.Usecase) *VolumesHandler {
	return &VolumesHandler{usecase: u}
}

// LatestVolume handles GET /api/books/latest-volume?title=作品名
// 楽天のキーが無い環境では {"configured": false, "found": false} を 200 で返す。
func (h *VolumesHandler) LatestVolume(c echo.Context) error {
	title := strings.TrimSpace(c.QueryParam("title"))
	if title == "" {
		return writeError(c, http.StatusBadRequest, "title is required")
	}
	if utf8.RuneCountInString(title) > maxVolumeTitleRunes {
		return writeError(c, http.StatusBadRequest, "title is too long")
	}
	result, err := h.usecase.Latest(c.Request().Context(), title)
	if err != nil {
		// 詳細は外部に出さずログにだけ残す（BACKLOG 4-d）
		c.Logger().Errorf("volumes: latest volume: %v", err)
		return writeError(c, http.StatusBadGateway, "rakuten request failed")
	}
	return c.JSON(http.StatusOK, result)
}
