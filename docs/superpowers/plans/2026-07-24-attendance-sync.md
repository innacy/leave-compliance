# Attendance Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a one-shot Go CLI tool that reads eSSL attendance data from SQL Server and POSTs a sanitized JSON summary to a target API.

**Architecture:** Flat single-package Go program. Config loads from env vars, queries SQL Server for 7 days of punch data, deduplicates per employee per day, and POSTs the result as JSON. No internal packages — all files in `package main`.

**Tech Stack:** Go, `github.com/microsoft/go-mssqldb`, `github.com/joho/godotenv`, standard library (`database/sql`, `net/http`, `encoding/json`)

## Global Constraints

- Go 1.21+
- Only two external dependencies: `go-mssqldb` and `godotenv`
- All files in `package main`
- Database: `etimetrackerlite1` on SQL Server, tables `CHECKINOUT` and `USERINFO`
- Employee code field: `BADGENUMBER`
- Exit 0 on success, exit 1 on any failure
- Never log passwords or API keys

---

## File Map

| File | Responsibility |
|---|---|
| `main.go` | Entry point: load config, orchestrate steps, exit codes |
| `config.go` | `Config` struct, `LoadConfig()` from env, validation |
| `config_test.go` | Tests for config loading and validation |
| `db.go` | `FetchAttendance()` — connect to SQL Server, query, return raw rows |
| `process.go` | `ProcessAttendance()` — sanitize, dedup, group, sort |
| `process_test.go` | Tests for all processing logic |
| `api.go` | `PostAttendance()` — HTTP POST JSON to target API |
| `api_test.go` | Tests using `httptest` for API posting |
| `go.mod` | Module definition and dependencies |

---

### Task 1: Project Scaffold & Config

**Files:**
- Create: `go.mod`
- Create: `config.go`
- Create: `config_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `Config` struct with fields `DBHost string`, `DBPort int`, `DBName string`, `DBUser string`, `DBPassword string`, `APIURL string`, `APIKey string`, `APIKeyHeader string`. Function `LoadConfig() (*Config, error)`.

- [ ] **Step 1: Initialize Go module and install dependencies**

```bash
cd /home/widasinnacy/incy/leave-service
go mod init leave-service
go get github.com/microsoft/go-mssqldb
go get github.com/joho/godotenv
```

- [ ] **Step 2: Write failing tests for config loading**

Create `config_test.go`:

```go
package main

import (
	"os"
	"testing"
)

func clearConfigEnv() {
	for _, key := range []string{
		"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD",
		"API_URL", "API_KEY", "API_KEY_HEADER",
	} {
		os.Unsetenv(key)
	}
}

func setValidConfigEnv() {
	os.Setenv("DB_HOST", "192.168.1.100")
	os.Setenv("DB_PORT", "1433")
	os.Setenv("DB_NAME", "etimetrackerlite1")
	os.Setenv("DB_USER", "sa")
	os.Setenv("DB_PASSWORD", "secret")
	os.Setenv("API_URL", "https://example.com/api/attendance")
	os.Setenv("API_KEY", "testkey123")
	os.Setenv("API_KEY_HEADER", "X-API-Key")
}

