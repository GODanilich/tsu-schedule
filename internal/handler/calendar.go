package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/GODanilich/tsu-schedule/internal/googlecalendar"
	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

type CalendarHandler struct {
	service    *schedule.Service
	groupID    string
	calendar   *googlecalendar.Client
	setupError error
}

func NewCalendarHandler(service *schedule.Service, groupID string, calendar *googlecalendar.Client, setupError error) *CalendarHandler {
	return &CalendarHandler{service: service, groupID: groupID, calendar: calendar, setupError: setupError}
}

func (h *CalendarHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/sync", h.sync)
	r.Delete("/", h.clear)
	return r
}

func (h *CalendarHandler) sync(w http.ResponseWriter, r *http.Request) {
	if h.setupError != nil {
		writeError(w, http.StatusServiceUnavailable, h.setupError)
		return
	}

	date := time.Now()
	if value := r.URL.Query().Get("date"); value != "" {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			writeErrorMessage(w, http.StatusBadRequest, "date must have format YYYY-MM-DD")
			return
		}
		date = parsed
	}

	days, err := h.service.GetWeek(r.Context(), h.groupID, date)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	synced, err := h.calendar.Sync(r.Context(), days)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"week":   days[0].Date.Format("2006-01-02"),
		"synced": synced,
	})
}

func (h *CalendarHandler) clear(w http.ResponseWriter, r *http.Request) {
	if h.setupError != nil {
		writeError(w, http.StatusServiceUnavailable, h.setupError)
		return
	}

	deleted, err := h.calendar.Clear(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}
