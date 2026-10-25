package anilist

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// AniList の description は asHtml:false でも <br> や <i> などのタグが残ることがあるため取り除く。
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// SearchResult は検索結果として画面に返す1作品分の情報。
type SearchResult struct {
	AniListID     int64    `json:"anilistId"`
	Title         string   `json:"title"` // 日本作品: native（漢字/かな）優先。それ以外（KR/CN等）は english 優先。
	CoverImageURL string   `json:"coverImageUrl"`
	Genres        []string `json:"genres"`
	Total         *int     `json:"total"`                  // アニメ: 話数（episodes）/ 漫画: 巻数（volumes）。進捗の追跡単位。
	Volumes       *int     `json:"volumes,omitempty"`      // 漫画の既刊巻数（AniList volumes）。Total と同じ値だが、漫画であることが自明な文脈での表示に使う。
	Score         *int     `json:"score,omitempty"`        // AniListのaverageScore（0-100）
	Synopsis      string   `json:"synopsis,omitempty"`     // あらすじ（HTMLタグ除去済み）
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
      synonyms
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
	Synonyms          []string           `json:"synonyms"`
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

// seasonQuery はシーズン画面（20件×2ページ = 40件）とホームの今季アニメ関連の棚の両方で使う。
// perPage を40にしているのは、シーズン画面の2ページぶんをこの1リクエストで賄い、
// 同時にホームの「今週放送の人気アニメ TOP10」が放送予定で絞り込んでも10件を確保できる
// 母集団を作るため。AniList の Page.perPage 上限は50なので、40は1リクエストに収まる。
const seasonQuery = `
query ($season: MediaSeason, $year: Int) {
  Page(page: 1, perPage: 40) {
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

// relationsQuery は作品モーダルの「関連作品」セクション向けに、続編・前日譚・スピンオフ等の
// 直接の関連作品を取得する。
const relationsQuery = `
query ($id: Int) {
  Media(id: $id) {
    relations {
      edges {
        relationType(version: 2)
        node {
          id
          idMal
          type
          title { romaji native english }
          synonyms
          coverImage { medium }
          countryOfOrigin
        }
      }
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

// RelatedWork は作品モーダルの「関連作品」セクション向けの1件分の情報。
type RelatedWork struct {
	AniListID     int64  `json:"anilistId"`
	Title         string `json:"title"`
	CoverImageURL string `json:"coverImageUrl"`
	MediaType     string `json:"mediaType"`    // "anime" | "manga"
	RelationType  string `json:"relationType"` // SEQUEL/PREQUEL/SIDE_STORY/SPIN_OFF/ADAPTATION 等（AniListの生の値）
}

type relationNode struct {
	ID              int             `json:"id"`
	IDMal           *int            `json:"idMal"`
	Type            string          `json:"type"`
	Title           mediaTitle      `json:"title"`
	Synonyms        []string        `json:"synonyms"`
	CoverImage      mediaCoverImage `json:"coverImage"`
	CountryOfOrigin string          `json:"countryOfOrigin"`
}

type relationEdge struct {
	RelationType string       `json:"relationType"`
	Node         relationNode `json:"node"`
}

type mediaRelationsResponse struct {
	Media struct {
		Relations struct {
			Edges []relationEdge `json:"edges"`
		} `json:"relations"`
	} `json:"Media"`
}

// Relations は作品モーダルの「関連作品」セクション向けに、id直下の続編・前日譚・スピンオフ等を取得する。
func (c *Client) Relations(ctx context.Context, id int64) ([]RelatedWork, error) {
	var resp mediaRelationsResponse
	if err := c.do(ctx, relationsQuery, map[string]any{"id": int(id)}, &resp); err != nil {
		return nil, err
	}

	works := make([]RelatedWork, 0, len(resp.Media.Relations.Edges))
	for _, edge := range resp.Media.Relations.Edges {
		node := edge.Node
		works = append(works, RelatedWork{
			AniListID:     int64(node.ID),
			Title:         displayTitle(node.Title, node.Synonyms, node.CountryOfOrigin),
			CoverImageURL: node.CoverImage.Medium,
			MediaType:     strings.ToLower(node.Type),
			RelationType:  edge.RelationType,
		})
	}
	return works, nil
}

// displayTitle は日本語 UI に出す作品名を選ぶ。
//
// native は原語表記のため、日本作品なら漢字/かな、韓国・中国作品ならハングル/簡体字になる。
// ハングル/簡体字は日本語 UI では判読できないので、日本作品のときだけ native を優先する。
//
// 韓国・中国作品は、別名（synonyms）に日本語版のタイトルがあればそれを使う。AniList には
// 日本語タイトルの欄が無いが、"Solo Leveling" の別名に「俺だけレベルアップな件」が入っている
// ように、日本で出版・放送された作品は別名に載っていることが多い。無ければ広く読める英語を使う。
func displayTitle(t mediaTitle, synonyms []string, countryOfOrigin string) string {
	if countryOfOrigin == "" || countryOfOrigin == "JP" {
		return firstNonEmpty(t.Native, t.Romaji, t.English)
	}
	return firstNonEmpty(japaneseSynonym(synonyms), t.English, t.Romaji, t.Native)
}

// japaneseSynonym は別名のうち、かなを含む最初のものを返す。
//
// かなを条件にするのは、漢字だけの別名は中国語（"神之塔" など）と見分けられないため。
// 別名は AniList の利用者が登録する値で並び順に意味は無いが、日本語の別名が複数あることは
// まれなので最初のものを採る。
func japaneseSynonym(synonyms []string) string {
	for _, s := range synonyms {
		for _, r := range s {
			if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
				return s
			}
		}
	}
	return ""
}

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

func toSearchResults(mediaType domain.MediaType, list []media) []SearchResult {
	results := make([]SearchResult, 0, len(list))
	for _, m := range list {
		title := displayTitle(m.Title, m.Synonyms, m.CountryOfOrigin)
		// 進捗の追跡単位はアニメが話数、漫画が巻数。漫画の chapters は AniList 上の
		// 話数だが、読了記録は巻単位で付けるため total には volumes を入れる。
		total := m.Episodes
		if mediaType == domain.MediaTypeManga {
			total = m.Volumes
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
	return c.SeasonAnimeFor(ctx, season, year)
}

// ValidSeasons はAniListが受け付けるMediaSeason enumの値。
var ValidSeasons = map[string]bool{"WINTER": true, "SPRING": true, "SUMMER": true, "FALL": true}

// SeasonAnimeFor はシーズンブラウジングページ向けに、指定した season/year のアニメを人気順で返す。
func (c *Client) SeasonAnimeFor(ctx context.Context, season string, year int) ([]SearchResult, error) {
	var resp pageResponse
	err := c.do(ctx, seasonQuery, map[string]any{"season": season, "year": year}, &resp)
	if err != nil {
		return nil, err
	}
	return toSearchResults(domain.MediaTypeAnime, resp.Page.Media), nil
}

// TrendingAnime はログイン不要のホーム画面向けに、全体人気ランキング上位を返す。
func (c *Client) TrendingAnime(ctx context.Context) ([]SearchResult, error) {
	var resp pageResponse
	if err := c.do(ctx, trendingQuery, map[string]any{"type": "ANIME"}, &resp); err != nil {
		return nil, err
	}
	return toSearchResults(domain.MediaTypeAnime, resp.Page.Media), nil
}

// TrendingManga はホーム画面の「話題の漫画 TOP10」向けに、全体人気ランキング上位を返す。
func (c *Client) TrendingManga(ctx context.Context) ([]SearchResult, error) {
	var resp pageResponse
	if err := c.do(ctx, trendingQuery, map[string]any{"type": "MANGA"}, &resp); err != nil {
		return nil, err
	}
	return toSearchResults(domain.MediaTypeManga, resp.Page.Media), nil
}

// byIDsPageSize は byIDsQuery の perPage と揃える。AniList の Page.perPage 上限が50のため、
// それを超えるID数は複数リクエストに分割しないと51件目以降が黙って欠落する。
const byIDsPageSize = 50

// MediaByIDs は AniList の作品IDリストから最新情報をまとめて取得する。
// マイライブラリ画面で、保存済みレコードの総話数・放送状況・現在の話数を最新化するために使う。
// ids が50件を超える場合は50件ずつに分割して複数回リクエストする。
func (c *Client) MediaByIDs(ctx context.Context, ids []int64, mediaType domain.MediaType) ([]SearchResult, error) {
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
func (c *Client) ByGenres(ctx context.Context, genres []string, mediaType domain.MediaType) ([]SearchResult, error) {
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

// Search は作品名で AniList を検索する。mediaType は domain.MediaTypeAnime / MediaTypeManga。
func (c *Client) Search(ctx context.Context, query string, mediaType domain.MediaType) ([]SearchResult, error) {
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

// taxonomyQuery はジャンルとタグの一覧を 1 リクエストでまとめて取る。どちらも自然文検索・AI おすすめで
// 構造化出力の enum に使う（BACKLOG 9-b）。別々に取ると AniList の枠を 2 本使う。
const taxonomyQuery = `
query {
  GenreCollection
  MediaTagCollection {
    name
    description
    category
    isAdult
    isGeneralSpoiler
  }
}
`

// MediaTag は AniList のタグ 1 件。
type MediaTag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"` // "Theme-Fantasy" など。AI に渡すときの並べ替えや絞り込みに使える
	// IsGeneralSpoiler は、タグ自体がネタバレになるもの（"Time Loop" が終盤で明かされる作品など）。
	// 検索の条件に使うのは問題ないが、理由の文に書くとネタバレになる。
	IsGeneralSpoiler bool `json:"isGeneralSpoiler"`
}

// Taxonomy は AniList が受け付けるジャンル名とタグ名の一覧。
type Taxonomy struct {
	Genres []string
	Tags   []MediaTag
}

type taxonomyResponse struct {
	GenreCollection    []string `json:"GenreCollection"`
	MediaTagCollection []struct {
		MediaTag
		IsAdult bool `json:"isAdult"`
	} `json:"MediaTagCollection"`
}

// adultGenre は GenreCollection に含まれる成人向けジャンル。他の検索と同じく isAdult:false で
// 引くので、enum に入れても 0 件にしかならない。
const adultGenre = "Hentai"

// Taxonomy はジャンルとタグの一覧を返す。成人向けのジャンル・タグは除く。
//
// 一覧はめったに変わらないので、呼び出し側（usecase/taxonomy）で長い TTL でキャッシュする前提。
func (c *Client) Taxonomy(ctx context.Context) (Taxonomy, error) {
	var resp taxonomyResponse
	if err := c.do(ctx, taxonomyQuery, nil, &resp); err != nil {
		return Taxonomy{}, err
	}

	t := Taxonomy{
		Genres: make([]string, 0, len(resp.GenreCollection)),
		Tags:   make([]MediaTag, 0, len(resp.MediaTagCollection)),
	}
	for _, g := range resp.GenreCollection {
		if g != adultGenre {
			t.Genres = append(t.Genres, g)
		}
	}
	for _, tag := range resp.MediaTagCollection {
		if !tag.IsAdult {
			t.Tags = append(t.Tags, tag.MediaTag)
		}
	}
	return t, nil
}

// filterQuery はジャンル・タグ・除外ジャンル・除外タグ・キーワードで絞り込む。どの条件も
// 空なら AniList に null が渡り、その条件は無視される（omitEmpty を参照）。
//
// minimumTagRank は、作品に付いたタグの「どれだけ当てはまるか」（0〜100、AniList の利用者の投票）の
// 下限。下限が無いと、料理が 1 話だけ出てくる作品まで "Cooking" で引っかかる。60 は
// 根拠のある値ではない仮の目安で、実際の検索結果では確かめていない。
//
// genre_in / tag_in に複数の値を渡したとき「すべてを含む」か「どれかを含む」かは実機で確かめていない
// （byGenresQuery のコメントは「いずれか」としている）。9-b の着手時に確認すること。
const filterQuery = `
query ($type: MediaType, $search: String, $genres: [String], $tags: [String], $excludedGenres: [String], $excludedTags: [String], $minimumTagRank: Int, $perPage: Int) {
  Page(page: 1, perPage: $perPage) {
    media(type: $type, search: $search, genre_in: $genres, tag_in: $tags, genre_not_in: $excludedGenres, tag_not_in: $excludedTags, minimumTagRank: $minimumTagRank, sort: [POPULARITY_DESC], isAdult: false) {` + mediaFields + `
    }
  }
}
`

const (
	filterMinimumTagRank = 60
	// filterPerPage は byGenresQuery と同じ件数。AI おすすめの候補もこの中から並べ替える
	filterPerPage = 30
)

// Filter は SearchByFilter の条件。MediaType 以外はすべて省略できる。
//
// ジャンル名・タグ名は Taxonomy が返したものだけを渡すこと。AniList は知らないジャンル名でも
// エラーにせず 0 件を返すので、間違いに気付けない。
type Filter struct {
	MediaType      domain.MediaType
	Query          string // 作品名などのキーワード。AniList の search にそのまま渡る
	Genres         []string
	Tags           []string
	ExcludedGenres []string
	ExcludedTags   []string
}

// SearchByFilter は自然文検索・AI おすすめ向けに、Filter の条件で AniList を人気順に検索する。
func (c *Client) SearchByFilter(ctx context.Context, f Filter) ([]SearchResult, error) {
	if !f.MediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", f.MediaType)
	}

	var resp pageResponse
	err := c.do(ctx, filterQuery, filterVariables(f), &resp)
	if err != nil {
		return nil, err
	}
	return toSearchResults(f.MediaType, resp.Page.Media), nil
}

func filterVariables(f Filter) map[string]any {
	vars := map[string]any{
		"type":    strings.ToUpper(string(f.MediaType)),
		"perPage": filterPerPage,
	}
	// 空の配列や空文字を渡すと、AniList は「何にも当てはまらない」条件として扱いうるので、
	// 空の条件は変数ごと省いて null にする（Search が search:"" で 0 件になるのと同じ事情）
	if q := strings.TrimSpace(f.Query); q != "" {
		vars["search"] = q
	}
	omitEmpty(vars, "genres", f.Genres)
	omitEmpty(vars, "excludedGenres", f.ExcludedGenres)
	omitEmpty(vars, "excludedTags", f.ExcludedTags)
	if len(f.Tags) > 0 {
		vars["tags"] = f.Tags
		// 下限は「このタグで絞る」ときのためのもの。除外タグだけのときにも効くかは確かめていないので、
		// 絞るときだけ付ける
		vars["minimumTagRank"] = filterMinimumTagRank
	}
	return vars
}

func omitEmpty(vars map[string]any, key string, values []string) {
	if len(values) > 0 {
		vars[key] = values
	}
}

// characterFields はキャラクター検索 1 本ぶんのフィールド（自然文検索のキャラクター名、BACKLOG 9-b）。
//
// 2026-10-25 に AniList を直接叩いて確かめたこと:
//   - 漢字の名前の一部ではほぼ見つからない。「薫子」で当たったのは「和栗薫子」だけで、日本語名が
//     登録されている「萌田薫子」「天川薫子」も当たらず、残りの枠には名前と関係の無い人気キャラクター
//     （五条悟・リヴァイなど）が入った。ローマ字の "Kaoruko" なら 8 人とも Kaoruko だった。
//     なので search にはローマ字を渡し、名前が合わない結果は呼び出し側で捨てる前提にしている
//   - 出演作品（media）は type と sort が効き、作品ごとに isAdult と characterRole が取れる。
//     isAdult で絞る引数は無いので、受け取ってから除く（他の検索の isAdult:false と揃えるため）
//   - 名前の候補 3 つぶんのキャラクター検索と作品名の検索をエイリアスで 1 本にまとめても、
//     複雑さの上限には掛からなかった（応答は 125KB ほど）
//
// tags は SearchByFilter と同じ条件（minimumTagRank）でここで絞るために取る。
// キャラクターから引いた作品を、もう一度 AniList でジャンル・タグ検索すると枠を 2 本使うため。
const characterFields = `
    characters(search: %s, sort: [SEARCH_MATCH]) {
      id
      name { full native alternative }
      favourites
      media(type: $type, sort: [POPULARITY_DESC], perPage: $mediaPerPage) {
        edges {
          characterRole
          node {` + mediaFields + `
            isAdult
            tags { name rank }
          }
        }
      }
    }
`

// characterTitleFields は Filter.Query（作品名のキーワード）で作品を引き、出演作品をその作品に絞るための ID を取る。
// perPage は filterPerPage と同じ。続編・劇場版まで含めて同じ作品名の作品はこの中に収まる
const characterTitleFields = `
    media(search: $query, type: $type, isAdult: false) { id }
`

const (
	// characterPerPage は名前が合わない結果を捨てる前提で多めに取る
	characterPerPage = 10
	// characterMediaPerPage は 1 人あたりの出演作品。人気順なので、続編・劇場版が多いキャラクターでも
	// 本編は先頭に来る
	characterMediaPerPage = 10
	// maxCharacterNames は 1 本にまとめる名前の候補の数。確かめたのは 3 つまで
	maxCharacterNames = 3
)

// buildCharacterQuery は名前の候補ごとのキャラクター検索（c0, c1, ...）と、作品名があればその検索（t）を
// エイリアスで 1 本のクエリにまとめる。候補ごとに送ると AniList の枠（24 本/分）を候補の数だけ使う。
func buildCharacterQuery(names int, withTitle bool) string {
	var params, body strings.Builder
	params.WriteString("$type: MediaType, $mediaPerPage: Int")
	for i := range names {
		fmt.Fprintf(&params, ", $n%d: String", i)
		fmt.Fprintf(&body, "  c%d: Page(page: 1, perPage: %d) {", i, characterPerPage)
		fmt.Fprintf(&body, characterFields, fmt.Sprintf("$n%d", i))
		body.WriteString("  }\n")
	}
	if withTitle {
		params.WriteString(", $query: String")
		fmt.Fprintf(&body, "  t: Page(page: 1, perPage: %d) {%s  }\n", filterPerPage, characterTitleFields)
	}
	return "query (" + params.String() + ") {\n" + body.String() + "}\n"
}

// CharacterName は AniList のキャラクター名。呼び出し側が「探した名前と合うか」を判定するために返す。
type CharacterName struct {
	Full        string
	Native      string
	Alternative []string
}

// CharacterMatch はキャラクター 1 人と、Filter の条件を満たした出演作品。
type CharacterMatch struct {
	Name       CharacterName
	Favourites int
	Works      []SearchResult
}

type characterPage struct {
	Characters []struct {
		ID   int `json:"id"`
		Name struct {
			Full        string   `json:"full"`
			Native      string   `json:"native"`
			Alternative []string `json:"alternative"`
		} `json:"name"`
		Favourites int `json:"favourites"`
		Media      struct {
			Edges []struct {
				CharacterRole string `json:"characterRole"`
				Node          struct {
					media
					IsAdult bool `json:"isAdult"`
					Tags    []struct {
						Name string `json:"name"`
						Rank int    `json:"rank"`
					} `json:"tags"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"media"`
	} `json:"characters"`
	Media []struct {
		ID int `json:"id"`
	} `json:"media"`
}

// SearchByCharacter は names（ローマ字や英語の名前の候補。先頭から maxCharacterNames 個）をキャラクター名として探し、
// 出演作品のうち f の条件を満たすものをキャラクターごとに返す。条件の意味は SearchByFilter と同じ
// （ジャンル・タグはすべてを含む、タグは minimumTagRank 以上、除外はどれも含まない、Query は作品名で絞る）。
// 成人向けと、背景（BACKGROUND）としてだけ出ている作品は除く。ちょい役の作品を「このキャラが出る作品」として
// 出さないため。同じキャラクターが複数の候補で当たっても 1 人にまとめる。
//
// AniList はキャラクター名と関係の無い結果も返すので、どのキャラクターを採るかは呼び出し側が
// CharacterMatch.Name で判定すること。
func (c *Client) SearchByCharacter(ctx context.Context, names []string, f Filter) ([]CharacterMatch, error) {
	if !f.MediaType.Valid() {
		return nil, fmt.Errorf("invalid media type: %s", f.MediaType)
	}
	var cleaned []string
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && !slices.Contains(cleaned, n) && len(cleaned) < maxCharacterNames {
			cleaned = append(cleaned, n)
		}
	}
	if len(cleaned) == 0 {
		return []CharacterMatch{}, nil
	}

	title := strings.TrimSpace(f.Query)
	vars := map[string]any{
		"type":         strings.ToUpper(string(f.MediaType)),
		"mediaPerPage": characterMediaPerPage,
	}
	for i, n := range cleaned {
		vars[fmt.Sprintf("n%d", i)] = n
	}
	if title != "" {
		vars["query"] = title
	}

	var resp map[string]characterPage
	if err := c.do(ctx, buildCharacterQuery(len(cleaned), title != ""), vars, &resp); err != nil {
		return nil, err
	}

	// 作品名で絞るときは、その検索で出た作品だけを残す。作品名の検索が 0 件なら、
	// どのキャラクターの作品も条件を満たさない（SearchByFilter で search が 0 件のときと同じ）
	var titleIDs map[int]bool
	if title != "" {
		titleIDs = map[int]bool{}
		for _, m := range resp["t"].Media {
			titleIDs[m.ID] = true
		}
	}

	seen := map[int]bool{}
	matches := []CharacterMatch{}
	for i := range cleaned {
		for _, ch := range resp[fmt.Sprintf("c%d", i)].Characters {
			if seen[ch.ID] {
				continue
			}
			seen[ch.ID] = true

			var works []media
			for _, e := range ch.Media.Edges {
				n := e.Node
				if n.IsAdult || e.CharacterRole == "BACKGROUND" {
					continue
				}
				if titleIDs != nil && !titleIDs[n.ID] {
					continue
				}
				tagRanks := make(map[string]int, len(n.Tags))
				for _, t := range n.Tags {
					tagRanks[t.Name] = t.Rank
				}
				if !matchesFilter(n.Genres, tagRanks, f) {
					continue
				}
				works = append(works, n.media)
			}
			matches = append(matches, CharacterMatch{
				Name:       CharacterName{Full: ch.Name.Full, Native: ch.Name.Native, Alternative: ch.Name.Alternative},
				Favourites: ch.Favourites,
				Works:      toSearchResults(f.MediaType, works),
			})
		}
	}
	return matches, nil
}

// matchesFilter は filterQuery が AniList 側でしている絞り込みを手元で再現する。
func matchesFilter(genres []string, tagRanks map[string]int, f Filter) bool {
	for _, g := range f.Genres {
		if !slices.Contains(genres, g) {
			return false
		}
	}
	for _, t := range f.Tags {
		if tagRanks[t] < filterMinimumTagRank {
			return false
		}
	}
	for _, g := range f.ExcludedGenres {
		if slices.Contains(genres, g) {
			return false
		}
	}
	for _, t := range f.ExcludedTags {
		if _, ok := tagRanks[t]; ok {
			return false
		}
	}
	return true
}

// newSeriesQuery は最近始まった日本の漫画の連載を人気順に取る（「新着」タブの新連載）。
//
// 2026-10-25 に AniList を直接叩いて確かめたこと:
//   - 開始日の新しい順（START_DATE_DESC）にすると、利用者のほぼいない作品ばかりになるので、
//     「開始日が since より後」で絞って人気順にした
//   - 国で絞らないと上位が韓国のウェブトゥーンで埋まり、題名がハングルになる。画面は日本語表示だけに
//     する方針（BACKLOG 6）なので、countryOfOrigin: "JP" に絞った
//   - startDate_greater で絞っても、2021 年開始の作品が 1 件混ざった。AniList 側のデータの都合とみられるので、
//     受け取ってから開始日で絞り直す（NewSeriesManga）
const newSeriesQuery = `
query ($since: FuzzyDateInt, $perPage: Int) {
  Page(page: 1, perPage: $perPage) {
    media(type: MANGA, status: RELEASING, countryOfOrigin: "JP", startDate_greater: $since, sort: [POPULARITY_DESC], isAdult: false) {` + mediaFields + `
      startDate { year month day }
    }
  }
}
`

// newSeriesPerPage は開始日で絞り直して減るぶんを見込み、AniList の Page.perPage 上限（50）まで取る
const newSeriesPerPage = 50

// NewSeries は新連載 1 件。StartedOn は連載の開始日（"2026-10-23"）。
type NewSeries struct {
	SearchResult
	StartedOn string `json:"startedOn"`
}

type newSeriesResponse struct {
	Page struct {
		Media []struct {
			media
			StartDate struct {
				Year  *int `json:"year"`
				Month *int `json:"month"`
				Day   *int `json:"day"`
			} `json:"startDate"`
		} `json:"media"`
	} `json:"Page"`
}

// NewSeriesManga は since の翌日以降に始まった日本の漫画の連載を、人気順に返す。
// 開始日が年月日まで分からない作品は、since より後かを判定できないので除く。
func (c *Client) NewSeriesManga(ctx context.Context, since time.Time) ([]NewSeries, error) {
	var resp newSeriesResponse
	err := c.do(ctx, newSeriesQuery, map[string]any{
		"since":   fuzzyDateInt(since),
		"perPage": newSeriesPerPage,
	}, &resp)
	if err != nil {
		return nil, err
	}

	sinceDay := since.Format(time.DateOnly)
	out := make([]NewSeries, 0, len(resp.Page.Media))
	for _, m := range resp.Page.Media {
		d := m.StartDate
		if d.Year == nil || d.Month == nil || d.Day == nil {
			continue
		}
		startedOn := fmt.Sprintf("%04d-%02d-%02d", *d.Year, *d.Month, *d.Day)
		if startedOn <= sinceDay {
			continue
		}
		out = append(out, NewSeries{
			SearchResult: toSearchResults(domain.MediaTypeManga, []media{m.media})[0],
			StartedOn:    startedOn,
		})
	}
	return out, nil
}

// fuzzyDateInt は AniList の FuzzyDateInt（YYYYMMDD の整数）にする。
func fuzzyDateInt(t time.Time) int {
	return t.Year()*10000 + int(t.Month())*100 + t.Day()
}
