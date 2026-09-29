package cache_test

import (
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
		v, err := c.Get(key, func() (string, error) {
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

func TestTTLCache_RefetchesAfterTTL(t *testing.T) {
	c := cache.New[int](10 * time.Millisecond)
	var calls int32

	fetch := func() (int, error) { return int(atomic.AddInt32(&calls, 1)), nil }

	if v, _ := c.Get("k", fetch); v != 1 {
		t.Errorf("got %d, want 1", v)
	}
	time.Sleep(20 * time.Millisecond)
	if v, _ := c.Get("k", fetch); v != 2 {
		t.Errorf("expected refetch after TTL, got %d", v)
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
			v, err := c.Get("same", func() (string, error) {
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

	_, err := c.Get("k", func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}

	// 失敗は保持しないので、次の呼び出しで再試行される
	v, err := c.Get("k", func() (string, error) {
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

	fetch := func(key string) func() (string, error) {
		return func() (string, error) {
			atomic.AddInt32(&calls, 1)
			return "value-" + key, nil
		}
	}

	// 上限を超えるまで別々のキーを詰める。fetchedAtで古い順に捨てるため、間隔を空ける
	keys := []string{"0", "1", "2", "3"}
	for _, k := range keys {
		if _, err := c.Get(k, fetch(k)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if calls != 4 {
		t.Fatalf("expected 4 fetches while filling, got %d", calls)
	}

	// 最新の3件はキャッシュに残っている
	for _, k := range keys[1:] {
		if _, err := c.Get(k, fetch(k)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if calls != 4 {
		t.Errorf("expected recent keys to stay cached, got %d fetches", calls)
	}

	// 最も古いキーは追い出されているので取り直しになる
	if _, err := c.Get("0", fetch("0")); err != nil {
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
		if _, err := c.Get(strconv.Itoa(i), func() (int, error) { return i, nil }); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// 上限を超えたぶんが捨てられていれば、直近 maxEntries 件だけがfetchなしで返る
	var refetched int
	for i := 50 - maxEntries; i < 50; i++ {
		if _, err := c.Get(strconv.Itoa(i), func() (int, error) {
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
