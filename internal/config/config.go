// Package config はアプリケーションの環境変数を読み込み・検証する。
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config はアプリ起動に必要な設定値を保持する。
type Config struct {
	Port        string
	DatabaseURL string
}

// Load は .env（存在すれば）と環境変数から設定を読み込む。
// .env はローカル開発専用で、本番環境では実際の環境変数を使う想定。
func Load() (*Config, error) {
	_ = godotenv.Load() // .env が無くてもエラーにしない

	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
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
