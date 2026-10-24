// Command server はアニメ・漫画記録管理アプリのAPIサーバーと静的フロントエンドを起動する。
package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/hatodayo30/anime-manga-tracker/internal/anilist"
	"github.com/hatodayo30/anime-manga-tracker/internal/claude"
	"github.com/hatodayo30/anime-manga-tracker/internal/config"
	"github.com/hatodayo30/anime-manga-tracker/internal/handler"
	"github.com/hatodayo30/anime-manga-tracker/internal/infrastructure/postgres"
	"github.com/hatodayo30/anime-manga-tracker/internal/jikan"
	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/scheduler"
	"github.com/hatodayo30/anime-manga-tracker/internal/translate"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/airing"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/home"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/nlsearch"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/recommend"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/record"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/search"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/taxonomy"
	translateusecase "github.com/hatodayo30/anime-manga-tracker/internal/usecase/translate"
	"github.com/hatodayo30/anime-manga-tracker/migrations"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := postgres.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// RDS では docker-entrypoint-initdb.d が使えないので、起動時に未適用分を流す。
	// スキーマが揃う前にリクエストを受けないよう、サーバー起動より先に実行する。
	if err := postgres.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}

	recordRepo := postgres.NewRecordRepository(pool)
	recordUsecase := record.NewUsecase(recordRepo)
	recordHandler := handler.NewRecordHandler(recordUsecase)

	userRepo := postgres.NewUserRepository(pool)
	sessionRepo := postgres.NewSessionRepository(pool)
	authUsecase := auth.NewService(userRepo, sessionRepo)
	authHandler := handler.NewAuthHandler(authUsecase, cfg.CookieSecure)
	authMiddleware := middleware.NewAuth(authUsecase)

	anilistClient := anilist.NewClient()
	jikanClient := jikan.NewClient()
	searchUsecase := search.NewUsecase(anilistClient, jikanClient)
	searchHandler := handler.NewSearchHandler(searchUsecase)
	homeUsecase := home.NewUsecase(anilistClient, jikanClient)
	homeHandler := handler.NewHomeHandler(homeUsecase)
	airingUsecase := airing.NewUsecase(recordRepo, anilistClient)

	// Claude のクライアントは AI の全機能で 1 つを共有する。日次上限（Budget）はプロセス全体の
	// 上限なので、機能ごとに作ると上限がその数だけ緩む。API キーが無い環境では nil のままにし、
	// 各機能は従来の経路だけで動く。
	var claudeClient *claude.Client
	if cfg.AnthropicAPIKey != "" {
		claudeClient = claude.NewClient(cfg.AnthropicAPIKey, claude.NewBudget(cfg.ClaudeDailyTokenLimit, time.Now))
		log.Printf("ai: enabled (daily limit %d tokens)", cfg.ClaudeDailyTokenLimit)
	} else {
		log.Printf("ai: disabled (ANTHROPIC_API_KEY is not set)")
	}

	// AniList のジャンル・タグの一覧。自然文検索（9-b）と AI おすすめ（9-c）で共有する。
	// 下の `_ =` は、両方がマージされるまで未使用の変数でビルドが落ちないためのもの。9-b と 9-c が
	// それぞれ消すと同じ行で衝突するので、両方のマージ後に消す。
	taxonomyUsecase := taxonomy.NewUsecase(anilistClient)
	_ = taxonomyUsecase

	// nil の Gateway を usecase に渡すと MyMemory だけで動く。
	// （*claude.Client の nil をそのまま渡すと、インターフェースとしては nil にならないので分岐で渡し分ける）
	var aiTranslator translateusecase.AIGateway
	if claudeClient != nil {
		aiTranslator = claudeClient
	}
	translateUsecase := translateusecase.NewUsecase(
		postgres.NewTranslationRepository(pool), aiTranslator, translate.NewClient())
	translateHandler := handler.NewTranslateHandler(translateUsecase)

	// 自然文検索（9-b）。aiTranslator と同じく、nil の *claude.Client をインターフェースに入れない
	var aiSearchParser nlsearch.AIGateway
	if claudeClient != nil {
		aiSearchParser = claudeClient
	}
	nlsearchUsecase := nlsearch.NewUsecase(aiSearchParser, taxonomyUsecase, anilistClient, searchUsecase)
	nlsearchHandler := handler.NewNLSearchHandler(nlsearchUsecase)

	// AI のおすすめ（9-c）。aiTranslator と同じく、キーが無いときは nil のインターフェースを渡し、
	// ジャンルベースのおすすめだけで動かす
	var aiRecommender recommend.AIGateway
	if claudeClient != nil {
		aiRecommender = claudeClient
	}
	recommendUsecase := recommend.NewUsecase(recordRepo, taxonomyUsecase, anilistClient, aiRecommender, time.Now)
	recommendHandler := handler.NewRecommendHandler(recommendUsecase)

	healthHandler := handler.NewHealthHandler(pool)

	e := handler.NewRouter(handler.Handlers{
		Record:       recordHandler,
		Auth:         authHandler,
		Search:       searchHandler,
		Home:         homeHandler,
		Translate:    translateHandler,
		Health:       healthHandler,
		Recommend:    recommendHandler,
		RequireUser:  authMiddleware.RequireUser,
		OptionalUser: authMiddleware.OptionalUser,
		WebDir:       "frontend/dist",
		TrustProxy:   cfg.TrustProxy,
		NLSearch:     nlsearchHandler,
	})

	waitJobs := scheduler.New(
		// ホーム画面向けキャッシュを起動時に温め、以後も定期的に取り直す。利用者が冷えたキャッシュや
		// TTL切れを踏んで、外部APIの数秒の待ち時間を肩代わりするのを防ぐ。
		scheduler.Job{
			Name:     "home-cache-warm",
			Interval: home.WarmInterval,
			Run:      homeUsecase.Warm,
		},
		// ライブラリに保存済みのアニメの次話放送日時を AniList から引き直す。放送が1話進むと
		// 保存済みの値は過去の日時のまま固定されるため、これが無いと「次話」表示と既定ソートが狂う。
		scheduler.Job{
			Name:     "next-airing-sync",
			Interval: airing.SyncInterval,
			Run:      airingUsecase.Sync,
		},
	).Start(ctx)

	// ReadHeaderTimeout だけだとヘッダ受信後のボディ読み取りに期限が無く、ボディを 1 バイトずつ
	// 送り続ける接続が goroutine と接続を無期限に握れる（BodyLimit はサイズしか見ない）。
	//   - ReadTimeout 15s: 接続受付からボディ読み終わりまで。最大のボディは BodyLimit の 64K で、
	//     低速回線（数十 KB/s）でも数秒で届く。ヘッダ 5s ＋ボディ分の余裕で 15s。
	//   - WriteTimeout 60s: ヘッダ読み終わりからレスポンス書き終わりまで、つまりハンドラの処理時間を含む。
	//     最も遅い正当な経路はキャッシュが冷えた状態の AniList 呼び出しで、トークンバケット
	//     （24 req/分 = 2.5 秒に 1 本）の待ち行列に並ぶと数十秒かかりうる。ここを短くすると
	//     処理は終わったのに書き込めず、空の応答で切れる。60s は ALB のアイドルタイムアウト既定値と同じで、
	//     それより長くしても ALB 側で先に切れるだけなので意味が無い。
	//     なお WriteTimeout が切るのは書き込みだけで、リクエストの context は取り消されない。
	//     ハンドラ自体は最後まで走るので、上流呼び出しの上限は各クライアントの http.Client.Timeout が担う。
	//   - IdleTimeout 120s: keep-alive の待機。ALB のアイドルタイムアウト（60s）より長くしておかないと、
	//     ALB が再利用しようとした接続をこちらが先に閉じて 502 になる。
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on :%s", cfg.Port)
	err = srv.ListenAndServe()

	stop()     // シグナル以外の理由で抜けた場合も定期ジョブを止める
	waitJobs() // 実行中のジョブが終わるのを待ってから、deferのpool.Closeに進む

	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
