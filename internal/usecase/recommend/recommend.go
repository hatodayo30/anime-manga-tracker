// Package recommend はホームの「AI のおすすめ」（BACKLOG 9-c）を担う。
//
// 3 段で作る。AI に作品名を挙げさせないのは、知識が古く今期の新作が出ない・存在しない作品を挙げる・
// 挙がった作品を 1 件ずつ AniList で確かめると AniList の枠を数人で使い切るため。
//  1. 記録から好みを検索条件（ジャンル・タグ）に変換する（AI）
//  2. その条件で AniList から候補を取る
//  3. 候補を並べ替え、作品ごとに一言の理由を付ける（AI）
//
// 表示する作品はすべて手順 2 の検索結果なので、捏造は混ざらない。
//
// usecase/search の Recommendations（ジャンルベースのおすすめ）とは別のパッケージにした。
// あちらはログイン不要でクライアントがジャンルを決める API で、9-b が同じパッケージを並行して触っている。
package recommend

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
)

// Source は結果をどちらの経路で作ったか。フロントは "genre" のとき理由の欄を出さない。
const (
	SourceAI    = "ai"
	SourceGenre = "genre"
)

const (
	// minTasteRecords は AI を使うのに要る「見た・見てる」の記録の数。これより少ないと好みを
	// 推し量る材料が無く、AI 2 回ぶんの費用をかけても人気作が並ぶだけになる。
	minTasteRecords = 3

	// maxTasteRecords は AI に渡す記録の数の上限（最近更新した順）。入力トークン（＝費用）の上限で、
	// 好みを推し量るには直近の 50 件で足りる。
	maxTasteRecords = 50

	// maxMemoRunes はメモを AI に渡すときの長さの上限。メモは利用者が自由に書くので長さに上限が無い。
	maxMemoRunes = 200

	// maxPreferences は好み・避けたいもののそれぞれで使う数の上限。構造化出力では配列の長さを
	// 縛れない（maxItems は使えない）ので、ここで切る。
	maxPreferences = 5

	// maxFilterQueries は手順 2 で AniList に投げる本数。BACKLOG 9-c の「AniList 1〜2 本」。
	maxFilterQueries = 2

	// maxCandidates は手順 3 で AI に渡す候補の数。1 本 30 件 × 2 本から記録済みを除いた残りを
	// 入力トークンの都合でここまでに絞る。
	maxCandidates = 40

	// maxItems は棚に並べる数。
	maxItems = 10

	// maxReasonRunes は理由の文の長さの上限。プロンプトでは 40 字程度と頼んでいるが、構造化出力では
	// 文字列の長さを縛れないので、カードからあふれる長さはここで切る。
	maxReasonRunes = 80

	// fallbackGenres はジャンルベースのおすすめで使うジャンルの数。ホームの「あなたへのおすすめ」
	// （フロントの RecommendPreviewShelf）と同じく、よく見ているジャンルの上位 3 つ。
	fallbackGenres = 3
)

const (
	// aiCacheTTL は AI のおすすめを持っておく時間。キーに JST の日付を含めるので、実際に作り直すのは
	// 日付が変わったとき（BACKLOG 9-c の「1 人 1 日 1 回」）。TTL はその日のうちに消えない長さにしてある。
	aiCacheTTL = 24 * time.Hour

	// aiCacheMaxEntries はキー（利用者 × 種別 × 日付）の数の上限。1 件は作品 10 件ぶん（あらすじ込みで
	// 十数 KB）なので、512 件で 10MB 弱。上限を超えると古い順に捨て、捨てられた利用者は
	// その日のうちにもう一度作り直す（BACKLOG 9-c「残っている穴」）。
	aiCacheMaxEntries = 512

	// failureCooldown は AI の経路が失敗した利用者に、AI をもう一度試させるまでの時間。
	// 失敗はキャッシュに残らないので、何もしないとホームを開くたびに AI を呼び直す。refusal や
	// max_tokens で切れた応答にも課金されるので、同じ失敗を繰り返して請求を積まないようにする。
	failureCooldown = 30 * time.Minute

	// generateTimeout は 3 段をまとめて待つ上限。AI 1 回は最悪 40 秒（internal/claude の
	// 20 秒 × 再試行 1 回）かかりうるので、上限を置かないとサーバーの WriteTimeout（60 秒）を超え、
	// ジャンルベースへのフォールバックも返せない。ジャンルベースの AniList 1 本ぶんを残して 40 秒。
	generateTimeout = 40 * time.Second

	// fallbackCacheTTL / fallbackCacheMaxEntries は usecase/search のおすすめキャッシュと同じ値。
	// キーは種別とジャンルの組で、AI のキャッシュと違って利用者をまたいで共有できる。
	fallbackCacheTTL        = 3 * time.Minute
	fallbackCacheMaxEntries = 128
)

