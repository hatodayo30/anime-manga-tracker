// Package jikan は Jikan API (https://jikan.moe) — MyAnimeList の非公式REST API —
// への最小限のクライアントを提供する。パブリックデータのみを扱うためAPIキーは不要。
// AniListにはない「MAL上のランキング順位」「掲載誌（serializations）」を補うために使う。
package jikan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "https://api.jikan.moe/v4"

// Client は Jikan API へのHTTPクライアント。
type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// MangaStats はホーム画面の「話題の漫画」「掲載誌で探す」向けに使う、Jikan由来の補足情報。
type MangaStats struct {
	Rank       *int     `json:"rank,omitempty"`    // MAL上の人気ランキング順位
	Members    *int     `json:"members,omitempty"` // 記録している会員数
	Magazines  []string `json:"magazines"`         // 掲載誌名（serializations）
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
	url := fmt.Sprintf("%s/manga/%d", baseURL, malID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
