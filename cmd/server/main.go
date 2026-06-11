package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/benchanczh/shanji/internal/api"
	"github.com/benchanczh/shanji/internal/config"
	"github.com/benchanczh/shanji/internal/store"
)

func main() {
	log, _ := zap.NewDevelopment()
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("load config", zap.Error(err))
	}

	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatal("migrate", zap.Error(err))
	}
	log.Info("migrations applied")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal("connect database", zap.Error(err))
	}
	defer pool.Close()

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      api.NewServer(cfg, log, pool).Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Info("server listening", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", zap.Error(err))
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", zap.Error(err))
		os.Exit(1)
	}
}
