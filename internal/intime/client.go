package intime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	location   *time.Location
}

func NewClient(baseURL string, httpClient *http.Client, location *time.Location) *Client {
	return &Client{baseURL: baseURL, httpClient: httpClient, location: location}
}

type apiResponse struct {
	Grid []apiDay `json:"grid"`
}

type apiDay struct {
	Date    string      `json:"date"`
	Lessons []apiLesson `json:"lessons"`
}

type apiLesson struct {
	Type         string        `json:"type"`
	LessonNumber int           `json:"lessonNumber"`
	Title        string        `json:"title"`
	LessonType   string        `json:"lessonType"`
	Starts       int64         `json:"starts"`
	Ends         int64         `json:"ends"`
	Professor    *apiProfessor `json:"professor"`
	Audience     *apiAudience  `json:"audience"`
}

type apiProfessor struct {
	FullName string `json:"fullName"`
}

type apiAudience struct {
	Name string `json:"name"`
}

func (c *Client) Get(ctx context.Context, groupID string, from, to time.Time) ([]schedule.Day, error) {
	endpoint, err := url.Parse(c.baseURL + "/schedule/group")
	if err != nil {
		return nil, fmt.Errorf("parse InTime URL: %w", err)
	}

	query := endpoint.Query()
	query.Set("id", groupID)
	query.Set("dateFrom", from.Format("2006-01-02"))
	query.Set("dateTo", to.AddDate(0, 0, -1).Format("2006-01-02"))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create InTime request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request InTime: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("InTime returned HTTP %d", resp.StatusCode)
	}

	var data apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode InTime response: %w", err)
	}

	return c.convert(data)
}

func (c *Client) convert(data apiResponse) ([]schedule.Day, error) {
	result := make([]schedule.Day, 0, len(data.Grid))

	for _, day := range data.Grid {
		date, err := parseDate(day.Date, c.location)
		if err != nil {
			return nil, err
		}

		resultDay := schedule.Day{
			Date:    date,
			Lessons: make([]schedule.Lesson, 0, len(day.Lessons)),
		}

		for _, lesson := range day.Lessons {
			if lesson.Type == "EMPTY" {
				continue
			}

			resultDay.Lessons = append(resultDay.Lessons, schedule.Lesson{
				Number:    lesson.LessonNumber,
				Title:     lesson.Title,
				Type:      lesson.LessonType,
				Professor: professorName(lesson.Professor),
				Audience:  audienceName(lesson.Audience),
				Start:     timeFromSeconds(date, lesson.Starts, c.location),
				End:       timeFromSeconds(date, lesson.Ends, c.location),
			})
		}

		result = append(result, resultDay)
	}

	return result, nil
}

func parseDate(value string, location *time.Location) (time.Time, error) {
	date, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse InTime date %q: %w", value, err)
	}
	return date, nil
}

func timeFromSeconds(date time.Time, seconds int64, location *time.Location) time.Time {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	seconds %= 60

	return time.Date(
		date.Year(), date.Month(), date.Day(),
		int(hours), int(minutes), int(seconds), 0, location,
	)
}

func professorName(professor *apiProfessor) string {
	if professor == nil || professor.FullName == "" {
		return "Не указан"
	}
	return professor.FullName
}

func audienceName(audience *apiAudience) string {
	if audience == nil || audience.Name == "" {
		return "Не указана"
	}
	return audience.Name
}
