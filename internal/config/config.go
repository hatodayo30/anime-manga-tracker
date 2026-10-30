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

	// AnthropicAPIKey は AI 翻訳（Claude）の API キー。空なら AI を使わず、従来どおり
	// MyMemory だけで動く（ローカル開発・CI・キーを SSM に入れる前の本番）。
	// 本物の秘密なので、本番では DATABASE_URL と同じく SSM から secrets で渡す。
	AnthropicAPIKey string

	// ClaudeDailyTokenLimit はプロセス全体で 1 日（JST）に Claude へ使ってよいトークン数。
	// 既定の 200 万は、Haiku 5.5 で全部が出力トークン（$0.50/MTok）だったとしても 1 日 $1。
	// 翻訳 1 回は数千トークンなので、正当な利用で数百作品ぶんに相当し、普段は届かない。
	ClaudeDailyTokenLimit int64

	// RakutenApplicationID / RakutenAccessKey は楽天ブックス書籍検索 API の認証情報（新刊、BACKLOG 11-b）。
	// どちらも任意で、両方揃っていなければ新刊の API は「未設定」を返す（起動は止めない）。
	// アクセスキーは本物の秘密なので、本番では ANTHROPIC_API_KEY と同じく SSM から secrets で渡す。
	RakutenApplicationID string
	RakutenAccessKey     string
	// RakutenReferer は楽天へのリクエストに付ける Referer。楽天のアプリ設定の「許可されたウェブサイト」に
	// 登録したサイトでないと 403 になる（rakuten.NewClient のコメント）。手元でも本番と同じ値を送れば通る。
	// 秘密ではないので、本番は environment で変えられる
	RakutenReferer string
	// RakutenAffiliateID は楽天アフィリエイト ID（任意）。付けると商品ページのリンクがアフィリエイト経由になる。
	// 規約（第 3 条）はアフィリエイトの対象の商品へのリンクをアフィリエイト経由で作るよう定めている（BACKLOG 11-b）
	RakutenAffiliateID string

	// GoogleClientID / GoogleClientSecret / GoogleRedirectURL は Google ログイン（BACKLOG 8-b）の設定。
	// 3 つとも揃っていなければ Google ログインは無効で、ログイン画面にボタンも出ない（起動は止めない）。
	// クライアントシークレットは本物の秘密なので、本番では DATABASE_URL と同じく SSM から secrets で渡す。
	// リダイレクト URI は Google Cloud Console に登録したものと完全一致が要る（…/api/auth/google/callback）。
	// 秘密ではないが環境ごとに違うので、コードに決め打ちしない
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
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

	claudeDailyTokenLimit, err := getEnvInt64("CLAUDE_DAILY_TOKEN_LIMIT", 2_000_000)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Port:         getEnv("PORT", "8080"),
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		CookieSecure: cookieSecure,
		TrustProxy:   trustProxy,

		AnthropicAPIKey:       os.Getenv("ANTHROPIC_API_KEY"),
		ClaudeDailyTokenLimit: claudeDailyTokenLimit,

		RakutenApplicationID: os.Getenv("RAKUTEN_APPLICATION_ID"),
		RakutenAccessKey:     os.Getenv("RAKUTEN_ACCESS_KEY"),
		RakutenReferer:       getEnv("RAKUTEN_REFERER", "https://mirokuapp.com/"),
		RakutenAffiliateID:   os.Getenv("RAKUTEN_AFFILIATE_ID"),

		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
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

// getEnvInt64 は正の整数の環境変数を読む。getEnvBool と同じ理由で、解釈できない値はエラーにする
// （上限の綴り間違いで黙って既定値に戻ると、絞ったつもりの費用上限が効いていないことに気付けない）。
func getEnvInt64(key string, fallback int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer (got %q)", key, v)
	}
	return n, nil
}
