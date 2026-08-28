package anilist

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/model"
)

// AniList の description は asHtml:false でも <br> や <i> などのタグが残ることがあるため取り除く。
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// SearchResult は検索結果として画面に返す1作品分の情報。
type SearchResult struct {
	AniListID     int64    `json:"anilistId"`
	Title         string   `json:"title"` // 日本作品: native（漢字/かな）優先。それ以外（KR/CN等）は english 優先。
	CoverImageURL string   `json:"coverImageUrl"`
	Genres        []string `json:"genres"`
	Total         *int     `json:"total"` // アニメ: 話数 / 漫画: 話数（chapters）。進捗の追跡単位。
	Volumes       *int     `json:"volumes,omitempty"`  // 漫画の既刊巻数（AniList volumes）。参考情報として表示するのみで進捗追跡には使わない。
	Score         *int     `json:"score,omitempty"`    // AniListのaverageScore（0-100）
	Synopsis      string   `json:"synopsis,omitempty"` // あらすじ（HTMLタグ除去済み）
	NextAiringAt  *int64   `json:"nextAiringAt,omitempty"` // unix seconds
	NextEpisode   *int     `json:"nextEpisode,omitempty"`  // 次に放送される話数
	AiringStatus  string   `json:"airingStatus,omitempty"` // AniListのstatus（RELEASING/FINISHED等）
	Popularity    int      `json:"popularity"`             // AniListの人気値。複数の検索結果をマージして並べ替える際に使う
	MalID         *int64   `json:"malId,omitempty"`        // MyAnimeListのID。Jikan APIから追加情報を引くためのキー
}

const mediaFields = `
      id
      idMal
      title { romaji native english }
      coverImage { medium }
      genres
      episodes
      chapters
      volumes
      averageScore
      popularity
      description(asHtml: false)
      status
      countryOfOrigin
      nextAiringEpisode { airingAt episode }
`

const searchQuery = `
query ($search: String, $type: MediaType) {
  Page(page: 1, perPage: 24) {
    media(search: $search, type: $type, sort: [POPULARITY_DESC], isAdult: false) {` + mediaFields + `
    }
  }
}
`

// popularQuery は検索語なしで人気順の一覧を返す（検索欄が空のときのデフォルト表示用）。
const popularQuery = `
query ($type: MediaType) {
  Page(page: 1, perPage: 24) {
    media(type: $type, sort: [POPULARITY_DESC], isAdult: false) {` + mediaFields + `
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
	Episode  int   `json:"episode"`
}

type media struct {
	ID                int                `json:"id"`
	IDMal             *int               `json:"idMal"`
	Title             mediaTitle         `json:"title"`
	CoverImage        mediaCoverImage    `json:"coverImage"`
	Genres            []string           `json:"genres"`
	Episodes          *int               `json:"episodes"`
	Chapters          *int               `json:"chapters"`
	Volumes           *int               `json:"volumes"`
	AverageScore      *int               `json:"averageScore"`
	Popularity        int                `json:"popularity"`
	Description       *string            `json:"description"`
	Status            string             `json:"status"`
	CountryOfOrigin   string             `json:"countryOfOrigin"`
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
    media(season: $season, seasonYear: $year, type: ANIME, sort: POPULARITY_DESC, isAdult: false) {` + mediaFields + `
    }
  }
}
`

const trendingQuery = `
query ($type: MediaType) {
  Page(page: 1, perPage: 10) {
    media(type: $type, sort: POPULARITY_DESC, isAdult: false) {` + mediaFields + `
    }
  }
}
`

// byIDsQuery はライブラリに保存済みの作品を、AniListの最新情報（総話数・放送状況・現在の話数）で
// 再取得するために使う。id_in で複数件をまとめて取得する。
const byIDsQuery = `
query ($ids: [Int], $type: MediaType) {
  Page(page: 1, perPage: 50) {
    media(id_in: $ids, type: $type) {` + mediaFields + `
    }
  }
}
`

