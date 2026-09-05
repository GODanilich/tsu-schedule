package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/GODanilich/tsu-schedule/internal/config"
	"github.com/GODanilich/tsu-schedule/internal/handler"
	"github.com/GODanilich/tsu-schedule/internal/intimeparser"
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

	scheduleService, err := intimeparser.NewScheduleService(cfg)
	if err != nil {
		slog.Error("initialize InTime parser", "error", err)
		os.Exit(1)
	}

	scheduleHandler := handler.NewScheduleHandler(scheduleService, cfg.GroupID)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	router.Mount("/schedule", scheduleHandler.Routes())

	addr := ":" + cfg.HTTPPort
	slog.Info("server started", "address", addr, "timezone", cfg.Timezone)

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
