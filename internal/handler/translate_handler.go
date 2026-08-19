package handler

import (
	"net/http"

	"github.com/hatodayo30/anime-manga-tracker/internal/translate"
)

type TranslateHandler struct {
	client *translate.Client
}

func NewTranslateHandler(client *translate.Client) *TranslateHandler {
	return &TranslateHandler{client: client}
}

type translateRequest struct {
	Text   string `json:"text"`
	Target string `json:"target"`
}

// Translate handles POST /api/translate { "text": "...", "target": "ja" }
// 作品モーダルのあらすじを表示言語（JA）に合わせて翻訳するために使う。
// あらすじは数百〜千文字を超えることがあるため、GETクエリではなくPOSTボディで受け取る。
func (h *TranslateHandler) Translate(w http.ResponseWriter, r *http.Request) {
	var in translateRequest
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	target := in.Target
	if target == "" {
		target = "ja"
	}

	translated, err := h.client.Translate(r.Context(), in.Text, target)
	if err != nil {
		writeError(w, http.StatusBadGateway, "translate failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"translated": translated})
}
