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
	"github.com/hatodayo30/anime-manga-tracker/internal/config"
	"github.com/hatodayo30/anime-manga-tracker/internal/handler"
	"github.com/hatodayo30/anime-manga-tracker/internal/infrastructure/postgres"
	"github.com/hatodayo30/anime-manga-tracker/internal/jikan"
	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/scheduler"
	"github.com/hatodayo30/anime-manga-tracker/internal/translate"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/home"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/record"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/search"
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

	translateClient := translate.NewClient()
	translateUsecase := translateusecase.NewUsecase(translateClient)
	translateHandler := handler.NewTranslateHandler(translateUsecase)

	healthHandler := handler.NewHealthHandler(pool)

	e := handler.NewRouter(handler.Handlers{
		Record:      recordHandler,
		Auth:        authHandler,
		Search:      searchHandler,
		Home:        homeHandler,
		Translate:   translateHandler,
		Health:      healthHandler,
		RequireUser: authMiddleware.RequireUser,
		WebDir:      "frontend/dist",
	})

	// ホーム画面向けキャッシュを起動時に温め、以後も定期的に取り直す。利用者が冷えたキャッシュや
	// TTL切れを踏んで、外部APIの数秒の待ち時間を肩代わりするのを防ぐ。
	waitJobs := scheduler.New(scheduler.Job{
		Name:     "home-cache-warm",
		Interval: home.WarmInterval,
		Run:      homeUsecase.Warm,
	}).Start(ctx)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           e,
		ReadHeaderTimeout: 5 * time.Second,
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