func TestLoadConfig_AllValid(t *testing.T) {
	clearConfigEnv()
	setValidConfigEnv()
	defer clearConfigEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBHost != "192.168.1.100" {
		t.Errorf("DBHost = %q, want %q", cfg.DBHost, "192.168.1.100")
	}
	if cfg.DBPort != 1433 {
		t.Errorf("DBPort = %d, want %d", cfg.DBPort, 1433)
	}
	if cfg.DBName != "etimetrackerlite1" {
		t.Errorf("DBName = %q, want %q", cfg.DBName, "etimetrackerlite1")
	}
	if cfg.APIURL != "https://example.com/api/attendance" {
		t.Errorf("APIURL = %q, want %q", cfg.APIURL, "https://example.com/api/attendance")
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	clearConfigEnv()
	defer clearConfigEnv()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for missing env vars, got nil")
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	clearConfigEnv()
	setValidConfigEnv()
	os.Setenv("DB_PORT", "notanumber")
	defer clearConfigEnv()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestLoadConfig_InvalidURL(t *testing.T) {
	clearConfigEnv()
	setValidConfigEnv()
	os.Setenv("API_URL", "://bad-url")
	defer clearConfigEnv()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test -run TestLoadConfig -v
```

Expected: compilation error — `LoadConfig` not defined.

- [ ] **Step 4: Implement config.go**

Create `config.go`:

```go
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
}

func LoadConfig() (*Config, error) {
	godotenv.Load()

	required := []string{
		"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD",
		"API_URL", "API_KEY", "API_KEY_HEADER",
	}

	var missing []string
	for _, key := range required {
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
	if _, err := url.ParseRequestURI(apiURL); err != nil {
		return nil, fmt.Errorf("API_URL must be a valid URL: %w", err)
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
	}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test -run TestLoadConfig -v
```

Expected: all 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum config.go config_test.go
git commit -m "feat: add config loading with env var validation"
```

---

### Task 2: Data Processing

**Files:**
- Create: `process.go`
- Create: `process_test.go`

**Interfaces:**
- Consumes: `AttendanceRow` — raw row struct with `BadgeNumber string` and `CheckTime time.Time`
- Produces: `DayAttendance` struct with `Date string` and `Present []string`. Function `ProcessAttendance(rows []AttendanceRow) []DayAttendance`.

- [ ] **Step 1: Write failing tests for processing logic**

Create `process_test.go`:

```go
package main

import (
	"testing"
	"time"
)

func makeTime(year, month, day, hour, min int) time.Time {
	return time.Date(year, time.Month(month), day, hour, min, 0, 0, time.UTC)
}

func TestProcessAttendance_GroupsByDate(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "EMP002", CheckTime: makeTime(2026, 7, 20, 9, 30)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 21, 8, 45)},
	}

	result := ProcessAttendance(rows)

	if len(result) != 2 {
		t.Fatalf("got %d days, want 2", len(result))
	}
	if result[0].Date != "2026-07-20" {
		t.Errorf("first date = %q, want %q", result[0].Date, "2026-07-20")
	}
	if len(result[0].Present) != 2 {
		t.Errorf("day 1 present count = %d, want 2", len(result[0].Present))
	}
	if result[1].Date != "2026-07-21" {
		t.Errorf("second date = %q, want %q", result[1].Date, "2026-07-21")
	}
}

func TestProcessAttendance_DeduplicatesSameDay(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 13, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 18, 0)},
	}

	result := ProcessAttendance(rows)

	if len(result) != 1 {
		t.Fatalf("got %d days, want 1", len(result))
	}
	if len(result[0].Present) != 1 {
		t.Errorf("present count = %d, want 1", len(result[0].Present))
	}
	if result[0].Present[0] != "EMP001" {
		t.Errorf("present[0] = %q, want %q", result[0].Present[0], "EMP001")
	}
}

func TestProcessAttendance_TrimsWhitespace(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "  EMP001  ", CheckTime: makeTime(2026, 7, 20, 9, 0)},
	}

	result := ProcessAttendance(rows)

	if result[0].Present[0] != "EMP001" {
		t.Errorf("present[0] = %q, want %q", result[0].Present[0], "EMP001")
	}
}

func TestProcessAttendance_SkipsBlanks(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "   ", CheckTime: makeTime(2026, 7, 20, 9, 30)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 10, 0)},
	}

	result := ProcessAttendance(rows)

	if len(result) != 1 {
		t.Fatalf("got %d days, want 1", len(result))
	}
	if len(result[0].Present) != 1 {
		t.Errorf("present count = %d, want 1", len(result[0].Present))
	}
}

func TestProcessAttendance_SortsDatesChronologically(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 22, 9, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 21, 9, 0)},
	}

	result := ProcessAttendance(rows)

	if result[0].Date != "2026-07-20" {
		t.Errorf("first date = %q, want %q", result[0].Date, "2026-07-20")
	}
	if result[1].Date != "2026-07-21" {
		t.Errorf("second date = %q, want %q", result[1].Date, "2026-07-21")
	}
	if result[2].Date != "2026-07-22" {
		t.Errorf("third date = %q, want %q", result[2].Date, "2026-07-22")
	}
}

func TestProcessAttendance_SortsEmpcodesAlphabetically(t *testing.T) {
	rows := []AttendanceRow{
		{BadgeNumber: "EMP003", CheckTime: makeTime(2026, 7, 20, 9, 0)},
		{BadgeNumber: "EMP001", CheckTime: makeTime(2026, 7, 20, 9, 30)},
		{BadgeNumber: "EMP002", CheckTime: makeTime(2026, 7, 20, 10, 0)},
	}

	result := ProcessAttendance(rows)

	want := []string{"EMP001", "EMP002", "EMP003"}
	for i, code := range result[0].Present {
		if code != want[i] {
			t.Errorf("present[%d] = %q, want %q", i, code, want[i])
		}
	}
}

func TestProcessAttendance_EmptyInput(t *testing.T) {
	result := ProcessAttendance(nil)

	if len(result) != 0 {
		t.Errorf("got %d days, want 0", len(result))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run TestProcessAttendance -v
```

Expected: compilation error — `AttendanceRow`, `DayAttendance`, `ProcessAttendance` not defined.

- [ ] **Step 3: Implement process.go**

Create `process.go`:

```go
package main

import (
	"sort"
	"strings"
	"time"
)

type AttendanceRow struct {
	BadgeNumber string
	CheckTime   time.Time
}

type DayAttendance struct {
	Date    string   `json:"date"`
	Present []string `json:"present"`
}

func ProcessAttendance(rows []AttendanceRow) []DayAttendance {
	grouped := make(map[string]map[string]bool)

	for _, row := range rows {
		code := strings.TrimSpace(row.BadgeNumber)
		if code == "" {
			continue
		}

		dateKey := row.CheckTime.Format("2006-01-02")

		if grouped[dateKey] == nil {
			grouped[dateKey] = make(map[string]bool)
		}
		grouped[dateKey][code] = true
	}

	dates := make([]string, 0, len(grouped))
	for d := range grouped {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	result := make([]DayAttendance, 0, len(dates))
	for _, date := range dates {
		codes := make([]string, 0, len(grouped[date]))
		for code := range grouped[date] {
			codes = append(codes, code)
		}
		sort.Strings(codes)

		result = append(result, DayAttendance{
			Date:    date,
			Present: codes,
		})
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -run TestProcessAttendance -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add process.go process_test.go
git commit -m "feat: add attendance data processing with dedup and sorting"
```

---

### Task 3: API Client

**Files:**
- Create: `api.go`
- Create: `api_test.go`

**Interfaces:**
- Consumes: `Config` (from Task 1), `[]DayAttendance` (from Task 2)
- Produces: Function `PostAttendance(cfg *Config, data []DayAttendance) error`

- [ ] **Step 1: Write failing tests for API posting**

Create `api_test.go`:

```go
package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostAttendance_Success(t *testing.T) {
	var receivedBody []DayAttendance
	var receivedHeader string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedHeader = r.Header.Get("X-API-Key")

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	cfg := &Config{
		APIURL:       server.URL,
		APIKey:       "testkey123",
		APIKeyHeader: "X-API-Key",
	}

	data := []DayAttendance{
		{Date: "2026-07-20", Present: []string{"EMP001", "EMP002"}},
	}

	err := PostAttendance(cfg, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", receivedContentType, "application/json")
	}
	if receivedHeader != "testkey123" {
		t.Errorf("API key header = %q, want %q", receivedHeader, "testkey123")
	}
	if len(receivedBody) != 1 {
		t.Fatalf("received %d items, want 1", len(receivedBody))
	}
	if receivedBody[0].Date != "2026-07-20" {
		t.Errorf("date = %q, want %q", receivedBody[0].Date, "2026-07-20")
	}
}

func TestPostAttendance_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	cfg := &Config{
		APIURL:       server.URL,
		APIKey:       "testkey123",
		APIKeyHeader: "X-API-Key",
	}

	err := PostAttendance(cfg, []DayAttendance{})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestPostAttendance_NetworkError(t *testing.T) {
	cfg := &Config{
		APIURL:       "http://localhost:1",
		APIKey:       "testkey123",
		APIKeyHeader: "X-API-Key",
	}

	err := PostAttendance(cfg, []DayAttendance{})
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test -run TestPostAttendance -v
```

Expected: compilation error — `PostAttendance` not defined.

- [ ] **Step 3: Implement api.go**

Create `api.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func PostAttendance(cfg *Config, data []DayAttendance) error {
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal attendance data: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodPost, cfg.APIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(cfg.APIKeyHeader, cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test -run TestPostAttendance -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add api.go api_test.go
git commit -m "feat: add API client for posting attendance data"
```

---

### Task 4: Database Layer

**Files:**
- Create: `db.go`

**Interfaces:**
- Consumes: `Config` (from Task 1)
- Produces: Function `FetchAttendance(cfg *Config) ([]AttendanceRow, error)`. Uses `AttendanceRow` from Task 2.

Note: This task does not include unit tests because `FetchAttendance` directly queries a real SQL Server. Testing requires a live database which is an integration test concern. The query logic is simple and the processing logic (which is the complex part) is fully tested in Task 2.

- [ ] **Step 1: Implement db.go**

Create `db.go`:

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/microsoft/go-mssqldb"
)

func FetchAttendance(cfg *Config) ([]AttendanceRow, error) {
	connStr := fmt.Sprintf(
		"sqlserver://%s:%s@%s:%d?database=%s&connection+timeout=30",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName,
	)

	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	now := time.Now()
	todayMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := todayMidnight.AddDate(0, 0, -7)

	query := `
		SELECT u.BADGENUMBER, c.CHECKTIME
		FROM CHECKINOUT c
		JOIN USERINFO u ON c.USERID = u.USERID
		WHERE c.CHECKTIME >= @startDate
		  AND c.CHECKTIME < @todayDate
		ORDER BY c.CHECKTIME
	`

	rows, err := db.QueryContext(ctx, query,
		sql.Named("startDate", startDate),
		sql.Named("todayDate", todayMidnight),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var result []AttendanceRow
	for rows.Next() {
		var row AttendanceRow
		if err := rows.Scan(&row.BadgeNumber, &row.CheckTime); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return result, nil
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./...
```

Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add db.go
git commit -m "feat: add database layer for fetching attendance from SQL Server"
```

---

### Task 5: Main Orchestration

**Files:**
- Create: `main.go`

**Interfaces:**
- Consumes: `LoadConfig()` (Task 1), `FetchAttendance()` (Task 4), `ProcessAttendance()` (Task 2), `PostAttendance()` (Task 3)
- Produces: CLI entry point — exit 0 on success, exit 1 on failure

- [ ] **Step 1: Implement main.go**

Create `main.go`:

```go
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

	if err := PostAttendance(cfg, attendance); err != nil {
		log.Printf("API error: %v", err)
		os.Exit(1)
	}

	log.Println("attendance data posted successfully")
}
```

- [ ] **Step 2: Verify full build**

```bash
go build -o attendance-sync .
```

Expected: produces `attendance-sync` binary without errors.

- [ ] **Step 3: Run all tests**

```bash
go test -v ./...
```

Expected: all tests pass (config: 4, process: 7, api: 3 = 14 total).

- [ ] **Step 4: Add attendance-sync binary to .gitignore**

Create `.gitignore`:

```
attendance-sync
.env
```

- [ ] **Step 5: Commit**

```bash
git add main.go .gitignore
git commit -m "feat: add main orchestration and complete attendance sync tool"
```

---

## Self-Review

**Spec coverage:**
- Configuration from env vars: Task 1 ✓
- SQL Server connection and query: Task 4 ✓
- Data sanitization (trim, skip blanks): Task 2 ✓
- Deduplication per employee per day: Task 2 ✓
- Sorting (dates chronological, empcodes alphabetical): Task 2 ✓
- JSON structure `[{date, present}]`: Task 2 ✓
- API POST with custom header: Task 3 ✓
- Error handling and exit codes: Task 5 ✓
- No logging of secrets: Task 1 (validation) + Task 5 (main logs host/port/db only) ✓

**Placeholder scan:** No TBDs, TODOs, or vague instructions found.

**Type consistency:** `Config` struct, `AttendanceRow`, `DayAttendance` — names and signatures are consistent across all tasks. `LoadConfig() (*Config, error)`, `FetchAttendance(cfg *Config) ([]AttendanceRow, error)`, `ProcessAttendance(rows []AttendanceRow) []DayAttendance`, `PostAttendance(cfg *Config, data []DayAttendance) error` — all match between producer and consumer tasks.
