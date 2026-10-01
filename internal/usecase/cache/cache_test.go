package cache_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/cache"
)

func TestTTLCache_CachesPerKey(t *testing.T) {
	c := cache.New[string](time.Minute)
	var calls int32

	get := func(key string) string {
		v, err := c.Get(context.Background(), key, func(context.Context) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "value-" + key, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return v
	}

	if v := get("a"); v != "value-a" {
		t.Errorf("got %q, want %q", v, "value-a")
	}
	if v := get("b"); v != "value-b" {
		t.Errorf("got %q, want %q", v, "value-b")
	}
	// 同じキーの2回目はfetchを呼ばない
	if v := get("a"); v != "value-a" {
		t.Errorf("got %q, want %q", v, "value-a")
	}

	if calls != 2 {
		t.Errorf("expected 2 fetches (one per key), got %d", calls)
	}
}

// 同じキーへの取得が同時に走っても fetch は1回しか呼ばれず、全員が同じ結果を受け取る。
// シーズン画面の二重リクエストがAniListのスロットルに二重に並ぶのを防ぐための挙動。
func TestTTLCache_SingleFlight(t *testing.T) {
	c := cache.New[string](time.Minute)
	var calls int32
	release := make(chan struct{})

	const goroutines = 8
	var wg sync.WaitGroup
	results := make([]string, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.Get(context.Background(), "same", func(context.Context) (string, error) {
				atomic.AddInt32(&calls, 1)
				<-release // 全員が待ち合わせに入るまでfetchを終わらせない
				return "shared", nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			results[i] = v
		}()
	}

	// 最初の1本がfetchに入るまで待ってから解放する
	for atomic.LoadInt32(&calls) == 0 {
		time.Sleep(time.Millisecond)
	}
	close(release)
	wg.Wait()

	if calls != 1 {
		t.Errorf("expected fetch to run once for concurrent gets, got %d", calls)
	}
	for i, v := range results {
		if v != "shared" {
			t.Errorf("goroutine %d got %q, want %q", i, v, "shared")
		}
	}
}

func TestTTLCache_DoesNotCacheErrors(t *testing.T) {
	c := cache.New[string](time.Minute)
	wantErr := errors.New("boom")
	var calls int32

	_, err := c.Get(context.Background(), "k", func(context.Context) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}

	// 失敗は保持しないので、次の呼び出しで再試行される
	v, err := c.Get(context.Background(), "k", func(context.Context) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "recovered" {
		t.Errorf("got %q, want %q", v, "recovered")
	}
	if calls != 2 {
		t.Errorf("expected failed fetch to be retried, got %d calls", calls)
	}
}

func TestTTLCache_BoundedEvictsOldest(t *testing.T) {
	const maxEntries = 3
	c := cache.NewBounded[string](time.Minute, maxEntries)
	var calls int32

	fetch := func(key string) func(context.Context) (string, error) {
		return func(context.Context) (string, error) {
			atomic.AddInt32(&calls, 1)
			return "value-" + key, nil
		}
	}

	// 上限を超えるまで別々のキーを詰める。fetchedAtで古い順に捨てるため、間隔を空ける
	keys := []string{"0", "1", "2", "3"}
	for _, k := range keys {
		if _, err := c.Get(context.Background(), k, fetch(k)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if calls != 4 {
		t.Fatalf("expected 4 fetches while filling, got %d", calls)
	}

	// 最新の3件はキャッシュに残っている
	for _, k := range keys[1:] {
		if _, err := c.Get(context.Background(), k, fetch(k)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if calls != 4 {
		t.Errorf("expected recent keys to stay cached, got %d fetches", calls)
	}

	// 最も古いキーは追い出されているので取り直しになる
	if _, err := c.Get(context.Background(), "0", fetch("0")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 5 {
		t.Errorf("expected oldest key to be evicted and refetched, got %d fetches", calls)
	}
}

func TestTTLCache_BoundedKeepsEntryCountAtLimit(t *testing.T) {
	const maxEntries = 4
	c := cache.NewBounded[int](time.Minute, maxEntries)

	for i := range 50 {
		if _, err := c.Get(context.Background(), strconv.Itoa(i), func(context.Context) (int, error) { return i, nil }); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// 上限を超えたぶんが捨てられていれば、直近 maxEntries 件だけがfetchなしで返る
	var refetched int
	for i := 50 - maxEntries; i < 50; i++ {
		if _, err := c.Get(context.Background(), strconv.Itoa(i), func(context.Context) (int, error) {
			refetched++
			return i, nil
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if refetched != 0 {
		t.Errorf("expected the %d most recent keys to remain cached, %d were refetched", maxEntries, refetched)
	}
}

// staleFactor はキャッシュ側の定数（TTLの何倍まで古い値を返してよいか）に合わせたもの。
const staleFactor = 10

// waitFor は cond が真になるまで待つ。バックグラウンド更新の完了のように、
// 完了時刻がスケジューラ任せになるものを待つために使う。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TTLが切れていても古い値は即座に返り、取り直しはバックグラウンドで走る。
// TTLが切れた瞬間に当たった人だけが外部APIの数秒を肩代わりする状態を無くすための挙動。
func TestTTLCache_ServesStaleWhileRevalidating(t *testing.T) {
	const ttl = 30 * time.Millisecond
	c := cache.New[int](ttl)

	var calls int32
	release := make(chan struct{})
	fetch := func(context.Context) (int, error) {
		n := int(atomic.AddInt32(&calls, 1))
		if n > 1 {
			<-release // バックグラウンド更新は、テストが許すまで終わらせない
		}
		return n, nil
	}

	if v, err := c.Get(context.Background(), "k", fetch); v != 1 || err != nil {
		t.Fatalf("got (%d, %v), want (1, nil)", v, err)
	}
	time.Sleep(ttl * 2)

	// 更新が終わっていなくても古い値がそのまま返る（ここでブロックしたらテストはタイムアウトする）
	if v, err := c.Get(context.Background(), "k", fetch); v != 1 || err != nil {
		t.Fatalf("got (%d, %v), want stale (1, nil)", v, err)
	}
	waitFor(t, "background refresh to start", func() bool { return atomic.LoadInt32(&calls) >= 2 })

	// 更新が終われば新しい値に入れ替わる
	close(release)
	waitFor(t, "refreshed value", func() bool {
		v, err := c.Get(context.Background(), "k", fetch)
		return err == nil && v > 1
	})
}

// 古い値を返してよいのは staleFactor×TTL まで。それを超えたら待たせてでも取り直す。
// 上流が落ちている間ずっと腐ったデータを配り続けないための上限。
func TestTTLCache_RefetchesBeyondStaleLimit(t *testing.T) {
	const ttl = 5 * time.Millisecond
	c := cache.New[int](ttl)

	var calls int32
	fetch := func(context.Context) (int, error) { return int(atomic.AddInt32(&calls, 1)), nil }

	if v, _ := c.Get(context.Background(), "k", fetch); v != 1 {
		t.Fatalf("got %d, want 1", v)
	}

	// この間は誰もアクセスしないのでバックグラウンド更新も走らない
	time.Sleep(ttl * (staleFactor + 1))

	if v, _ := c.Get(context.Background(), "k", fetch); v != 2 {
		t.Errorf("expected a fresh value past the stale limit, got %d", v)
	}
}

// バックグラウンド更新は、きっかけとなったリクエストのctxがキャンセルされても走り切る。
// ハンドラのctxはレスポンスを返した時点でキャンセルされるため、引き継ぐと更新が必ず失敗する。
func TestTTLCache_BackgroundRefreshOutlivesRequestContext(t *testing.T) {
	const ttl = 20 * time.Millisecond
	c := cache.New[int](ttl)

	var calls int32
	started := make(chan struct{})
	fetch := func(ctx context.Context) (int, error) {
		n := int(atomic.AddInt32(&calls, 1))
		if n > 1 {
			close(started)
			select {
			case <-time.After(50 * time.Millisecond):
			case <-ctx.Done():
				return 0, ctx.Err() // キャンセルが引き継がれていたらここに来る
			}
		}
		return n, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	if v, _ := c.Get(ctx, "k", fetch); v != 1 {
		t.Fatalf("got %d, want 1", v)
	}
	time.Sleep(ttl * 2)
	if v, _ := c.Get(ctx, "k", fetch); v != 1 {
		t.Fatalf("got %d, want stale 1", v)
	}

	<-started
	cancel() // リクエストが終わってハンドラのctxが切れた状況

	waitFor(t, "refresh to finish despite the canceled request context", func() bool {
		v, err := c.Get(context.Background(), "k", fetch)
		return err == nil && v == 2
	})
}

// staleな値への同時アクセスが重なっても、バックグラウンド更新は1本しか走らない。
// レート制限付きの上流に更新リクエストを積み上げないための挙動。
func TestTTLCache_SingleBackgroundRefresh(t *testing.T) {
	const ttl = 20 * time.Millisecond
	c := cache.New[int](ttl)

	var calls int32
	release := make(chan struct{})
	fetch := func(context.Context) (int, error) {
		n := int(atomic.AddInt32(&calls, 1))
		if n > 1 {
			<-release
		}
		return n, nil
	}

	if _, err := c.Get(context.Background(), "k", fetch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(ttl * 2)

	const goroutines = 8
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := c.Get(context.Background(), "k", fetch); v != 1 || err != nil {
				t.Errorf("got (%d, %v), want stale (1, nil)", v, err)
			}
		}()
	}
	wg.Wait()

	waitFor(t, "background refresh to start", func() bool { return atomic.LoadInt32(&calls) >= 2 })
	time.Sleep(10 * time.Millisecond) // 2本目が走るなら、この間に現れる
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Errorf("expected exactly one background refresh, got %d fetches in total", n)
	}
	close(release)
}

// バックグラウンド更新が失敗しても古い値は捨てない。上流が一時的に落ちている間、
// stale上限までは手元の値で凌ぐ。
func TestTTLCache_KeepsStaleWhenRefreshFails(t *testing.T) {
	const ttl = 20 * time.Millisecond
	c := cache.New[string](ttl)

	var calls int32
	fetch := func(context.Context) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return "first", nil
		}
		return "", errors.New("upstream down")
	}

	if v, err := c.Get(context.Background(), "k", fetch); v != "first" || err != nil {
		t.Fatalf("got (%q, %v), want (\"first\", nil)", v, err)
	}
	time.Sleep(ttl * 2)

	if v, err := c.Get(context.Background(), "k", fetch); v != "first" || err != nil {
		t.Fatalf("got (%q, %v), want stale (\"first\", nil)", v, err)
	}
	waitFor(t, "background refresh to fail", func() bool { return atomic.LoadInt32(&calls) >= 2 })

	// 更新に失敗しても、stale上限内なら古い値がエラーなしで返り続ける
	if v, err := c.Get(context.Background(), "k", fetch); v != "first" || err != nil {
		t.Errorf("got (%q, %v), want the stale value to survive a failed refresh", v, err)
	}
}

// Refresh はTTLが残っていても取り直す。定期リフレッシュが「TTLが切れるまで何もしない」
// のでは、利用者がTTL切れを踏む機会を減らせない。
func TestTTLCache_RefreshBypassesTTL(t *testing.T) {
	c := cache.New[int](time.Hour) // TTLは十分に残っている状況
	var calls int32
	fetch := func(context.Context) (int, error) { return int(atomic.AddInt32(&calls, 1)), nil }

	if v, _ := c.Get(context.Background(), "k", fetch); v != 1 {
		t.Fatalf("got %d, want 1", v)
	}
	if err := c.Refresh(context.Background(), "k", fetch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v, _ := c.Get(context.Background(), "k", fetch); v != 2 {
		t.Errorf("expected Refresh to replace the cached value, got %d", v)
	}
	if calls != 2 {
		t.Errorf("expected exactly 2 fetches, got %d", calls)
	}
}

// 値がまだ無いキーでは、Refresh は通常の取得と同じように値を詰める（起動時のウォームアップ）。
func TestTTLCache_RefreshFillsEmptyCache(t *testing.T) {
	c := cache.New[string](time.Hour)
	var calls int32
	fetch := func(context.Context) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "warmed", nil
	}

	if err := c.Refresh(context.Background(), "k", fetch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, err := c.Get(context.Background(), "k", fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "warmed" {
		t.Errorf("got %q, want %q", v, "warmed")
	}
	if calls != 1 {
		t.Errorf("expected the warmed value to be served from cache, got %d fetches", calls)
	}
}

// Refresh の実行中も、既存の値は待たされずに返る。定期リフレッシュが利用者を
// 止めてしまっては、ウォームアップの意味がない。
func TestTTLCache_RefreshDoesNotBlockGet(t *testing.T) {
	c := cache.New[int](time.Hour)
	release := make(chan struct{})
	var calls int32
	fetch := func(context.Context) (int, error) {
		n := int(atomic.AddInt32(&calls, 1))
		if n > 1 {
			<-release
		}
		return n, nil
	}

	if v, _ := c.Get(context.Background(), "k", fetch); v != 1 {
		t.Fatalf("got %d, want 1", v)
	}

	refreshDone := make(chan error, 1)
	go func() { refreshDone <- c.Refresh(context.Background(), "k", fetch) }()
	waitFor(t, "refresh to start", func() bool { return atomic.LoadInt32(&calls) >= 2 })

	// 取り直しが終わるまでは従来の値が返る（ここでブロックしたらテストはタイムアウトする）
	if v, err := c.Get(context.Background(), "k", fetch); v != 1 || err != nil {
		t.Errorf("got (%d, %v), want the previous value (1, nil)", v, err)
	}

	close(release)
	if err := <-refreshDone; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, _ := c.Get(context.Background(), "k", fetch); v != 2 {
		t.Errorf("got %d, want the refreshed value 2", v)
	}
}

// 既に取得・取り直しが走っているキーでは、Refresh は何もしない。
// レート制限付きの上流に、定期リフレッシュぶんの重複リクエストを積まないための挙動。
func TestTTLCache_RefreshSkipsWhenFetchInFlight(t *testing.T) {
	c := cache.New[int](time.Hour)
	release := make(chan struct{})
	started := make(chan struct{})
	var calls int32

	fetch := func(context.Context) (int, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(started)
			<-release
		}
		return 1, nil
	}

	got := make(chan int, 1)
	go func() {
		v, _ := c.Get(context.Background(), "k", fetch)
		got <- v
	}()
	<-started

	if err := c.Refresh(context.Background(), "k", fetch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("expected Refresh to skip while a fetch was in flight, got %d fetches", n)
	}

	close(release)
	<-got
}

// Refresh が失敗しても古い値は捨てず、エラーだけを返す（呼び出し元がログに出せるように）。
func TestTTLCache_RefreshKeepsValueOnFailure(t *testing.T) {
	c := cache.New[string](time.Hour)
	wantErr := errors.New("upstream down")
	var calls int32
	fetch := func(context.Context) (string, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return "first", nil
		}
		return "", wantErr
	}

	if _, err := c.Get(context.Background(), "k", fetch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.Refresh(context.Background(), "k", fetch); !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}

	if v, err := c.Get(context.Background(), "k", fetch); v != "first" || err != nil {
		t.Errorf("got (%q, %v), want the previous value to survive a failed refresh", v, err)
	}
}
