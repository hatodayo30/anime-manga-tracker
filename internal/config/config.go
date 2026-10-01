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
}

// Load は .env（存在すれば）と環境変数から設定を読み込む。
// .env はローカル開発専用で、本番環境では実際の環境変数を使う想定。
func Load() (*Config, error) {
	_ = godotenv.Load() // .env が無くてもエラーにしない

	cookieSecure, err := getEnvBool("COOKIE_SECURE", false)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:         getEnv("PORT", "8080"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		CookieSecure: cookieSecure,
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
// （"ture" のような綴り間違いで Secure が黙って外れると気付けない）。
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
