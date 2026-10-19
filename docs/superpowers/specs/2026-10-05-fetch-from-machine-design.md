# fetchFromMachine Design Spec

**Date:** 2026-10-05  
**Status:** Draft  

## Overview

Replace the current SQL Server-based attendance fetch with a direct HTTP fetch from the ESSL biometric machine's web admin interface. The script will run on Windows via Task Scheduler every 2 hours, parsing the HTML report response and posting attendance data to the backend API.

## Requirements

| Requirement | Decision |
|-------------|----------|
| Purpose | Replace SQL Server approach with HTTP fetch from ESSL web UI |
| Environment | Same Windows machine as ESSL admin (localhost:8089) |
| Output | POST to backend API (same as `PostAttendance`) |
| Scheduling | Windows Task Scheduler, every 2 hours |
| Data | Presence only (employee code/name + date) |
| Auth | Login first, hardcoded encrypted credentials |
| VIEWSTATE | Dynamic extraction (fetch page, parse, submit) |
| Date Range | Last 7 days |
| Errors | Silent exit with non-zero code |
| Config | File-based (.env) |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     fetchFromMachine.exe                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Login()                                                     │
│     POST /iclock/Default.aspx → session cookie                  │
│                        ↓                                        │
│  2. FetchViewState()                                            │
│     GET /iclock/Reports/DailyAttendanceReport.aspx              │
│     Parse __VIEWSTATE, __VIEWSTATEGENERATOR                     │
│                        ↓                                        │
│  3. FetchReport(fromDate, toDate)                               │
│     POST report request with VIEWSTATE + date range             │
│     Returns HTML response                                       │
│                        ↓                                        │
│  4. ParseAttendance(html)                                       │
│     Extract employee codes/names + dates from HTML              │
│     Returns []AttendanceRow                                     │
│                        ↓                                        │
│  5. ProcessAttendance() [reuse existing]                        │
│     Groups by date → []DayAttendance                            │
│                        ↓                                        │
│  6. PostAttendance() [reuse existing]                           │
│     POST JSON to backend API                                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## File Structure

```
scripts/fetchFromMachine/
├── handler.go          # Main logic (follows existing handler pattern)
├── client.go           # HTTP client with cookie jar, login, viewstate extraction
├── parser.go           # HTML parsing to extract attendance data
├── config.go           # Environment variable loading
└── handler_test.go     # Unit tests with mocked HTTP responses

cmd/fetchFromMachine/
└── main.go             # Standalone entry point for Windows Task Scheduler
```

## Data Types

**Input (from ESSL HTML):**
- Employee Code (e.g., "00123")
- Employee Name (e.g., "John Doe")
- Date (e.g., "01-Sep-2026")
- Status (e.g., "Present", "Absent", etc.)

**Intermediate type (reuse existing):**
```go
type AttendanceRow struct {
    BadgeNumber  string    // Employee code
    EmployeeName string
    CheckTime    time.Time // Date (time portion ignored)
}
```

**Output type (reuse existing):**
```go
type DayAttendance struct {
    Date            string   `json:"date"`            // "2026-09-01"
    Present         []string `json:"present"`         // ["00123", "00124", ...]
    PresentEmpNames []string `json:"presentEmpNames"` // ["John Doe", ...]
}
```

**Transformation flow:**
```
HTML → parse → []AttendanceRow → ProcessAttendance() → []DayAttendance → JSON → POST
```

**Status filtering:**
- Include as "Present": `Present`, `1/2Present`, `1/2 Present`
- Exclude: `Absent`, `WeeklyOff`, `On Leave`, `Holiday`

## Configuration

**File: `.env` (same directory as executable)**

```env
# ESSL Biometric System
ESSL_URL=http://localhost:8089
ESSL_USERNAME=essl
ESSL_PASSWORD_ENCRYPTED=j8JTOGSIQb6qUwJ1Q3vucQ==
ESSL_ENCRYPTION_KEY=3656676143217408

# Backend API
API_URL=https://api.example.com/attendance
API_KEY=sk-xxx
API_KEY_HEADER=X-API-Key

# Options
DAYS_TO_FETCH=7
```

**Config struct:**
```go
type Config struct {
    ESSLURL               string
    ESSLUsername          string
    ESSLPasswordEncrypted string
    ESSLEncryptionKey     string
    APIURL                string
    APIKey                string
    APIKeyHeader          string
    DaysToFetch           int
}
```

## HTML Parsing Strategy

**Response format:** ASP.NET UpdatePanel response, pipe-delimited:
```
1|#||4|70333|updatePanel|ReportViewer1_ctl09_ReportArea|<HTML_CONTENT>|...|hiddenField|__VIEWSTATE|...
```

**Report structure:**
- Employee blocks containing:
  - `Emp Code:` label followed by employee code
  - `Employee Name :` label followed by name
  - Date rows with format `DD-MMM-YYYY` (e.g., "06-Sep-2026")
  - Status column with values like "Present", "Absent", "WeeklyOff"

