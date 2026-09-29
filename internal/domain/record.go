// Package domain はアプリのドメインモデルを定義する。
package domain

import "time"

// MediaType は記録対象がアニメか漫画かを表す。
type MediaType string

const (
	MediaTypeAnime MediaType = "anime"
	MediaTypeManga MediaType = "manga"
)

// Status は視聴/読了ステータスを表す。
type Status string

const (
	StatusWant   Status = "want"   // 見たい / 読みたい
	StatusActive Status = "active" // 見てる / 読んでる
	StatusDone   Status = "done"   // 見た / 読んだ
)

// SortKey はライブラリ一覧の並び順を表す。
type SortKey string

const (
	SortDefault SortKey = "default" // 次回放送が近い順（放送予定なしは後ろ）→ 追加が新しい順
	SortTitle   SortKey = "title"   // タイトル順（日本語コレーション）
	SortScore   SortKey = "score"   // 自分の評価が高い順（未評価は後ろ）
	SortUpdated SortKey = "updated" // 最近進捗を更新した順
	SortAdded   SortKey = "added"   // ライブラリに追加した順
)

func (s SortKey) Valid() bool {
	switch s {
	case SortDefault, SortTitle, SortScore, SortUpdated, SortAdded:
		return true
	}
	return false
}

// Record は1作品分の記録（AniListの作品情報 + ユーザーのステータス）を表す。
// Progress / Total の単位は MediaType で決まる（アニメ=話数、漫画=巻数）。
type Record struct {
	ID            int64      `json:"id"`
	AniListID     int64      `json:"anilistId"`
	MediaType     MediaType  `json:"mediaType"`
	Title         string     `json:"title"`
	CoverImageURL string     `json:"coverImageUrl"`
	Genres        []string   `json:"genres"`
	Status        Status     `json:"status"`
	Progress      int        `json:"progress"`
	Total         *int       `json:"total"`
	NextAiringAt  *time.Time `json:"nextAiringAt"`
	Rating        *int       `json:"rating"`
	Memo          string     `json:"memo"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// NewRecordInput は記録追加リクエストのボディ。
type NewRecordInput struct {
	AniListID     int64     `json:"anilistId"`
	MediaType     MediaType `json:"mediaType"`
	Title         string    `json:"title"`
	CoverImageURL string    `json:"coverImageUrl"`
	Genres        []string  `json:"genres"`
	Total         *int      `json:"total"`
	Status        Status    `json:"status"`
	// NextAiringAt は次話放送日時（unix秒）。検索結果からそのまま渡される。
	NextAiringAt *int64 `json:"nextAiringAt"`
}

// UpdateRecordInput は記録更新リクエストのボディ。どのフィールドも省略可（nilは「変更なし」を意味する）。
// Rating は 1〜5 で評価をセットし、0 を渡すと評価を未入力（NULL）に戻す。
// Total は作品モーダルがAniListの最新情報を取得した際、保存済みの総数がズレていれば同期するために使う。
type UpdateRecordInput struct {
	Status   *Status `json:"status"`
	Progress *int    `json:"progress"`
	Total    *int    `json:"total"`
	Rating   *int    `json:"rating"`
	Memo     *string `json:"memo"`
}

func (s Status) Valid() bool {
	switch s {
	case StatusWant, StatusActive, StatusDone:
		return true
	}
	return false
}

func (t MediaType) Valid() bool {
	switch t {
	case MediaTypeAnime, MediaTypeManga:
		return true
	}
	return false
}
