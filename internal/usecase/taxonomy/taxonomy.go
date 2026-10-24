// Package taxonomy は AniList のジャンル・タグの一覧を長い TTL でキャッシュして配る。
//
// 自然文検索（9-b）と AI おすすめ（9-c）がどちらも構造化出力の enum に使う。それぞれの usecase が
// 自分でキャッシュを持つと、同じ一覧のために AniList の枠を 2 本使うので、main.go で 1 つ作って共有する。
package taxonomy

import (
	"context"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
)

// cacheTTL は一覧を取り直す間隔。AniList のジャンル・タグはめったに増減しない（BACKLOG 9-b
// 「長い TTL でキャッシュする」）。毎回取ると検索 1 回で AniList 2 本になる。
// TTL 切れの後も cache の staleFactor 倍（10 日）までは古い一覧を返しながら裏で取り直すので、
// 取り直しに失敗し続けても AI の機能はしばらく止まらない。
const cacheTTL = 24 * time.Hour

// cacheKey は 1 つしかない値のキー。
const cacheKey = "all"

// AniListGateway は一覧の取得元。実装は internal/anilist が提供する。
type AniListGateway interface {
	Taxonomy(ctx context.Context) (anilist.Taxonomy, error)
}

type Usecase struct {
	anilist AniListGateway
	cache   *cache.TTLCache[anilist.Taxonomy]
}

func NewUsecase(anilistGateway AniListGateway) *Usecase {
	return &Usecase{
		anilist: anilistGateway,
		cache:   cache.New[anilist.Taxonomy](cacheTTL),
	}
}

// Get はジャンル・タグの一覧を返す。初回だけ AniList を待つ。
//
// 起動時に温めないのは、サーバー起動直後の AniList の枠が home-cache-warm で埋まっているため
// （CLAUDE.md「外部 API のレート」）。一覧を使うのはログイン済みの利用者が AI の機能を
// 使ったときだけなので、そのときに 1 本足すほうが枠を無駄にしない。
func (u *Usecase) Get(ctx context.Context) (anilist.Taxonomy, error) {
	return u.cache.Get(ctx, cacheKey, func(ctx context.Context) (anilist.Taxonomy, error) {
		return u.anilist.Taxonomy(ctx)
	})
}
