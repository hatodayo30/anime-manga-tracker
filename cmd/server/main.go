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
	"github.com/hatodayo30/anime-manga-tracker/internal/repository"
	"github.com/hatodayo30/anime-manga-tracker/internal/service"
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

	anilistClient := anilist.NewClient()
	searchHandler := handler.NewSearchHandler(anilistClient)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/records", recordHandler.List)
	mux.HandleFunc("POST /api/records", recordHandler.Create)
	mux.HandleFunc("PATCH /api/records/{id}", recordHandler.Update)
	mux.HandleFunc("GET /api/search", searchHandler.Search)
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
