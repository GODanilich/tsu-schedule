package googlecalendar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

const calendarScope = "https://www.googleapis.com/auth/calendar.events"
const sourceProperty = "tsu-schedule=true"

type Client struct {
	service    *calendar.Service
	calendarID string
	timezone   string
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
	for _, eventID := range existing.legacy {
		if err := c.service.Events.Delete(c.calendarID, eventID).Context(ctx).Do(); err != nil {
			return 0, fmt.Errorf("delete legacy lesson event: %w", err)
		}
	}

	for _, day := range days {
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
			_, err = c.service.Events.Update(c.calendarID, eventID, event).Context(ctx).Do()
		} else {
			_, err = c.service.Events.Insert(c.calendarID, event).Context(ctx).Do()
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
		if err := c.service.Events.Delete(c.calendarID, eventID).Context(ctx).Do(); err != nil {
			return synced, fmt.Errorf("delete outdated lesson event: %w", err)
		}
	}
	return synced, nil
}

// Clear removes every event from the configured calendar.
func (c *Client) Clear(ctx context.Context) (int, error) {
	eventIDs, err := c.eventIDs(ctx)
	if err != nil {
		return 0, err
	}

	for _, eventID := range eventIDs {
		if err := c.service.Events.Delete(c.calendarID, eventID).Context(ctx).Do(); err != nil {
			return 0, fmt.Errorf("delete calendar event: %w", err)
		}
	}

	return len(eventIDs), nil
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

	call := c.service.Events.List(c.calendarID).
		TimeMin(from.Format(time.RFC3339)).
		TimeMax(to.AddDate(0, 0, 1).Format(time.RFC3339)).
		PrivateExtendedProperty(sourceProperty).
		SingleEvents(true).
		ShowDeleted(false)
	for {
		response, err := call.Context(ctx).Do()
		if err != nil {
			return existingEvents{}, fmt.Errorf("list Google Calendar events: %w", err)
		}
		for _, event := range response.Items {
			if event.ExtendedProperties == nil {
				continue
			}
			key := event.ExtendedProperties.Private["lesson_key"]
			if key != "" {
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

func (c *Client) eventIDs(ctx context.Context) ([]string, error) {
	var eventIDs []string
	call := c.service.Events.List(c.calendarID).ShowDeleted(false)
	for {
		response, err := call.Context(ctx).Do()
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
