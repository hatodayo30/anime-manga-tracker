package scheduler_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/scheduler"
)

// ジョブは起動直後に1回走り、その後も Interval ごとに繰り返される。
// 起動直後の1回がキャッシュのウォームアップに当たる。
func TestScheduler_RunsAtStartThenOnInterval(t *testing.T) {
	runs := make(chan struct{}, 16)
	s := scheduler.New(scheduler.Job{
		Name:     "job",
		Interval: 10 * time.Millisecond,
		Run: func(context.Context) error {
			select {
			case runs <- struct{}{}:
			default: // テスト側が読み取る前でもジョブを止めない
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	wait := s.Start(ctx)
	defer func() {
		cancel()
		wait()
	}()

	for i := 1; i <= 3; i++ {
		select {
		case <-runs:
		case <-time.After(2 * time.Second):
			t.Fatalf("run %d did not happen", i)
		}
	}
}

// ctx をキャンセルするとジョブは止まり、Start が返した関数は実行中の処理の完了を待つ。
// DBプールのようにジョブが使う資源を、使用中に閉じてしまわないために必要な保証。
func TestScheduler_WaitBlocksUntilRunningJobFinishes(t *testing.T) {
	started := make(chan struct{})
	var finished atomic.Bool

	s := scheduler.New(scheduler.Job{
		Name:     "slow",
		Interval: time.Hour,
		Run: func(context.Context) error {
			close(started)
			time.Sleep(50 * time.Millisecond) // キャンセルに即応しないジョブを模す
			finished.Store(true)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	wait := s.Start(ctx)

	<-started
	cancel()
	wait()

	if !finished.Load() {
		t.Error("wait returned before the running job finished")
	}
}

// ジョブに渡される ctx はシャットダウン時にキャンセルされる。外部API呼び出しが
// シャットダウンを引き止めないために必要。
func TestScheduler_CancelsJobContext(t *testing.T) {
	jobErr := make(chan error, 1)
	s := scheduler.New(scheduler.Job{
		Name:     "waits-on-ctx",
		Interval: time.Hour,
		Run: func(ctx context.Context) error {
			<-ctx.Done()
			jobErr <- ctx.Err()
			return ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	wait := s.Start(ctx)
	cancel()
	wait()

	select {
	case err := <-jobErr:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("got %v, want context.Canceled", err)
		}
	default:
		t.Error("job context was not canceled")
	}
}

// ジョブが失敗しても、スケジューラは止まらず次の実行に進む。
// 外部APIの一時的な失敗でウォームアップが二度と走らなくなるのを防ぐ。
func TestScheduler_ContinuesAfterJobError(t *testing.T) {
	var calls int32
	done := make(chan struct{})

	s := scheduler.New(scheduler.Job{
		Name:     "flaky",
		Interval: 10 * time.Millisecond,
		Run: func(context.Context) error {
			if atomic.AddInt32(&calls, 1) == 2 {
				close(done)
			}
			return errors.New("boom")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	wait := s.Start(ctx)
	defer func() {
		cancel()
		wait()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected the job to run again after a failure, got %d calls", atomic.LoadInt32(&calls))
	}
}

// 複数のジョブはそれぞれ独立に動く。片方が長引いても、もう片方は待たされない。
func TestScheduler_RunsJobsIndependently(t *testing.T) {
	blocked := make(chan struct{})
	quick := make(chan struct{}, 1)

	s := scheduler.New(
		scheduler.Job{
			Name:     "blocking",
			Interval: time.Hour,
			Run: func(ctx context.Context) error {
				<-ctx.Done()
				close(blocked)
				return nil
			},
		},
		scheduler.Job{
			Name:     "quick",
			Interval: time.Hour,
			Run: func(context.Context) error {
				select {
				case quick <- struct{}{}:
				default:
				}
				return nil
			},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	wait := s.Start(ctx)

	select {
	case <-quick:
	case <-time.After(2 * time.Second):
		t.Fatal("the quick job did not run while another job was blocked")
	}

	cancel()
	wait()
	<-blocked
}

// Interval の設定ミス（0以下）でスケジューラ全体を落とさない。該当ジョブだけ走らせない。
func TestScheduler_SkipsJobWithNonPositiveInterval(t *testing.T) {
	var calls int32
	s := scheduler.New(scheduler.Job{
		Name:     "misconfigured",
		Interval: 0,
		Run: func(context.Context) error {
			atomic.AddInt32(&calls, 1)
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)()

	if calls != 0 {
		t.Errorf("expected the misconfigured job not to run, got %d calls", calls)
	}
}
