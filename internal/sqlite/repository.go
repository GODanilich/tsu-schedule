package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/GODanilich/tsu-schedule/internal/schedule"
)

type Repository struct{ db *sql.DB }

type storedLesson struct {
	rowID  int64
	day    time.Time
	lesson schedule.Lesson
}

type storedWeek struct {
	byID   map[string]storedLesson
	bySlot map[string]storedLesson
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
			group_id TEXT NOT NULL, source_id TEXT NOT NULL DEFAULT '', day_date TEXT NOT NULL,
			number INTEGER NOT NULL, title TEXT NOT NULL, type TEXT NOT NULL, professor TEXT NOT NULL,
			audience TEXT NOT NULL, start_time TEXT NOT NULL, end_time TEXT NOT NULL,
			PRIMARY KEY (group_id, day_date, number, start_time)
		);
		CREATE INDEX IF NOT EXISTS lessons_range_idx ON lessons (group_id, start_time);
		CREATE TABLE IF NOT EXISTS blacklist_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			group_id TEXT NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			day_date TEXT NOT NULL DEFAULT '',
			lesson_number INTEGER NOT NULL DEFAULT 0,
			CHECK ((subject != '' AND day_date = '' AND lesson_number = 0) OR
			       (subject = '' AND day_date != '' AND lesson_number > 0)),
			UNIQUE (group_id, subject, day_date, lesson_number)
		);
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize SQLite database: %w", err)
	}
	if err := ensureSourceIDColumn(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS lessons_source_id_idx ON lessons (group_id, source_id) WHERE source_id != ''`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create SQLite source ID index: %w", err)
	}
	return &Repository{db: db}, nil
}

func (r *Repository) ListBlacklist(ctx context.Context, groupID string) ([]schedule.BlacklistRule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, subject, day_date, lesson_number FROM blacklist_rules WHERE group_id = ? ORDER BY id`, groupID)
	if err != nil {
		return nil, fmt.Errorf("query SQLite blacklist: %w", err)
	}
	defer rows.Close()
	var rules []schedule.BlacklistRule
	for rows.Next() {
		var rule schedule.BlacklistRule
		if err := rows.Scan(&rule.ID, &rule.Subject, &rule.Date, &rule.Number); err != nil {
			return nil, fmt.Errorf("scan SQLite blacklist: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SQLite blacklist: %w", err)
	}
	return rules, nil
}

func (r *Repository) AddBlacklist(ctx context.Context, groupID string, rule schedule.BlacklistRule) (schedule.BlacklistRule, error) {
	if rule.Subject == "" && (rule.Date == "" || rule.Number <= 0) {
		return schedule.BlacklistRule{}, fmt.Errorf("blacklist rule must contain subject or date and positive number")
	}
	if rule.Subject != "" && (rule.Date != "" || rule.Number != 0) {
		return schedule.BlacklistRule{}, fmt.Errorf("subject rule cannot contain date or number")
	}
	if rule.Date != "" {
		if _, err := time.Parse("2006-01-02", rule.Date); err != nil {
			return schedule.BlacklistRule{}, fmt.Errorf("blacklist date must have format YYYY-MM-DD")
		}
	}
	insertResult, err := r.db.ExecContext(ctx, `INSERT INTO blacklist_rules (group_id, subject, day_date, lesson_number) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`, groupID, rule.Subject, rule.Date, rule.Number)
	if err != nil {
		return schedule.BlacklistRule{}, fmt.Errorf("insert SQLite blacklist rule: %w", err)
	}
	insertedID, _ := insertResult.LastInsertId()
	if insertedID != 0 {
		rule.ID = insertedID
		return rule, nil
	}
	rows, err := r.ListBlacklist(ctx, groupID)
	if err != nil {
		return schedule.BlacklistRule{}, err
	}
	for _, existing := range rows {
		if existing.Subject == rule.Subject && existing.Date == rule.Date && existing.Number == rule.Number {
			return existing, nil
		}
	}
	return schedule.BlacklistRule{}, fmt.Errorf("blacklist rule was not saved")
}

