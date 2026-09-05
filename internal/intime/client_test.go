package intime

import (
	"testing"
	"time"
)

func TestTimeFromSecondsConvertsUTCToConfiguredLocation(t *testing.T) {
	location, err := time.LoadLocation("Asia/Tomsk")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, time.September, 2, 0, 0, 0, 0, location)

	got := timeFromSeconds(date, 19_500, location)
	want := time.Date(2026, time.September, 2, 12, 25, 0, 0, location)
	if !got.Equal(want) {
		t.Fatalf("timeFromSeconds() = %s, want %s", got, want)
	}
}