// jst は「1 日」の区切り。internal/claude の日次上限と同じく、本番イメージに tzdata が無いので固定オフセット。
var jst = time.FixedZone("JST", 9*60*60)

// Item は棚の 1 件。SearchResult を埋め込み、フロントには作品の項目と同じ階層で reason を返す。
type Item struct {
	anilist.SearchResult
	Reason string `json:"reason,omitempty"`
}

type Result struct {
	Source string `json:"source"`
	Items  []Item `json:"items"`
}

type Usecase struct {
	records  RecordRepository
	taxonomy TaxonomyProvider
	anilist  AniListGateway
	ai       AIGateway
	now      func() time.Time

	aiCache       *cache.TTLCache[[]Item]
	fallbackCache *cache.TTLCache[[]anilist.SearchResult]

	mu       sync.Mutex
	failures map[string]time.Time // キーは aiCache と同じ。値は失敗した時刻
}

// NewUsecase は ai に nil を許す。ANTHROPIC_API_KEY が無い環境では常にジャンルベースで返す。
// now は日付の区切りと失敗の待ち時間をテストで差し替えるためのもので、本番では time.Now を渡す。
func NewUsecase(records RecordRepository, taxonomy TaxonomyProvider, anilistGateway AniListGateway, ai AIGateway, now func() time.Time) *Usecase {
	return &Usecase{
		records:       records,
		taxonomy:      taxonomy,
		anilist:       anilistGateway,
		ai:            ai,
		now:           now,
		aiCache:       cache.NewBounded[[]Item](aiCacheTTL, aiCacheMaxEntries),
		fallbackCache: cache.NewBounded[[]anilist.SearchResult](fallbackCacheTTL, fallbackCacheMaxEntries),
		failures:      make(map[string]time.Time),
	}
}

// For は userID の利用者に向けたおすすめを返す。
//
// 記録が少ない・AI を使えない・AI の経路のどこかが失敗した、のいずれでもジャンルベースのおすすめを返す
// （CLAUDE.md「失敗の扱い」）。エラーを返すのは記録の読み出しか、ジャンルベースの取得まで失敗したときだけ。
func (u *Usecase) For(ctx context.Context, userID int64, mediaType domain.MediaType) (Result, error) {
	records, err := u.records.List(ctx, userID, mediaType, "", domain.SortUpdated)
	if err != nil {
		return Result{}, err
	}
	recorded := make(map[int64]bool, len(records))
	for _, r := range records {
		recorded[r.AniListID] = true
	}

	if u.ai != nil && countTasteRecords(records) >= minTasteRecords {
		key := u.cacheKey(userID, mediaType)
		if !u.inCooldown(key) {
			items, err := u.aiCache.Get(ctx, key, func(ctx context.Context) ([]Item, error) {
				// 利用者がページを離れても、払った AI の費用を無駄にしないよう最後まで作ってキャッシュに入れる
				ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), generateTimeout)
				defer cancel()
				return u.generate(ctx, mediaType, records, recorded)
			})
			if err == nil {
				// 作った後に記録した作品は、翌日の作り直しを待たずに棚から外す
				return Result{Source: SourceAI, Items: withoutRecorded(items, recorded)}, nil
			}
			u.recordFailure(key)
			log.Printf("recommend: ai recommendations failed, falling back to genres: %v", err)
		}
	}

	return u.byGenres(ctx, mediaType, records, recorded)
}

