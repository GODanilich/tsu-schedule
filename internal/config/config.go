package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort              string
	InTimeBaseURL         string
	GroupID               string
	Timezone              string
	HTTPTimeout           time.Duration
	GoogleCredentialsFile string
	GoogleCalendarID      string
	SQLiteFile            string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	timeout, err := time.ParseDuration(getEnv("HTTP_TIMEOUT", "10s"))
	if err != nil {
		return Config{}, fmt.Errorf("parse HTTP_TIMEOUT: %w", err)
	}

	cfg := Config{
		HTTPPort:              getEnv("HTTP_PORT", "8080"),
		InTimeBaseURL:         getEnv("INTIME_BASE_URL", "https://intime.tsu.ru/api/web/v1"),
		GroupID:               getEnv("GROUP_ID", ""),
		Timezone:              getEnv("TIMEZONE", "Asia/Tomsk"),
		HTTPTimeout:           timeout,
		GoogleCredentialsFile: getEnv("GOOGLE_CREDENTIALS_FILE", "google-service-account.json"),
		GoogleCalendarID:      getEnv("GOOGLE_CALENDAR_ID", ""),
		SQLiteFile:            getEnv("SQLITE_FILE", "schedule.db"),
	}

	if cfg.GroupID == "" {
		return Config{}, fmt.Errorf("GROUP_ID is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
