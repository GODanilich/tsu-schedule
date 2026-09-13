package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

func TestApplyWeekIsIdempotentWithoutSourceID(t *testing.T) {
	location := time.FixedZone("Asia/Tomsk", 7*60*60)
	repository, err := New(t.TempDir() + "/schedule.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	day := time.Date(2026, time.September, 7, 0, 0, 0, 0, location)
	lesson := schedule.Lesson{
		Number: 1,
		Title:  "Математика",
		Type:   "Лекция",
		Start:  day.Add(9 * time.Hour),
		End:    day.Add(10 * time.Hour),
	}
	days := []schedule.Day{{Date: day, Lessons: []schedule.Lesson{lesson}}}

	if _, err := repository.ApplyWeek(context.Background(), "group", days); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyWeek(context.Background(), "group", days); err != nil {
		t.Fatal(err)
	}

	stored, err := repository.Get(context.Background(), "group", day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || len(stored[0].Lessons) != 1 {
		t.Fatalf("expected one stored lesson, got %#v", stored)
	}
}
