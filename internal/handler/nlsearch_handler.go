package handler

import (
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/nlsearch"
)

// maxNLSearchQueryRunes は /api/search/natural の q の長さ上限（文字数）。
// 作品名だけの /api/search（100 文字）より長い文を想定するが、q はそのままプロンプトに入るので
// 1 回あたりの入力トークン（＝費用）の上限として切る。「〜みたいな、〜で、〜じゃない漫画」でも 200 文字あれば足りる。
const maxNLSearchQueryRunes = 200

type NLSearchHandler struct {
	usecase *nlsearch.Usecase
}

func NewNLSearchHandler(u *nlsearch.Usecase) *NLSearchHandler {
	return &NLSearchHandler{usecase: u}
}

// Search handles GET /api/search/natural?type=manga&q=異世界で料理する漫画
// 検索画面の「AI で探す」から呼ぶ。ルーターで RequireUser を被せており、未ログインは 401（BACKLOG 9-b）。
// AI が使えないときも 200 で従来の検索結果を返し、fallback に理由を入れる。
func (h *NLSearchHandler) Search(c echo.Context) error {
	mediaType := domain.MediaType(c.QueryParam("type"))
	if !mediaType.Valid() {
		return writeError(c, http.StatusBadRequest, "type must be 'anime' or 'manga'")
	}
	query := c.QueryParam("q")
	if utf8.RuneCountInString(query) > maxNLSearchQueryRunes {
		return writeError(c, http.StatusBadRequest, "q must be "+strconv.Itoa(maxNLSearchQueryRunes)+" characters or fewer")
	}

	result, err := h.usecase.Search(c.Request().Context(), mediaType, query)
	if err != nil {
		// 詳細は外部に出さずログにだけ残す（BACKLOG 4-d）。ここに来るのは従来の検索まで失敗したとき
		c.Logger().Errorf("nlsearch: %v", err)
		return writeError(c, http.StatusBadGateway, "search failed")
	}
	return c.JSON(http.StatusOK, result)
}
