package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	r.Post("/events", h.createEvent)
	r.Get("/events/{id}", h.getEvent)
	r.Put("/events/{id}", h.updateEvent)
	r.Delete("/events/{id}", h.deleteEvent)
	r.Delete("/", h.clear)
	return r
}

func (h *CalendarHandler) createEvent(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	var event googlecalendar.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateEvent(event); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	created, err := h.calendar.CreateEvent(r.Context(), event)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *CalendarHandler) getEvent(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	event, err := h.calendar.GetEvent(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (h *CalendarHandler) updateEvent(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	var event googlecalendar.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateEvent(event); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	updated, err := h.calendar.UpdateEvent(r.Context(), chi.URLParam(r, "id"), event)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *CalendarHandler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	if !h.available(w) {
		return
	}
	if err := h.calendar.DeleteEvent(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *CalendarHandler) available(w http.ResponseWriter) bool {
	if h.setupError != nil {
		writeError(w, http.StatusServiceUnavailable, h.setupError)
		return false
	}
	return true
}

func validateEvent(event googlecalendar.Event) error {
	if strings.TrimSpace(event.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	if event.Start.IsZero() || event.End.IsZero() {
		return fmt.Errorf("start and end are required")
	}
	if !event.End.After(event.Start) {
		return fmt.Errorf("end must be after start")
	}
	return nil
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
