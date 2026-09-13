package googlecalendar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

const calendarScope = "https://www.googleapis.com/auth/calendar.events"
const sourceProperty = "source=tsu-schedule"

const (
	maxGoogleAPIRetries = 5
	googleRetryBase     = 200 * time.Millisecond
	googleRetryMax      = 5 * time.Second
)

type Client struct {
	service    *calendar.Service
	calendarID string
	timezone   string
}

type Event struct {
	ID          string    `json:"id,omitempty"`
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	Location    string    `json:"location,omitempty"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
}

type existingEvents struct {
	byKey  map[string]string
	legacy []string
}

func NewClient(credentialsFile, calendarID, timezone string) (*Client, error) {
	if calendarID == "" {
		return nil, fmt.Errorf("GOOGLE_CALENDAR_ID is required")
	}

	credentials, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read Google service account key %q: %w", credentialsFile, err)
	}
	serviceAccountConfig, err := google.JWTConfigFromJSON(credentials, calendarScope)
	if err != nil {
		return nil, fmt.Errorf("parse Google service account key: %w", err)
	}

	ctx := context.Background()
	service, err := calendar.NewService(ctx, option.WithTokenSource(serviceAccountConfig.TokenSource(ctx)))
	if err != nil {
		return nil, fmt.Errorf("create Google Calendar service: %w", err)
	}

	return &Client{service: service, calendarID: calendarID, timezone: timezone}, nil
}

func (c *Client) Sync(ctx context.Context, days []schedule.Day) (int, error) {
	existing, err := c.eventsCreatedByApp(ctx, days)
	if err != nil {
		return 0, err
	}

	desired := make(map[string]*calendar.Event)
	location := c.calendarLocation()
	currentWeekStart := startOfWeek(time.Now().In(location))
	for _, eventID := range existing.legacy {
		if err := retryGoogleAPI(ctx, func() error {
			return c.service.Events.Delete(c.calendarID, eventID).Context(ctx).Do()
		}); err != nil {
			return 0, fmt.Errorf("delete legacy lesson event: %w", err)
		}
	}

	for _, day := range days {
		if day.Date.Before(currentWeekStart) {
			continue
		}
		for _, lesson := range day.Lessons {
			event := c.eventFor(lesson)
			desired[eventKey(lesson)] = event
		}
	}

	synced := 0
	for key, event := range desired {
		var err error
		if eventID, ok := existing.byKey[key]; ok {
			event.Id = eventID
			err = retryGoogleAPI(ctx, func() error {
				_, err := c.service.Events.Update(c.calendarID, eventID, event).Context(ctx).Do()
				return err
			})
		} else {
			err = retryGoogleAPI(ctx, func() error {
				_, err := c.service.Events.Insert(c.calendarID, event).Context(ctx).Do()
				return err
			})
		}
		if err != nil {
			return synced, fmt.Errorf("sync lesson %q: %w", event.Summary, err)
		}
		synced++
	}

	for key, eventID := range existing.byKey {
		if _, ok := desired[key]; ok {
			continue
		}
		if err := retryGoogleAPI(ctx, func() error {
			return c.service.Events.Delete(c.calendarID, eventID).Context(ctx).Do()
		}); err != nil {
			return synced, fmt.Errorf("delete outdated lesson event: %w", err)
		}
	}
	return synced, nil
}

// SyncLesson creates or updates exactly one lesson event.
func (c *Client) SyncLesson(ctx context.Context, lesson schedule.Lesson) error {
	existing, err := c.eventsCreatedByApp(ctx, []schedule.Day{{Date: lesson.Start, Lessons: []schedule.Lesson{lesson}}})
	if err != nil {
		return err
	}
	for _, eventID := range existing.legacy {
		if err := retryGoogleAPI(ctx, func() error {
			return c.service.Events.Delete(c.calendarID, eventID).Context(ctx).Do()
		}); err != nil {
			return fmt.Errorf("delete legacy lesson event: %w", err)
		}
	}

	key := eventKey(lesson)
	event := c.eventFor(lesson)
	if eventID, ok := existing.byKey[key]; ok {
		event.Id = eventID
		if err := retryGoogleAPI(ctx, func() error {
			_, err := c.service.Events.Update(c.calendarID, eventID, event).Context(ctx).Do()
			return err
		}); err != nil {
			return fmt.Errorf("update lesson %q: %w", lesson.Title, err)
		}
		return nil
	}
	if err := retryGoogleAPI(ctx, func() error {
		_, err := c.service.Events.Insert(c.calendarID, event).Context(ctx).Do()
		return err
	}); err != nil {
		return fmt.Errorf("create lesson %q: %w", lesson.Title, err)
	}
	return nil
}

// DeleteLesson removes only the event corresponding to the given lesson.
func (c *Client) DeleteLesson(ctx context.Context, lesson schedule.Lesson) error {
	existing, err := c.eventsCreatedByApp(ctx, []schedule.Day{{Date: lesson.Start, Lessons: []schedule.Lesson{lesson}}})
	if err != nil {
		return err
	}
	eventID, ok := existing.byKey[eventKey(lesson)]
	if !ok {
		return nil
	}
	if err := retryGoogleAPI(ctx, func() error {
		return c.service.Events.Delete(c.calendarID, eventID).Context(ctx).Do()
	}); err != nil {
		return fmt.Errorf("delete lesson %q: %w", lesson.Title, err)
	}
	return nil
}

// Clear removes every event from the configured calendar.
func (c *Client) Clear(ctx context.Context) (int, error) {
	eventIDs, err := c.eventIDs(ctx)
	if err != nil {
		return 0, err
	}

	for _, eventID := range eventIDs {
		if err := retryGoogleAPI(ctx, func() error {
			return c.service.Events.Delete(c.calendarID, eventID).Context(ctx).Do()
		}); err != nil {
			return 0, fmt.Errorf("delete calendar event: %w", err)
		}
	}

	return len(eventIDs), nil
}

func (c *Client) CreateEvent(ctx context.Context, event Event) (Event, error) {
	var created *calendar.Event
	err := retryGoogleAPI(ctx, func() error {
		var err error
		created, err = c.service.Events.Insert(c.calendarID, c.eventFrom(event)).Context(ctx).Do()
		return err
	})
	if err != nil {
		return Event{}, fmt.Errorf("create calendar event: %w", err)
	}
	return c.eventTo(created), nil
}

func (c *Client) GetEvent(ctx context.Context, id string) (Event, error) {
	var event *calendar.Event
	err := retryGoogleAPI(ctx, func() error {
		var err error
		event, err = c.service.Events.Get(c.calendarID, id).Context(ctx).Do()
		return err
	})
	if err != nil {
		return Event{}, fmt.Errorf("get calendar event: %w", err)
	}
	return c.eventTo(event), nil
}

func (c *Client) UpdateEvent(ctx context.Context, id string, event Event) (Event, error) {
	var updated *calendar.Event
	err := retryGoogleAPI(ctx, func() error {
		var err error
		updated, err = c.service.Events.Update(c.calendarID, id, c.eventFrom(event)).Context(ctx).Do()
		return err
	})
	if err != nil {
		return Event{}, fmt.Errorf("update calendar event: %w", err)
	}
	return c.eventTo(updated), nil
}

func (c *Client) DeleteEvent(ctx context.Context, id string) error {
	if err := retryGoogleAPI(ctx, func() error {
		return c.service.Events.Delete(c.calendarID, id).Context(ctx).Do()
	}); err != nil {
		return fmt.Errorf("delete calendar event: %w", err)
	}
	return nil
}

func (c *Client) eventFrom(event Event) *calendar.Event {
	return &calendar.Event{
		Summary: event.Summary, Description: event.Description, Location: event.Location,
		Start: &calendar.EventDateTime{DateTime: event.Start.Format(time.RFC3339), TimeZone: c.timezone},
		End:   &calendar.EventDateTime{DateTime: event.End.Format(time.RFC3339), TimeZone: c.timezone},
	}
}

func (c *Client) eventTo(event *calendar.Event) Event {
	result := Event{ID: event.Id, Summary: event.Summary, Description: event.Description, Location: event.Location}
	if event.Start != nil {
		result.Start, _ = time.Parse(time.RFC3339, event.Start.DateTime)
	}
	if event.End != nil {
		result.End, _ = time.Parse(time.RFC3339, event.End.DateTime)
	}
	return result
}

func (c *Client) eventFor(lesson schedule.Lesson) *calendar.Event {
	return &calendar.Event{
		Summary: lesson.Title,
		Description: fmt.Sprintf("Тип: %s\nПреподаватель: %s\nАудитория: %s\n\nСоздано tsu-schedule.",
			lesson.Type, lesson.Professor, lesson.Audience),
		Location: lesson.Audience,
		Start:    &calendar.EventDateTime{DateTime: lesson.Start.Format(time.RFC3339), TimeZone: c.timezone},
		End:      &calendar.EventDateTime{DateTime: lesson.End.Format(time.RFC3339), TimeZone: c.timezone},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"source":     "tsu-schedule",
				"lesson_key": eventKey(lesson),
			},
		},
	}
}

func eventKey(lesson schedule.Lesson) string {
	if lesson.ID != "" {
		return lesson.ID
	}
	value := lesson.Start.Format("2006-01-02") + "\x00" + fmt.Sprint(lesson.Number) + "\x00" + lesson.Title + "\x00" + lesson.Professor + "\x00" + lesson.Audience
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func (c *Client) eventsCreatedByApp(ctx context.Context, days []schedule.Day) (existingEvents, error) {
	existing := existingEvents{byKey: make(map[string]string)}
	if len(days) == 0 {
		return existing, nil
	}

	from := days[0].Date
	to := from
	for _, day := range days[1:] {
		if day.Date.Before(from) {
			from = day.Date
		}
		if day.Date.After(to) {
			to = day.Date
		}
	}
	// Automatic reconciliation must never touch events from completed weeks.
	// The worker starts at the current week, but this also protects manual syncs
	// and future changes to the worker's date range.
	location := c.calendarLocation()
	currentWeekStart := startOfWeek(time.Now().In(location))
	if from.Before(currentWeekStart) {
		from = currentWeekStart
	}
	if !from.Before(to.AddDate(0, 0, 1)) {
		return existing, nil
	}

	call := c.service.Events.List(c.calendarID).
		TimeMin(from.Format(time.RFC3339)).
		TimeMax(to.AddDate(0, 0, 1).Format(time.RFC3339)).
		PrivateExtendedProperty(sourceProperty).
		SingleEvents(true).
		ShowDeleted(false)
	for {
		var response *calendar.Events
		err := retryGoogleAPI(ctx, func() error {
			var err error
			response, err = call.Context(ctx).Do()
			return err
		})
		if err != nil {
			return existingEvents{}, fmt.Errorf("list Google Calendar events: %w", err)
		}
		for _, event := range response.Items {
			if event.ExtendedProperties == nil {
				continue
			}
			key := event.ExtendedProperties.Private["lesson_key"]
			if key != "" {
				if previousID, exists := existing.byKey[key]; exists {
					existing.legacy = append(existing.legacy, previousID)
				}
				existing.byKey[key] = event.Id
				continue
			}
			existing.legacy = append(existing.legacy, event.Id)
		}
		if response.NextPageToken == "" {
			return existing, nil
		}
		call.PageToken(response.NextPageToken)
	}
}

func (c *Client) calendarLocation() *time.Location {
	location, err := time.LoadLocation(c.timezone)
	if err != nil {
		return time.UTC
	}
	return location
}

func startOfWeek(value time.Time) time.Time {
	value = time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	daysFromMonday := (int(value.Weekday()) + 6) % 7
	return value.AddDate(0, 0, -daysFromMonday)
}

func retryGoogleAPI(ctx context.Context, operation func() error) error {
	for attempt := 0; ; attempt++ {
		err := operation()
		if err == nil || !isRetryableGoogleError(err) || attempt >= maxGoogleAPIRetries {
			return err
		}

		backoff := googleRetryBase << attempt
		if backoff > googleRetryMax {
			backoff = googleRetryMax
		}
		// Full jitter avoids synchronized retries when several requests hit the limit together.
		wait := time.Duration(rand.Int63n(int64(backoff) + 1))
		slog.Warn("Google Calendar API request throttled; retrying", "attempt", attempt+1, "wait", wait, "error", err)
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func isRetryableGoogleError(err error) bool {
	var apiError *googleapi.Error
	if !errors.As(err, &apiError) {
		return false
	}
	if apiError.Code == 429 || apiError.Code >= 500 {
		return true
	}
	if apiError.Code != 403 {
		return false
	}
	for _, reason := range apiError.Errors {
		if reason.Reason == "rateLimitExceeded" || reason.Reason == "userRateLimitExceeded" || reason.Reason == "backendError" {
			return true
		}
	}
	return false
}

func (c *Client) eventIDs(ctx context.Context) ([]string, error) {
	var eventIDs []string
	call := c.service.Events.List(c.calendarID).ShowDeleted(false)
	for {
		var response *calendar.Events
		err := retryGoogleAPI(ctx, func() error {
			var err error
			response, err = call.Context(ctx).Do()
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("list Google Calendar events: %w", err)
		}
		for _, event := range response.Items {
			eventIDs = append(eventIDs, event.Id)
		}
		if response.NextPageToken == "" {
			return eventIDs, nil
		}
		call.PageToken(response.NextPageToken)
	}
}
