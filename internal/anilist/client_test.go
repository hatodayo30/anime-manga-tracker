package anilist

import (
	"context"
	"errors"
	"testing"
	"time"
)

// anilistLimitPerMinute はAniListが公開しているレート制限の目安（30req/分）。
const anilistLimitPerMinute = 30

// スロットルの設定値が、最悪ケースでもAniListのレート制限を超えないことを保証する。
// アイドル明けに burstCapacity 本を一気に送り、その後は tokenInterval 間隔で送り続けるのが最悪ケース。
func TestThrottleConfig_StaysWithinAniListRateLimit(t *testing.T) {
	worstCasePerMinute := burstCapacity + int(time.Minute/tokenInterval)
	if worstCasePerMinute > anilistLimitPerMinute {
		t.Errorf("throttle config allows up to %d req/min, which exceeds AniList's %d req/min limit (burstCapacity=%d, tokenInterval=%v)",
			worstCasePerMinute, anilistLimitPerMinute, burstCapacity, tokenInterval)
	}
}

// バケットに残っているぶんは待たされず、使い切ったあとは次の回復まで待たされる。
func TestClient_Throttle_AllowsBurstThenWaits(t *testing.T) {
	c := NewClient()

	start := time.Now()
	for i := range burstCapacity {
		if err := c.throttle(context.Background()); err != nil {
			t.Fatalf("burst request %d: unexpected error: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("expected the first %d requests to pass without waiting, took %v", burstCapacity, elapsed)
	}

	// バケットを使い切った直後の呼び出しは待たされる。実際に待つと遅いので、
	// 短いタイムアウトを付けて「待ちに入ったこと」を確認する。
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.throttle(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected the request after the burst to wait for a token, got %v", err)
	}
}

// アイドル状態が続いてもトークンは burstCapacity を超えて溜まらない。
func TestClient_Throttle_CapsAccumulatedTokens(t *testing.T) {
	c := NewClient()
	// 十分に長くアイドルしていた状態を作る（実時間で待たずに内部状態を直接古くする）
	c.allowAt = time.Now().Add(-time.Hour)

	for i := range burstCapacity {
		if err := c.throttle(context.Background()); err != nil {
			t.Fatalf("burst request %d: unexpected error: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.throttle(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected tokens to be capped at burstCapacity=%d after a long idle, got %v", burstCapacity, err)
	}
}

// 429を受けたときのペナルティは共有バケットに効き、他の呼び出しもまとめて待たされる。
func TestClient_Penalize_BlocksSubsequentRequests(t *testing.T) {
	c := NewClient()
	c.penalize(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.throttle(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected penalize to hold back requests, got %v", err)
	}
}

func TestRetryAfter(t *testing.T) {
	fallback := 5 * time.Second

	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{name: "uses Retry-After seconds", header: "30", want: 30 * time.Second},
		{name: "falls back when absent", header: "", want: fallback},
		{name: "falls back when not a number", header: "soon", want: fallback},
		{name: "falls back when zero", header: "0", want: fallback},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := make(map[string][]string)
			if tt.header != "" {
				header["Retry-After"] = []string{tt.header}
			}
			if got := retryAfter(header, fallback); got != tt.want {
				t.Errorf("retryAfter(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}
