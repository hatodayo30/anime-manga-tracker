package handler

import (
	"net/http"
	"strconv"
	"strings"

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

// ByIDs handles GET /api/anilist/media?type=anime&ids=1,2,3
// マイライブラリ画面で、保存済み作品の総話数・放送状況・現在の話数を最新化するために使う。
func (h *SearchHandler) ByIDs(w http.ResponseWriter, r *http.Request) {
	mediaType := model.MediaType(r.URL.Query().Get("type"))
	if !mediaType.Valid() {
		writeError(w, http.StatusBadRequest, "type must be 'anime' or 'manga'")
		return
	}

	idsParam := strings.TrimSpace(r.URL.Query().Get("ids"))
	var ids []int64
	if idsParam != "" {
		for s := range strings.SplitSeq(idsParam, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid id in ids: "+s)
				return
			}
			ids = append(ids, id)
		}
	}

	results, err := h.client.MediaByIDs(r.Context(), ids, mediaType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}
	if results == nil {
		results = []anilist.SearchResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

// Recommendations handles GET /api/recommendations?type=anime&genres=Action,Fantasy
// おすすめ画面向けに、指定ジャンルのいずれかに合致する作品を人気順で返す。
// 「どのジャンルを見るか」はクライアント（ログイン中ユーザーのライブラリ集計）が決めるため、
// このエンドポイント自体はログイン不要。
func (h *SearchHandler) Recommendations(w http.ResponseWriter, r *http.Request) {
	mediaType := model.MediaType(r.URL.Query().Get("type"))
	if !mediaType.Valid() {
		writeError(w, http.StatusBadRequest, "type must be 'anime' or 'manga'")
		return
	}

	genresParam := strings.TrimSpace(r.URL.Query().Get("genres"))
	var genres []string
	if genresParam != "" {
		for g := range strings.SplitSeq(genresParam, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				genres = append(genres, g)
			}
		}
	}

	results, err := h.client.ByGenres(r.Context(), genres, mediaType)
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}
	if results == nil {
		results = []anilist.SearchResult{}
	}
	writeJSON(w, http.StatusOK, results)
}
