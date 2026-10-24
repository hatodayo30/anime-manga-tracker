package claude

import (
	"sync"
	"time"
)

// jst は日次上限の「1 日」の区切り。time.LoadLocation("Asia/Tokyo") は実行環境の tzdata に
// 依存し、本番イメージ（alpine）には入っていないので固定オフセットで持つ。日本に夏時間は無い。
var jst = time.FixedZone("JST", 9*60*60)

// Budget はプロセス全体で 1 日に使ってよいトークン数を管理する。AniList のトークンバケットと同じく
// 1 プロセスのメモリ上にあり、全リクエストで 1 つを共有する。
//
// 守りたいのは「連打されたぶんだけ請求が積み上がる」こと（BACKLOG 9）。ログイン必須にしても
// アカウントを作り直せば利用者ごとの制限は回避できるので、最後に効くのはこの上限になる。
//
// 再起動で使用量は 0 に戻る。デプロイを繰り返すと上限は実質的に緩むので、
// Anthropic Console 側の月の上限と併用する前提。
type Budget struct {
	mu    sync.Mutex
	limit int64
	used  int64
	day   string
	now   func() time.Time
}

// NewBudget は 1 日あたり limit トークンまで使える Budget を作る。now はテストで時刻を差し替えるためのもので、
// 本番では time.Now を渡す。
func NewBudget(limit int64, now func() time.Time) *Budget {
	return &Budget{limit: limit, now: now}
}

// Allow は今日の使用量が上限に達していなければ true を返す。
//
// 呼び出しの前に判定し、使用量は応答を受け取ってから Add で足す。応答が返るまで消費量は
// 分からないので、同時に走った呼び出しのぶんだけ上限をわずかに超えうる。1 回の翻訳は
// 数千トークンで、上限（既定 200 万）に対して誤差の範囲なので、予約制にはしていない。
func (b *Budget) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollover()
	return b.used < b.limit
}

// Add は今日の使用量に tokens を足す。
func (b *Budget) Add(tokens int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollover()
	b.used += tokens
}

// rollover は日付（JST）が変わっていれば使用量を 0 に戻す。呼び出し側で mu を握っていること。
func (b *Budget) rollover() {
	today := b.now().In(jst).Format(time.DateOnly)
	if today != b.day {
		b.day = today
		b.used = 0
	}
}
