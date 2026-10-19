# fetchFromMachine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go executable that fetches attendance data from ESSL biometric web interface and posts to the backend API, replacing the current SQL Server approach.

**Architecture:** HTTP client with cookie jar performs login → extracts VIEWSTATE → fetches report → parses HTML → transforms to DayAttendance → POSTs to API. Reuses existing `ProcessAttendance` and `PostAttendance` logic.

**Tech Stack:** Go 1.21+, `net/http` with cookie jar, `regexp` for HTML parsing, `godotenv` for config, cross-compile for Windows.

## Global Constraints

- Go 1.21+ required
- No external HTML parsing libraries (use stdlib `regexp`)
- Reuse existing types: `AttendanceRow`, `DayAttendance`
- Config via `.env` file using `godotenv`
- Silent failure (exit code 1, no stderr output in production)
- Cross-compile target: `GOOS=windows GOARCH=amd64`

---

## File Structure

```
scripts/fetchFromMachine/
├── config.go           # Config struct and LoadConfig()
├── client.go           # ESSLClient with Login(), FetchViewState(), FetchReport()
├── parser.go           # ParseAttendance() - HTML to []AttendanceRow
├── handler.go          # Execute() orchestrating the full flow
└── handler_test.go     # Unit tests

cmd/fetchFromMachine/
└── main.go             # Standalone entry point
```

---

### Task 1: Configuration

**Files:**
- Create: `scripts/fetchFromMachine/config.go`
- Test: `scripts/fetchFromMachine/config_test.go`

**Interfaces:**
- Consumes: None
- Produces: `Config` struct, `LoadConfig() (*Config, error)`

- [ ] **Step 1: Write the failing test for LoadConfig**

Create `scripts/fetchFromMachine/config_test.go`:

```go
package fetchfrommachine

import (
	"os"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	// Set required env vars
	os.Setenv("ESSL_URL", "http://localhost:8089")
	os.Setenv("ESSL_USERNAME", "testuser")
	os.Setenv("ESSL_PASSWORD_ENCRYPTED", "encpass123")
	os.Setenv("ESSL_ENCRYPTION_KEY", "key123")
	os.Setenv("API_URL", "https://api.example.com/attendance")
	os.Setenv("API_KEY", "sk-test")
	defer func() {
		os.Unsetenv("ESSL_URL")
		os.Unsetenv("ESSL_USERNAME")
		os.Unsetenv("ESSL_PASSWORD_ENCRYPTED")
		os.Unsetenv("ESSL_ENCRYPTION_KEY")
		os.Unsetenv("API_URL")
		os.Unsetenv("API_KEY")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.ESSLURL != "http://localhost:8089" {
		t.Errorf("ESSLURL = %q, want %q", cfg.ESSLURL, "http://localhost:8089")
	}
	if cfg.DaysToFetch != 7 {
		t.Errorf("DaysToFetch = %d, want 7 (default)", cfg.DaysToFetch)
	}
}

func TestLoadConfig_MissingRequired(t *testing.T) {
	os.Clearenv()

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() should fail when required vars missing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run TestLoadConfig`

Expected: FAIL with "undefined: LoadConfig"

- [ ] **Step 3: Create the config.go file**

Create `scripts/fetchFromMachine/config.go`:

