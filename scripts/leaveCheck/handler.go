package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

type LeaveTransactionsResponse struct {
	Data  []EmployeeLeaveData `json:"data"`
	Pages PageInfo            `json:"pages"`
}

type EmployeeLeaveData struct {
	EmployeeID int                `json:"employeeId"`
	List       []LeaveTransaction `json:"list"`
}

type PageInfo struct {
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	HasNext       bool `json:"hasNext"`
	Size          int  `json:"size"`
}

type LeaveTransaction struct {
	ID                   int     `json:"id"`
	LeaveTypeCategory    int     `json:"leaveTypeCategory"`
	LeaveTransactionType int     `json:"leaveTransactionType"`
	FromDate             string  `json:"fromDate"`
	ToDate               string  `json:"toDate"`
	Days                 float64 `json:"days"`
	FromSession          int     `json:"fromSession"`
	ToSession            int     `json:"toSession"`
	Remarks              string  `json:"remarks"`
	Cancelled            bool    `json:"cancelled"`
	Reason               string  `json:"reason"`
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
	log.Info("inside leave check handler")

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

	// token := vars["greythrToken"]
	token := "ory_at_FydDDLZvE_Ff92uNLlmC9bjd99sR1d5v-mkRs3Arjuw.r1SgEnK6s9jAHqhhBBURVB-_VZXZzYbpTKNTEl0w9zs"
	excludeSet := csvToSet(vars["EXCLUDE_EMP_CODES"])
	wfhSet := csvToSet(vars["WFH_EMP_CODES"])

	empCodeToName := inputEvent.Employees.EmpCodeToName
	empCodeToEmail := inputEvent.Employees.EmpCodeToEmail
	empCodeToID := inputEvent.Employees.EmpCodeToID
	activeEmpCodes := inputEvent.Employees.ActiveEmpCodes

	log.Info("Active Employees: %d", len(activeEmpCodes))

	minDate, maxDate := dateRange(records)
	log.Info("date range: %s to %s", minDate, maxDate)

	leaveData, err := fetchAllLeaveTransactions(token, config, minDate, maxDate, log)
	if err != nil {
		return "", fmt.Errorf("fetch leave transactions: %w", err)
	}
	log.Info("fetched leave data for %d employees", len(leaveData))

	empLeaves := map[int][]LeaveTransaction{}
	for _, eld := range leaveData {
		empLeaves[eld.EmployeeID] = eld.List
	}

	var report []AbsenteeReport
	for _, rec := range records {
		presentSet := sliceToSet(rec.PresentList)

		if len(presentSet) < 5 {
			log.Info("skipping date %s: only %d present (likely a holiday)", rec.Date, len(presentSet))
			continue
		}
		// log.Info("Present Employees on date %s: %v", rec.Date, presentSet)

		var absentees []AbsenteeDetail
		var absenteeNames []string
		for code := range activeEmpCodes {
			if presentSet[code] || excludeSet[code] || wfhSet[code] {
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
		log.Info("date %s: %d absentees", rec.Date, len(absentees))
		log.Info("date %s: absentees %v", rec.Date, absenteeNames)
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

func dateRange(records []AttendanceRecord) (string, string) {
	min, max := records[0].Date, records[0].Date
	for _, r := range records[1:] {
		if r.Date < min {
			min = r.Date
		}
		if r.Date > max {
			max = r.Date
		}
	}
	return min, max
}

func fetchAllLeaveTransactions(token string, config map[string]string, startDate, endDate string, log *log.Logger) ([]EmployeeLeaveData, error) {
	var all []EmployeeLeaveData
	page := 0

	for {
		data, pages, err := fetchLeaveTransactionsPage(token, config, startDate, endDate, page)
		if err != nil {
			return nil, err
		}
		all = append(all, data...)
		// log.Info("leave transactions page %d: %d employee records", page, len(data))

		if !pages.HasNext {
			break
		}
		page++
	}
	return all, nil
}

func fetchLeaveTransactionsPage(token string, config map[string]string, startDate, endDate string, page int) ([]EmployeeLeaveData, PageInfo, error) {
	url := fmt.Sprintf("%s/leave/v2/employee/transactions?start=%s&end=%s&page=%d", config["baseURL"], startDate, endDate, page)
	body, err := greythrGet(url, token, config["x_greythr_domain"])
	if err != nil {
		return nil, PageInfo{}, fmt.Errorf("leave transactions page %d: %w", page, err)
	}

	var resp LeaveTransactionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, PageInfo{}, fmt.Errorf("unmarshal leave transactions page %d: %w", page, err)
	}
	return resp.Data, resp.Pages, nil
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
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
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
