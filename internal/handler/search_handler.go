package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/jikan"
	"github.com/hatodayo30/anime-manga-tracker/internal/model"
)

type SearchHandler struct {
	client      *anilist.Client
	jikanClient *jikan.Client

	recommendCache keyedCache[[]anilist.SearchResult]
}

func NewSearchHandler(client *anilist.Client, jikanClient *jikan.Client) *SearchHandler {
	return &SearchHandler{client: client, jikanClient: jikanClient}
}

// jikanFallbackThreshold 以下の件数しかAniListの検索結果が無いとき、Jikan（MyAnimeList）でも
// 検索してマージする。AniListには無い/ヒットしにくい作品を拾うためのフォールバック。
const jikanFallbackThreshold = 3

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

	if strings.TrimSpace(query) != "" && len(results) <= jikanFallbackThreshold {
		results = h.withJikanFallback(r, mediaType, query, results)
	}

	writeJSON(w, http.StatusOK, results)
}

// withJikanFallback はAniListの検索結果にJikanの検索結果を足し、タイトルの重複を除いて
// 人気順にまとめる。Jikan側が失敗してもAniListの結果はそのまま返す。
func (h *SearchHandler) withJikanFallback(r *http.Request, mediaType model.MediaType, query string, aniListResults []anilist.SearchResult) []anilist.SearchResult {
	var jikanResults []anilist.SearchResult
	var err error
	if mediaType == model.MediaTypeManga {
		jikanResults, err = h.jikanClient.SearchManga(r.Context(), query)
	} else {
		jikanResults, err = h.jikanClient.SearchAnime(r.Context(), query)
	}
	if err != nil || len(jikanResults) == 0 {
		return aniListResults
	}

	seenTitles := make(map[string]bool, len(aniListResults))
	for _, res := range aniListResults {
		seenTitles[strings.ToLower(res.Title)] = true
	}

	merged := append([]anilist.SearchResult{}, aniListResults...)
	for _, res := range jikanResults {
		key := strings.ToLower(res.Title)
		if seenTitles[key] {
			continue
		}
		seenTitles[key] = true
		merged = append(merged, res)
	}

	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Popularity > merged[j].Popularity })
	return merged
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

// Relations handles GET /api/anilist/relations?id=123
// 作品モーダルの「関連作品」セクション向けに、続編・前日譚・スピンオフ等を返す。
func (h *SearchHandler) Relations(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a number")
		return
	}

	results, err := h.client.Relations(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}
	if results == nil {
		results = []anilist.RelatedWork{}
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

	sortedGenres := append([]string{}, genres...)
	sort.Strings(sortedGenres)
	cacheKey := string(mediaType) + "|" + strings.Join(sortedGenres, ",")

	results, err := h.recommendCache.get(cacheKey, func() ([]anilist.SearchResult, error) {
		return h.client.ByGenres(r.Context(), genres, mediaType)
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}
	if results == nil {
		results = []anilist.SearchResult{}
	}
	writeJSON(w, http.StatusOK, results)
}