// generate は 3 段を通しで実行する。どこかで失敗したらエラーを返し、呼び出し側がジャンルベースに落とす。
func (u *Usecase) generate(ctx context.Context, mediaType domain.MediaType, records []*domain.Record, recorded map[int64]bool) ([]Item, error) {
	tax, err := u.taxonomy.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("taxonomy: %w", err)
	}
	if len(tax.Genres) == 0 {
		return nil, errors.New("taxonomy has no genres")
	}
	tastes := toTastes(records)

	// 1. 好みを検索条件に変換する
	prefs, err := u.ai.TastePreferences(ctx, PreferenceInput{
		MediaType: mediaType,
		Records:   tastes,
		Genres:    tax.Genres,
		Tags:      searchableTags(tax.Tags),
	})
	if err != nil {
		return nil, fmt.Errorf("preferences: %w", err)
	}
	prefs = sanitizePreferences(prefs, tax)

	// 2. 候補を取る
	candidates := u.candidates(ctx, mediaType, prefs, recorded)
	if len(candidates) == 0 {
		return nil, errors.New("no candidates")
	}

	// 3. 並べ替えと理由
	picks, err := u.ai.RankRecommendations(ctx, RankInput{MediaType: mediaType, Records: tastes, Candidates: candidates})
	if err != nil {
		return nil, fmt.Errorf("rank: %w", err)
	}
	items := toItems(picks, candidates)
	if len(items) == 0 {
		return nil, errors.New("no valid picks")
	}
	return items, nil
}

// candidates は好みの条件で AniList から候補を取り、記録済みを除いて交互に並べる。
//
// 1 本の検索に渡すジャンル・タグは 1 つずつにしている。genre_in / tag_in に複数渡したとき
// 「すべてを含む」か「どれかを含む」かを確かめていないため（BACKLOG 9「残っている穴」）。
// 「すべて」だと AI が条件を盛ったときに 0 件になる。1 つずつなら、どちらの意味でも同じ結果になる。
// その代わり好みの上位の 1〜2 個しか検索に使わない。並べ替えの段で記録全体を見て選び直すので、
// 候補の幅としてはこれで足りる、という判断。
//
// 1 本が失敗してももう 1 本の結果で進める（「失敗の扱い」）。
func (u *Usecase) candidates(ctx context.Context, mediaType domain.MediaType, prefs Preferences, recorded map[int64]bool) []anilist.SearchResult {
	var lists [][]anilist.SearchResult
	for _, f := range filtersFor(mediaType, prefs) {
		results, err := u.anilist.SearchByFilter(ctx, f)
		if err != nil {
			log.Printf("recommend: search by filter: %v", err)
			continue
		}
		lists = append(lists, results)
	}

	// 片方の検索結果だけで maxCandidates が埋まらないよう、交互に取る
	var merged []anilist.SearchResult
	seen := make(map[int64]bool)
	for i := 0; len(merged) < maxCandidates; i++ {
		progressed := false
		for _, list := range lists {
			if i >= len(list) {
				continue
			}
			progressed = true
			r := list[i]
			if recorded[r.AniListID] || seen[r.AniListID] || len(merged) >= maxCandidates {
				continue
			}
			seen[r.AniListID] = true
			merged = append(merged, r)
		}
		if !progressed {
			break
		}
	}
	return merged
}

// filtersFor は好みの上位から、ジャンル・タグを 1 つずつ持つ検索条件を最大 maxFilterQueries 本作る。
// タグを先にするのは、ジャンル（20 種類ほど）よりタグのほうが好みを細かく表せるため。
func filtersFor(mediaType domain.MediaType, prefs Preferences) []anilist.Filter {
	base := anilist.Filter{
		MediaType:      mediaType,
		ExcludedGenres: prefs.AvoidGenres,
		ExcludedTags:   prefs.AvoidTags,
	}
	var filters []anilist.Filter
	for i := 0; len(filters) < maxFilterQueries && (i < len(prefs.Tags) || i < len(prefs.Genres)); i++ {
		if i < len(prefs.Tags) && len(filters) < maxFilterQueries {
			f := base
			f.Tags = []string{prefs.Tags[i]}
			filters = append(filters, f)
		}
		if i < len(prefs.Genres) && len(filters) < maxFilterQueries {
			f := base
			f.Genres = []string{prefs.Genres[i]}
			filters = append(filters, f)
		}
	}
	return filters
}

