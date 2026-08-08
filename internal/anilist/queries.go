package anilist

import (
	"context"
	"fmt"
	"strings"

	"github.com/hatodayo30/anime-manga-tracker/internal/model"
)

// SearchResult は検索結果として画面に返す1作品分の情報。
type SearchResult struct {
	AniListID     int64    `json:"anilistId"`
	Title         string   `json:"title"`
	CoverImageURL string   `json:"coverImageUrl"`
	Genres        []string `json:"genres"`
	Total         *int     `json:"total"` // アニメ: 話数 / 漫画: 巻数相当（chapters）
	NextAiringAt  *int64   `json:"nextAiringAt,omitempty"` // unix seconds
}

const searchQuery = `
query ($search: String, $type: MediaType) {
  Page(page: 1, perPage: 24) {
    media(search: $search, type: $type, sort: SEARCH_MATCH) {
      id
      title { romaji native }
      coverImage { medium }
      genres
      episodes
      chapters
      nextAiringEpisode { airingAt }
    }
  }
}
`

type mediaTitle struct {
	Romaji string `json:"romaji"`
	Native string `json:"native"`
}

type mediaCoverImage struct {
	Medium string `json:"medium"`
}

type nextAiringEpisode struct {
	AiringAt int64 `json:"airingAt"`
}

type media struct {
	ID                int               `json:"id"`
	Title             mediaTitle        `json:"title"`
	CoverImage        mediaCoverImage   `json:"coverImage"`
	Genres            []string          `json:"genres"`
	Episodes          *int              `json:"episodes"`
	Chapters          *int              `json:"chapters"`
	NextAiringEpisode *nextAiringEpisode `json:"nextAiringEpisode"`
}

type pageResponse struct {
	Page struct {
		Media []media `json:"media"`
	} `json:"Page"`
}

// Search は作品名で AniList を検索する。mediaType は model.MediaTypeAnime / MediaTypeManga。
func (c *Client) Search(ctx context.Context, query string, mediaType model.MediaType) ([]SearchResult, error) {
	if !mediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}

	var resp pageResponse
	err := c.do(ctx, searchQuery, map[string]any{
		"search": query,
		"type":   strings.ToUpper(string(mediaType)),
	}, &resp)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(resp.Page.Media))
	for _, m := range resp.Page.Media {
		title := m.Title.Romaji
		if title == "" {
			title = m.Title.Native
		}

		total := m.Episodes
		if mediaType == model.MediaTypeManga {
			total = m.Chapters
		}

		result := SearchResult{
			AniListID:     int64(m.ID),
			Title:         title,
			CoverImageURL: m.CoverImage.Medium,
			Genres:        m.Genres,
			Total:         total,
		}
		if m.NextAiringEpisode != nil {
			at := m.NextAiringEpisode.AiringAt
			result.NextAiringAt = &at
		}
		results = append(results, result)
	}
	return results, nil
}