// byGenresQuery はおすすめ機能向けに、指定ジャンルのいずれかに合致する作品を人気順で取得する。
const byGenresQuery = `
query ($genres: [String], $type: MediaType) {
  Page(page: 1, perPage: 30) {
    media(genre_in: $genres, type: $type, sort: [POPULARITY_DESC], isAdult: false) {` + mediaFields + `
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

// cleanSynopsis は AniList の description に含まれる改行の連続や前後の空白を整える。
func cleanSynopsis(desc *string) string {
	if desc == nil {
		return ""
	}
	s := htmlTagPattern.ReplaceAllString(*desc, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func toSearchResults(mediaType model.MediaType, list []media) []SearchResult {
	results := make([]SearchResult, 0, len(list))
	for _, m := range list {
		// native は原語表記のため、日本作品なら漢字/かな、韓国・中国作品なら
		// ハングル/簡体字になる。この場合、native (ハングル/簡体字) → 日本語UIでは
		// 判読できないため、日本作品のときだけ native を優先し、それ以外（KR/CN等）は
		// 広く読める英語タイトルを優先する。
		var title string
		if m.CountryOfOrigin == "" || m.CountryOfOrigin == "JP" {
			title = firstNonEmpty(m.Title.Native, m.Title.Romaji, m.Title.English)
		} else {
			title = firstNonEmpty(m.Title.English, m.Title.Romaji, m.Title.Native)
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
			Volumes:       m.Volumes,
			Score:         m.AverageScore,
			Synopsis:      cleanSynopsis(m.Description),
			AiringStatus:  m.Status,
			Popularity:    m.Popularity,
		}
		if m.IDMal != nil {
			malID := int64(*m.IDMal)
			result.MalID = &malID
		}
		if m.NextAiringEpisode != nil {
			at := m.NextAiringEpisode.AiringAt
			result.NextAiringAt = &at
			ep := m.NextAiringEpisode.Episode
			result.NextEpisode = &ep
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
	if err := c.do(ctx, trendingQuery, map[string]any{"type": "ANIME"}, &resp); err != nil {
		return nil, err
	}
	return toSearchResults(model.MediaTypeAnime, resp.Page.Media), nil
}

// TrendingManga はホーム画面の「話題の漫画 TOP10」向けに、全体人気ランキング上位を返す。
func (c *Client) TrendingManga(ctx context.Context) ([]SearchResult, error) {
	var resp pageResponse
	if err := c.do(ctx, trendingQuery, map[string]any{"type": "MANGA"}, &resp); err != nil {
		return nil, err
	}
	return toSearchResults(model.MediaTypeManga, resp.Page.Media), nil
}

// byIDsPageSize は byIDsQuery の perPage と揃える。AniList の Page.perPage 上限が50のため、
// それを超えるID数は複数リクエストに分割しないと51件目以降が黙って欠落する。
const byIDsPageSize = 50

// MediaByIDs は AniList の作品IDリストから最新情報をまとめて取得する。
// マイライブラリ画面で、保存済みレコードの総話数・放送状況・現在の話数を最新化するために使う。
// ids が50件を超える場合は50件ずつに分割して複数回リクエストする。
func (c *Client) MediaByIDs(ctx context.Context, ids []int64, mediaType model.MediaType) ([]SearchResult, error) {
	if !mediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}
	if len(ids) == 0 {
		return []SearchResult{}, nil
	}

	results := make([]SearchResult, 0, len(ids))
	for start := 0; start < len(ids); start += byIDsPageSize {
		end := min(start+byIDsPageSize, len(ids))
		chunk := ids[start:end]

		intIDs := make([]int, len(chunk))
		for i, id := range chunk {
			intIDs[i] = int(id)
		}

		var resp pageResponse
		err := c.do(ctx, byIDsQuery, map[string]any{
			"ids":  intIDs,
			"type": strings.ToUpper(string(mediaType)),
		}, &resp)
		if err != nil {
			return nil, err
		}
		results = append(results, toSearchResults(mediaType, resp.Page.Media)...)
	}

	return results, nil
}

// ByGenres はおすすめ機能向けに、指定ジャンルのいずれかに合致する作品を人気順で取得する。
func (c *Client) ByGenres(ctx context.Context, genres []string, mediaType model.MediaType) ([]SearchResult, error) {
	if !mediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}
	if len(genres) == 0 {
		return []SearchResult{}, nil
	}

	var resp pageResponse
	err := c.do(ctx, byGenresQuery, map[string]any{
		"genres": genres,
		"type":   strings.ToUpper(string(mediaType)),
	}, &resp)
	if err != nil {
		return nil, err
	}

	return toSearchResults(mediaType, resp.Page.Media), nil
}

// Search は作品名で AniList を検索する。mediaType は model.MediaTypeAnime / MediaTypeManga。
func (c *Client) Search(ctx context.Context, query string, mediaType model.MediaType) ([]SearchResult, error) {
	if !mediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", mediaType)
	}

	// 検索語が空のときは popularQuery（検索語なしの人気順一覧）を使う。
	// AniList は search:"" では0件を返すため、素の検索クエリでは空状態を表現できない。
	q := searchQuery
	vars := map[string]any{
		"search": query,
		"type":   strings.ToUpper(string(mediaType)),
	}
	if strings.TrimSpace(query) == "" {
		q = popularQuery
		vars = map[string]any{"type": strings.ToUpper(string(mediaType))}
	}

	var resp pageResponse
	if err := c.do(ctx, q, vars, &resp); err != nil {
		return nil, err
	}

	return toSearchResults(mediaType, resp.Page.Media), nil
}
