package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zinscky/log"
)

func TestMain(m *testing.M) {
	leaveFetchBaseBackoff = time.Millisecond
	os.Exit(m.Run())
}

// ---------- test helpers ----------

type testEmp struct {
	Code  string
	ID    int
	Name  string
	Email string
}

// 5 always-present employees so dates pass the holiday check (< 5 present → skip).
// Use high codes to avoid collisions with test employees.
var presentCodes = []string{"00090", "00091", "00092", "00093", "00094"}

func empData(emps ...testEmp) EmpData {
	active := map[string]bool{
		"00090": true, "00091": true, "00092": true, "00093": true, "00094": true,
	}
	ids := map[string]int{}
	names := map[string]string{}
	emails := map[string]string{}
	for _, e := range emps {
		active[e.Code] = true
		ids[e.Code] = e.ID
		names[e.Code] = e.Name
		emails[e.Code] = e.Email
	}
	return EmpData{
		ActiveEmpCodes: active,
		EmpCodeToID:    ids,
		EmpCodeToName:  names,
		EmpCodeToEmail: emails,
	}
}

func buildEvent(t *testing.T, records []AttendanceRecord, emp EmpData) string {
	t.Helper()
	b, err := json.Marshal(InputEvent{PresentList: records, Employees: emp})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func parseReport(t *testing.T, out string) []AbsenteeReport {
	t.Helper()
	var r []AbsenteeReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	return r
}

func findAbsentee(report []AbsenteeReport, date, code string) *AbsenteeDetail {
	for _, r := range report {
		if r.Date != date {
			continue
		}
		for i := range r.Absentees {
			if r.Absentees[i].EmpCode == code {
				return &r.Absentees[i]
			}
		}
	}
	return nil
}

func assertNotInReport(t *testing.T, report []AbsenteeReport, code string) {
	t.Helper()
	for _, r := range report {
		for _, a := range r.Absentees {
			if a.EmpCode == code {
				t.Errorf("employee %s must NOT be in report but found on date %s", code, r.Date)
			}
		}
	}
}

// fakeGreytHR simulates the greytHR leave transaction API.
// respond receives (employeeID, per-employee call sequence 1-based, start, end)
// and returns (HTTP status [0 = 200], leave data).
type fakeGreytHR struct {
	mu       sync.Mutex
	respond  func(empID, callN int, start, end string) (int, *EmployeeLeaveData)
	reqs     []grReq
	empCalls map[int]int
}

type grReq struct {
	EmpID      int
	Start, End string
}

func (f *fakeGreytHR) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var empID int
	parts := strings.Split(r.URL.Path, "/")
	for i, p := range parts {
		if p == "employee" && i+1 < len(parts) {
			fmt.Sscanf(parts[i+1], "%d", &empID)
			break
		}
	}

	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")

	f.mu.Lock()
	if f.empCalls == nil {
		f.empCalls = map[int]int{}
	}
	f.empCalls[empID]++
	n := f.empCalls[empID]
	f.reqs = append(f.reqs, grReq{EmpID: empID, Start: start, End: end})
	f.mu.Unlock()

	status, data := f.respond(empID, n, start, end)
	if status == 0 {
		status = 200
	}
	if status != 200 {
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"error":"test %d"}`, status)
		return
	}
	json.NewEncoder(w).Encode(data)
}

func (f *fakeGreytHR) callsFor(empID int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.empCalls[empID]
}

const testDate = "2026-10-01"

// run executes the handler against the fake server with the given employees
// absent on testDate (the presentCodes employees are always present).
func run(t *testing.T, fake *fakeGreytHR, emps ...testEmp) []AbsenteeReport {
	t.Helper()
	srv := httptest.NewServer(fake)
	defer srv.Close()

	event := buildEvent(t, []AttendanceRecord{{
		Date:        testDate,
		PresentList: presentCodes,
	}}, empData(emps...))

	lg := log.New(0, "test")
	out, err := Execute(event, map[string]string{"baseURL": srv.URL}, map[string]string{}, lg)
	if err != nil {
		t.Fatal(err)
	}
	return parseReport(t, out)
}

func emptyLeave(empID int) (int, *EmployeeLeaveData) {
	return 0, &EmployeeLeaveData{EmployeeID: empID}
}

func leaveOnTestDate(empID int) (int, *EmployeeLeaveData) {
	return 0, &EmployeeLeaveData{
		EmployeeID: empID,
		List: []LeaveTransaction{
			{ID: 100 + empID, FromDate: testDate, ToDate: testDate, Days: 1},
		},
	}
}

// ---------- Cycle 1: tracer bullet — leave found in first month ----------

func TestLeaveFoundInFirstMonth(t *testing.T) {
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID == 1 {
			return leaveOnTestDate(1)
		}
		return emptyLeave(empID)
	}}

	report := run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})

	a := findAbsentee(report, testDate, "00001")
	if a == nil {
		t.Fatal("expected 00001 in report")
	}
	if !a.LeaveApplied {
		t.Error("expected leaveApplied=true")
	}
	if len(a.Leaves) != 1 {
		t.Errorf("expected 1 leave, got %d", len(a.Leaves))
	}
	// Should only hit greytHR once for this employee (no deeper check needed).
	if got := fake.callsFor(1); got != 1 {
		t.Errorf("expected 1 API call for emp 1, got %d", got)
	}
}

// ---------- Cycle 2: deeper check — leave found in second month ----------

func TestNoLeaveMonth1_FoundInMonth2(t *testing.T) {
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID == 1 {
			if n == 1 {
				return emptyLeave(1) // month-1: nothing
			}
			return leaveOnTestDate(1) // month-2: found
		}
		return emptyLeave(empID)
	}}

	report := run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})

	a := findAbsentee(report, testDate, "00001")
	if a == nil {
		t.Fatal("expected 00001 in report")
	}
	if !a.LeaveApplied {
		t.Error("expected leaveApplied=true after deeper check")
	}
	// Should hit greytHR twice: initial + one deeper window.
	if got := fake.callsFor(1); got != 2 {
		t.Errorf("expected 2 API calls for emp 1, got %d", got)
	}
}

// ---------- Cycle 3: deepest check — leave found only in month-3 ----------

func TestNoLeaveMonth1And2_FoundInMonth3(t *testing.T) {
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID == 1 {
			if n <= 2 {
				return emptyLeave(1) // month-1 and month-2: nothing
			}
			return leaveOnTestDate(1) // month-3: found
		}
		return emptyLeave(empID)
	}}

	report := run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})

	a := findAbsentee(report, testDate, "00001")
	if a == nil {
		t.Fatal("expected 00001 in report")
	}
	if !a.LeaveApplied {
		t.Error("expected leaveApplied=true after deepest check")
	}
	// 3 calls: initial + 2 deeper windows.
	if got := fake.callsFor(1); got != 3 {
		t.Errorf("expected 3 API calls for emp 1, got %d", got)
	}
}

// ---------- Cycle 4: no leave in any month — leaveApplied stays false ----------

func TestNoLeaveInAnyMonth(t *testing.T) {
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		return emptyLeave(empID)
	}}

	report := run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})

	a := findAbsentee(report, testDate, "00001")
	if a == nil {
		t.Fatal("expected 00001 in report")
	}
	if a.LeaveApplied {
		t.Error("expected leaveApplied=false when no leave exists")
	}
	// 3 calls: initial + 2 deeper windows, all empty.
	if got := fake.callsFor(1); got != 3 {
		t.Errorf("expected 3 API calls for emp 1, got %d", got)
	}
}

// ---------- Cycle 5: error on initial fetch → skip (no mail) ----------

func TestErrorOnInitialFetch_EmployeeSkipped(t *testing.T) {
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID == 1 {
			return http.StatusNotFound, nil // non-retryable error
		}
		return emptyLeave(empID)
	}}

	report := run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})
	assertNotInReport(t, report, "00001")
}

// ---------- Cycle 6: error on deeper fetch → skip (no mail) ----------

func TestErrorOnDeeperFetch_EmployeeSkipped(t *testing.T) {
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID == 1 {
			if n == 1 {
				return emptyLeave(1) // month-1: no leave
			}
			return http.StatusGatewayTimeout, nil // deeper check fails
		}
		return emptyLeave(empID)
	}}

	report := run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})
	assertNotInReport(t, report, "00001")
}

// ---------- Cycle 7: error on third-month fetch → skip ----------

func TestErrorOnThirdMonthFetch_EmployeeSkipped(t *testing.T) {
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID == 1 {
			if n <= 2 {
				return emptyLeave(1) // months 1-2: no leave
			}
			return http.StatusGatewayTimeout, nil // month-3 fails
		}
		return emptyLeave(empID)
	}}

	report := run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})
	assertNotInReport(t, report, "00001")
}

// ---------- Cycle 8: gateway timeout → retry → success on deeper check ----------

func TestDeeperCheck_RetrySucceeds(t *testing.T) {
	// callN counts every request the fake server sees for an employee (inc retries).
	// n=1: initial fetch (empty), n=2: deeper first try (504),
	// n=3: deeper retry (success).
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID == 1 {
			switch n {
			case 1:
				return emptyLeave(1)
			case 2:
				return http.StatusGatewayTimeout, nil
			default:
				return leaveOnTestDate(1)
			}
		}
		return emptyLeave(empID)
	}}

	report := run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})

	a := findAbsentee(report, testDate, "00001")
	if a == nil {
		t.Fatal("expected 00001 in report after successful retry")
	}
	if !a.LeaveApplied {
		t.Error("expected leaveApplied=true after retry found leave")
	}
}

// ---------- Cycle 9: safety invariant — multi-employee mixed scenario ----------
// emp 1: leave in month-1 (OK)
// emp 2: no leave in month-1, found in month-2 (OK)
// emp 3: initial fetch errors (SKIP)
// emp 4: no leave in month-1, deeper check errors (SKIP)
// emp 5: no leave anywhere (leaveApplied=false, but still in report — mail IS sent)
// Invariant: employees 3 and 4 must NEVER appear in the report.

func TestSafetyInvariant_NoFailedEmployeeInReport(t *testing.T) {
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		switch empID {
		case 1:
			return leaveOnTestDate(1)
		case 2:
			if n == 1 {
				return emptyLeave(2)
			}
			return leaveOnTestDate(2)
		case 3:
			return http.StatusNotFound, nil
		case 4:
			if n == 1 {
				return emptyLeave(4)
			}
			return http.StatusGatewayTimeout, nil
		case 5:
			return emptyLeave(5)
		}
		return emptyLeave(empID)
	}}

	report := run(t, fake,
		testEmp{"00001", 1, "Alice", "a@t.com"},
		testEmp{"00002", 2, "Bob", "b@t.com"},
		testEmp{"00003", 3, "Carol", "c@t.com"},
		testEmp{"00004", 4, "Dave", "d@t.com"},
		testEmp{"00005", 5, "Eve", "e@t.com"},
	)

	// emp 1: in report, leave applied
	if a := findAbsentee(report, testDate, "00001"); a == nil {
		t.Error("emp 1 should be in report")
	} else if !a.LeaveApplied {
		t.Error("emp 1 should have leaveApplied=true")
	}

	// emp 2: in report, leave found via deeper check
	if a := findAbsentee(report, testDate, "00002"); a == nil {
		t.Error("emp 2 should be in report")
	} else if !a.LeaveApplied {
		t.Error("emp 2 should have leaveApplied=true via deeper check")
	}

	// emp 3: MUST NOT be in report (initial fetch error)
	assertNotInReport(t, report, "00003")

	// emp 4: MUST NOT be in report (deeper fetch error)
	assertNotInReport(t, report, "00004")

	// emp 5: in report, no leave
	if a := findAbsentee(report, testDate, "00005"); a == nil {
		t.Error("emp 5 should be in report")
	} else if a.LeaveApplied {
		t.Error("emp 5 should have leaveApplied=false")
	}
}

// ---------- Cycle 10: parallel fetch — 20 employees, correct results ----------

func TestParallelFetch_ManyEmployees(t *testing.T) {
	// Even-ID employees have leave, odd-ID don't.
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID%2 == 0 {
			return leaveOnTestDate(empID)
		}
		return emptyLeave(empID)
	}}

	var emps []testEmp
	for i := 1; i <= 20; i++ {
		code := fmt.Sprintf("%05d", i)
		emps = append(emps, testEmp{code, i, fmt.Sprintf("Emp%d", i), fmt.Sprintf("e%d@t.com", i)})
	}

	report := run(t, fake, emps...)

	for i := 1; i <= 20; i++ {
		code := fmt.Sprintf("%05d", i)
		a := findAbsentee(report, testDate, code)
		if a == nil {
			t.Errorf("emp %s should be in report", code)
			continue
		}
		if i%2 == 0 {
			if !a.LeaveApplied {
				t.Errorf("emp %s (even) should have leaveApplied=true", code)
			}
		} else {
			if a.LeaveApplied {
				t.Errorf("emp %s (odd) should have leaveApplied=false", code)
			}
		}
	}
}

// ---------- Cycle 11: verify each API window is ≤30 days ----------

func TestWindowsAreMax30Days(t *testing.T) {
	// Use a single employee with no leave so all 3 windows are queried.
	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		return emptyLeave(empID)
	}}

	_ = run(t, fake, testEmp{"00001", 1, "Alice", "a@t.com"})

	reqs := fake.reqs
	var empReqs []grReq
	for _, r := range reqs {
		if r.EmpID == 1 {
			empReqs = append(empReqs, r)
		}
	}
	if len(empReqs) != 3 {
		t.Fatalf("expected 3 API calls for emp 1, got %d", len(empReqs))
	}

	for i, r := range empReqs {
		start, err1 := time.Parse("2006-01-02", r.Start)
		end, err2 := time.Parse("2006-01-02", r.End)
		if err1 != nil || err2 != nil {
			t.Fatalf("window %d: invalid dates start=%q end=%q", i+1, r.Start, r.End)
		}
		days := int(end.Sub(start).Hours() / 24)
		if days > 31 || days < 27 {
			t.Errorf("window %d: span %d days (start=%s end=%s), expected ~30", i+1, days, r.Start, r.End)
		}
	}

	// Windows should be contiguous: window N+1 end == window N start.
	for i := 1; i < len(empReqs); i++ {
		if empReqs[i].End != empReqs[i-1].Start {
			t.Errorf("window %d end (%s) != window %d start (%s): not contiguous",
				i+1, empReqs[i].End, i, empReqs[i-1].Start)
		}
	}
}

// ---------- Cycle 12: multiple dates, deeper check covers subset ----------
// Employee absent on 2 dates. Month-1 has leave for date-A only.
// Deeper check finds leave for date-B.

func TestMultipleDates_DeeperCheckFindsRemaining(t *testing.T) {
	dateA := "2026-10-01"
	dateB := "2026-10-02"

	fake := &fakeGreytHR{respond: func(empID, n int, start, end string) (int, *EmployeeLeaveData) {
		if empID == 1 {
			if n == 1 {
				// month-1: leave only on dateA
				return 0, &EmployeeLeaveData{EmployeeID: 1, List: []LeaveTransaction{
					{ID: 1, FromDate: dateA, ToDate: dateA, Days: 1},
				}}
			}
			// deeper: leave on dateB
			return 0, &EmployeeLeaveData{EmployeeID: 1, List: []LeaveTransaction{
				{ID: 2, FromDate: dateB, ToDate: dateB, Days: 1},
			}}
		}
		return emptyLeave(empID)
	}}

	srv := httptest.NewServer(fake)
	defer srv.Close()

	event := buildEvent(t, []AttendanceRecord{
		{Date: dateA, PresentList: presentCodes},
		{Date: dateB, PresentList: presentCodes},
	}, empData(testEmp{"00001", 1, "Alice", "a@t.com"}))

	lg := log.New(0, "test")
	out, err := Execute(event, map[string]string{"baseURL": srv.URL}, map[string]string{}, lg)
	if err != nil {
		t.Fatal(err)
	}
	report := parseReport(t, out)

	// dateA: leave found in month-1
	aA := findAbsentee(report, dateA, "00001")
	if aA == nil {
		t.Fatal("expected 00001 on dateA")
	}
	if !aA.LeaveApplied {
		t.Error("dateA: expected leaveApplied=true")
	}

	// dateB: leave found via deeper check
	aB := findAbsentee(report, dateB, "00001")
	if aB == nil {
		t.Fatal("expected 00001 on dateB")
	}
	if !aB.LeaveApplied {
		t.Error("dateB: expected leaveApplied=true via deeper check")
	}
}
