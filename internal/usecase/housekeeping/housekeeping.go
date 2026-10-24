// Package housekeeping は DB に溜まり続ける不要な行を定期的に片付ける。
//
// 今の対象は期限切れのセッションだけ。sessions は FindUser が expires_at > now() で弾いているので
// 認証としては効かないが、行は消えずに残る（BACKLOG 4-d）。
// BACKLOG 8-a のゲストアカウント削除もここに相乗りさせる予定なので、片付けの単位（task）を
// 並べて順に回す形にしてある。足すときは Repository にメソッドを 1 つ、tasks に 1 行足す。
package housekeeping

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// Interval は後始末を回す間隔。internal/scheduler から使う。
//
// 期限切れのセッションは認証に使われないので、急いで消す理由は無い。1 時間にしているのは、
// 8-a のゲストアカウント（寿命は未決定だが 24 時間前後の想定）を相乗りさせたときに、
// 寿命を過ぎてから消えるまでの遅れを寿命に対して小さく保つため。
// 外部 API は叩かず DELETE を数本流すだけなので、起動直後に走ってもレートの心配は無い。
const Interval = time.Hour

// Usecase は定期的な後始末を提供する。利用者のリクエストからは呼ばれない。
type Usecase struct {
	repo Repository
}

func NewUsecase(repo Repository) *Usecase {
	return &Usecase{repo: repo}
}

// task は片付けの 1 単位。name はログとエラーに出す。
type task struct {
	name string
	run  func(ctx context.Context) (int64, error)
}

func (u *Usecase) tasks() []task {
	return []task{
		{name: "expired sessions", run: u.repo.DeleteExpiredSessions},
	}
}

// Run は全ての片付けを順に実行する。起動直後と、その後 Interval ごとに internal/scheduler から呼ばれる。
//
// 1 つが失敗しても残りは回す。互いに独立した掃除なので、1 つの失敗で他の行まで溜め続ける理由は無い。
// 失敗はまとめて返し、scheduler のログに出す。
// ただしシャットダウン（ctx のキャンセル）のときは残りを回しても失敗するだけなので、そこで止める。
func (u *Usecase) Run(ctx context.Context) error {
	var errs []error
	for _, t := range u.tasks() {
		n, err := t.run(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", t.name, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		// 消すものが無い回は毎時のログを増やすだけなので黙る（実行の有無は scheduler が出す）
		if n > 0 {
			log.Printf("housekeeping: deleted %d %s", n, t.name)
		}
	}
	return errors.Join(errs...)
}