```go
package fetchfrommachine

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

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

func LoadConfig() (*Config, error) {
	godotenv.Load()

	required := []string{
		"ESSL_URL",
		"ESSL_USERNAME",
		"ESSL_PASSWORD_ENCRYPTED",
		"ESSL_ENCRYPTION_KEY",
		"API_URL",
		"API_KEY",
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

	daysToFetch := 7
	if val := os.Getenv("DAYS_TO_FETCH"); val != "" {
		if d, err := strconv.Atoi(val); err == nil && d > 0 {
			daysToFetch = d
		}
	}

	apiKeyHeader := os.Getenv("API_KEY_HEADER")
	if apiKeyHeader == "" {
		apiKeyHeader = "X-API-Key"
	}

	return &Config{
		ESSLURL:               os.Getenv("ESSL_URL"),
		ESSLUsername:          os.Getenv("ESSL_USERNAME"),
		ESSLPasswordEncrypted: os.Getenv("ESSL_PASSWORD_ENCRYPTED"),
		ESSLEncryptionKey:     os.Getenv("ESSL_ENCRYPTION_KEY"),
		APIURL:                os.Getenv("API_URL"),
		APIKey:                os.Getenv("API_KEY"),
		APIKeyHeader:          apiKeyHeader,
		DaysToFetch:           daysToFetch,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run TestLoadConfig`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add scripts/fetchFromMachine/config.go scripts/fetchFromMachine/config_test.go
git commit -m "feat(fetchFromMachine): add config loading"
```

---

### Task 2: ESSL HTTP Client - Login

**Files:**
- Create: `scripts/fetchFromMachine/client.go`
- Test: `scripts/fetchFromMachine/client_test.go`

**Interfaces:**
- Consumes: `Config` from Task 1
- Produces: `ESSLClient` struct, `NewESSLClient(cfg *Config) *ESSLClient`, `(c *ESSLClient) Login() error`

- [ ] **Step 1: Write the failing test for Login**

Create `scripts/fetchFromMachine/client_test.go`:

```go
package fetchfrommachine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestESSLClient_Login_Success(t *testing.T) {
	// Mock server that accepts login and sets session cookie
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "Default.aspx") {
			t.Errorf("expected path containing Default.aspx, got %s", r.URL.Path)
		}

		// Verify form data
		r.ParseForm()
		if r.FormValue("StaffloginDialog$txt_LoginName") != "testuser" {
			t.Errorf("unexpected username: %s", r.FormValue("StaffloginDialog$txt_LoginName"))
		}

		// Set session cookie
		http.SetCookie(w, &http.Cookie{
			Name:  "ASP.NET_SessionId",
			Value: "testsession123",
		})
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		ESSLURL:               server.URL,
		ESSLUsername:          "testuser",
		ESSLPasswordEncrypted: "encpass",
		ESSLEncryptionKey:     "key123",
	}

	client := NewESSLClient(cfg)
	err := client.Login()
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
}

func TestESSLClient_Login_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := &Config{
		ESSLURL:               server.URL,
		ESSLUsername:          "baduser",
		ESSLPasswordEncrypted: "badpass",
		ESSLEncryptionKey:     "badkey",
	}

	client := NewESSLClient(cfg)
	err := client.Login()
	if err == nil {
		t.Fatal("Login() should fail on 401")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run TestESSLClient_Login`

Expected: FAIL with "undefined: NewESSLClient"

- [ ] **Step 3: Implement the ESSLClient with Login**

Create `scripts/fetchFromMachine/client.go`:

```go
package fetchfrommachine

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

type ESSLClient struct {
	cfg        *Config
	httpClient *http.Client
}

func NewESSLClient(cfg *Config) *ESSLClient {
	jar, _ := cookiejar.New(nil)
	return &ESSLClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Jar:     jar,
		},
	}
}