func (r *Repository) DeleteBlacklist(ctx context.Context, groupID string, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM blacklist_rules WHERE group_id = ? AND id = ?`, groupID, id)
	if err != nil {
		return fmt.Errorf("delete SQLite blacklist rule: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return fmt.Errorf("blacklist rule %d not found", id)
	}
	return nil
}

func ensureSourceIDColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(lessons)`)
	if err != nil {
		return fmt.Errorf("inspect SQLite lessons schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("read SQLite lessons schema: %w", err)
		}
		if name == "source_id" {
			return nil
		}
	}
	if _, err := db.Exec(`ALTER TABLE lessons ADD COLUMN source_id TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add SQLite source ID column: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, groupID string, from, to time.Time) ([]schedule.Day, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT source_id, day_date, number, title, type, professor, audience, start_time, end_time
		FROM lessons WHERE group_id = ? AND start_time >= ? AND start_time < ? ORDER BY start_time, number
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
		if err := rows.Scan(&lesson.ID, &dayDate, &lesson.Number, &lesson.Title, &lesson.Type, &lesson.Professor, &lesson.Audience, &startValue, &endValue); err != nil {
			return nil, fmt.Errorf("scan SQLite schedule: %w", err)
		}
		if lesson.Start, err = time.Parse(time.RFC3339, startValue); err != nil {
			return nil, fmt.Errorf("parse lesson start: %w", err)
		}
		if lesson.End, err = time.Parse(time.RFC3339, endValue); err != nil {
			return nil, fmt.Errorf("parse lesson end: %w", err)
		}
		day, ok := byDate[dayDate]
		if !ok {
			date, parseErr := time.ParseInLocation("2006-01-02", dayDate, from.Location())
			if parseErr != nil {
				return nil, fmt.Errorf("parse lesson date: %w", parseErr)
			}
			day = &schedule.Day{Date: date, Lessons: []schedule.Lesson{}}
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
	_, err := r.ApplyWeek(ctx, groupID, days)
	return err
}

func (r *Repository) ApplyWeek(ctx context.Context, groupID string, days []schedule.Day) ([]schedule.LessonChange, error) {
	if len(days) == 0 {
		return nil, nil
	}
	from := days[0].Date
	to := from.AddDate(0, 0, 7)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin SQLite schedule update: %w", err)
	}
	defer tx.Rollback()
	existing, err := loadWeek(ctx, tx, groupID, from, to)
	if err != nil {
		return nil, err
	}
	changes := make([]schedule.LessonChange, 0)
	seen := make(map[int64]struct{})
	seenSlots := make(map[string]struct{})
	for _, day := range days {
		for _, lesson := range day.Lessons {
			slotKey := lessonSlotKey(day.Date, lesson)
			if _, duplicate := seenSlots[slotKey]; duplicate {
				continue
			}
			seenSlots[slotKey] = struct{}{}

			stored, found := existing.byID[lesson.ID]
			if !found {
				stored, found = existing.bySlot[slotKey]
			}
			if !found {
				if err := insertLesson(ctx, tx, groupID, day.Date, lesson); err != nil {
					return nil, err
				}
				after := lesson
				changes = append(changes, schedule.LessonChange{After: &after})
				continue
			}
			seen[stored.rowID] = struct{}{}
			if lessonsEqual(stored.lesson, lesson) && stored.day.Equal(day.Date) {
				continue
			}
			if err := updateLesson(ctx, tx, stored.rowID, day.Date, lesson); err != nil {
				return nil, err
			}
			before, after := stored.lesson, lesson
			changes = append(changes, schedule.LessonChange{Before: &before, After: &after})
		}
	}
	for _, stored := range existing.bySlot {
		if _, ok := seen[stored.rowID]; ok {
			continue
		}
		if err := deleteLesson(ctx, tx, stored.rowID); err != nil {
			return nil, err
		}
		before := stored.lesson
		changes = append(changes, schedule.LessonChange{Before: &before})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit SQLite schedule update: %w", err)
	}
	return changes, nil
}

func loadWeek(ctx context.Context, tx *sql.Tx, groupID string, from, to time.Time) (storedWeek, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT rowid, source_id, day_date, number, title, type, professor, audience, start_time, end_time
		FROM lessons WHERE group_id = ? AND day_date >= ? AND day_date < ?
	`, groupID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return storedWeek{}, fmt.Errorf("query SQLite schedule week: %w", err)
	}
	defer rows.Close()
	result := storedWeek{byID: make(map[string]storedLesson), bySlot: make(map[string]storedLesson)}
	for rows.Next() {
		var stored storedLesson
		var dayDate, startValue, endValue string
		if err := rows.Scan(&stored.rowID, &stored.lesson.ID, &dayDate, &stored.lesson.Number, &stored.lesson.Title, &stored.lesson.Type, &stored.lesson.Professor, &stored.lesson.Audience, &startValue, &endValue); err != nil {
			return storedWeek{}, fmt.Errorf("scan SQLite schedule week: %w", err)
		}
		var parseErr error
		stored.day, parseErr = time.ParseInLocation("2006-01-02", dayDate, from.Location())
		if parseErr != nil {
			return storedWeek{}, fmt.Errorf("parse SQLite lesson date: %w", parseErr)
		}
		stored.lesson.Start, parseErr = time.Parse(time.RFC3339, startValue)
		if parseErr != nil {
			return storedWeek{}, fmt.Errorf("parse SQLite lesson start: %w", parseErr)
		}
		stored.lesson.End, parseErr = time.Parse(time.RFC3339, endValue)
		if parseErr != nil {
			return storedWeek{}, fmt.Errorf("parse SQLite lesson end: %w", parseErr)
		}
		result.bySlot[lessonSlotKey(stored.day, stored.lesson)] = stored
		if stored.lesson.ID != "" {
			result.byID[stored.lesson.ID] = stored
		}
	}
	if err := rows.Err(); err != nil {
		return storedWeek{}, fmt.Errorf("iterate SQLite schedule week: %w", err)
	}
	return result, nil
}

