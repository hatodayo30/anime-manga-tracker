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
	"github.com/hatodayo30/anime-manga-tracker/internal/middleware"
	"github.com/hatodayo30/anime-manga-tracker/internal/repository"
	"github.com/hatodayo30/anime-manga-tracker/internal/service"
	"github.com/hatodayo30/anime-manga-tracker/internal/translate"
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

	pool, err := repository.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	recordRepo := repository.NewRecordRepository(pool)
	recordService := service.NewRecordService(recordRepo)
	recordHandler := handler.NewRecordHandler(recordService)

	userRepo := repository.NewUserRepository(pool)
	sessionRepo := repository.NewSessionRepository(pool)
	authService := service.NewAuthService(userRepo, sessionRepo)
	authHandler := handler.NewAuthHandler(authService)
	auth := middleware.NewAuth(authService)

	anilistClient := anilist.NewClient()
	searchHandler := handler.NewSearchHandler(anilistClient)
	homeHandler := handler.NewHomeHandler(anilistClient)

	translateClient := translate.NewClient()
	translateHandler := handler.NewTranslateHandler(translateClient)

	mux := http.NewServeMux()

	// ログイン必須（自分のライブラリ）
	mux.HandleFunc("GET /api/records", auth.RequireUser(recordHandler.List))
	mux.HandleFunc("POST /api/records", auth.RequireUser(recordHandler.Create))
	mux.HandleFunc("PATCH /api/records/{id}", auth.RequireUser(recordHandler.Update))

	// ログイン不要（公開データ）
	mux.HandleFunc("GET /api/search", searchHandler.Search)
	mux.HandleFunc("GET /api/anilist/media", searchHandler.ByIDs)
	mux.HandleFunc("GET /api/recommendations", searchHandler.Recommendations)
	mux.HandleFunc("GET /api/home/season-anime", homeHandler.SeasonAnime)
	mux.HandleFunc("GET /api/home/trending", homeHandler.Trending)
	mux.HandleFunc("POST /api/translate", translateHandler.Translate)

	// 認証
	mux.HandleFunc("POST /api/auth/signup", authHandler.SignUp)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)
	mux.HandleFunc("POST /api/auth/logout", authHandler.Logout)
	mux.HandleFunc("GET /api/auth/me", authHandler.Me)

	mux.Handle("/", handler.NewStaticHandler("web"))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
