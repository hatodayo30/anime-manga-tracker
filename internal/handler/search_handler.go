package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/search"
)

type SearchHandler struct {
	usecase *search.Usecase
}

func NewSearchHandler(u *search.Usecase) *SearchHandler {
	return &SearchHandler{usecase: u}
}

// Search handles GET /api/search?type=anime&q=蒼穹
func (h *SearchHandler) Search(c echo.Context) error {
	mediaType := domain.MediaType(c.QueryParam("type"))
	if !mediaType.Valid() {
		return writeError(c, http.StatusBadRequest, "type must be 'anime' or 'manga'")
	}
	query := c.QueryParam("q")

	results, err := h.usecase.Search(c.Request().Context(), mediaType, query)
	if err != nil {
		return writeError(c, http.StatusBadGateway, "anilist search failed: "+err.Error())
	}
	if results == nil {
		results = []anilist.SearchResult{}
	}
	return c.JSON(http.StatusOK, results)
}

// ByIDs handles GET /api/anilist/media?type=anime&ids=1,2,3
// マイライブラリ画面で、保存済み作品の総話数・放送状況・現在の話数を最新化するために使う。
func (h *SearchHandler) ByIDs(c echo.Context) error {
	mediaType := domain.MediaType(c.QueryParam("type"))
	if !mediaType.Valid() {
		return writeError(c, http.StatusBadRequest, "type must be 'anime' or 'manga'")
	}

	idsParam := strings.TrimSpace(c.QueryParam("ids"))
	var ids []int64
	if idsParam != "" {
		for s := range strings.SplitSeq(idsParam, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err != nil {
				return writeError(c, http.StatusBadRequest, "invalid id in ids: "+s)
			}
			ids = append(ids, id)
		}
	}

	results, err := h.usecase.ByIDs(c.Request().Context(), mediaType, ids)
	if err != nil {
		return writeError(c, http.StatusBadGateway, "anilist request failed: "+err.Error())
	}
	if results == nil {
		results = []anilist.SearchResult{}
	}
	return c.JSON(http.StatusOK, results)
}

// Relations handles GET /api/anilist/relations?id=123
// 作品モーダルの「関連作品」セクション向けに、続編・前日譚・スピンオフ等を返す。
func (h *SearchHandler) Relations(c echo.Context) error {
	id, err := strconv.ParseInt(c.QueryParam("id"), 10, 64)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "id must be a number")
	}

	results, err := h.usecase.Relations(c.Request().Context(), id)
	if err != nil {
		return writeError(c, http.StatusBadGateway, "anilist request failed: "+err.Error())
	}
	if results == nil {
		results = []anilist.RelatedWork{}
	}
	return c.JSON(http.StatusOK, results)
}

// Recommendations handles GET /api/recommendations?type=anime&genres=Action,Fantasy
// おすすめ画面向けに、指定ジャンルのいずれかに合致する作品を人気順で返す。
// 「どのジャンルを見るか」はクライアント（ログイン中ユーザーのライブラリ集計）が決めるため、
// このエンドポイント自体はログイン不要。
func (h *SearchHandler) Recommendations(c echo.Context) error {
	mediaType := domain.MediaType(c.QueryParam("type"))
	if !mediaType.Valid() {
		return writeError(c, http.StatusBadRequest, "type must be 'anime' or 'manga'")
	}

	genresParam := strings.TrimSpace(c.QueryParam("genres"))
	var genres []string
	if genresParam != "" {
		for g := range strings.SplitSeq(genresParam, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				genres = append(genres, g)
			}
		}
	}

	results, err := h.usecase.Recommendations(c.Request().Context(), mediaType, genres)
	if err != nil {
		return writeError(c, http.StatusBadGateway, "anilist request failed: "+err.Error())
	}
	if results == nil {
		results = []anilist.SearchResult{}
	}
	return c.JSON(http.StatusOK, results)
}
