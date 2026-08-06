package main

import (
	"encoding/json"
	"log"
	"os"
)

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("configuration error: %v", err)
		os.Exit(1)
	}

	log.Printf("connecting to %s:%d/%s", cfg.DBHost, cfg.DBPort, cfg.DBName)

	rows, err := FetchAttendance(cfg)
	if err != nil {
		log.Printf("database error: %v", err)
		os.Exit(1)
	}

	log.Printf("fetched %d raw punch records", len(rows))

	attendance := ProcessAttendance(rows)

	log.Printf("processed into %d day(s) of attendance data", len(attendance))

	if len(attendance) == 0 {
		log.Println("no attendance data found for the last 7 days, nothing to post")
		os.Exit(0)
	}

	payload, _ := json.MarshalIndent(attendance, "", "  ")
	log.Printf("payload:\n%s", string(payload))

	if cfg.APIURL == "" {
		log.Println("API_URL not set, running in dry-run mode (no POST)")
		os.Exit(0)
	}

	statusCode, err := PostAttendance(cfg, attendance)
	if err != nil {
		log.Printf("API error: %v", err)
		os.Exit(1)
	}
	log.Printf("attendance data posted successfully (HTTP %d)", statusCode)
}
