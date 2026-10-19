package fetchfrommachine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

type DayAttendance struct {
	Date            string   `json:"date"`
	Present         []string `json:"present"`
	PresentEmpNames []string `json:"presentEmpNames,omitempty"`
}

type FullAttendance struct {
	Data []DayAttendance `json:"data"`
}

func Execute(cfg *Config) error {
	client := NewESSLClient(cfg)

	log.Printf("logging in to ESSL at %s...", cfg.ESSLURL)
	if err := client.Login(); err != nil {
		log.Printf("ERROR: %v", err)
		return fmt.Errorf("login: %w", err)
	}
	log.Println("login successful")

	log.Println("fetching viewstate from report page...")
	if _, _, err := client.FetchViewState(); err != nil {
		log.Printf("ERROR: %v", err)
		return fmt.Errorf("fetch viewstate: %w", err)
	}
	log.Println("viewstate fetched")

	now := time.Now()
	toDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1)
	fromDate := toDate.AddDate(0, 0, -(cfg.DaysToFetch - 1))

	log.Printf("fetching report from %s to %s...", fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	html, err := client.FetchReport(fromDate, toDate)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return fmt.Errorf("fetch report: %w", err)
	}
	log.Printf("report fetched (%d bytes)", len(html))

	log.Println("parsing attendance data...")
	rows, err := ParseAttendance(html)
	if err != nil {
		log.Printf("ERROR: %v", err)
		return fmt.Errorf("parse attendance: %w", err)
	}
	log.Printf("parsed %d attendance records", len(rows))

	if len(rows) == 0 && !strings.Contains(html, "Emp Code") {
		return fmt.Errorf("report response has no attendance table (%d bytes): %s", len(html), snippet([]byte(html)))
	}

	if len(rows) == 0 {
		log.Println("no present employees in range, nothing to post")
		return nil
	}

	attendance := processAttendance(rows)
	log.Printf("processed into %d day(s) of attendance data", len(attendance))

	if len(attendance) == 0 {
		log.Printf("no attendance data found for the last %d days, nothing to post", cfg.DaysToFetch)
		return nil
	}

	for _, day := range attendance {
		log.Printf("Date: %s, Present: %d", day.Date, len(day.PresentEmpNames))
	}

	if cfg.APIURL == "" {
		log.Println("API_URL not set, running in dry-run mode (no POST)")
		return nil
	}

	log.Printf("posting attendance to %s...", cfg.APIURL)
	if err := postAttendance(cfg, attendance); err != nil {
		log.Printf("ERROR: %v", err)
		return err
	}
	log.Println("attendance data posted successfully")

	return nil
}

func processAttendance(rows []AttendanceRow) []DayAttendance {
	type employee struct {
		Code string
		Name string
	}
	grouped := make(map[string]map[string]employee)

	for _, row := range rows {
		code := strings.TrimSpace(row.BadgeNumber)
		if code == "" {
			continue
		}
		code = padEmpCode(code, 5)

		dateKey := row.CheckTime.Format("2006-01-02")

		if grouped[dateKey] == nil {
			grouped[dateKey] = make(map[string]employee)
		}
		grouped[dateKey][code] = employee{Code: code, Name: row.EmployeeName}
	}

	dates := make([]string, 0, len(grouped))
	for d := range grouped {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	result := make([]DayAttendance, 0, len(dates))
	for _, date := range dates {
		employees := make([]employee, 0, len(grouped[date]))
		for _, emp := range grouped[date] {
			employees = append(employees, emp)
		}

		sort.Slice(employees, func(i, j int) bool {
			return employees[i].Code < employees[j].Code
		})

		codes := make([]string, len(employees))
		names := make([]string, len(employees))

		for i, emp := range employees {
			codes[i] = emp.Code
			names[i] = emp.Name
		}

		result = append(result, DayAttendance{
			Date:            date,
			Present:         codes,
			PresentEmpNames: names,
		})
	}

	return result
}

func padEmpCode(code string, width int) string {
	for len(code) < width {
		code = "0" + code
	}
	return code
}

func postAttendance(cfg *Config, data []DayAttendance) error {
	payload := FullAttendance{Data: data}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, cfg.APIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(cfg.APIKeyHeader, cfg.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post to %s: %w", cfg.APIURL, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API HTTP %d from %s: %s", resp.StatusCode, cfg.APIURL, string(respBody))
	}

	return nil
}
