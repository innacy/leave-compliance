package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zinscky/log"
)

type InputEvent struct {
	PresentList []AttendanceRecord `json:"presentList"`
	Employees   EmpData            `json:"employees"`
}

type EmpData struct {
	ActiveEmpCodes map[string]bool   `json:"activeEmpCodes"`
	EmpCodeToName  map[string]string `json:"empCodeToName"`
	EmpCodeToEmail map[string]string `json:"empCodeToEmail"`
	EmpCodeToID    map[string]int    `json:"empCodeToID"`
	EmpIDToCode    map[int]string    `json:"empIDToCode"`
}

type AttendanceRecord struct {
	Date        string   `json:"date,omitempty"`
	PresentList []string `json:"presentList,omitempty"`
	CreatedAt   string   `json:"createdAt,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`
}

type EmployeeLeaveData struct {
	EmployeeID int                `json:"employeeId"`
	List       []LeaveTransaction `json:"list"`
}

type LeaveTransaction struct {
	ID          int     `json:"id"`
	FromDate    string  `json:"fromDate"`
	ToDate      string  `json:"toDate"`
	Days        float64 `json:"days"`
	FromSession string  `json:"fromSession"`
	ToSession   string  `json:"toSession"`
	Remarks     string  `json:"remarks"`
	Cancelled   bool    `json:"cancelled"`
	Reason      string  `json:"reason"`
}

type AbsenteeReport struct {
	Date      string           `json:"date"`
	Absentees []AbsenteeDetail `json:"absentees"`
}

type AbsenteeDetail struct {
	EmpCode      string             `json:"empCode"`
	EmpName      string             `json:"empName,omitempty"`
	Email        string             `json:"email,omitempty"`
	LeaveApplied bool               `json:"leaveApplied"`
	Leaves       []LeaveTransaction `json:"leaves,omitempty"`
}

