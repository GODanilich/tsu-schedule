package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

type ScheduleHandler struct {
	service *schedule.Service
	groupID string
}

func NewScheduleHandler(service *schedule.Service, groupID string) *ScheduleHandler {
	return &ScheduleHandler{service: service, groupID: groupID}
}

func (h *ScheduleHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/today", h.getToday)
	r.Get("/day", h.getDay)
	r.Get("/week", h.getWeek)
	return r
}

func (h *ScheduleHandler) getToday(w http.ResponseWriter, r *http.Request) {
	day, err := h.service.GetToday(r.Context(), h.groupID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, day)
}

func (h *ScheduleHandler) getDay(w http.ResponseWriter, r *http.Request) {
	date, ok := parseQueryDate(w, r)
	if !ok {
		return
	}

	day, err := h.service.GetDay(r.Context(), h.groupID, date)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, day)
}

func (h *ScheduleHandler) getWeek(w http.ResponseWriter, r *http.Request) {
	date, ok := parseQueryDate(w, r)
	if !ok {
		return
	}

	days, err := h.service.GetWeek(r.Context(), h.groupID, date)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, days)
}

func parseQueryDate(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	value := r.URL.Query().Get("date")
	if value == "" {
		writeErrorMessage(w, http.StatusBadRequest, "query parameter 'date' is required")
		return time.Time{}, false
	}

	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "date must have format YYYY-MM-DD")
		return time.Time{}, false
	}

	return date, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeErrorMessage(w, status, err.Error())
}

func writeErrorMessage(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
