package router

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/GODanilich/tsu-schedule/internal/config"
	"github.com/GODanilich/tsu-schedule/internal/intimeparser"
)

// New creates the HTTP router for the application.
func New(cfg config.Config) (http.Handler, error) {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Get("/health", health)

	if err := intimeparser.RegisterRoutes(router, cfg); err != nil {
		return nil, fmt.Errorf("register InTime parser routes: %w", err)
	}

	return router, nil
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
