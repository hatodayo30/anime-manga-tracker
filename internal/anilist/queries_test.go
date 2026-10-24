package anilist

import "testing"

// 作品名の選び方を保証する。韓国・中国作品で別名に日本語版のタイトルがあればそれを使い、
// 漢字だけの別名（中国語と見分けられない）は使わないこと。
func TestDisplayTitle(t *testing.T) {
	tests := []struct {
		name     string
		title    mediaTitle
		synonyms []string
		country  string
		want     string
	}{
		{
			name:     "日本作品は native を使い、別名は見ない",
			title:    mediaTitle{Romaji: "Ao no Hako", Native: "アオのハコ", English: "Blue Box"},
			synonyms: []string{"青の箱"},
			country:  "JP",
			want:     "アオのハコ",
		},
		{
			name:     "韓国作品は別名のうち、かなを含むものを使う",
			title:    mediaTitle{Romaji: "Na Honjaman Level Up", Native: "나 혼자만 레벨업", English: "Solo Leveling"},
			synonyms: []string{"I Level Up Alone", "我獨自升級", "俺だけレベルアップな件", "Ore dake Level Up na Ken"},
			country:  "KR",
			want:     "俺だけレベルアップな件",
		},
		{
			name:     "漢字だけの別名は中国語と見分けられないので使わず、英語にする",
			title:    mediaTitle{Romaji: "Sinui Tap", Native: "신의 탑", English: "Tower of God"},
			synonyms: []string{"Kami no Tou", "神之塔"},
			country:  "KR",
			want:     "Tower of God",
		},
		{
			name:    "別名も英語も無ければ romaji にする",
			title:   mediaTitle{Romaji: "Sinui Tap", Native: "신의 탑"},
			country: "KR",
			want:    "Sinui Tap",
		},
		{
			name:    "国が不明なら日本作品として扱う",
			title:   mediaTitle{Romaji: "Ao no Hako", Native: "アオのハコ", English: "Blue Box"},
			country: "",
			want:    "アオのハコ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayTitle(tt.title, tt.synonyms, tt.country); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
