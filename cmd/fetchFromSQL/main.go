package main

import (
	"log"
	"os"

	fetchfromsql "leave-compliance/scripts/fetchFromSQL"
)

func main() {
	cfg, err := fetchfromsql.LoadConfig()
	if err != nil {
		log.Printf("configuration error: %v", err)
		os.Exit(1)
	}

	log.Printf("connecting to %s:%d/%s", cfg.DBHost, cfg.DBPort, cfg.DBName)

	rows, err := fetchfromsql.FetchAttendance(cfg)
	if err != nil {
		log.Printf("database error: %v", err)
		os.Exit(1)
	}

	log.Printf("fetched %d raw punch records", len(rows))

	attendance := fetchfromsql.ProcessAttendance(rows)

	log.Printf("processed %d day(s) of attendance data", len(attendance))

	if len(attendance) == 0 {
		log.Printf("no attendance data found for the last %d days, nothing to post", cfg.DaysToFetch)
		os.Exit(0)
	}

	for _, day := range attendance {
		log.Printf("Date: %s, Present: %d", day.Date, len(day.PresentEmpNames))
	}

	// payload, _ := json.MarshalIndent(attendance, "", "  ")
	// log.Printf("payload:\n%s", string(payload))

	if cfg.APIURL == "" {
		log.Println("API_URL not set, running in dry-run mode (no POST)")
		os.Exit(0)
	}

	statusCode, err := fetchfromsql.PostAttendance(cfg, attendance)
	if err != nil {
		log.Printf("API error: %v", err)
		os.Exit(1)
	}
	log.Printf("attendance data posted successfully (HTTP %d)", statusCode)
}
