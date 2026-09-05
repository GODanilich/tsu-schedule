package router

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/GODanilich/tsu-schedule/internal/config"
	"github.com/GODanilich/tsu-schedule/internal/googlecalendar"
	"github.com/GODanilich/tsu-schedule/internal/handler"
	"github.com/GODanilich/tsu-schedule/internal/intimeparser"
	"github.com/GODanilich/tsu-schedule/internal/schedule"
	"github.com/GODanilich/tsu-schedule/internal/sqlite"
)

// New creates the HTTP router for the application.
func New(cfg config.Config) (http.Handler, error) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Get("/health", health)

	sourceService, err := intimeparser.NewScheduleService(cfg)
	if err != nil {
		return nil, fmt.Errorf("register InTime parser routes: %w", err)
	}
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", cfg.Timezone, err)
	}
	cache, err := sqlite.New(cfg.SQLiteFile)
	if err != nil {
		return nil, fmt.Errorf("create schedule cache: %w", err)
	}
	cachedService := schedule.NewService(cache, location)
	refresher := schedule.NewRefresher(sourceService, cache, location)

	refresh := func() {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTPTimeout)
		defer cancel()
		if _, err := refresher.RefreshWeek(ctx, cfg.GroupID, time.Now().In(location)); err != nil {
			slog.Error("refresh schedule cache", "error", err)
			return
		}
		slog.Info("schedule cache refreshed")
	}
	refresh()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			refresh()
		}
	}()

	router.Mount("/schedule", handler.NewScheduleHandler(sourceService, cfg.GroupID, refresher).Routes())

	calendarClient, calendarErr := googlecalendar.NewClient(
		cfg.GoogleCredentialsFile,
		cfg.GoogleCalendarID,
		cfg.Timezone,
	)
	router.Mount("/calendar", handler.NewCalendarHandler(
		cachedService,
		cfg.GroupID,
		calendarClient,
		calendarErr,
	).Routes())

	return router, nil
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