// byGenres はジャンルベースのおすすめ。よく見ているジャンルのいずれかに当てはまる作品を人気順で返す。
func (u *Usecase) byGenres(ctx context.Context, mediaType domain.MediaType, records []*domain.Record, recorded map[int64]bool) (Result, error) {
	genres := topGenres(records, fallbackGenres)
	if len(genres) == 0 {
		return Result{Source: SourceGenre, Items: []Item{}}, nil
	}

	key := string(mediaType) + "|" + strings.Join(genres, ",")
	results, err := u.fallbackCache.Get(ctx, key, func(ctx context.Context) ([]anilist.SearchResult, error) {
		return u.anilist.ByGenres(ctx, genres, mediaType)
	})
	if err != nil {
		return Result{}, err
	}

	items := make([]Item, 0, maxItems)
	for _, r := range results {
		if recorded[r.AniListID] {
			continue
		}
		items = append(items, Item{SearchResult: r})
		if len(items) == maxItems {
			break
		}
	}
	return Result{Source: SourceGenre, Items: items}, nil
}

// cacheKey は利用者 × 種別 × JST の日付。日付を含めることで、TTL の残りに関係なく日付が変わったら作り直す。
func (u *Usecase) cacheKey(userID int64, mediaType domain.MediaType) string {
	return strconv.FormatInt(userID, 10) + "|" + string(mediaType) + "|" + u.now().In(jst).Format(time.DateOnly)
}

func (u *Usecase) inCooldown(key string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	failedAt, ok := u.failures[key]
	return ok && u.now().Sub(failedAt) < failureCooldown
}

// recordFailure は失敗した時刻を残す。ついでに待ち時間を過ぎたものを捨て、map が
// 「直近 failureCooldown に失敗した利用者の数」より大きくならないようにする。
func (u *Usecase) recordFailure(key string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	now := u.now()
	for k, t := range u.failures {
		if now.Sub(t) >= failureCooldown {
			delete(u.failures, k)
		}
	}
	u.failures[key] = now
}

// countTasteRecords は好みの材料になる記録（見た・見てる）の数。「見たい」は見ていないので数えない。
func countTasteRecords(records []*domain.Record) int {
	n := 0
	for _, r := range records {
		if r.Status == domain.StatusDone || r.Status == domain.StatusActive {
			n++
		}
	}
	return n
}

// toTastes は AI に渡す形に変換する。「見たい」も興味の印として渡す（プロンプトで区別させる）。
// records は最近更新した順なので、先頭から maxTasteRecords 件を使う。
func toTastes(records []*domain.Record) []Taste {
	n := min(len(records), maxTasteRecords)
	tastes := make([]Taste, 0, n)
	for _, r := range records[:n] {
		tastes = append(tastes, Taste{
			Title:  r.Title,
			Status: r.Status,
			Rating: r.Rating,
			Genres: r.Genres,
			Memo:   truncateRunes(strings.TrimSpace(r.Memo), maxMemoRunes),
		})
	}
	return tastes
}

// searchableTags は AI に選ばせるタグ名。ネタバレになるタグ（isGeneralSpoiler）は除く。
//
// ネタバレのタグも検索の条件としては使えるが、理由の文に書かれるとネタバレになる。
// 除いておけば enum も小さくなる。タグ全部（400 件前後）の enum が構造化出力の制限に収まるかは
// 確かめていない（BACKLOG 9「残っている穴」）ので、小さくできるところは小さくしておく。
// 収まらなければ手順 1 が失敗し、ジャンルベースに落ちるだけで済む。
func searchableTags(tags []anilist.MediaTag) []string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		if !t.IsGeneralSpoiler {
			names = append(names, t.Name)
		}
	}
	return names
}

