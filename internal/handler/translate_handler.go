package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/translate"
)

type TranslateHandler struct {
	usecase *translate.Usecase
}

func NewTranslateHandler(u *translate.Usecase) *TranslateHandler {
	return &TranslateHandler{usecase: u}
}

type translateRequest struct {
	Text   string `json:"text"`
	Target string `json:"target"`
}

// Translate handles POST /api/translate { "text": "...", "target": "ja" }
// 作品モーダルのあらすじを表示言語（JA）に合わせて翻訳するために使う。
// あらすじは数百〜千文字を超えることがあるため、GETクエリではなくPOSTボディで受け取る。
func (h *TranslateHandler) Translate(c echo.Context) error {
	var in translateRequest
	if err := c.Bind(&in); err != nil {
		return writeError(c, http.StatusBadRequest, "invalid request body")
	}

	translated, err := h.usecase.Translate(c.Request().Context(), in.Text, in.Target)
	if err != nil {
		return writeError(c, http.StatusBadGateway, "translate failed: "+err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"translated": translated})
}
