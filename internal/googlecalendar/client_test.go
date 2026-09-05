package googlecalendar

import (
	"testing"
	"time"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

func TestEventKeyKeepsIdentityWhenLessonTimeChanges(t *testing.T) {
	location := time.FixedZone("Asia/Tomsk", 7*60*60)
	client := &Client{timezone: location.String()}
	lesson := schedule.Lesson{
		Number:   2,
		Title:    "Математика",
		Start:    time.Date(2026, time.September, 7, 10, 25, 0, 0, location),
		End:      time.Date(2026, time.September, 7, 12, 0, 0, 0, location),
		Audience: "101",
	}

	first := client.eventFor(lesson)
	lesson.Start = lesson.Start.Add(15 * time.Minute)
	second := client.eventFor(lesson)

	if first.Id != "" || second.Id != "" {
		t.Fatal("Google Calendar must assign event IDs when creating events")
	}
	if first.ExtendedProperties.Private["lesson_key"] != second.ExtendedProperties.Private["lesson_key"] {
		t.Fatal("lesson key changed after lesson time update")
	}
	if first.ExtendedProperties.Private["source"] != "tsu-schedule" {
		t.Fatal("event is not marked as created by tsu-schedule")
	}
}
