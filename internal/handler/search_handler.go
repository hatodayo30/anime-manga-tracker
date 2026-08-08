package handler

import (
	"net/http"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/model"
)

type SearchHandler struct {
	client *anilist.Client
}

func NewSearchHandler(client *anilist.Client) *SearchHandler {
	return &SearchHandler{client: client}
}

// Search handles GET /api/search?type=anime&q=蒼穹
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	mediaType := model.MediaType(r.URL.Query().Get("type"))
	if !mediaType.Valid() {
		writeError(w, http.StatusBadRequest, "type must be 'anime' or 'manga'")
		return
	}
	query := r.URL.Query().Get("q")

	results, err := h.client.Search(r.Context(), query, mediaType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist search failed: "+err.Error())
		return
	}
	if results == nil {
		results = []anilist.SearchResult{}
	}
	writeJSON(w, http.StatusOK, results)
}