// sanitizePreferences は AI の答えから、一覧に無い名前・重複・「好き」と「避けたい」の両方に入った名前を除き、
// 数を maxPreferences までに切る。
//
// 構造化出力の enum で一覧の外の名前は出ないはずだが、AniList は知らないジャンル名でも 0 件を返すだけで
// 気付けないので（anilist.Filter）、usecase でも確かめる。両方に入った名前は、そのまま渡すと
// 「含む」と「含まない」が打ち消して 0 件になるので、どちらからも外す。
func sanitizePreferences(p Preferences, tax anilist.Taxonomy) Preferences {
	genres := make(map[string]bool, len(tax.Genres))
	for _, g := range tax.Genres {
		genres[g] = true
	}
	tags := make(map[string]bool, len(tax.Tags))
	for _, t := range tax.Tags {
		tags[t.Name] = true
	}

	likedGenres := filterNames(p.Genres, genres)
	avoidGenres := filterNames(p.AvoidGenres, genres)
	likedTags := filterNames(p.Tags, tags)
	avoidTags := filterNames(p.AvoidTags, tags)

	return Preferences{
		Genres:      firstN(withoutAny(likedGenres, avoidGenres), maxPreferences),
		AvoidGenres: firstN(withoutAny(avoidGenres, likedGenres), maxPreferences),
		Tags:        firstN(withoutAny(likedTags, avoidTags), maxPreferences),
		AvoidTags:   firstN(withoutAny(avoidTags, likedTags), maxPreferences),
	}
}

func filterNames(names []string, allowed map[string]bool) []string {
	var out []string
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if allowed[n] && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func withoutAny(names, remove []string) []string {
	var out []string
	for _, n := range names {
		if !slices.Contains(remove, n) {
			out = append(out, n)
		}
	}
	return out
}

func firstN(names []string, n int) []string {
	if len(names) > n {
		return names[:n]
	}
	return names
}

// toItems は AI が選んだ順に候補を並べる。候補に無い ID・重複・理由が空のものは捨てる。
// 理由が空のものを捨てるのは、この棚が「理由付き」であることを売りにしているため。
func toItems(picks []Pick, candidates []anilist.SearchResult) []Item {
	byID := make(map[int64]anilist.SearchResult, len(candidates))
	for _, c := range candidates {
		byID[c.AniListID] = c
	}

	items := make([]Item, 0, maxItems)
	used := make(map[int64]bool, len(picks))
	for _, p := range picks {
		c, ok := byID[p.AniListID]
		reason := strings.TrimSpace(p.Reason)
		if !ok || used[p.AniListID] || reason == "" {
			continue
		}
		used[p.AniListID] = true
		items = append(items, Item{SearchResult: c, Reason: truncateRunes(reason, maxReasonRunes)})
		if len(items) == maxItems {
			break
		}
	}
	return items
}

func withoutRecorded(items []Item, recorded map[int64]bool) []Item {
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if !recorded[it.AniListID] {
			out = append(out, it)
		}
	}
	return out
}

// topGenres は「見た・見てる」の記録でよく出るジャンルの上位 n 個。数が同じなら名前順にして、
// 同じ記録からは同じキー（fallbackCache）ができるようにする。
func topGenres(records []*domain.Record, n int) []string {
	counts := make(map[string]int)
	for _, r := range records {
		if r.Status != domain.StatusDone && r.Status != domain.StatusActive {
			continue
		}
		for _, g := range r.Genres {
			counts[g]++
		}
	}
	genres := make([]string, 0, len(counts))
	for g := range counts {
		genres = append(genres, g)
	}
	sort.Slice(genres, func(i, j int) bool {
		if counts[genres[i]] != counts[genres[j]] {
			return counts[genres[i]] > counts[genres[j]]
		}
		return genres[i] < genres[j]
	})
	return firstN(genres, n)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
