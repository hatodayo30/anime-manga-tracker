package handler

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/search"
)

// 未認証で叩ける AniList プロキシの入力上限。AniList のトークンバケットはプロセス全体で
// 24 req/分しかないので、上限が無いと 1 リクエストで全ユーザーぶんと定期ジョブぶんの
// トークンを食い潰せる（BACKLOG 4-a）。
const (
	// maxMediaIDs は /api/anilist/media の ids の件数上限。上流へは
	// anilist.byIDsPageSize（50 件）ごとに分割して投げるので、1 リクエストあたり
	// 最大 2 本に収まる。マイライブラリ 1 画面ぶんとしては十分な数。
	maxMediaIDs = 100

	// maxSearchQueryRunes は /api/search の q の長さ上限（文字数）。
	// 日本語タイトルを考慮してバイトではなく文字で数える。作品名としてはこれで足りるし、
	// 長い q は上流で無駄にヒットしない（= Jikan フォールバックも走る）だけ。
	maxSearchQueryRunes = 100

	// maxRecommendGenres は /api/recommendations の genres の件数上限。
	// AniList のジャンルは 20 種類ほどしかないので、これを超える指定は正当な利用ではない。
	maxRecommendGenres = 20

	// maxGenresParamRunes は genres パラメータ全体の長さ上限（文字数）。
	// 件数と別に長さも見るのは、この文字列がそのまま
	// おすすめキャッシュのキー（`type|genres`）になるため。
	// 長さを縛らないと、1 エントリあたりのメモリを呼び出し側が決められてしまう。
	maxGenresParamRunes = 400
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
	if utf8.RuneCountInString(query) > maxSearchQueryRunes {
		return writeError(c, http.StatusBadRequest, "q must be "+strconv.Itoa(maxSearchQueryRunes)+" characters or fewer")
	}

	results, err := h.usecase.Search(c.Request().Context(), mediaType, query)
	if err != nil {
		// 詳細は外部に出さずログにだけ残す。上流の URL や内部事情がクライアントに漏れるため（BACKLOG 4-d）。
		c.Logger().Errorf("search: %v", err)
		return writeError(c, http.StatusBadGateway, "anilist search failed")
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
			// パースの途中で打ち切る。5,000 件のような入力を最後まで読む必要はない。
			if len(ids) > maxMediaIDs {
				return writeError(c, http.StatusBadRequest, "ids must contain "+strconv.Itoa(maxMediaIDs)+" ids or fewer")
			}
		}
	}

	results, err := h.usecase.ByIDs(c.Request().Context(), mediaType, ids)
	if err != nil {
		c.Logger().Errorf("search: by ids: %v", err)
		return writeError(c, http.StatusBadGateway, "anilist request failed")
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
		c.Logger().Errorf("search: relations: %v", err)
		return writeError(c, http.StatusBadGateway, "anilist request failed")
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
	if utf8.RuneCountInString(genresParam) > maxGenresParamRunes {
		return writeError(c, http.StatusBadRequest, "genres must be "+strconv.Itoa(maxGenresParamRunes)+" characters or fewer")
	}

	var genres []string
	if genresParam != "" {
		for g := range strings.SplitSeq(genresParam, ",") {
			g = strings.TrimSpace(g)
			if g == "" {
				continue
			}
			genres = append(genres, g)
			if len(genres) > maxRecommendGenres {
				return writeError(c, http.StatusBadRequest, "genres must contain "+strconv.Itoa(maxRecommendGenres)+" genres or fewer")
			}
		}
	}

	results, err := h.usecase.Recommendations(c.Request().Context(), mediaType, genres)
	if err != nil {
		c.Logger().Errorf("search: recommendations: %v", err)
		return writeError(c, http.StatusBadGateway, "anilist request failed")
	}
	if results == nil {
		results = []anilist.SearchResult{}
	}
	return c.JSON(http.StatusOK, results)
}
