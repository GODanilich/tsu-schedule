package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

type BlacklistStore interface {
	schedule.BlacklistRepository
	AddBlacklist(ctx context.Context, groupID string, rule schedule.BlacklistRule) (schedule.BlacklistRule, error)
	DeleteBlacklist(ctx context.Context, groupID string, id int64) error
}

type BlacklistHandler struct {
	store    BlacklistStore
	service  *schedule.Service
	calendar CalendarSyncer
	groupID  string
}

type CalendarSyncer interface {
	Sync(ctx context.Context, days []schedule.Day) (int, error)
}

func NewBlacklistHandler(store BlacklistStore, service *schedule.Service, calendar CalendarSyncer, groupID string) *BlacklistHandler {
	return &BlacklistHandler{store: store, service: service, calendar: calendar, groupID: groupID}
}

func (h *BlacklistHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.add)
	r.Delete("/{id}", h.delete)
	return r
}

func (h *BlacklistHandler) list(w http.ResponseWriter, r *http.Request) {
	rules, err := h.store.ListBlacklist(r.Context(), h.groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (h *BlacklistHandler) add(w http.ResponseWriter, r *http.Request) {
	var rule schedule.BlacklistRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	created, err := h.store.AddBlacklist(r.Context(), h.groupID, rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.syncCalendar(r); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *BlacklistHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErrorMessage(w, http.StatusBadRequest, "id must be a positive integer")
		return
	}
	if err := h.store.DeleteBlacklist(r.Context(), h.groupID, id); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := h.syncCalendar(r); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *BlacklistHandler) syncCalendar(r *http.Request) error {
	if h.calendar == nil || h.service == nil {
		return nil
	}
	for week := 0; week < 5; week++ {
		days, err := h.service.GetWeek(r.Context(), h.groupID, time.Now().AddDate(0, 0, week*7))
		if err != nil {
			return fmt.Errorf("load week for calendar sync: %w", err)
		}
		if _, err := h.calendar.Sync(r.Context(), days); err != nil {
			return fmt.Errorf("sync blacklist change with Google Calendar: %w", err)
		}
	}
	return nil
}
