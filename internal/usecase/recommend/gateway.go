package recommend

import (
	"context"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// RecordRepository は利用者の記録の読み出し元。実装は internal/infrastructure/postgres の
// RecordRepository で、userID によるスコープは SQL で強制される。
type RecordRepository interface {
	List(ctx context.Context, userID int64, mediaType domain.MediaType, status domain.Status, sort domain.SortKey) ([]*domain.Record, error)
}

// TaxonomyProvider はジャンル・タグの一覧の取得元。実装は main.go で共有している
// usecase/taxonomy の Usecase（自然文検索と同じキャッシュを使う）。
type TaxonomyProvider interface {
	Get(ctx context.Context) (anilist.Taxonomy, error)
}

// AniListGateway は候補の取得元。実装は internal/anilist。
type AniListGateway interface {
	// SearchByFilter は AI が作った条件で候補を取る。
	SearchByFilter(ctx context.Context, f anilist.Filter) ([]anilist.SearchResult, error)
	// ByGenres は AI を使えないときのジャンルベースのおすすめに使う。usecase/search の
	// Recommendations と同じ AniList のクエリ（ジャンルの「いずれか」）。
	ByGenres(ctx context.Context, genres []string, mediaType domain.MediaType) ([]anilist.SearchResult, error)
}

// AIGateway は 3 段のうち AI が担う 2 段。実装は internal/claude。
//
// どちらも渡したデータの加工だけで、AI に作品を挙げさせる口は無い（BACKLOG 9-c）。
// 作品は必ず AniList の検索結果から選ばせる。
type AIGateway interface {
	// TastePreferences は記録から好みを検索条件（ジャンル名・タグ名）に変換する。
	// 名前は in.Genres / in.Tags の中からだけ選ぶ。
	TastePreferences(ctx context.Context, in PreferenceInput) (Preferences, error)
	// RankRecommendations は候補から選んで並べ、作品ごとに一言の理由を付ける。
	RankRecommendations(ctx context.Context, in RankInput) ([]Pick, error)
}

// Taste は AI に渡す記録 1 件。AI に渡すのはこの形に絞った値だけで、ID や日時は渡さない。
type Taste struct {
	Title  string
	Status domain.Status
	Rating *int // 1〜5。未評価は nil
	Genres []string
	Memo   string
}

type PreferenceInput struct {
	MediaType domain.MediaType
	Records   []Taste
	// Genres / Tags は AI が選んでよい名前の一覧（構造化出力の enum になる）。
	Genres []string
	Tags   []string
}

// Preferences は好みを表す検索条件。どれも好み・避けたさが強い順。
type Preferences struct {
	Genres      []string
	Tags        []string
	AvoidGenres []string
	AvoidTags   []string
}

type RankInput struct {
	MediaType  domain.MediaType
	Records    []Taste
	Candidates []anilist.SearchResult
}

// Pick は AI が選んだ候補 1 件。AniListID は RankInput.Candidates のどれかであるはずだが、
// usecase 側でも確かめる。
type Pick struct {
	AniListID int64
	Reason    string
}
