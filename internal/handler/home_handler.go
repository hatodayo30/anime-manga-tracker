package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/jikan"
)

// HomeHandler はログイン不要のホーム画面向けデータ（今季アニメ・人気ランキング）を提供する。
type HomeHandler struct {
	client      *anilist.Client
	jikanClient *jikan.Client
}

func NewHomeHandler(client *anilist.Client, jikanClient *jikan.Client) *HomeHandler {
	return &HomeHandler{client: client, jikanClient: jikanClient}
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

// MangaRankingItem は「話題の漫画 TOP10」「掲載誌で探す」向けに、AniListの検索結果へ
// Jikan（MyAnimeList）由来のランキング順位・会員数・掲載誌を足したもの。
type MangaRankingItem struct {
	anilist.SearchResult
	MalRank   *int     `json:"malRank,omitempty"`
	Members   *int     `json:"members,omitempty"`
	Magazines []string `json:"magazines"`
}

// jikanEnrichTimeout は1件あたりのJikan呼び出しに許すタイムアウト。
// Jikanはレート制限（3req/秒程度）があるパブリックAPIのため、1件失敗しても全体は継続する。
const jikanEnrichTimeout = 4 * time.Second

// TrendingManga handles GET /api/home/trending-manga
// AniListの人気順TOP10を取得し、各作品のMAL IDを使ってJikanからランキング順位・会員数・掲載誌を補う。
func (h *HomeHandler) TrendingManga(w http.ResponseWriter, r *http.Request) {
	results, err := h.client.TrendingManga(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "anilist request failed: "+err.Error())
		return
	}

	items := make([]MangaRankingItem, len(results))
	for i, res := range results {
		items[i] = MangaRankingItem{SearchResult: res, Magazines: []string{}}
		if res.MalID == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(r.Context(), jikanEnrichTimeout)
		stats, err := h.jikanClient.GetManga(ctx, *res.MalID)
		cancel()
		if err != nil {
			continue // Jikan側の失敗はランキング自体を止めない。この作品の補足情報だけ空になる
		}

		items[i].MalRank = stats.Rank
		items[i].Members = stats.Members
		items[i].Magazines = stats.Magazines

		// Jikanはパブリックのレート制限（目安 3req/秒）があるため、直列呼び出しの間隔を空ける。
		time.Sleep(350 * time.Millisecond)
	}

	writeJSON(w, http.StatusOK, items)
}
