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
	"github.com/hatodayo30/anime-manga-tracker/internal/translate"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/auth"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/home"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/record"
	"github.com/hatodayo30/anime-manga-tracker/internal/usecase/search"
	translateusecase "github.com/hatodayo30/anime-manga-tracker/internal/usecase/translate"
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

	recordRepo := postgres.NewRecordRepository(pool)
	recordUsecase := record.NewUsecase(recordRepo)
	recordHandler := handler.NewRecordHandler(recordUsecase)

	userRepo := postgres.NewUserRepository(pool)
	sessionRepo := postgres.NewSessionRepository(pool)
	authUsecase := auth.NewService(userRepo, sessionRepo)
	authHandler := handler.NewAuthHandler(authUsecase)
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

	e := handler.NewRouter(handler.Handlers{
		Record:      recordHandler,
		Auth:        authHandler,
		Search:      searchHandler,
		Home:        homeHandler,
		Translate:   translateHandler,
		RequireUser: authMiddleware.RequireUser,
		WebDir:      "web",
	})

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
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
