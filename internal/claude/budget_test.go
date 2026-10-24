package claude_test

import (
	"testing"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/claude"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

// 使用量が上限に達するまでは許可し、達したら止めることを保証する。
func TestBudget_BlocksOnceLimitReached(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)}
	b := claude.NewBudget(1000, clock.now)

	if !b.Allow() {
		t.Fatal("expected fresh budget to allow")
	}
	b.Add(999)
	if !b.Allow() {
		t.Fatal("expected budget below limit to allow")
	}
	b.Add(1)
	if b.Allow() {
		t.Fatal("expected budget at limit to block")
	}
}

// 日付が JST で変わった時点（UTC 15:00）で使用量が 0 に戻ることを保証する。
// UTC の日付で区切ると、日本の利用者には朝 9 時に上限が戻る挙動になる。
func TestBudget_ResetsAtJSTMidnight(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 22, 14, 59, 0, 0, time.UTC)} // JST 23:59
	b := claude.NewBudget(1000, clock.now)

	b.Add(1000)
	if b.Allow() {
		t.Fatal("expected exhausted budget to block")
	}

	clock.t = time.Date(2026, 10, 22, 15, 0, 0, 0, time.UTC) // JST 翌 0:00
	if !b.Allow() {
		t.Fatal("expected budget to reset at JST midnight")
	}
}