func Execute(event string, config map[string]string, vars map[string]string, log *log.Logger) (string, error) {
	// log.Info("inside leave check handler")

	var inputEvent InputEvent
	if err := json.Unmarshal([]byte(event), &inputEvent); err != nil {
		log.Error("failed to unmarshal attendance records: %v", err)
		return "", fmt.Errorf("unmarshal attendance records: %w", err)
	}
	records := inputEvent.PresentList
	if len(records) == 0 {
		log.Info("no attendance records in event")
		return "[]", nil
	}

	token := vars["greythrToken"]
	excludeSet := csvToSet(vars["EXCLUDE_EMP_CODES"])
	wfhSet := csvToSet(vars["WFH_EMP_CODES"])

	// WFH_EMP_DAYS: JSON object of empCode -> default WFH weekday, e.g. {"00123":"Tuesday","00456":"Thursday"}
	wfhDays := map[string]string{}
	if s := strings.TrimSpace(vars["WFH_EMP_DAYS"]); s != "" {
		if err := json.Unmarshal([]byte(s), &wfhDays); err != nil {
			return "", fmt.Errorf("unmarshal WFH_EMP_DAYS: %w", err)
		}
	}

	// ponytail: one bool works because records are always a single Mon-Fri week.
	// If a Tue/Wed/Thu is a holiday, WFH_EMP_DAYS employees may move their WFH to another day.
	// A holiday date may come as a record with <5 present, or be missing entirely
	// (no attendance rows that day), so look for a Tue-Thu with no working record.
	workingDays := map[time.Weekday]bool{}
	for _, rec := range records {
		if len(sliceToSet(rec.PresentList)) < 5 {
			continue
		}
		if d, err := time.Parse("2006-01-02", rec.Date); err == nil {
			workingDays[d.Weekday()] = true
		}
	}
	midWeekHoliday := !workingDays[time.Tuesday] || !workingDays[time.Wednesday] || !workingDays[time.Thursday]

	empCodeToName := inputEvent.Employees.EmpCodeToName
	empCodeToEmail := inputEvent.Employees.EmpCodeToEmail
	empCodeToID := inputEvent.Employees.EmpCodeToID
	activeEmpCodes := inputEvent.Employees.ActiveEmpCodes

	now := time.Now()

	// Step 1: Build absent set across all dates FIRST (before any API calls).
	// This determines which employees we actually need to fetch leaves for.
	type absentEntry struct {
		code  string
		empID int
		dates []string
	}
	absentByCode := map[string]*absentEntry{}
	dateAbsentCodes := map[string][]string{} // date -> list of absent codes

	for _, rec := range records {
		presentSet := sliceToSet(rec.PresentList)
		if len(presentSet) < 5 {
			log.Info("Skipping date %s: only %d present (likely a holiday)", rec.Date, len(presentSet))
			continue
		}

		weekday := ""
		if d, err := time.Parse("2006-01-02", rec.Date); err == nil {
			weekday = d.Weekday().String()
		}

		var absentCodes []string
		for code := range activeEmpCodes {
			if presentSet[code] || excludeSet[code] || wfhSet[code] {
				continue
			}
			if day, ok := wfhDays[code]; ok {
				if !midWeekHoliday && strings.EqualFold(strings.TrimSpace(day), weekday) {
					continue // default WFH day
				}
				// Holiday week: all absences stay in; one WFH day is picked after leave checks.
			}
			empID, hasID := empCodeToID[code]
			if !hasID {
				continue
			}

			if entry, ok := absentByCode[code]; ok {
				entry.dates = append(entry.dates, rec.Date)
			} else {
				absentByCode[code] = &absentEntry{code: code, empID: empID, dates: []string{rec.Date}}
			}
			absentCodes = append(absentCodes, code)
		}
		dateAbsentCodes[rec.Date] = absentCodes
	}

	log.Info("Found %d unique absent employees across %d dates", len(absentByCode), len(dateAbsentCodes))

	if len(absentByCode) == 0 {
		log.Info("No absentees to check")
		return "[]", nil
	}

	// Step 2: Fetch leaves ONLY for absent employees (month-1 window).
	leaveStart := now.AddDate(0, -1, 0).Format("2006-01-02")
	leaveEnd := now.Format("2006-01-02")
	log.Info("Fetching greytHR leave transactions from %s to %s for %d absent employees", leaveStart, leaveEnd, len(absentByCode))

	type fetchResult struct {
		code   string
		empID  int
		leaves []LeaveTransaction
		err    error
	}

	results := make(chan fetchResult, len(absentByCode))
	sem := make(chan struct{}, leaveFetchConcurrency)
	var wg sync.WaitGroup
	for _, entry := range absentByCode {
		wg.Add(1)
		go func(code string, empID int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			leaves, err := fetchEmployeeLeaveTransactions(token, config, empID, leaveStart, leaveEnd, log)
			results <- fetchResult{code: code, empID: empID, leaves: leaves, err: err}
		}(entry.code, entry.empID)
	}
	go func() { wg.Wait(); close(results) }()

	empLeaves := map[int][]LeaveTransaction{}
	failedFetch := map[string]bool{}
	for r := range results {
		if r.err != nil {
			log.Error("failed to fetch leaves for %s (id=%d), employee will be skipped: %v", r.code, r.empID, r.err)
			failedFetch[r.code] = true
			continue
		}
		if len(r.leaves) > 0 {
			empLeaves[r.empID] = r.leaves
		}
	}
	log.Info("Fetched leave data for %d employees, %d failed", len(empLeaves), len(failedFetch))

	// Step 3: Build the absentee report.
	var report []AbsenteeReport
	for _, rec := range records {
		absentCodes, ok := dateAbsentCodes[rec.Date]
		if !ok {
			continue // date was skipped (holiday)
		}

		var absentees []AbsenteeDetail
		var absenteeNames []string
		for _, code := range absentCodes {
			if failedFetch[code] {
				log.Warn("date %s: skipping %s, leave data unavailable", rec.Date, code)
				continue
			}

			empID := empCodeToID[code]
			leaves := leavesOnDate(empLeaves[empID], rec.Date)

			absentees = append(absentees, AbsenteeDetail{
				EmpCode:      code,
				EmpName:      empCodeToName[code],
				Email:        empCodeToEmail[code],
				LeaveApplied: len(leaves) > 0,
				Leaves:       leaves,
			})
			absenteeNames = append(absenteeNames, empCodeToName[code])
		}

		report = append(report, AbsenteeReport{
			Date:      rec.Date,
			Absentees: absentees,
		})
		log.Info("Date %s: %d Absentees [%s]", rec.Date, len(absentees), strings.Join(absenteeNames, ", "))
	}

	// Validation pass: for employees marked absent without leave, check
	// deeper windows (month-2 then month-3, each ≤30 days) before sending
	// mail. If a deeper fetch errors, the employee is removed entirely.
	deeperWindows := []struct{ start, end string }{
		{now.AddDate(0, -2, 0).Format("2006-01-02"), now.AddDate(0, -1, 0).Format("2006-01-02")},
		{now.AddDate(0, -3, 0).Format("2006-01-02"), now.AddDate(0, -2, 0).Format("2006-01-02")},
	}

	needsCheck := map[string]bool{}
	for _, rep := range report {
		for _, a := range rep.Absentees {
			if !a.LeaveApplied {
				needsCheck[a.EmpCode] = true
			}
		}
	}

	for _, w := range deeperWindows {
		if len(needsCheck) == 0 {
			break
		}
		log.Info("Deeper check window %s to %s for %d employees", w.start, w.end, len(needsCheck))

		stillNeeds := map[string]bool{}
		for code := range needsCheck {
			empID := empCodeToID[code]
			leaves, err := fetchEmployeeLeaveTransactions(token, config, empID, w.start, w.end, log)
			if err != nil {
				log.Error("deeper check failed for %s (id=%d), will be skipped: %v", code, empID, err)
				failedFetch[code] = true
				continue
			}

			found := false
			for i := range report {
				for j := range report[i].Absentees {
					a := &report[i].Absentees[j]
					if a.EmpCode != code || a.LeaveApplied {
						continue
					}
					matching := leavesOnDate(leaves, report[i].Date)
					if len(matching) > 0 {
						a.LeaveApplied = true
						a.Leaves = matching
						found = true
					}
				}
			}
			if !found {
				stillNeeds[code] = true
			}
		}
		needsCheck = stillNeeds
	}

	// Holiday week: the WFH day can move, but there is still only ONE.
	// Excuse one absence WITHOUT leave per WFH_EMP_DAYS employee, preferring
	// their default day; the rest get mail.
	// e.g. Wed holiday, WFH Tue: Tue leave + Thu absent -> Thu excused;
	// Tue + Thu absent, no leave -> Tue excused, Thu mailed.
	if midWeekHoliday {
		wfhDate := map[string]string{} // empCode -> excused date
		for _, rep := range report {
			weekday := ""
			if d, err := time.Parse("2006-01-02", rep.Date); err == nil {
				weekday = d.Weekday().String()
			}
			for _, a := range rep.Absentees {
				day, ok := wfhDays[a.EmpCode]
				if !ok || a.LeaveApplied {
					continue
				}
				if _, set := wfhDate[a.EmpCode]; !set || strings.EqualFold(strings.TrimSpace(day), weekday) {
					wfhDate[a.EmpCode] = rep.Date
				}
			}
		}
		for i := range report {
			var kept []AbsenteeDetail
			for _, a := range report[i].Absentees {
				if !a.LeaveApplied && wfhDate[a.EmpCode] == report[i].Date {
					continue
				}
				kept = append(kept, a)
			}
			report[i].Absentees = kept
		}
	}

	// Remove employees whose deeper check failed from the report entirely.
	for i := range report {
		var kept []AbsenteeDetail
		for _, a := range report[i].Absentees {
			if !failedFetch[a.EmpCode] {
				kept = append(kept, a)
			}
		}
		report[i].Absentees = kept
	}

	output, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("marshal report: %w", err)
	}
	return string(output), nil
}

func csvToSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			set[trimmed] = true
		}
	}
	return set
}

func sliceToSet(items []string) map[string]bool {
	set := map[string]bool{}
	for _, item := range items {
		code := strings.TrimSpace(item)
		code = padEmpCode(code, 5)
		set[code] = true
	}
	return set
}

func padEmpCode(code string, width int) string {
	for len(code) < width {
		code = "0" + code
	}
	return code
}

var (
	leaveFetchMaxAttempts = 5
	leaveFetchBaseBackoff = 2 * time.Second
	leaveFetchConcurrency = 5
)

func fetchEmployeeLeaveTransactions(token string, config map[string]string, empID int, startDate, endDate string, log *log.Logger) ([]LeaveTransaction, error) {
	url := fmt.Sprintf("%s/leave/v2/employee/%d/transactions?start=%s&end=%s", config["baseURL"], empID, startDate, endDate)

	var body []byte
	var err error
	for attempt := 1; attempt <= leaveFetchMaxAttempts; attempt++ {
		body, err = greythrGet(url, token, config["x_greythr_domain"])
		if err == nil || !isRetryable(err) || attempt == leaveFetchMaxAttempts {
			break
		}
		backoff := leaveFetchBaseBackoff * time.Duration(1<<(attempt-1))
		log.Warn("fetch leave for employee %d failed (attempt %d/%d), retrying in %s: %v", empID, attempt, leaveFetchMaxAttempts, backoff, err)
		time.Sleep(backoff)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch leave for employee %d: %w", empID, err)
	}

	var resp EmployeeLeaveData
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal leave for employee %d: %w", empID, err)
	}
	return resp.List, nil
}

func greythrGet(url, token, domain string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("ACCESS-TOKEN", token)
	req.Header.Set("x-greythr-domain", domain)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}

type httpStatusError struct {
	StatusCode int
	Body       string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("status %d: %s", e.StatusCode, e.Body)
}

// Network errors and client timeouts are retryable; of HTTP statuses only 429 and 5xx are.
func isRetryable(err error) bool {
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode == http.StatusTooManyRequests || statusErr.StatusCode >= 500
	}
	return true
}

func leavesOnDate(transactions []LeaveTransaction, date string) []LeaveTransaction {
	var matching []LeaveTransaction
	for _, t := range transactions {
		if t.Cancelled {
			continue
		}
		if date >= t.FromDate && date <= t.ToDate {
			matching = append(matching, t)
		}
	}
	return matching
}
