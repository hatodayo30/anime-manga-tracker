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

// Translate handles GET /api/translate?text=...&target=ja
// 作品モーダルのあらすじを表示言語（JA）に合わせて翻訳するために使う。
func (h *TranslateHandler) Translate(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	target := r.URL.Query().Get("target")
	if target == "" {
		target = "ja"
	}

	translated, err := h.client.Translate(r.Context(), text, target)
	if err != nil {
		writeError(w, http.StatusBadGateway, "translate failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"translated": translated})
}
