// Package cache はusecase層が外部APIの呼び出し結果を短時間メモリに保持するための
// 汎用TTLキャッシュを提供する。
package cache

import (
	"sync"
	"time"
)

// TTLCache はキーごとの値をTTL付きでメモリ上に保持する。キーを使わない用途では
// 固定のキー（例: 空文字）を渡せばよい。
//
// 同一キーへの取得が同時に走った場合、fetch は1回だけ実行され、残りの呼び出しは
// その結果を待って共有する（single-flight）。呼び出し先がレート制限付きの外部API
// （AniListは全呼び出し共通のスロットルを持つ）の場合、重複リクエストがそのまま
// スロットルの待ち行列に並んで後続を遅らせるのを防ぐ。
type TTLCache[T any] struct {
	ttl        time.Duration
	maxEntries int // 0以下なら上限なし

	mu    sync.Mutex
	items map[string]*entry[T]
}

// entry はキャッシュ1件分。fetch実行中は ready が未クローズで、完了時にクローズされる。
// data/err/fetchedAt は ready のクローズ前に書かれ、クローズ後にのみ読まれる。
type entry[T any] struct {
	ready chan struct{}

	data      T
	err       error
	fetchedAt time.Time
}

// done は fetch が完了済みかを返す。呼び出し元は c.mu を保持していなくてよい。
func (e *entry[T]) done() bool {
	select {
	case <-e.ready:
		return true
	default:
		return false
	}
}

func New[T any](ttl time.Duration) *TTLCache[T] {
	return &TTLCache[T]{ttl: ttl}
}

// NewBounded は保持するキー数に上限を設けたキャッシュを作る。上限を超えると
// 期限切れ・古い順にエントリを捨てる。season/yearのようにキーが際限なく増えうる
// （yearはクエリパラメータ由来で任意の数値を取りうる）用途で使う。
func NewBounded[T any](ttl time.Duration, maxEntries int) *TTLCache[T] {
	return &TTLCache[T]{ttl: ttl, maxEntries: maxEntries}
}

// Get はキーに対応する値がTTL内にキャッシュされていればそれを返し、なければ fetch を呼んで
// 結果をキャッシュしてから返す。同じキーのfetchが既に進行中なら、それの完了を待って結果を共有する。
// fetch が失敗した場合はキャッシュに残さず、次の呼び出しで再試行させる。
func (c *TTLCache[T]) Get(key string, fetch func() (T, error)) (T, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok {
		if !e.done() {
			// 同じキーのfetchが進行中。完了を待って結果を共有する
			c.mu.Unlock()
			<-e.ready
			if e.err != nil {
				var zero T
				return zero, e.err
			}
			return e.data, nil
		}
		if time.Since(e.fetchedAt) < c.ttl {
			c.mu.Unlock()
			return e.data, nil
		}
		// 期限切れ。以下で取り直す
	}

	// このゴルーチンがfetchを担当する。ロックを保持したままエントリを登録することで、
	// 同じキーの別の呼び出しは必ず上の待ち合わせ側に回る。
	e := &entry[T]{ready: make(chan struct{})}
	if c.items == nil {
		c.items = make(map[string]*entry[T])
	}
	c.items[key] = e
	c.mu.Unlock()

	e.data, e.err = fetch()
	e.fetchedAt = time.Now()
	close(e.ready)

	c.mu.Lock()
	if e.err != nil {
		// 失敗は保持しない。ただし自分が登録したエントリでなければ触らない
		if c.items[key] == e {
			delete(c.items, key)
		}
	} else {
		c.evictLocked(key)
	}
	c.mu.Unlock()

	return e.data, e.err
}

// evictLocked は maxEntries を超えたぶんのエントリを、期限切れ→古い順に捨てる。
// keep は今まさに書き込んだキーで、捨てない。実行中のfetchも、待っている呼び出しがいるため捨てない。
// 呼び出し時に c.mu を保持していること。
func (c *TTLCache[T]) evictLocked(keep string) {
	if c.maxEntries <= 0 || len(c.items) <= c.maxEntries {
		return
	}

	for k, e := range c.items {
		if k != keep && e.done() && time.Since(e.fetchedAt) >= c.ttl {
			delete(c.items, k)
		}
	}

	for len(c.items) > c.maxEntries {
		var oldestKey string
		var oldest time.Time
		found := false
		for k, e := range c.items {
			if k == keep || !e.done() {
				continue
			}
			if !found || e.fetchedAt.Before(oldest) {
				oldestKey, oldest, found = k, e.fetchedAt, true
			}
		}
		if !found {
			return // 捨てられるエントリがない（全てfetch実行中）
		}
		delete(c.items, oldestKey)
	}
}
