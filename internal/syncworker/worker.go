package syncworker

import (
	"context"
	"log/slog"
	"time"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

const weeksToSync = 5

type Calendar interface {
	SyncLesson(ctx context.Context, lesson schedule.Lesson) error
	DeleteLesson(ctx context.Context, lesson schedule.Lesson) error
}

type Worker struct {
	source   *schedule.Service
	cache    schedule.DiffCacheRepository
	calendar Calendar
	groupID  string
	location *time.Location
	timeout  time.Duration
}

func New(source *schedule.Service, cache schedule.DiffCacheRepository, calendar Calendar, groupID string, location *time.Location, timeout time.Duration) *Worker {
	return &Worker{source: source, cache: cache, calendar: calendar, groupID: groupID, location: location, timeout: timeout}
}

// Run synchronizes the current week and four following weeks one week at a time.
func (w *Worker) Run(ctx context.Context) {
	now := time.Now().In(w.location)
	for week := 0; week < weeksToSync; week++ {
		date := now.AddDate(0, 0, week*7)
		weekCtx, cancel := context.WithTimeout(ctx, w.timeout)
		days, err := w.source.GetWeek(weekCtx, w.groupID, date)
		cancel()
		progress := week*20 + 10
		if err != nil {
			slog.Error("load schedule week", "week", week+1, "error", err)
			slog.Info("schedule cache progress", "progress_percent", progress)
			continue
		}
		slog.Info("schedule cache progress", "progress_percent", progress)

		changes, err := w.cache.ApplyWeek(ctx, w.groupID, days)
		if err != nil {
			slog.Error("apply schedule week", "week", week+1, "error", err)
			slog.Info("schedule cache progress", "progress_percent", (week+1)*20)
			continue
		}
		w.syncCalendar(ctx, changes)
		slog.Info("schedule cache progress", "progress_percent", (week+1)*20, "changed_lessons", len(changes))
	}
}

func (w *Worker) syncCalendar(ctx context.Context, changes []schedule.LessonChange) {
	if w.calendar == nil {
		return
	}
	for _, change := range changes {
		if change.Before != nil {
			if err := w.calendar.DeleteLesson(ctx, *change.Before); err != nil {
				slog.Error("delete old lesson from Google Calendar", "error", err)
			}
		}
		if change.After != nil {
			if err := w.calendar.SyncLesson(ctx, *change.After); err != nil {
				slog.Error("sync changed lesson with Google Calendar", "error", err)
			}
		}
	}
}
