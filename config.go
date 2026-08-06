package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost       string
	DBPort       int
	DBName       string
	DBUser       string
	DBPassword   string
	APIURL       string
	APIKey       string
	APIKeyHeader string
	DaysToFetch  int
}

func LoadConfig() (*Config, error) {
	godotenv.Load()

	requiredDB := []string{
		"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD",
	}

	var missing []string
	for _, key := range requiredDB {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missing)
	}

	port, err := strconv.Atoi(os.Getenv("DB_PORT"))
	if err != nil {
		return nil, fmt.Errorf("DB_PORT must be a valid integer: %w", err)
	}

	apiURL := os.Getenv("API_URL")
	if apiURL != "" {
		if _, err := url.ParseRequestURI(apiURL); err != nil {
			return nil, fmt.Errorf("API_URL must be a valid URL: %w", err)
		}
	}

	daysToFetch, err := strconv.Atoi(os.Getenv("DAYS_TO_FETCH"))
	if err != nil {
		return nil, fmt.Errorf("DAYS_TO_FETCH must be a valid integer: %w", err)
	}

	if daysToFetch <= 0 {
		daysToFetch = 7 // default to 7 days if not set or invalid
	}

	return &Config{
		DBHost:       os.Getenv("DB_HOST"),
		DBPort:       port,
		DBName:       os.Getenv("DB_NAME"),
		DBUser:       os.Getenv("DB_USER"),
		DBPassword:   os.Getenv("DB_PASSWORD"),
		APIURL:       apiURL,
		APIKey:       os.Getenv("API_KEY"),
		APIKeyHeader: os.Getenv("API_KEY_HEADER"),
		DaysToFetch:  daysToFetch,
	}, nil
}
