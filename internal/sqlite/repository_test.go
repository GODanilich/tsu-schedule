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

func TestBlacklistRules(t *testing.T) {
	location := time.FixedZone("Asia/Tomsk", 7*60*60)
	repository, err := New(t.TempDir() + "/schedule.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	day := time.Date(2026, time.September, 7, 0, 0, 0, 0, location)
	days := []schedule.Day{{Date: day, Lessons: []schedule.Lesson{
		{Number: 1, Title: "Математика", Start: day.Add(9 * time.Hour), End: day.Add(10 * time.Hour)},
		{Number: 2, Title: "Физика", Start: day.Add(11 * time.Hour), End: day.Add(12 * time.Hour)},
	}}}
	if err := repository.Replace(context.Background(), "group", days); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.AddBlacklist(context.Background(), "group", schedule.BlacklistRule{Subject: "Математика"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.AddBlacklist(context.Background(), "group", schedule.BlacklistRule{Date: "2026-09-07", Number: 2}); err != nil {
		t.Fatal(err)
	}

	service := schedule.NewService(repository, location)
	service.SetBlacklist(repository)
	filtered, err := service.GetWeek(context.Background(), "group", day)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 7 || len(filtered[0].Lessons) != 0 {
		t.Fatalf("expected all lessons to be filtered, got %#v", filtered[0])
	}
}

func TestBlacklistRemovalRestoresLesson(t *testing.T) {
	location := time.FixedZone("Asia/Tomsk", 7*60*60)
	repository, err := New(t.TempDir() + "/schedule.db")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	day := time.Date(2026, time.September, 7, 0, 0, 0, 0, location)
	lesson := schedule.Lesson{Number: 1, Title: "Математика", Start: day.Add(9 * time.Hour), End: day.Add(10 * time.Hour)}
	if err := repository.Replace(context.Background(), "group", []schedule.Day{{Date: day, Lessons: []schedule.Lesson{lesson}}}); err != nil {
		t.Fatal(err)
	}
	rule, err := repository.AddBlacklist(context.Background(), "group", schedule.BlacklistRule{Subject: lesson.Title})
	if err != nil {
		t.Fatal(err)
	}
	service := schedule.NewService(repository, location)
	service.SetBlacklist(repository)
	filtered, err := service.GetWeek(context.Background(), "group", day)
	if err != nil || len(filtered[0].Lessons) != 0 {
		t.Fatalf("expected lesson to be hidden, got %v, %#v", err, filtered[0])
	}
	if err := repository.DeleteBlacklist(context.Background(), "group", rule.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := service.GetWeek(context.Background(), "group", day)
	if err != nil || len(restored[0].Lessons) != 1 {
		t.Fatalf("expected lesson to be restored, got %v, %#v", err, restored[0])
	}
}
