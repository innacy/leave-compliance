# Attendance Sync — Design Specification

**Date:** 2026-07-24
**Status:** Approved

## Purpose

A one-shot Go CLI tool that connects to an eSSL eTimeTrackLite SQL Server database, extracts attendance punch data for the last 7 days (excluding today), sanitizes and deduplicates it, and POSTs the result as structured JSON to a configurable API endpoint.

The program is designed to be triggered externally (cron, task scheduler, or CI pipeline). It runs once, does its job, and exits.

## Architecture

```text
┌──────────────────────┐
│  Environment Vars    │
│  (DB + API config)   │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  Config Loader       │  config.go
│  (validate, parse)   │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────┐
│  SQL Server Query    │  db.go
│  etimetrackerlite1   │
│  CHECKINOUT + USERINFO│
└──────────┬───────────┘
           │ Raw rows: (BADGENUMBER, CHECKTIME)
           ▼
┌──────────────────────┐
│  Data Processor      │  process.go
│  Sanitize, dedup,    │
│  group by date       │
└──────────┬───────────┘
           │ []DayAttendance JSON
           ▼
┌──────────────────────┐
│  API Client          │  api.go
│  POST to target API  │
│  with API key header │
└──────────────────────┘
```

## Configuration

All configuration is read from environment variables. No CLI flags, no config files (except optional `.env` for local development).

### Required Environment Variables

| Variable | Purpose | Example |
|---|---|---|
| `DB_HOST` | SQL Server host or IP address | `192.168.1.100` |
| `DB_PORT` | SQL Server port | `1433` |
| `DB_NAME` | Database name | `etimetrackerlite1` |
| `DB_USER` | SQL Server username | `sa` |
| `DB_PASSWORD` | SQL Server password | `secret` |
| `API_URL` | Target API endpoint (full URL) | `https://example.com/api/attendance` |
| `API_KEY` | API key value | `abc123` |
| `API_KEY_HEADER` | HTTP header name for the API key | `X-API-Key` |

### Validation Rules

- All 8 variables are required. If any are missing, log the names of missing variables (never log passwords) and exit with code 1.
- `DB_PORT` must be a valid integer.
- `API_URL` must be a valid URL (parseable by `net/url`).

## Database Layer

### Connection

- Driver: `github.com/microsoft/go-mssqldb` (official Microsoft Go driver for SQL Server)
- Connection string format: `sqlserver://user:password@host:port?database=dbname`
- Connection timeout: 30 seconds
- Query timeout: 60 seconds
- No connection pooling needed — single query, then close

### Query

```sql
SELECT u.BADGENUMBER, c.CHECKTIME
FROM CHECKINOUT c
JOIN USERINFO u ON c.USERID = u.USERID
WHERE c.CHECKTIME >= @startDate
  AND c.CHECKTIME < @todayDate
ORDER BY c.CHECKTIME
```

- `@startDate` = 7 days ago at 00:00:00 (midnight)
- `@todayDate` = today at 00:00:00 (midnight), so today's punches are excluded
- Both dates are computed relative to the system clock of the machine running the program (which should be on the same network/timezone as the SQL Server)

## Data Processing

### Sanitization

1. **Trim whitespace:** `strings.TrimSpace()` on every `BADGENUMBER` value
2. **Skip blanks:** Discard rows where the trimmed `BADGENUMBER` is empty

### Grouping and Deduplication

1. Extract the date portion from each `CHECKTIME` (format: `YYYY-MM-DD`)
2. Group by date using a `map[string]map[string]bool` (date string → set of employee codes)
3. Each employee code appears at most once per date, regardless of how many punches they made

### Sorting

- Dates: sorted chronologically (ascending)
- Employee codes within each date: sorted alphabetically (ascending) for deterministic output

### Output Data Structure

```go
type DayAttendance struct {
    Date    string   `json:"date"`
    Present []string `json:"present"`
}
```

Example output:

```json
[
  {
    "date": "2026-07-17",
    "present": ["EMP001", "EMP002", "EMP005"]
  },
  {
    "date": "2026-07-18",
    "present": ["EMP001", "EMP003"]
  }
]
```

## API Client

### Request

- Method: `POST`
- URL: value of `API_URL`
- Headers:
  - `Content-Type: application/json`
  - `{API_KEY_HEADER}: {API_KEY}` (e.g., `X-API-Key: abc123`)
- Body: JSON-encoded `[]DayAttendance`
- Timeout: 30 seconds

### Response Handling

- **2xx:** Log success message with HTTP status code. Exit 0.
- **Non-2xx:** Log HTTP status code and response body. Exit 1.
- **Network error:** Log the error with context. Exit 1.

## Error Handling

- All errors are logged to stderr via `log.Printf` with contextual messages
- Exit code 0 = success
- Exit code 1 = any failure (config, DB, processing, API)
- No automatic retries — the external scheduler handles re-triggering
- Errors include the step that failed (e.g., "failed to connect to database", "API returned 401")
- Passwords and API keys are never included in log output

## File Structure

```
leave-service/
├── main.go          # Entry point: load config, orchestrate steps, handle exit codes
├── config.go        # Config struct, env var loading, validation
├── db.go            # SQL Server connection, query execution, row scanning
├── process.go       # Sanitization, grouping, deduplication, sorting
├── api.go           # HTTP POST to target API
├── go.mod
├── go.sum
└── arch.md          # Existing architecture reference
```

All files are in `package main`. No internal packages.

## Dependencies

| Module | Purpose |
|---|---|
| `github.com/microsoft/go-mssqldb` | SQL Server driver |
| `github.com/joho/godotenv` | Optional `.env` file loading for local development |

Standard library covers everything else: `net/http`, `encoding/json`, `net/url`, `database/sql`, `os`, `log`, `strings`, `sort`, `time`.

## Build and Run

```bash
# Build
go build -o attendance-sync .

# Run (with env vars set)
DB_HOST=192.168.1.100 DB_PORT=1433 DB_NAME=etimetrackerlite1 \
DB_USER=sa DB_PASSWORD=secret \
API_URL=https://example.com/api/attendance \
API_KEY=abc123 API_KEY_HEADER=X-API-Key \
./attendance-sync

# Or with .env file
./attendance-sync
```

## Execution Flow Summary

1. Load environment variables (with optional `.env` fallback)
2. Validate all required config is present
3. Connect to SQL Server
4. Execute attendance query for last 7 days (excluding today)
5. Sanitize: trim whitespace, skip blanks
6. Deduplicate: one entry per employee per date
7. Sort: dates chronological, empcodes alphabetical
8. Marshal to JSON
9. POST to target API with API key header
10. Log result, exit with appropriate code
