package syncworker

import (
	"context"
	"log/slog"
	"time"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

const weeksToSync = 5

type Calendar interface {
	Sync(ctx context.Context, days []schedule.Day) (int, error)
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
	visible  *schedule.Service
	initial  bool
}

func New(source *schedule.Service, cache schedule.DiffCacheRepository, calendar Calendar, groupID string, location *time.Location, timeout time.Duration) *Worker {
	return &Worker{source: source, cache: cache, calendar: calendar, groupID: groupID, location: location, timeout: timeout, initial: true}
}

func (w *Worker) SetCalendarService(service *schedule.Service) {
	w.visible = service
}

// Run synchronizes the current week and four following weeks one week at a time.
func (w *Worker) Run(ctx context.Context) {
	now := time.Now().In(w.location)
	initial := w.initial
	for week := 0; week < weeksToSync; week++ {
		date := now.AddDate(0, 0, week*7)
		weekCtx, cancel := context.WithTimeout(ctx, w.timeout)
		days, err := w.source.GetWeek(weekCtx, w.groupID, date)
		cancel()
		if err != nil {
			slog.Error("load schedule week", "week", week+1, "error", err)
			continue
		}

		changes, err := w.cache.ApplyWeek(ctx, w.groupID, days)
		if err != nil {
			slog.Error("apply schedule week", "week", week+1, "error", err)
			continue
		}
		if initial {
			w.syncCalendarWeek(ctx, date)
		} else {
			w.syncCalendar(ctx, changes)
		}
		slog.Info("schedule week loaded", "week", week+1, "week_start", date.Format("2006-01-02"), "changed_lessons", len(changes))
	}
	w.initial = false
}

func (w *Worker) syncCalendarWeek(ctx context.Context, date time.Time) {
	if w.calendar == nil || w.visible == nil {
		return
	}
	days, err := w.visible.GetWeek(ctx, w.groupID, date)
	if err != nil {
		slog.Error("load cached week for initial calendar sync", "error", err)
		return
	}
	if _, err := w.calendar.Sync(ctx, days); err != nil {
		slog.Error("initially sync calendar week", "error", err)
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
		if change.After != nil && w.lessonVisible(ctx, *change.After) {
			if err := w.calendar.SyncLesson(ctx, *change.After); err != nil {
				slog.Error("sync changed lesson with Google Calendar", "error", err)
			}
		}
	}
}

func (w *Worker) lessonVisible(ctx context.Context, lesson schedule.Lesson) bool {
	if w.visible == nil {
		return true
	}
	days, err := w.visible.FilterDays(ctx, w.groupID, []schedule.Day{{Date: lesson.Start, Lessons: []schedule.Lesson{lesson}}})
	if err != nil {
		slog.Error("check lesson blacklist", "error", err)
		return false
	}
	return len(days) > 0 && len(days[0].Lessons) > 0
}
