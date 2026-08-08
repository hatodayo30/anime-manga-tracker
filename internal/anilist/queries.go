package anilist

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/model"
)

// SearchResult は検索結果として画面に返す1作品分の情報。
type SearchResult struct {
	AniListID     int64    `json:"anilistId"`
	Title         string   `json:"title"`   // 表示優先順位: native → romaji → english
	TitleEn       string   `json:"titleEn"` // 表示優先順位: english → romaji → native
	CoverImageURL string   `json:"coverImageUrl"`
	Genres        []string `json:"genres"`
	Total         *int     `json:"total"` // アニメ: 話数 / 漫画: 巻数相当（chapters）
	NextAiringAt  *int64   `json:"nextAiringAt,omitempty"` // unix seconds
}

const searchQuery = `
query ($search: String, $type: MediaType) {
  Page(page: 1, perPage: 24) {
    media(search: $search, type: $type, sort: [POPULARITY_DESC], isAdult: false) {
      id
      title { romaji native english }
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
	Romaji  string `json:"romaji"`
	Native  string `json:"native"`
	English string `json:"english"`
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

const seasonQuery = `
query ($season: MediaSeason, $year: Int) {
  Page(page: 1, perPage: 12) {
    media(season: $season, seasonYear: $year, type: ANIME, sort: POPULARITY_DESC) {
      id
      title { romaji native english }
      coverImage { medium }
      genres
      episodes
      nextAiringEpisode { airingAt }
    }
  }
}
`

const trendingQuery = `
query {
  Page(page: 1, perPage: 10) {
    media(type: ANIME, sort: POPULARITY_DESC) {
      id
      title { romaji native english }
      coverImage { medium }
      genres
      episodes
    }
  }
}
`

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func toSearchResults(mediaType model.MediaType, list []media) []SearchResult {
	results := make([]SearchResult, 0, len(list))
	for _, m := range list {
		title := firstNonEmpty(m.Title.Native, m.Title.Romaji, m.Title.English)
		titleEn := firstNonEmpty(m.Title.English, m.Title.Romaji, m.Title.Native)

		total := m.Episodes
		if mediaType == model.MediaTypeManga {
			total = m.Chapters
		}

		result := SearchResult{
			AniListID:     int64(m.ID),
			Title:         title,
			TitleEn:       titleEn,
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
	return results
}

// currentSeason は現在の日付から AniList の season/year を計算する。
// 12月は翌年のWINTERシーズンに属する（AniListの慣例）。
func currentSeason(now time.Time) (string, int) {
	month := now.Month()
	year := now.Year()
	switch {
	case month == time.December:
		return "WINTER", year + 1
	case month <= time.February:
		return "WINTER", year
	case month <= time.May:
		return "SPRING", year
	case month <= time.August:
		return "SUMMER", year
	default:
		return "FALL", year
	}
}

// SeasonAnime はログイン不要のホーム画面向けに、今季放送中アニメを人気順で返す。
func (c *Client) SeasonAnime(ctx context.Context) ([]SearchResult, error) {
	season, year := currentSeason(time.Now())

	var resp pageResponse
	err := c.do(ctx, seasonQuery, map[string]any{"season": season, "year": year}, &resp)
	if err != nil {
		return nil, err
	}
	return toSearchResults(model.MediaTypeAnime, resp.Page.Media), nil
}

// TrendingAnime はログイン不要のホーム画面向けに、全体人気ランキング上位を返す。
func (c *Client) TrendingAnime(ctx context.Context) ([]SearchResult, error) {
	var resp pageResponse
	if err := c.do(ctx, trendingQuery, nil, &resp); err != nil {
		return nil, err
	}
	return toSearchResults(model.MediaTypeAnime, resp.Page.Media), nil
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

	return toSearchResults(mediaType, resp.Page.Media), nil
}
