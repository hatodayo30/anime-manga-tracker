package anilist

import (
	"strings"
	"testing"

	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

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

// 空の条件は変数ごと省くことを保証する。空の配列や空文字を渡すと、AniList が「何にも当てはまらない」
// 条件として扱い、0 件になりうる。
func TestFilterVariables_OmitsEmptyConditions(t *testing.T) {
	got := filterVariables(Filter{MediaType: domain.MediaTypeAnime, Query: "  ", Genres: []string{}})

	for _, key := range []string{"search", "genres", "tags", "excludedGenres", "excludedTags", "minimumTagRank"} {
		if _, ok := got[key]; ok {
			t.Errorf("expected %q to be omitted, got %v", key, got[key])
		}
	}
	if got["type"] != "ANIME" {
		t.Errorf("type = %v, want ANIME", got["type"])
	}
}

// タグの下限は、タグで絞るときだけ付けることを保証する。
func TestFilterVariables_MinimumTagRankOnlyWithTags(t *testing.T) {
	withTags := filterVariables(Filter{MediaType: domain.MediaTypeManga, Tags: []string{"Isekai"}, ExcludedTags: []string{"Gore"}})
	if withTags["minimumTagRank"] != filterMinimumTagRank {
		t.Errorf("expected minimumTagRank with tags, got %v", withTags["minimumTagRank"])
	}
	if withTags["type"] != "MANGA" {
		t.Errorf("type = %v, want MANGA", withTags["type"])
	}

	excludeOnly := filterVariables(Filter{MediaType: domain.MediaTypeManga, ExcludedTags: []string{"Gore"}})
	if _, ok := excludeOnly["minimumTagRank"]; ok {
		t.Error("expected minimumTagRank to be omitted without tags")
	}
}

// キャラクターから引いた作品の絞り込みが、filterQuery（AniList 側の絞り込み）と同じ意味になることを保証する。
// ずれると、同じ文でもキャラクター名の有無で結果の基準が変わる。
func TestMatchesFilter(t *testing.T) {
	genres := []string{"Romance", "Drama"}
	tags := map[string]int{"School": 90, "Cooking": 30}

	tests := []struct {
		name string
		f    Filter
		want bool
	}{
		{name: "条件なしなら通す", f: Filter{}, want: true},
		{name: "ジャンルはすべてを含めば通す", f: Filter{Genres: []string{"Romance", "Drama"}}, want: true},
		{name: "含まないジャンルが 1 つでもあれば落とす", f: Filter{Genres: []string{"Romance", "Action"}}, want: false},
		{name: "タグは下限以上なら通す", f: Filter{Tags: []string{"School"}}, want: true},
		{name: "タグが付いていても下限未満なら落とす", f: Filter{Tags: []string{"Cooking"}}, want: false},
		{name: "除外ジャンルを含めば落とす", f: Filter{ExcludedGenres: []string{"Drama"}}, want: false},
		{name: "除外タグは下限未満でも付いていれば落とす", f: Filter{ExcludedTags: []string{"Cooking"}}, want: false},
		{name: "付いていないタグの除外は通す", f: Filter{ExcludedTags: []string{"Gore"}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesFilter(genres, tags, tt.f); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// 名前の候補と作品名の検索を、エイリアスで 1 本のクエリにまとめることを保証する。
// 候補ごとに送ると AniList の枠（24 本/分）を候補の数だけ使う。
func TestBuildCharacterQuery(t *testing.T) {
	q := buildCharacterQuery(3, true)
	for _, want := range []string{"c0: Page", "c1: Page", "c2: Page", "t: Page", "$n2: String", "$query: String", "search: $n1"} {
		if !strings.Contains(q, want) {
			t.Errorf("expected query to contain %q", want)
		}
	}

	q = buildCharacterQuery(1, false)
	for _, unwanted := range []string{"c1: Page", "t: Page", "$query"} {
		if strings.Contains(q, unwanted) {
			t.Errorf("expected query not to contain %q", unwanted)
		}
	}
}
