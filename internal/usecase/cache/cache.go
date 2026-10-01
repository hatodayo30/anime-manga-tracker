// Package cache はusecase層が外部APIの呼び出し結果を短時間メモリに保持するための
// 汎用TTLキャッシュを提供する。
package cache

import (
	"context"
	"sync"
	"time"
)

// staleFactor はTTL切れの値を「古いまま返してよい」上限をTTLの何倍にするかを決める。
// TTLを過ぎた値は返しつつ裏で更新するが、際限なく返し続けると上流が落ちている間ずっと
// 腐ったデータを配ることになる。TTL×10を超えたら古い値は捨て、待たせてでも取り直す。
const staleFactor = 10

// refreshTimeout はバックグラウンド更新1回に許す時間。呼び出し元のリクエストは既に
// 終わっているため、ここで打ち切らないとgoroutineが上流の応答待ちで残り続ける。
// ホームの人気漫画（AniList 1回 + Jikan 10回、Jikanは350ms間隔）でも十分に収まる長さ。
const refreshTimeout = time.Minute

// TTLCache はキーごとの値をTTL付きでメモリ上に保持する。キーを使わない用途では
// 固定のキー（例: 空文字）を渡せばよい。
//
// 同一キーへの取得が同時に走った場合、fetch は1回だけ実行され、残りの呼び出しは
// その結果を待って共有する（single-flight）。呼び出し先がレート制限付きの外部API
// （AniListは全呼び出し共通のスロットルを持つ）の場合、重複リクエストがそのまま
// スロットルの待ち行列に並んで後続を遅らせるのを防ぐ。
//
// TTLが切れた値は捨てずに stale として保持し、次の取得では古い値を即座に返したうえで
// 更新をバックグラウンドで走らせる（stale-while-revalidate）。外部APIの取得には数秒
// かかるため、TTLが切れた瞬間に当たった利用者だけがその待ち時間を肩代わりする状態を避ける。
// 古い値を返してよいのは staleFactor×TTL まで。
type TTLCache[T any] struct {
	ttl        time.Duration
	staleTTL   time.Duration // これを超えた値はもう返さない
	maxEntries int           // 0以下なら上限なし

	mu    sync.Mutex
	items map[string]*entry[T]
}

// entry はキャッシュ1件分。fetch実行中は ready が未クローズで、完了時にクローズされる。
// data/err/fetchedAt は ready のクローズ前に書かれ、クローズ後にのみ読まれる。以降は
// 不変で、バックグラウンド更新も既存エントリを書き換えず新しいエントリに差し替える。
type entry[T any] struct {
	ready chan struct{}

	data      T
	err       error
	fetchedAt time.Time

	refreshing bool // バックグラウンド更新が進行中（c.mu の保護下）
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
	return &TTLCache[T]{ttl: ttl, staleTTL: ttl * staleFactor}
}

// NewBounded は保持するキー数に上限を設けたキャッシュを作る。上限を超えると
// 期限切れ・古い順にエントリを捨てる。season/yearのようにキーが際限なく増えうる
// （yearはクエリパラメータ由来で任意の数値を取りうる）用途で使う。
func NewBounded[T any](ttl time.Duration, maxEntries int) *TTLCache[T] {
	return &TTLCache[T]{ttl: ttl, staleTTL: ttl * staleFactor, maxEntries: maxEntries}
}

// Get はキーに対応する値がTTL内にキャッシュされていればそれを返す。TTLは切れているが
// staleTTL内であれば古い値を即座に返し、更新はバックグラウンドで行う。値が無いか
// staleTTLも超えている場合だけ fetch の完了を待つ。同じキーのfetchが既に進行中なら、
// それの完了を待って結果を共有する。
//
// fetch が失敗した場合はキャッシュに残さず、次の呼び出しで再試行させる。
// ctx は待たせる側のfetchにそのまま渡される。バックグラウンド更新には、呼び出し元の
// リクエストが終わっても打ち切られないよう、キャンセルを外した派生ctxを渡す。
func (c *TTLCache[T]) Get(ctx context.Context, key string, fetch func(context.Context) (T, error)) (T, error) {
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
		if age := time.Since(e.fetchedAt); age < c.staleTTL {
			if age >= c.ttl && !e.refreshing {
				// TTL切れ。古い値を返しつつ、裏で取り直す
				e.refreshing = true
				go c.refresh(ctx, key, e, fetch)
			}
			data := e.data
			c.mu.Unlock()
			return data, nil
		}
		// 古すぎる。以下で取り直し、完了まで待たせる
	}

	// このゴルーチンがfetchを担当する。ロックを保持したままエントリを登録することで、
	// 同じキーの別の呼び出しは必ず上の待ち合わせ側に回る。
	e := &entry[T]{ready: make(chan struct{})}
	c.putLocked(key, e)
	c.mu.Unlock()

	e.data, e.err = fetch(ctx)
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

// refresh はstaleなエントリをバックグラウンドで取り直し、成功したら新しいエントリに差し替える。
// 失敗した場合は古い値をそのまま残す（staleTTLを超えるまでは古い値を返し続け、次の取得で再試行する）。
func (c *TTLCache[T]) refresh(ctx context.Context, key string, stale *entry[T], fetch func(context.Context) (T, error)) {
	// 呼び出し元のリクエストは既に終わっている可能性が高いのでキャンセルを引き継がない。
	// 代わりに上限時間を設けて、上流が応答しないときにgoroutineが残り続けるのを防ぐ。
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refreshTimeout)
	defer cancel()

	data, err := fetch(ctx)
	fetchedAt := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items[key] != stale {
		// 待たせる側のfetchが先に結果を入れた、または追い出された。古い結果で上書きしない
		return
	}
	stale.refreshing = false
	if err != nil {
		return
	}

	fresh := &entry[T]{ready: closedChan(), data: data, fetchedAt: fetchedAt}
	c.putLocked(key, fresh)
	c.evictLocked(key)
}

func closedChan() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// putLocked はエントリを登録する。呼び出し時に c.mu を保持していること。
func (c *TTLCache[T]) putLocked(key string, e *entry[T]) {
	if c.items == nil {
		c.items = make(map[string]*entry[T])
	}
	c.items[key] = e
}

// evictLocked は maxEntries を超えたぶんのエントリを、staleTTL切れ→古い順に捨てる。
// keep は今まさに書き込んだキーで、捨てない。実行中のfetchも、待っている呼び出しがいるため捨てない。
// 呼び出し時に c.mu を保持していること。
func (c *TTLCache[T]) evictLocked(keep string) {
	if c.maxEntries <= 0 || len(c.items) <= c.maxEntries {
		return
	}

	for k, e := range c.items {
		if k != keep && e.done() && time.Since(e.fetchedAt) >= c.staleTTL {
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