func (c *ESSLClient) Login() error {
	loginURL := c.cfg.ESSLURL + "/iclock/Default.aspx"

	formData := url.Values{}
	formData.Set("__EVENTTARGET", "")
	formData.Set("__EVENTARGUMENT", "")
	formData.Set("__VIEWSTATE", "/wEPDwUJNjI4Njk3ODgwZGSEOin1LKPhkvLLZzzptEZXxlPAggLLbdQg1fMFBnckRQ==")
	formData.Set("__VIEWSTATEGENERATOR", "AEA5C644")
	formData.Set("StaffloginDialog$txt_LoginName", c.cfg.ESSLUsername)
	formData.Set("StaffloginDialog$Txt_Password", c.cfg.ESSLPasswordEncrypted)
	formData.Set("StaffloginDialog$txtKey", c.cfg.ESSLEncryptionKey)
	formData.Set("StaffloginDialog$Btn_Ok", "Login")

	req, err := http.NewRequest(http.MethodPost, loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.cfg.ESSLURL)
	req.Header.Set("Referer", loginURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run TestESSLClient_Login`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add scripts/fetchFromMachine/client.go scripts/fetchFromMachine/client_test.go
git commit -m "feat(fetchFromMachine): add ESSL client with login"
```

---

### Task 3: ESSL Client - FetchViewState and FetchReport

**Files:**
- Modify: `scripts/fetchFromMachine/client.go`
- Modify: `scripts/fetchFromMachine/client_test.go`

**Interfaces:**
- Consumes: `ESSLClient` from Task 2
- Produces: `(c *ESSLClient) FetchViewState() (viewState, viewStateGenerator string, err error)`, `(c *ESSLClient) FetchReport(fromDate, toDate time.Time) (string, error)`

- [ ] **Step 1: Write failing tests for FetchViewState and FetchReport**

Add to `scripts/fetchFromMachine/client_test.go`:

```go
func TestESSLClient_FetchViewState(t *testing.T) {
	htmlResponse := `
	<html>
	<input type="hidden" name="__VIEWSTATE" id="__VIEWSTATE" value="testviewstate123" />
	<input type="hidden" name="__VIEWSTATEGENERATOR" id="__VIEWSTATEGENERATOR" value="testgenerator456" />
	</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(htmlResponse))
	}))
	defer server.Close()

	cfg := &Config{ESSLURL: server.URL}
	client := NewESSLClient(cfg)

	vs, vsg, err := client.FetchViewState()
	if err != nil {
		t.Fatalf("FetchViewState() error = %v", err)
	}
	if vs != "testviewstate123" {
		t.Errorf("viewState = %q, want %q", vs, "testviewstate123")
	}
	if vsg != "testgenerator456" {
		t.Errorf("viewStateGenerator = %q, want %q", vsg, "testgenerator456")
	}
}

func TestESSLClient_FetchReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		r.ParseForm()
		if r.FormValue("__VIEWSTATE") != "vs123" {
			t.Errorf("missing or wrong viewstate")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1|#||4|1000|updatePanel|ReportViewer1_ctl09_ReportArea|<div>report content</div>"))
	}))
	defer server.Close()

	cfg := &Config{ESSLURL: server.URL}
	client := NewESSLClient(cfg)
	client.viewState = "vs123"
	client.viewStateGenerator = "vsg456"

	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	html, err := client.FetchReport(from, to)
	if err != nil {
		t.Fatalf("FetchReport() error = %v", err)
	}
	if !strings.Contains(html, "report content") {
		t.Errorf("html should contain report content")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run "TestESSLClient_FetchViewState|TestESSLClient_FetchReport"`

Expected: FAIL with "undefined: FetchViewState" or similar

- [ ] **Step 3: Implement FetchViewState and FetchReport**

Update `scripts/fetchFromMachine/client.go` - add fields and methods:

```go
package fetchfrommachine

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type ESSLClient struct {
	cfg                *Config
	httpClient         *http.Client
	viewState          string
	viewStateGenerator string
}

func NewESSLClient(cfg *Config) *ESSLClient {
	jar, _ := cookiejar.New(nil)
	return &ESSLClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Jar:     jar,
		},
	}
}

