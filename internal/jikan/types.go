package jikan

// jikanImage は複数解像度のカバー画像URLを持つ。表示にはjpg.image_urlを使う。
type jikanImage struct {
	JPG struct {
		ImageURL string `json:"image_url"`
	} `json:"jpg"`
}

type jikanGenre struct {
	Name string `json:"name"`
}

// searchEntry はJikanの検索結果1件分（アニメ・漫画で共通のフィールドのみ）。
// アニメは episodes、漫画は volumes/chapters を持つため両方optionalで受ける。
type searchEntry struct {
	MalID      int64        `json:"mal_id"`
	Titles     []titleEntry `json:"titles"`
	Title      string       `json:"title"`
	Images     jikanImage   `json:"images"`
	Genres     []jikanGenre `json:"genres"`
	Score      *float64     `json:"score"`
	Episodes   *int         `json:"episodes"`
	Volumes    *int         `json:"volumes"`
	Chapters   *int         `json:"chapters"`
	Synopsis   *string      `json:"synopsis"`
	Popularity *int         `json:"popularity"` // MAL上の人気順位（値が小さいほど人気）。ソート用途。
}

type titleEntry struct {
	Type  string `json:"type"`
	Title string `json:"title"`
}

type searchResponse struct {
	Data []searchEntry `json:"data"`
}
