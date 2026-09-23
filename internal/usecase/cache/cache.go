// Package cache はusecase層が外部APIの呼び出し結果を短時間メモリに保持するための
// 汎用TTLキャッシュを提供する。
package cache

import (
	"sync"
	"time"
)

// TTLCache はキーごとの値をTTL付きでメモリ上に保持する。キーを使わない用途では
// 固定のキー（例: 空文字）を渡せばよい。
type TTLCache[T any] struct {
	ttl time.Duration

	mu    sync.Mutex
	items map[string]entry[T]
}

type entry[T any] struct {
	data      T
	fetchedAt time.Time
}

func New[T any](ttl time.Duration) *TTLCache[T] {
	return &TTLCache[T]{ttl: ttl}
}

// Get はキーに対応する値がTTL内にキャッシュされていればそれを返し、なければ fetch を呼んで
// 結果をキャッシュしてから返す。
func (c *TTLCache[T]) Get(key string, fetch func() (T, error)) (T, error) {
	c.mu.Lock()
	if e, ok := c.items[key]; ok && time.Since(e.fetchedAt) < c.ttl {
		c.mu.Unlock()
		return e.data, nil
	}
	c.mu.Unlock()

	data, err := fetch()
	if err != nil {
		var zero T
		return zero, err
	}

	c.mu.Lock()
	if c.items == nil {
		c.items = make(map[string]entry[T])
	}
	c.items[key] = entry[T]{data: data, fetchedAt: time.Now()}
	c.mu.Unlock()
	return data, nil
}
