// Package scheduler はサーバープロセス内で定期ジョブを回す。キャッシュのウォームアップの
// ように、利用者のリクエストとは独立に動かしたい処理をここに乗せる。
//
// デプロイ先の ECS Fargate は desiredCount:1 の常時起動タスクなので、ticker で回す
// プロセス内実装で足りる。ただし**ローリングデプロイ中に2タスクが並走すると同じジョブが
// 二重に実行される**ため、デプロイ戦略は単一タスク保証（minimumHealthyPercent:0 /
// maximumPercent:100）に設定しておくこと。複数タスクを常用するようになったら、
// ここはプロセス外のスケジューラか分散ロックに置き換える必要がある。
package scheduler

import (
	"context"
	"log"
	"sync"
	"time"
)

// Job は定期実行する処理1つぶん。
type Job struct {
	// Name はログに出す識別子。
	Name string
	// Interval は実行間隔。正の値であること。
	Interval time.Duration
	// Run は実行する処理。ctx はシャットダウン時にキャンセルされるので、
	// 外部API呼び出しなど待ちうる処理には必ず渡すこと。
	Run func(ctx context.Context) error
}

// Scheduler は登録されたジョブを、起動直後に1回ずつ、その後は各自の Interval で実行する。
type Scheduler struct {
	jobs []Job
}

func New(jobs ...Job) *Scheduler {
	return &Scheduler{jobs: jobs}
}

// Start は全ジョブの実行を開始し、終了を待つ関数を返す。ジョブはそれぞれ独立した
// goroutine で動く。ctx がキャンセルされると各ジョブは実行中の処理を終えてから止まり、
// 返された関数はそれを待つ。DBプールのように、ジョブが使う資源を閉じる前に呼ぶこと。
func (s *Scheduler) Start(ctx context.Context) (wait func()) {
	var wg sync.WaitGroup
	for _, job := range s.jobs {
		if job.Interval <= 0 {
			// 設定ミス。ここで起動ごと落とすほどではないが、黙って動かないのは避ける
			log.Printf("scheduler: %s has a non-positive interval (%s); not scheduled", job.Name, job.Interval)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			runJob(ctx, job)
		}()
	}
	return wg.Wait
}

// runJob は1つのジョブを、起動直後に1回実行してから Interval ごとに繰り返す。
// 1本のgoroutineで直列に回すので、前回の実行が Interval を超えて長引いても実行が重なることはない
// （その間に溜まったtickは捨てられる）。レート制限付きの外部APIを叩くジョブを二重に走らせない
// ためにも、この直列性は保つこと。
func runJob(ctx context.Context, job Job) {
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()

	for {
		start := time.Now()
		switch err := job.Run(ctx); {
		case err != nil && ctx.Err() != nil:
			// シャットダウンによる中断。異常ではないので騒がない
			log.Printf("scheduler: %s interrupted by shutdown", job.Name)
			return
		case err != nil:
			log.Printf("scheduler: %s failed after %s: %v", job.Name, elapsed(start), err)
		default:
			log.Printf("scheduler: %s done in %s", job.Name, elapsed(start))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func elapsed(start time.Time) time.Duration {
	return time.Since(start).Round(time.Millisecond)
}
