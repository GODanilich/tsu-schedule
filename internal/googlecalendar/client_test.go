package googlecalendar

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/api/googleapi"

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

func TestSourcePropertyMatchesEventMetadata(t *testing.T) {
	if sourceProperty != "source=tsu-schedule" {
		t.Fatalf("unexpected Google Calendar property filter: %q", sourceProperty)
	}
}

func TestRetryableGoogleErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "rate limit", err: &googleapi.Error{Code: 429}, want: true},
		{name: "server", err: &googleapi.Error{Code: 503}, want: true},
		{name: "quota reason", err: &googleapi.Error{Code: 403, Errors: []googleapi.ErrorItem{{Reason: "userRateLimitExceeded"}}}, want: true},
		{name: "forbidden", err: &googleapi.Error{Code: 403}, want: false},
		{name: "other", err: fmt.Errorf("rate limit"), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isRetryableGoogleError(test.err); got != test.want {
				t.Fatalf("isRetryableGoogleError() = %v, want %v", got, test.want)
			}
		})
	}
}
