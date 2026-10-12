// Package config はアプリケーションの環境変数を読み込み・検証する。
package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config はアプリ起動に必要な設定値を保持する。
type Config struct {
	Port        string
	DatabaseURL string

	// CookieSecure はセッションCookieに Secure 属性を立てるかどうか。
	// 平文HTTPでアクセスする環境（ローカル開発、ALB を入れる前のECS）で true にすると
	// ブラウザがCookieを送らずログインできないため、既定は false。
	// HTTPS化したら COOKIE_SECURE=true にする。
	CookieSecure bool

	// TrustProxy はクライアント IP を X-Forwarded-For から取るかどうか（レート制限のキーになる）。
	// 正解は前段の構成で変わるので、コードに決め打ちせず環境変数で切り替える。
	//   - Phase 1（タスクのパブリック IP に直接アクセス）: RemoteAddr が本物の接続元。
	//     ここで XFF を信じると、クライアントが好きな値を書くだけでレート制限を回避できる。
	//   - Phase 2（ALB 経由）: タスク SG が ALB SG からの通信しか許可しないので RemoteAddr は
	//     常に ALB になり、ALB が XFF の右端に追記した値が本物の接続元になる。
	// 安全側（XFF を信じない）に倒すため既定は false。ALB を入れたら TRUST_PROXY=true にする。
	// false のまま ALB の裏に置いても全員が ALB の IP で 1 つのバケットを共有するだけで
	// 回避はされないが、正当な利用者同士で 429 を踏み合う。
	TrustProxy bool
}

// Load は .env（存在すれば）と環境変数から設定を読み込む。
// .env はローカル開発専用で、本番環境では実際の環境変数を使う想定。
func Load() (*Config, error) {
	_ = godotenv.Load() // .env が無くてもエラーにしない

	cookieSecure, err := getEnvBool("COOKIE_SECURE", false)
	if err != nil {
		return nil, err
	}

	trustProxy, err := getEnvBool("TRUST_PROXY", false)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:         getEnv("PORT", "8080"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		CookieSecure: cookieSecure,
		TrustProxy:   trustProxy,
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvBool は真偽値の環境変数を読む。解釈できない値は既定値に倒さずエラーにする
// （"ture" のような綴り間違いで Secure が黙って外れる、TRUST_PROXY が黙って false になり
// ALB の裏で全員が 1 つのバケットを共有する、といった事故に気付けない）。
func getEnvBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean (got %q)", key, v)
	}
	return b, nil
}