func lessonSlotKey(day time.Time, lesson schedule.Lesson) string {
	return fmt.Sprintf("%s\x00%d\x00%s", day.Format("2006-01-02"), lesson.Number, lesson.Start.Format(time.RFC3339))
}

func insertLesson(ctx context.Context, tx *sql.Tx, groupID string, day time.Time, lesson schedule.Lesson) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO lessons (group_id, source_id, day_date, number, title, type, professor, audience, start_time, end_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, groupID, lesson.ID, day.Format("2006-01-02"), lesson.Number, lesson.Title, lesson.Type, lesson.Professor, lesson.Audience, lesson.Start.Format(time.RFC3339), lesson.End.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save lesson in SQLite: %w", err)
	}
	return nil
}

func updateLesson(ctx context.Context, tx *sql.Tx, rowID int64, day time.Time, lesson schedule.Lesson) error {
	_, err := tx.ExecContext(ctx, `UPDATE lessons SET source_id = ?, day_date = ?, number = ?, title = ?, type = ?, professor = ?, audience = ?, start_time = ?, end_time = ? WHERE rowid = ?`, lesson.ID, day.Format("2006-01-02"), lesson.Number, lesson.Title, lesson.Type, lesson.Professor, lesson.Audience, lesson.Start.Format(time.RFC3339), lesson.End.Format(time.RFC3339), rowID)
	if err != nil {
		return fmt.Errorf("update lesson in SQLite: %w", err)
	}
	return nil
}

func deleteLesson(ctx context.Context, tx *sql.Tx, rowID int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM lessons WHERE rowid = ?`, rowID); err != nil {
		return fmt.Errorf("delete lesson from SQLite: %w", err)
	}
	return nil
}

func lessonsEqual(first, second schedule.Lesson) bool {
	return first.ID == second.ID && first.Number == second.Number && first.Title == second.Title && first.Type == second.Type && first.Professor == second.Professor && first.Audience == second.Audience && first.Start.Equal(second.Start) && first.End.Equal(second.End)
}

func (r *Repository) Close() error { return r.db.Close() }