**Parsing approach:**
1. Extract HTML payload from UpdatePanel response (split by `|`, find `updatePanel` section)
2. Find employee blocks using regex patterns
3. For each employee, extract code and name
4. Find all date rows within employee block
5. Extract status for each date
6. Filter to only "Present" status variants
7. Build `AttendanceRow` structs

**Regex patterns:**
```go
empCodePattern := `Emp Code:</DIV></TD>.*?<DIV[^>]*>(\d+)</DIV>`
empNamePattern := `Employee Name :</DIV></TD>.*?<DIV[^>]*>([^<]+)</DIV>`
datePattern := `<DIV[^>]*>(\d{2}-[A-Za-z]{3}-\d{4})</DIV>`
```

## HTTP Flow

### 1. Login Request

```http
POST /iclock/Default.aspx HTTP/1.1
Host: localhost:8089
Content-Type: application/x-www-form-urlencoded

__EVENTTARGET=&__EVENTARGUMENT=&__VIEWSTATE=<dynamic>&__VIEWSTATEGENERATOR=AEA5C644
&StaffloginDialog$txt_LoginName=essl
&StaffloginDialog$Txt_Password=<encrypted>
&StaffloginDialog$txtKey=<encryption_key>
&StaffloginDialog$Btn_Ok=Login
```

**Response:** Session cookie (ASP.NET_SessionId)

### 2. Fetch Report Page (for VIEWSTATE)

```http
GET /iclock/Reports/DailyAttendanceReport.aspx HTTP/1.1
Host: localhost:8089
Cookie: ASP.NET_SessionId=<session>
```

**Response:** HTML page containing `__VIEWSTATE` and `__VIEWSTATEGENERATOR` hidden fields

### 3. Generate Report

```http
POST /iclock/Reports/DailyAttendanceReport.aspx HTTP/1.1
Host: localhost:8089
Content-Type: application/x-www-form-urlencoded
Cookie: ASP.NET_SessionId=<session>

__EVENTTARGET=ReportProtoType$btn_GenerateReport
&__VIEWSTATE=<extracted>
&__VIEWSTATEGENERATOR=<extracted>
&ReportProtoType$Drp_FromDateDD=<day>
&ReportProtoType$Drp_FromDateMM=<month>
&ReportProtoType$Drp_FromDateYYYY=<year>
&ReportProtoType$Drp_ToDateDD=<day>
&ReportProtoType$Drp_ToDateMM=<month>
&ReportProtoType$Drp_ToDateYYYY=<year>
&ReportProtoType$drp_ReportType=Detailed+Summary+Report
&ReportProtoType$chk_RecalculateAttendance=on
...
```

**Response:** UpdatePanel response with HTML report

## Windows Deployment

**Package structure:**
```
C:\AttendanceSync\
├── fetchFromMachine.exe    # Go binary (cross-compiled)
├── .env                    # Configuration
└── setup-task.ps1          # Task Scheduler setup script
```

**Build command:**
```bash
GOOS=windows GOARCH=amd64 go build -o fetchFromMachine.exe ./cmd/fetchFromMachine
```

**Task Scheduler configuration:**
- Trigger: Every 2 hours, indefinitely
- Action: Run `fetchFromMachine.exe` with working directory `C:\AttendanceSync`
- Settings: Don't retry on failure, start when available

**PowerShell setup script:**
```powershell
$action = New-ScheduledTaskAction `
    -Execute "C:\AttendanceSync\fetchFromMachine.exe" `
    -WorkingDirectory "C:\AttendanceSync"

$trigger = New-ScheduledTaskTrigger `
    -Once -At (Get-Date) `
    -RepetitionInterval (New-TimeSpan -Hours 2) `
    -RepetitionDuration ([TimeSpan]::MaxValue)

$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 0

Register-ScheduledTask `
    -TaskName "ESSL Attendance Sync" `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Description "Fetches attendance from ESSL and posts to API every 2 hours"
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Login failed | Exit code 1, no output |
| Network error | Exit code 1, no output |
| Parse error | Exit code 1, no output |
| API POST failed | Exit code 1, no output |
| No data found | Exit code 0 (success, nothing to post) |
| Success | Exit code 0 |

Task Scheduler logs task runs; failures are visible in Task Scheduler history but don't trigger retries.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| ESSL software update changes HTML format | Unit tests with HTML snapshots; parser designed with fallback patterns |
| Encrypted password/key changes | Document how to recapture login curl; credentials in .env for easy update |
| Session timeout during long report | Single session per run; fresh login each execution |
| VIEWSTATE expiration | Dynamic VIEWSTATE extraction each run |

## Testing Strategy

1. **Unit tests** with mocked HTTP responses and HTML snapshots from `essl data.txt`
2. **Integration test** against live ESSL (manual, on Windows machine)
3. **Dry-run mode** (`API_URL` empty) to test fetch/parse without posting

## Out of Scope

- Multiple ESSL machines (single machine only)
- Detailed time tracking (in/out times, duration)
- Real-time sync (batch every 2 hours is sufficient)
- Logging to file (silent operation)
