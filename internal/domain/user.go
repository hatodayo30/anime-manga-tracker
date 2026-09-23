package domain

import "time"

// User は認証済みユーザー。パスワードハッシュはこの型には含めない
// （API レスポンスに誤って混入しないようにするため）。
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}
