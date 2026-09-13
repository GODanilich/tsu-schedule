package schedule

import (
	"context"
	"time"
)

type Lesson struct {
	ID        string    `json:"-"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Professor string    `json:"professor"`
	Audience  string    `json:"audience"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
}

type Day struct {
	Date    time.Time `json:"date"`
	Lessons []Lesson  `json:"lessons"`
}

type Repository interface {
	Get(ctx context.Context, groupID string, from, to time.Time) ([]Day, error)
}

type CacheRepository interface {
	Repository
	Replace(ctx context.Context, groupID string, days []Day) error
}

type LessonChange struct {
	Before *Lesson
	After  *Lesson
}

type DiffCacheRepository interface {
	CacheRepository
	ApplyWeek(ctx context.Context, groupID string, days []Day) ([]LessonChange, error)
}

type Service struct {
	repository Repository
	location   *time.Location
}

func NewService(repository Repository, location *time.Location) *Service {
	return &Service{repository: repository, location: location}
}

type Refresher struct {
	source   *Service
	cache    CacheRepository
	location *time.Location
}

func NewRefresher(source *Service, cache CacheRepository, location *time.Location) *Refresher {
	return &Refresher{source: source, cache: cache, location: location}
}

func (r *Refresher) RefreshWeek(ctx context.Context, groupID string, date time.Time) ([]Day, error) {
	days, err := r.source.GetWeek(ctx, groupID, date.In(r.location))
	if err != nil {
		return nil, err
	}
	if err := r.cache.Replace(ctx, groupID, days); err != nil {
		return nil, err
	}
	return days, nil
}

func (s *Service) GetToday(ctx context.Context, groupID string) (Day, error) {
	return s.GetDay(ctx, groupID, time.Now().In(s.location))
}

func (s *Service) GetDay(ctx context.Context, groupID string, date time.Time) (Day, error) {
	date = date.In(s.location)
	start := startOfDay(date)
	end := start.AddDate(0, 0, 1)

	days, err := s.repository.Get(ctx, groupID, start, end)
	if err != nil {
		return Day{}, err
	}
	if len(days) == 0 {
		return Day{Date: start, Lessons: []Lesson{}}, nil
	}
	return days[0], nil
}

func (s *Service) GetWeek(ctx context.Context, groupID string, date time.Time) ([]Day, error) {
	date = date.In(s.location)
	start := startOfWeek(date)
	end := start.AddDate(0, 0, 7)

	days, err := s.repository.Get(ctx, groupID, start, end)
	if err != nil {
		return nil, err
	}

	// Always return a complete Monday-Sunday week, including empty days.
	byDate := make(map[string]Day, len(days))
	for _, day := range days {
		byDate[day.Date.In(s.location).Format("2006-01-02")] = day
	}

	result := make([]Day, 0, 7)
	for i := 0; i < 7; i++ {
		current := start.AddDate(0, 0, i)
		key := current.Format("2006-01-02")
		if day, ok := byDate[key]; ok {
			result = append(result, day)
			continue
		}
		result = append(result, Day{Date: current, Lessons: []Lesson{}})
	}

	return result, nil
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func startOfWeek(t time.Time) time.Time {
	t = startOfDay(t)
	daysFromMonday := (int(t.Weekday()) + 6) % 7
	return t.AddDate(0, 0, -daysFromMonday)
}
