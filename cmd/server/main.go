package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GODanilich/tsu-schedule/internal/config"
	"github.com/GODanilich/tsu-schedule/internal/router"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	handler, err := router.New(cfg)
	if err != nil {
		slog.Error("create HTTP router", "error", err)
		os.Exit(1)
	}

	addr := ":" + cfg.HTTPPort
	slog.Info("server started", "address", addr, "timezone", cfg.Timezone)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
