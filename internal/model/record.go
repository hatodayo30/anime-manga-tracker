// Package model はアプリのドメインモデルを定義する。
package model

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

// Record は1作品分の記録（AniListの作品情報 + ユーザーのステータス）を表す。
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

// UpdateRecordInput は記録更新リクエストのボディ。ステータスか進捗のどちらか一方でも良い。
type UpdateRecordInput struct {
	Status   *Status `json:"status"`
	Progress *int    `json:"progress"`
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
