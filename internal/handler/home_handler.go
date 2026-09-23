package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/home"
)

// HomeHandler はログイン不要のホーム画面向けデータ（今季アニメ・人気ランキング）を提供する。
type HomeHandler struct {
	usecase *home.Usecase
}

func NewHomeHandler(u *home.Usecase) *HomeHandler {
	return &HomeHandler{usecase: u}
}

// SeasonAnime handles GET /api/home/season-anime[?season=WINTER&year=2026]
// season/yearを省略すると現在のシーズンを返す（ホーム画面向け）。
// 両方指定するとシーズンブラウジングページ向けに任意のシーズンを返す。
func (h *HomeHandler) SeasonAnime(c echo.Context) error {
	seasonParam := strings.ToUpper(strings.TrimSpace(c.QueryParam("season")))
	yearParam := strings.TrimSpace(c.QueryParam("year"))

	if seasonParam == "" && yearParam == "" {
		results, err := h.usecase.CurrentSeasonAnime(c.Request().Context())
		if err != nil {
			return writeError(c, http.StatusBadGateway, "anilist request failed: "+err.Error())
		}
		return c.JSON(http.StatusOK, results)
	}

	if !anilist.ValidSeasons[seasonParam] {
		return writeError(c, http.StatusBadRequest, "season must be one of WINTER, SPRING, SUMMER, FALL")
	}
	year, err := strconv.Atoi(yearParam)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "year must be a number")
	}

	results, err := h.usecase.SeasonAnimeFor(c.Request().Context(), seasonParam, year)
	if err != nil {
		return writeError(c, http.StatusBadGateway, "anilist request failed: "+err.Error())
	}
	return c.JSON(http.StatusOK, results)
}

// Trending handles GET /api/home/trending
func (h *HomeHandler) Trending(c echo.Context) error {
	results, err := h.usecase.TrendingAnime(c.Request().Context())
	if err != nil {
		return writeError(c, http.StatusBadGateway, "anilist request failed: "+err.Error())
	}
	return c.JSON(http.StatusOK, results)
}

// TrendingManga handles GET /api/home/trending-manga
// AniListの人気順TOP10を取得し、各作品のMAL IDを使ってJikanからランキング順位・会員数・掲載誌を補う。
func (h *HomeHandler) TrendingManga(c echo.Context) error {
	items, err := h.usecase.TrendingManga(c.Request().Context())
	if err != nil {
		return writeError(c, http.StatusBadGateway, "anilist request failed: "+err.Error())
	}
	return c.JSON(http.StatusOK, items)
}