func (c *ESSLClient) Login() error {
	loginURL := c.cfg.ESSLURL + "/iclock/Default.aspx"

	formData := url.Values{}
	formData.Set("__EVENTTARGET", "")
	formData.Set("__EVENTARGUMENT", "")
	formData.Set("__VIEWSTATE", "/wEPDwUJNjI4Njk3ODgwZGSEOin1LKPhkvLLZzzptEZXxlPAggLLbdQg1fMFBnckRQ==")
	formData.Set("__VIEWSTATEGENERATOR", "AEA5C644")
	formData.Set("StaffloginDialog$txt_LoginName", c.cfg.ESSLUsername)
	formData.Set("StaffloginDialog$Txt_Password", c.cfg.ESSLPasswordEncrypted)
	formData.Set("StaffloginDialog$txtKey", c.cfg.ESSLEncryptionKey)
	formData.Set("StaffloginDialog$Btn_Ok", "Login")

	req, err := http.NewRequest(http.MethodPost, loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.cfg.ESSLURL)
	req.Header.Set("Referer", loginURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	return nil
}

var (
	viewStateRegex     = regexp.MustCompile(`name="__VIEWSTATE"[^>]*value="([^"]*)"`)
	viewStateGenRegex  = regexp.MustCompile(`name="__VIEWSTATEGENERATOR"[^>]*value="([^"]*)"`)
)

func (c *ESSLClient) FetchViewState() (string, string, error) {
	reportURL := c.cfg.ESSLURL + "/iclock/Reports/DailyAttendanceReport.aspx"

	req, err := http.NewRequest(http.MethodGet, reportURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("create viewstate request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("viewstate request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read viewstate response: %w", err)
	}

	html := string(body)

	vsMatch := viewStateRegex.FindStringSubmatch(html)
	if len(vsMatch) < 2 {
		return "", "", fmt.Errorf("__VIEWSTATE not found in page")
	}

	vsgMatch := viewStateGenRegex.FindStringSubmatch(html)
	if len(vsgMatch) < 2 {
		return "", "", fmt.Errorf("__VIEWSTATEGENERATOR not found in page")
	}

	c.viewState = vsMatch[1]
	c.viewStateGenerator = vsgMatch[1]

	return c.viewState, c.viewStateGenerator, nil
}

func (c *ESSLClient) FetchReport(fromDate, toDate time.Time) (string, error) {
	reportURL := c.cfg.ESSLURL + "/iclock/Reports/DailyAttendanceReport.aspx"

	formData := url.Values{}
	formData.Set("__EVENTTARGET", "ReportProtoType$btn_GenerateReport")
	formData.Set("__EVENTARGUMENT", "")
	formData.Set("__VIEWSTATE", c.viewState)
	formData.Set("__VIEWSTATEGENERATOR", c.viewStateGenerator)

	// Date range
	formData.Set("ReportProtoType$Drp_FromDateDD", fmt.Sprintf("%d", fromDate.Day()))
	formData.Set("ReportProtoType$Drp_FromDateMM", fmt.Sprintf("%d", int(fromDate.Month())))
	formData.Set("ReportProtoType$Drp_FromDateYYYY", fmt.Sprintf("%d", fromDate.Year()))
	formData.Set("ReportProtoType$Drp_ToDateDD", fmt.Sprintf("%d", toDate.Day()))
	formData.Set("ReportProtoType$Drp_ToDateMM", fmt.Sprintf("%d", int(toDate.Month())))
	formData.Set("ReportProtoType$Drp_ToDateYYYY", fmt.Sprintf("%d", toDate.Year()))

	// Report options
	formData.Set("ReportProtoType$drp_ReportType", "Detailed Summary Report")
	formData.Set("ReportProtoType$chk_ContactNo", "on")
	formData.Set("ReportProtoType$chk_RecalculateAttendance", "on")
	formData.Set("ReportProtoType$chk_ShowCompanyLogo", "on")

	req, err := http.NewRequest(http.MethodPost, reportURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("create report request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.cfg.ESSLURL)
	req.Header.Set("Referer", reportURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("report request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read report response: %w", err)
	}

	return string(body), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run "TestESSLClient_FetchViewState|TestESSLClient_FetchReport"`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add scripts/fetchFromMachine/client.go scripts/fetchFromMachine/client_test.go
git commit -m "feat(fetchFromMachine): add FetchViewState and FetchReport"
```

---

### Task 4: HTML Parser

**Files:**
- Create: `scripts/fetchFromMachine/parser.go`
- Modify: `scripts/fetchFromMachine/client_test.go` (add parser tests)

**Interfaces:**
- Consumes: HTML string from `FetchReport()`
- Produces: `ParseAttendance(html string) ([]AttendanceRow, error)` where `AttendanceRow` has `BadgeNumber`, `EmployeeName`, `CheckTime`

- [ ] **Step 1: Write the failing test using sample HTML**

Create `scripts/fetchFromMachine/parser_test.go`:

```go
package fetchfrommachine

import (
	"testing"
	"time"
)

func TestParseAttendance_Success(t *testing.T) {
	// Simplified sample from actual ESSL response
	html := `1|#||4|70333|updatePanel|ReportViewer1_ctl09_ReportArea|<div>
	<DIV class="emp">Emp Code:</DIV></TD><TD><DIV>1</DIV></TD>
	<DIV>Employee Name :</DIV></TD><TD COLSPAN="7"><DIV>Test Employee1</DIV></TD>
	<TR VALIGN="top"><TD><DIV>08-Sep-2026</DIV></TD><TD><DIV>10:00</DIV></TD><TD><DIV>18:00</DIV></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD><DIV>Present</DIV></TD></TR>
	<TR VALIGN="top"><TD><DIV>09-Sep-2026</DIV></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD><DIV>Absent</DIV></TD></TR>
	<DIV class="emp">Emp Code:</DIV></TD><TD><DIV>2</DIV></TD>
	<DIV>Employee Name :</DIV></TD><TD COLSPAN="7"><DIV>VISHNU</DIV></TD>
	<TR VALIGN="top"><TD><DIV>08-Sep-2026</DIV></TD><TD><DIV>09:30</DIV></TD><TD><DIV>19:00</DIV></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD><DIV>Present</DIV></TD></TR>
	<TR VALIGN="top"><TD><DIV>09-Sep-2026</DIV></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD><DIV>1/2Present</DIV></TD></TR>
	</div>`

	rows, err := ParseAttendance(html)
	if err != nil {
		t.Fatalf("ParseAttendance() error = %v", err)
	}

	// Should have 3 present rows (2 Present + 1 1/2Present, excluding Absent)
	if len(rows) != 3 {
		t.Errorf("got %d rows, want 3", len(rows))
	}

	// Verify first row
	if rows[0].BadgeNumber != "1" {
		t.Errorf("row[0].BadgeNumber = %q, want %q", rows[0].BadgeNumber, "1")
	}
	if rows[0].EmployeeName != "Test Employee1" {
		t.Errorf("row[0].EmployeeName = %q, want %q", rows[0].EmployeeName, "Test Employee1")
	}
	expected := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	if !rows[0].CheckTime.Equal(expected) {
		t.Errorf("row[0].CheckTime = %v, want %v", rows[0].CheckTime, expected)
	}
}

func TestParseAttendance_NoData(t *testing.T) {
	html := `1|#||4|100|updatePanel|ReportViewer1_ctl09_ReportArea|<div>No records found</div>`

	rows, err := ParseAttendance(html)
	if err != nil {
		t.Fatalf("ParseAttendance() error = %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run TestParseAttendance`

Expected: FAIL with "undefined: ParseAttendance"

- [ ] **Step 3: Implement the parser**

Create `scripts/fetchFromMachine/parser.go`:

```go
package fetchfrommachine

import (
	"regexp"
	"strings"
	"time"
)

type AttendanceRow struct {
	BadgeNumber  string
	EmployeeName string
	CheckTime    time.Time
}

var (
	// Extract HTML content from UpdatePanel response
	updatePanelRegex = regexp.MustCompile(`updatePanel\|ReportViewer1[^|]*\|(.+?)(?:\|[0-9]+\|hiddenField|\z)`)

	// Find employee code blocks
	empCodeRegex = regexp.MustCompile(`Emp Code:</DIV></TD>.*?<DIV[^>]*>(\d+)</DIV>`)

	// Find employee name blocks
	empNameRegex = regexp.MustCompile(`Employee Name\s*:</DIV></TD>.*?<DIV[^>]*>([^<]+)</DIV>`)

	// Find date and status in table rows - captures date (DD-MMM-YYYY) and status
	dateStatusRegex = regexp.MustCompile(`<TR[^>]*VALIGN="top"[^>]*>.*?<DIV[^>]*>(\d{2}-[A-Za-z]{3}-\d{4})</DIV>.*?<DIV[^>]*>(Present|1/2Present|1/2 Present|Absent|WeeklyOff|On Leave|Holiday|Late Coming|Early Going|Short Worked|Extra Worked)[^<]*</DIV>`)
)

// Status values that count as "present"
var presentStatuses = map[string]bool{
	"Present":     true,
	"1/2Present":  true,
	"1/2 Present": true,
}

func ParseAttendance(html string) ([]AttendanceRow, error) {
	// Extract the report HTML from UpdatePanel response
	content := html
	if matches := updatePanelRegex.FindStringSubmatch(html); len(matches) > 1 {
		content = matches[1]
	}

	var rows []AttendanceRow

	// Find all employee codes and names
	empCodes := empCodeRegex.FindAllStringSubmatch(content, -1)
	empNames := empNameRegex.FindAllStringSubmatch(content, -1)

	if len(empCodes) == 0 {
		return rows, nil
	}

	// Build employee map by finding positions
	type employee struct {
		code string
		name string
		pos  int
	}
	var employees []employee

	for i, match := range empCodes {
		code := match[1]
		name := ""
		if i < len(empNames) {
			name = strings.TrimSpace(empNames[i][1])
		}
		pos := strings.Index(content, match[0])
		employees = append(employees, employee{code: code, name: name, pos: pos})
	}

	// For each employee, find their date rows
	for i, emp := range employees {
		// Determine the section for this employee
		startPos := emp.pos
		endPos := len(content)
		if i+1 < len(employees) {
			endPos = employees[i+1].pos
		}

		section := content[startPos:endPos]

		// Find all date/status pairs in this section
		dateMatches := dateStatusRegex.FindAllStringSubmatch(section, -1)

		for _, dm := range dateMatches {
			dateStr := dm[1]
			status := strings.TrimSpace(dm[2])

			// Only include if status indicates present
			if !presentStatuses[status] {
				continue
			}

			// Parse date (DD-MMM-YYYY format)
			checkTime, err := time.Parse("02-Jan-2006", dateStr)
			if err != nil {
				continue
			}

			rows = append(rows, AttendanceRow{
				BadgeNumber:  emp.code,
				EmployeeName: emp.name,
				CheckTime:    checkTime,
			})
		}
	}

	return rows, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run TestParseAttendance`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add scripts/fetchFromMachine/parser.go scripts/fetchFromMachine/parser_test.go
git commit -m "feat(fetchFromMachine): add HTML parser for attendance data"
```

---

### Task 5: Handler - Orchestrate Full Flow

**Files:**
- Create: `scripts/fetchFromMachine/handler.go`
- Test: `scripts/fetchFromMachine/handler_test.go`

**Interfaces:**
- Consumes: `Config`, `ESSLClient`, `ParseAttendance()`, existing `ProcessAttendance()` and `PostAttendance()` from main package
- Produces: `Execute(cfg *Config) error`

- [ ] **Step 1: Write the failing test for Execute**

Create `scripts/fetchFromMachine/handler_test.go`:

```go
package fetchfrommachine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecute_Success(t *testing.T) {
	// Mock ESSL server
	esslServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/iclock/Default.aspx":
			// Login
			http.SetCookie(w, &http.Cookie{Name: "ASP.NET_SessionId", Value: "test"})
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/iclock/Reports/DailyAttendanceReport.aspx" && r.Method == http.MethodGet:
			// ViewState page
			w.Write([]byte(`<input name="__VIEWSTATE" value="vs123"/><input name="__VIEWSTATEGENERATOR" value="vsg456"/>`))
		case r.URL.Path == "/iclock/Reports/DailyAttendanceReport.aspx" && r.Method == http.MethodPost:
			// Report
			w.Write([]byte(`1|#||4|1000|updatePanel|ReportViewer1_ctl09_ReportArea|
				<DIV>Emp Code:</DIV></TD><TD><DIV>1</DIV></TD>
				<DIV>Employee Name :</DIV></TD><TD><DIV>Test</DIV></TD>
				<TR VALIGN="top"><TD><DIV>01-Oct-2026</DIV></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD><DIV>Present</DIV></TD></TR>
			`))
		}
	}))
	defer esslServer.Close()

	// Mock API server
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("API: expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	cfg := &Config{
		ESSLURL:               esslServer.URL,
		ESSLUsername:          "test",
		ESSLPasswordEncrypted: "enc",
		ESSLEncryptionKey:     "key",
		APIURL:                apiServer.URL,
		APIKey:                "testkey",
		APIKeyHeader:          "X-API-Key",
		DaysToFetch:           7,
	}

	err := Execute(cfg)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run TestExecute`

Expected: FAIL with "undefined: Execute"

- [ ] **Step 3: Implement the handler**

Create `scripts/fetchFromMachine/handler.go`:

```go
package fetchfrommachine

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	// Step 1: Login
	if err := client.Login(); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	// Step 2: Fetch ViewState
	if _, _, err := client.FetchViewState(); err != nil {
		return fmt.Errorf("fetch viewstate: %w", err)
	}

	// Step 3: Calculate date range (last N days, excluding today)
	now := time.Now()
	toDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1)
	fromDate := toDate.AddDate(0, 0, -(cfg.DaysToFetch - 1))

	// Step 4: Fetch report
	html, err := client.FetchReport(fromDate, toDate)
	if err != nil {
		return fmt.Errorf("fetch report: %w", err)
	}

	// Step 5: Parse attendance
	rows, err := ParseAttendance(html)
	if err != nil {
		return fmt.Errorf("parse attendance: %w", err)
	}

	if len(rows) == 0 {
		return nil // No data, success (nothing to post)
	}

	// Step 6: Process into DayAttendance (group by date)
	attendance := processAttendance(rows)

	// Step 7: Post to API
	if cfg.APIURL == "" {
		return nil // Dry-run mode
	}

	return postAttendance(cfg, attendance)
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
		return fmt.Errorf("post request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API returned %d", resp.StatusCode)
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/widasinnacy/incy/leave-compliance && go test ./scripts/fetchFromMachine/... -v -run TestExecute`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add scripts/fetchFromMachine/handler.go scripts/fetchFromMachine/handler_test.go
git commit -m "feat(fetchFromMachine): add handler orchestrating full flow"
```

---

### Task 6: Main Entry Point for Windows

**Files:**
- Create: `cmd/fetchFromMachine/main.go`

**Interfaces:**
- Consumes: `fetchfrommachine.LoadConfig()`, `fetchfrommachine.Execute()`
- Produces: Standalone executable

- [ ] **Step 1: Create main.go**

Create `cmd/fetchFromMachine/main.go`:

```go
package main

import (
	"os"

	fetchfrommachine "leave-compliance/scripts/fetchFromMachine"
)

func main() {
	cfg, err := fetchfrommachine.LoadConfig()
	if err != nil {
		os.Exit(1)
	}

	if err := fetchfrommachine.Execute(cfg); err != nil {
		os.Exit(1)
	}

	os.Exit(0)
}
```

- [ ] **Step 2: Build for Linux (test)**

Run: `cd /home/widasinnacy/incy/leave-compliance && go build -o fetchFromMachine ./cmd/fetchFromMachine`

Expected: Builds successfully

- [ ] **Step 3: Build for Windows**

Run: `cd /home/widasinnacy/incy/leave-compliance && GOOS=windows GOARCH=amd64 go build -o fetchFromMachine.exe ./cmd/fetchFromMachine`

Expected: Creates `fetchFromMachine.exe`

- [ ] **Step 4: Commit**

```bash
git add cmd/fetchFromMachine/main.go
git commit -m "feat(fetchFromMachine): add Windows entry point"
```

---

### Task 7: Deployment Files

**Files:**
- Create: `scripts/fetchFromMachine/.env.example`
- Create: `scripts/fetchFromMachine/setup-task.ps1`

**Interfaces:**
- Consumes: None
- Produces: Deployment documentation and scripts

- [ ] **Step 1: Create .env.example**

Create `scripts/fetchFromMachine/.env.example`:

```env
# ESSL Biometric System
ESSL_URL=http://localhost:8089
ESSL_USERNAME=essl
ESSL_PASSWORD_ENCRYPTED=YOUR_ENCRYPTED_PASSWORD_HERE
ESSL_ENCRYPTION_KEY=YOUR_ENCRYPTION_KEY_HERE

# Backend API
API_URL=https://your-api.example.com/attendance
API_KEY=your-api-key-here
API_KEY_HEADER=X-API-Key

# Options (optional)
DAYS_TO_FETCH=7
```

- [ ] **Step 2: Create setup-task.ps1**

Create `scripts/fetchFromMachine/setup-task.ps1`:

```powershell
# ESSL Attendance Sync - Windows Task Scheduler Setup
# Run as Administrator

$installDir = "C:\AttendanceSync"
$taskName = "ESSL Attendance Sync"

# Create install directory if not exists
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force
    Write-Host "Created directory: $installDir"
}

# Check if exe exists
if (-not (Test-Path "$installDir\fetchFromMachine.exe")) {
    Write-Host "ERROR: fetchFromMachine.exe not found in $installDir"
    Write-Host "Please copy fetchFromMachine.exe and .env to $installDir first."
    exit 1
}

# Check if .env exists
if (-not (Test-Path "$installDir\.env")) {
    Write-Host "ERROR: .env not found in $installDir"
    Write-Host "Please copy .env.example to .env and configure it."
    exit 1
}

# Remove existing task if present
$existingTask = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if ($existingTask) {
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
    Write-Host "Removed existing task: $taskName"
}

# Create action
$action = New-ScheduledTaskAction `
    -Execute "$installDir\fetchFromMachine.exe" `
    -WorkingDirectory $installDir

# Create trigger (every 2 hours, indefinitely)
$trigger = New-ScheduledTaskTrigger `
    -Once `
    -At (Get-Date) `
    -RepetitionInterval (New-TimeSpan -Hours 2) `
    -RepetitionDuration ([TimeSpan]::MaxValue)

# Create settings
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 0 `
    -ExecutionTimeLimit (New-TimeSpan -Minutes 30)

# Register task
Register-ScheduledTask `
    -TaskName $taskName `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Description "Fetches attendance from ESSL biometric machine and posts to API every 2 hours"

Write-Host ""
Write-Host "SUCCESS: Task '$taskName' created!"
Write-Host ""
Write-Host "To verify: Open Task Scheduler and check '$taskName'"
Write-Host "To run now: Right-click the task and select 'Run'"
Write-Host "To view history: Enable 'All Tasks History' in Task Scheduler"
```

- [ ] **Step 3: Commit**

```bash
git add scripts/fetchFromMachine/.env.example scripts/fetchFromMachine/setup-task.ps1
git commit -m "docs(fetchFromMachine): add deployment files"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** All requirements from spec have corresponding tasks
- [x] **Placeholder scan:** No TBD/TODO, all code blocks complete
- [x] **Type consistency:** `AttendanceRow`, `DayAttendance`, `Config` used consistently
- [x] **Interface consistency:** Function signatures match between Consumes/Produces
