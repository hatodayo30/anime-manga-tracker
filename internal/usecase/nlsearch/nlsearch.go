// Package nlsearch は自然文検索（BACKLOG 9-b）のユースケースを担う。
//
// 「異世界で料理する漫画」のような文を AI に検索条件（ジャンル・タグ・キーワード）へ変換させ、
// その条件で AniList を検索する。AI は条件を作るだけで、画面に出す作品はすべて AniList の
// 検索結果にする（BACKLOG 9「AI はデータ源にしない」）。
//
// 従来の作品名検索（usecase/search）とはパッケージを分けた。9-c が usecase/search の近くで
// 並行して作業しているのと、こちらは AI の失敗時に従来の検索へ落とすだけで、
// 従来の検索の中身には関わらないため。
package nlsearch

import (
	"context"
	"errors"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
)

// Conditions は AI が自然文から作った検索条件。ジャンル名・タグ名は Taxonomy に含まれるものだけ。
type Conditions struct {
	// MediaType は文に「漫画」「アニメ」とはっきり書かれていたときだけ入る。空なら画面で選んでいる種別を使う
	MediaType      domain.MediaType `json:"mediaType,omitempty"`
	Genres         []string         `json:"genres"`
	Tags           []string         `json:"tags"`
	ExcludedGenres []string         `json:"excludedGenres"`
	ExcludedTags   []string         `json:"excludedTags"`
	// Query は文に作品名・作者名などの固有名詞があったときだけ入るキーワード
	Query string `json:"query"`
}

func (c Conditions) empty() bool {
	return len(c.Genres) == 0 && len(c.Tags) == 0 && len(c.ExcludedGenres) == 0 &&
		len(c.ExcludedTags) == 0 && strings.TrimSpace(c.Query) == ""
}

// AIGateway は自然文を検索条件に変換する。実装は internal/claude。
//
// taxonomy を渡すのは、ジャンル・タグを構造化出力の enum で縛るため。存在しないジャンル名を
// AniList に渡してもエラーにならず 0 件になるだけなので、出力できる値そのものを一覧に限る。
type AIGateway interface {
	ParseSearch(ctx context.Context, input string, defaultType domain.MediaType, taxonomy anilist.Taxonomy) (Conditions, error)
}

// TaxonomyProvider はジャンル・タグの一覧。実装は usecase/taxonomy（main.go で 9-c と共有する）。
type TaxonomyProvider interface {
	Get(ctx context.Context) (anilist.Taxonomy, error)
}

// AniListGateway は条件検索。実装は internal/anilist。
type AniListGateway interface {
	SearchByFilter(ctx context.Context, f anilist.Filter) ([]anilist.SearchResult, error)
}

// KeywordSearcher は AI が使えないときに落とす先の従来の作品名検索。実装は usecase/search。
// Jikan のフォールバックなど、従来の検索の振る舞いをそのまま使うため、AniList を直接叩かずに経由する。
type KeywordSearcher interface {
	Search(ctx context.Context, mediaType domain.MediaType, query string) ([]anilist.SearchResult, error)
}

// 条件の数の上限。AniList の genre_in / tag_in に複数渡したとき「すべてを含む」か「どれかを含む」かを
// 確かめていない（BACKLOG 9-b「残っている穴」）。AND だった場合に条件を盛りすぎて 0 件になるのを
// 避けるため、絞り込みに使う条件は少なく切る。構造化出力は maxItems を使えないので、ここで切る。
// 除外は 0 件の原因になりにくいので少し多めに残す。
const (
	maxGenres         = 2
	maxTags           = 3
	maxExcludedGenres = 3
	maxExcludedTags   = 3
)

// conditionsCacheTTL / conditionsCacheMaxEntries は AI が作った条件のキャッシュ。
// 同じ文で検索し直す（Enter の押し直し、種別の切り替えで戻る）たびに課金しないためのもの。
// 結果ではなく条件だけを持つのは、作品の人気順や放送状況は AniList 側で変わるため。
// キーは利用者が入力した文なので際限なく増えうる。上限を設ける（search.recommendCacheMaxEntries と同じ事情）。
const (
	conditionsCacheTTL        = 30 * time.Minute
	conditionsCacheMaxEntries = 256
)

// FallbackReason は AI の条件を使わずに従来の検索へ落とした理由。画面の案内文に使う。
type FallbackReason string

const (
	// FallbackNone は AI の条件で検索できた
	FallbackNone FallbackReason = ""
	// FallbackUnavailable は AI を組み込んでいない（ANTHROPIC_API_KEY が無い）
	FallbackUnavailable FallbackReason = "unavailable"
	// FallbackFailed は AI・タグ一覧の取得・条件検索のどれかが失敗した、または日次上限に達した
	FallbackFailed FallbackReason = "failed"
	// FallbackNoConditions は AI が文から条件を 1 つも作れなかった
	FallbackNoConditions FallbackReason = "no_conditions"
)

// Result は自然文検索の結果。
type Result struct {
	// MediaType は実際に検索した種別。文で種別が指定されていれば、画面で選んでいる種別と違うことがある
	MediaType domain.MediaType `json:"mediaType"`
	// Conditions は AI が作った条件。Fallback が空でないときは nil
	Conditions *Conditions            `json:"conditions"`
	Fallback   FallbackReason         `json:"fallback,omitempty"`
	Items      []anilist.SearchResult `json:"items"`
}

type Usecase struct {
	ai       AIGateway
	taxonomy TaxonomyProvider
	anilist  AniListGateway
	keyword  KeywordSearcher

	conditionsCache *cache.TTLCache[Conditions]
}

