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

type BlacklistRule struct {
	ID      int64  `json:"id"`
	Subject string `json:"subject,omitempty"`
	Date    string `json:"date,omitempty"`
	Number  int    `json:"number,omitempty"`
}

type BlacklistRepository interface {
	ListBlacklist(ctx context.Context, groupID string) ([]BlacklistRule, error)
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
	blacklist  BlacklistRepository
}

func NewService(repository Repository, location *time.Location) *Service {
	return &Service{repository: repository, location: location}
}

func (s *Service) SetBlacklist(repository BlacklistRepository) {
	s.blacklist = repository
}

func (s *Service) FilterDays(ctx context.Context, groupID string, days []Day) ([]Day, error) {
	return s.filterDays(ctx, groupID, days)
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
	filtered, err := s.filterDays(ctx, groupID, days)
	if err != nil {
		return Day{}, err
	}
	return filtered[0], nil
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

	return s.filterDays(ctx, groupID, result)
}

func (s *Service) filterDays(ctx context.Context, groupID string, days []Day) ([]Day, error) {
	if s.blacklist == nil {
		return days, nil
	}
	rules, err := s.blacklist.ListBlacklist(ctx, groupID)
	if err != nil {
		return nil, err
	}
	filtered := make([]Day, len(days))
	for index, day := range days {
		filtered[index] = Day{Date: day.Date, Lessons: make([]Lesson, 0, len(day.Lessons))}
		for _, lesson := range day.Lessons {
			blocked := false
			for _, rule := range rules {
				if rule.Subject != "" && lesson.Title == rule.Subject {
					blocked = true
					break
				}
				if rule.Date == day.Date.Format("2006-01-02") && rule.Number == lesson.Number {
					blocked = true
					break
				}
			}
			if !blocked {
				filtered[index].Lessons = append(filtered[index].Lessons, lesson)
			}
		}
	}
	return filtered, nil
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
