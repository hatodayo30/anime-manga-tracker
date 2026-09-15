// Package jikan は Jikan API (https://jikan.moe) — MyAnimeList の非公式REST API —
// への最小限のクライアントを提供する。パブリックデータのみを扱うためAPIキーは不要。
// AniListにはない「MAL上のランキング順位」「掲載誌（serializations）」を補うために使う。
package jikan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
)

const baseURL = "https://api.jikan.moe/v4"

// minRequestInterval はJikanのパブリックレート制限（目安 3req/秒）を守るための、
// リクエスト間の最小間隔。Client経由の全呼び出し（GetManga・Search系）で共有する。
const minRequestInterval = 350 * time.Millisecond

// Client は Jikan API へのHTTPクライアント。
type Client struct {
	httpClient *http.Client

	mu       sync.Mutex
	lastCall time.Time
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// throttle はリクエストを送る前に呼び出し、直前の呼び出しからminRequestInterval経つまで待つ。
func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := max(minRequestInterval-time.Since(c.lastCall), 0)
	c.lastCall = time.Now().Add(wait)
	c.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// MangaStats はホーム画面の「話題の漫画」「掲載誌で探す」向けに使う、Jikan由来の補足情報。
type MangaStats struct {
	Rank      *int     `json:"rank,omitempty"`    // MAL上の人気ランキング順位
	Members   *int     `json:"members,omitempty"` // 記録している会員数
	Magazines []string `json:"magazines"`         // 掲載誌名（serializations）
}

type serialization struct {
	Name string `json:"name"`
}

type mangaData struct {
	Rank           *int            `json:"rank"`
	Members        *int            `json:"members"`
	Serializations []serialization `json:"serializations"`
}

type mangaResponse struct {
	Data mangaData `json:"data"`
}

// GetManga はMAL IDから該当漫画のランキング・会員数・掲載誌を取得する。
func (c *Client) GetManga(ctx context.Context, malID int64) (*MangaStats, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/manga/%d", baseURL, malID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call jikan: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jikan request failed: %s", resp.Status)
	}

	var wrapper mangaResponse
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	magazines := make([]string, 0, len(wrapper.Data.Serializations))
	for _, s := range wrapper.Data.Serializations {
		magazines = append(magazines, s.Name)
	}

	return &MangaStats{
		Rank:      wrapper.Data.Rank,
		Members:   wrapper.Data.Members,
		Magazines: magazines,
	}, nil
}

// SearchAnime は検索語でJikanのアニメを検索する。AniList側の検索結果が少ないときの
// フォールバック用途で、結果はanilist.SearchResultと同じ形状に変換して返す
// （呼び出し元でAniListの検索結果とマージしやすくするため）。
func (c *Client) SearchAnime(ctx context.Context, query string) ([]anilist.SearchResult, error) {
	return c.search(ctx, "anime", query)
}

// SearchManga はSearchAnimeの漫画版。
func (c *Client) SearchManga(ctx context.Context, query string) ([]anilist.SearchResult, error) {
	return c.search(ctx, "manga", query)
}

func (c *Client) search(ctx context.Context, kind, query string) ([]anilist.SearchResult, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	q := url.Values{
		"q":        {query},
		"order_by": {"popularity"},
		"sort":     {"asc"}, // MALの人気順位は値が小さいほど人気なので昇順で最も人気のある作品から並ぶ
		"sfw":      {"true"},
		"limit":    {"10"},
	}
	reqURL := fmt.Sprintf("%s/%s?%s", baseURL, kind, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call jikan: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jikan request failed: %s", resp.Status)
	}

	var wrapper searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	results := make([]anilist.SearchResult, 0, len(wrapper.Data))
	for i, entry := range wrapper.Data {
		results = append(results, toSearchResult(kind, entry, len(wrapper.Data)-i))
	}
	return results, nil
}

func pickTitle(entry searchEntry) string {
	for _, t := range entry.Titles {
		if t.Type == "Japanese" {
			return t.Title
		}
	}
	return entry.Title
}

func genreNames(genres []jikanGenre) []string {
	names := make([]string, 0, len(genres))
	for _, g := range genres {
		names = append(names, g.Name)
	}
	return names
}

// toSearchResult はJikanの検索結果1件をanilist.SearchResultの形状に変換する。
// AniListに存在しないMAL専用作品でも一意に扱えるよう、AniListIDには実在のAniList IDと衝突しない
// 負のIDを合成して割り当てる（MAL IDを取り違えないよう符号を反転するだけ）。
// rank（1始まり、値が大きいほど人気）はAniListの検索結果とマージ後に人気順でソートするための
// 簡易的なスコアとして使う。
func toSearchResult(kind string, entry searchEntry, rank int) anilist.SearchResult {
	malID := entry.MalID
	synthAniListID := -malID

	var score *int
	if entry.Score != nil {
		s := int(*entry.Score*10 + 0.5)
		score = &s
	}

	var total *int
	if kind == "anime" {
		total = entry.Episodes
	} else {
		total = entry.Chapters
	}

	synopsis := ""
	if entry.Synopsis != nil {
		synopsis = *entry.Synopsis
	}

	return anilist.SearchResult{
		AniListID:     synthAniListID,
		Title:         pickTitle(entry),
		CoverImageURL: entry.Images.JPG.ImageURL,
		Genres:        genreNames(entry.Genres),
		Total:         total,
		Volumes:       entry.Volumes,
		Score:         score,
		Synopsis:      synopsis,
		Popularity:    rank * 100,
		MalID:         &malID,
	}
}
