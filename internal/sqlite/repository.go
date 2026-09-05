package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

type Repository struct {
	db *sql.DB
}

func New(path string) (*Repository, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		CREATE TABLE IF NOT EXISTS lessons (
			group_id TEXT NOT NULL,
			day_date TEXT NOT NULL,
			number INTEGER NOT NULL,
			title TEXT NOT NULL,
			type TEXT NOT NULL,
			professor TEXT NOT NULL,
			audience TEXT NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			PRIMARY KEY (group_id, day_date, number, start_time)
		);
		CREATE INDEX IF NOT EXISTS lessons_range_idx ON lessons (group_id, start_time);
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize SQLite database: %w", err)
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Get(ctx context.Context, groupID string, from, to time.Time) ([]schedule.Day, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT day_date, number, title, type, professor, audience, start_time, end_time
		FROM lessons
		WHERE group_id = ? AND start_time >= ? AND start_time < ?
		ORDER BY start_time, number
	`, groupID, from.Format(time.RFC3339), to.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("query SQLite schedule: %w", err)
	}
	defer rows.Close()

	byDate := make(map[string]*schedule.Day)
	order := make([]string, 0)
	for rows.Next() {
		var dayDate, startValue, endValue string
		var lesson schedule.Lesson
		if err := rows.Scan(&dayDate, &lesson.Number, &lesson.Title, &lesson.Type, &lesson.Professor, &lesson.Audience, &startValue, &endValue); err != nil {
			return nil, fmt.Errorf("scan SQLite schedule: %w", err)
		}
		lesson.Start, err = time.Parse(time.RFC3339, startValue)
		if err != nil {
			return nil, fmt.Errorf("parse lesson start: %w", err)
		}
		lesson.End, err = time.Parse(time.RFC3339, endValue)
		if err != nil {
			return nil, fmt.Errorf("parse lesson end: %w", err)
		}
		day, ok := byDate[dayDate]
		if !ok {
			parsedDate, parseErr := time.ParseInLocation("2006-01-02", dayDate, from.Location())
			if parseErr != nil {
				return nil, fmt.Errorf("parse lesson date: %w", parseErr)
			}
			day = &schedule.Day{Date: parsedDate, Lessons: []schedule.Lesson{}}
			byDate[dayDate] = day
			order = append(order, dayDate)
		}
		day.Lessons = append(day.Lessons, lesson)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SQLite schedule: %w", err)
	}

	result := make([]schedule.Day, 0, len(order))
	for _, dayDate := range order {
		result = append(result, *byDate[dayDate])
	}
	return result, nil
}

func (r *Repository) Replace(ctx context.Context, groupID string, days []schedule.Day) error {
	if len(days) == 0 {
		return nil
	}
	from := days[0].Date
	to := from.AddDate(0, 0, len(days))
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite schedule update: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM lessons WHERE group_id = ? AND start_time >= ? AND start_time < ?`, groupID, from.Format(time.RFC3339), to.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("clear SQLite schedule week: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO lessons (group_id, day_date, number, title, type, professor, audience, start_time, end_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare SQLite schedule update: %w", err)
	}
	defer stmt.Close()

	for _, day := range days {
		for _, lesson := range day.Lessons {
			if _, err := stmt.ExecContext(ctx, groupID, day.Date.Format("2006-01-02"), lesson.Number, lesson.Title, lesson.Type, lesson.Professor, lesson.Audience, lesson.Start.Format(time.RFC3339), lesson.End.Format(time.RFC3339)); err != nil {
				return fmt.Errorf("save lesson in SQLite: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SQLite schedule update: %w", err)
	}
	return nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}