// NewUsecase は ai に nil を許す。ANTHROPIC_API_KEY が無い環境では常に従来の検索に落ちる。
func NewUsecase(ai AIGateway, taxonomy TaxonomyProvider, anilistGateway AniListGateway, keyword KeywordSearcher) *Usecase {
	return &Usecase{
		ai:              ai,
		taxonomy:        taxonomy,
		anilist:         anilistGateway,
		keyword:         keyword,
		conditionsCache: cache.NewBounded[Conditions](conditionsCacheTTL, conditionsCacheMaxEntries),
	}
}

// Search は input を AI で検索条件に変換して AniList を検索する。mediaType は画面で選んでいる種別。
//
// ログイン済みの利用者からだけ呼ぶこと（ルーターで RequireUser を被せる）。AI は呼ばれたぶんだけ
// 請求が積み上がる（BACKLOG 9「決めたこと」）。
//
// AI の側の失敗は検索を止めず、input をそのままキーワードにして従来の検索に落とす（CLAUDE.md
// 「失敗の扱い」）。エラーを返すのは、落とした先の従来の検索まで失敗したときだけ。
func (u *Usecase) Search(ctx context.Context, mediaType domain.MediaType, input string) (Result, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		// 空の文で AI を呼んでも条件は作れない。従来の検索の「空なら人気順」に任せる
		return u.fallback(ctx, mediaType, input, FallbackNoConditions)
	}
	if u.ai == nil {
		return u.fallback(ctx, mediaType, input, FallbackUnavailable)
	}

	cond, err := u.conditions(ctx, mediaType, input)
	if err != nil {
		log.Printf("nlsearch: ai failed, falling back to keyword search: %v", err)
		return u.fallback(ctx, mediaType, input, FallbackFailed)
	}
	if cond.empty() {
		return u.fallback(ctx, mediaType, input, FallbackNoConditions)
	}

	searchType := mediaType
	if cond.MediaType.Valid() {
		searchType = cond.MediaType
	}
	items, err := u.anilist.SearchByFilter(ctx, anilist.Filter{
		MediaType:      searchType,
		Query:          cond.Query,
		Genres:         cond.Genres,
		Tags:           cond.Tags,
		ExcludedGenres: cond.ExcludedGenres,
		ExcludedTags:   cond.ExcludedTags,
	})
	if err != nil {
		// 条件検索が失敗するなら従来の検索も AniList で失敗しやすいが、Jikan のフォールバックが
		// 残っているので試す価値はある
		log.Printf("nlsearch: filter search failed, falling back to keyword search: %v", err)
		return u.fallback(ctx, mediaType, input, FallbackFailed)
	}
	if items == nil {
		items = []anilist.SearchResult{}
	}
	return Result{MediaType: searchType, Conditions: &cond, Items: items}, nil
}

// conditions は input の検索条件をキャッシュから返し、無ければ AI に作らせる。
// 失敗はキャッシュしない（cache.TTLCache の仕様）ので、AI が戻れば次の検索から使える。
func (u *Usecase) conditions(ctx context.Context, mediaType domain.MediaType, input string) (Conditions, error) {
	key := string(mediaType) + "|" + input
	return u.conditionsCache.Get(ctx, key, func(ctx context.Context) (Conditions, error) {
		tax, err := u.taxonomy.Get(ctx)
		if err != nil {
			return Conditions{}, err
		}
		if len(tax.Genres) == 0 || len(tax.Tags) == 0 {
			// 空の enum はスキーマとして不正で、どのみち API が 400 を返す
			return Conditions{}, errors.New("nlsearch: taxonomy is empty")
		}
		cond, err := u.ai.ParseSearch(ctx, input, mediaType, tax)
		if err != nil {
			return Conditions{}, err
		}
		return sanitize(cond, tax), nil
	})
}

// sanitize は AI の条件から一覧に無い値・重複・上限を超えた分を落とす。
//
// enum で縛っているので一覧に無い値は出ないはずだが、AniList は知らないジャンル名でも
// エラーにせず 0 件を返すので、間違いに気付けるよう二重に確かめる。絞り込みと除外の両方に
// 同じ値があると必ず 0 件になるので、除外のほうから落とす。
func sanitize(c Conditions, tax anilist.Taxonomy) Conditions {
	genres := make(map[string]bool, len(tax.Genres))
	for _, g := range tax.Genres {
		genres[g] = true
	}
	tags := make(map[string]bool, len(tax.Tags))
	for _, t := range tax.Tags {
		tags[t.Name] = true
	}

	out := Conditions{Query: strings.TrimSpace(c.Query)}
	if c.MediaType.Valid() {
		out.MediaType = c.MediaType
	}
	out.Genres = pick(c.Genres, genres, nil, maxGenres)
	out.Tags = pick(c.Tags, tags, nil, maxTags)
	out.ExcludedGenres = pick(c.ExcludedGenres, genres, out.Genres, maxExcludedGenres)
	out.ExcludedTags = pick(c.ExcludedTags, tags, out.Tags, maxExcludedTags)
	return out
}

func pick(values []string, known map[string]bool, conflicts []string, limit int) []string {
	out := []string{}
	for _, v := range values {
		if len(out) == limit {
			break
		}
		if !known[v] || slices.Contains(out, v) || slices.Contains(conflicts, v) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// fallback は input をそのままキーワードにして従来の検索を行う。
func (u *Usecase) fallback(ctx context.Context, mediaType domain.MediaType, input string, reason FallbackReason) (Result, error) {
	items, err := u.keyword.Search(ctx, mediaType, input)
	if err != nil {
		return Result{}, err
	}
	if items == nil {
		items = []anilist.SearchResult{}
	}
	return Result{MediaType: mediaType, Fallback: reason, Items: items}, nil
}
