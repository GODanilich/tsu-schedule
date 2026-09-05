package intimeparser

import (
	"fmt"
	"net/http"
	"time"

	"github.com/GODanilich/tsu-schedule/internal/config"
	"github.com/GODanilich/tsu-schedule/internal/intime"
	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

// NewScheduleService creates a schedule service backed by the InTime parser.
func NewScheduleService(cfg config.Config) (*schedule.Service, error) {
	location, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", cfg.Timezone, err)
	}

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}
	intimeClient := intime.NewClient(cfg.InTimeBaseURL, httpClient, location)

	return schedule.NewService(intimeClient, location), nil
}
