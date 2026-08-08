package handler

import (
	"net/http"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
)

// HomeHandler はログイン不要のホーム画面向けデータ（今季アニメ・人気ランキング）を提供する。
type HomeHandler struct {
	client *anilist.Client
}

func NewHomeHandler(client *anilist.Client) *HomeHandler {
	return &HomeHandler{client: client}
}

// SeasonAnime handles GET /api/home/season-anime
func (h *HomeHandler) SeasonAnime(w http.ResponseWriter, r *http.Request) {
	results, err := h.client.SeasonAnime(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

// Trending handles GET /api/home/trending
func (h *HomeHandler) Trending(w http.ResponseWriter, r *http.Request) {
	results, err := h.client.TrendingAnime(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}
