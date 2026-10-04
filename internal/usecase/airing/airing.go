// Package airing はライブラリに保存済みのアニメの次話放送日時（records.next_airing_at）を
// AniList の最新情報で定期的に取り直す。
//
// next_airing_at はライブラリ追加時の Upsert でクライアントから渡された値を書くだけなので、
// 放送が1話進むと過去の日時のまま固定され、ライブラリの「次話」表示と既定ソート
// （next_airing_at ASC）が狂う。それを直すのがこのパッケージの役目。
package airing

import (
	"context"
	"fmt"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/domain"
)

// SyncInterval は next_airing_at を取り直す間隔。internal/scheduler から使う。
//
// 放送済みの話を次話として表示してしまう時間がこの間隔ぶん残るが、1クールのアニメが
// 次話に進むのは週1回なので、これ以上詰めても見えるものは変わらない。
const SyncInterval = 30 * time.Minute

// batchLimit は1回の実行で取り直す AniList ID の上限。AniList のスロットルは全呼び出しで
// 共有されており、ここで使ったぶんは利用者のリクエストが待たされる時間になる。
// anilist.MediaByIDs は50件を1リクエストにまとめるので、100件 = 最大2リクエスト。
// ウォームアップ（8分ごとにAniList 3本）と合わせても平均レート上限（30req/分）に対して十分小さい。
const batchLimit = 100

// Usecase は next_airing_at の定期更新を提供する。利用者のリクエストからは呼ばれない。
type Usecase struct {
	repo    Repository
	anilist AniListGateway
}

func NewUsecase(repo Repository, anilistGateway AniListGateway) *Usecase {
	return &Usecase{repo: repo, anilist: anilistGateway}
}

// Sync は次話放送日時が過去になったアニメを AniList から引き直して書き戻す。
// 起動直後と、その後 SyncInterval ごとに internal/scheduler から呼ばれる。
//
// 対象を「next_airing_at が過去のレコード」に限っているため、処理すべき件数は放っておくと
// 0に収束する。取り直した結果は未来の日時になるか（次話が決まっている）、NULL になるか
// （放送終了・AniListに情報なし）のどちらかで、いずれも次回以降は対象から外れる。
// batchLimit を超えて溜まっていても、古い順に毎回 batchLimit 件ずつ片付ければいずれ追いつく。
func (u *Usecase) Sync(ctx context.Context) error {
	ids, err := u.repo.StaleAiringAnimeIDs(ctx, batchLimit)
	if err != nil {
		return fmt.Errorf("list stale anime ids: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	results, err := u.anilist.MediaByIDs(ctx, ids, domain.MediaTypeAnime)
	if err != nil {
		return fmt.Errorf("fetch media by ids: %w", err)
	}

	if _, err := u.repo.UpdateNextAiringAt(ctx, nextAiringUpdates(ids, results)); err != nil {
		return fmt.Errorf("update next airing at: %w", err)
	}
	return nil
}

// nextAiringUpdates は取得した作品情報を、問い合わせたID全件ぶんの更新内容に変換する。
//
// AniList が返さなかったIDにも NULL を書く。作品が削除された・IDが変わった等でいつまでも
// 返ってこないレコードを放置すると、過去の日時が残り続けて毎回の対象に居座り、
// batchLimit の枠を食い潰すため。次話が分からない以上、表示としても NULL が正しい。
func nextAiringUpdates(ids []int64, results []anilist.SearchResult) []NextAiring {
	fetched := make(map[int64]*time.Time, len(results))
	for _, res := range results {
		var at *time.Time
		if res.NextAiringAt != nil {
			t := time.Unix(*res.NextAiringAt, 0)
			at = &t
		}
		fetched[res.AniListID] = at
	}

	updates := make([]NextAiring, 0, len(ids))
	for _, id := range ids {
		updates = append(updates, NextAiring{AniListID: id, At: fetched[id]})
	}
	return updates
}
