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

type EmployeesResponse struct {
	Data  []EmployeeInfo `json:"data"`
	Pages PageInfo       `json:"pages"`
}

type EmployeeInfo struct {
	EmployeeID int    `json:"employeeId"`
	EmployeeNo string `json:"employeeNo"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Leftorg    bool   `json:"leftorg"`
}

type PageInfo struct {
	TotalPages    int  `json:"totalPages"`
	TotalElements int  `json:"totalElements"`
	HasNext       bool `json:"hasNext"`
	Size          int  `json:"size"`
}

type EmpData struct {
	ActiveEmpCodes map[string]bool   `json:"activeEmpCodes"`
	EmpCodeToName  map[string]string `json:"empCodeToName"`
	EmpCodeToEmail map[string]string `json:"empCodeToEmail"`
	EmpCodeToID    map[string]int    `json:"empCodeToID"`
	EmpIDToCode    map[int]string    `json:"empIDToCode"`
}

func Execute(event string, config map[string]string, vars map[string]string, log *log.Logger) (string, error) {
	log.Info("inside get all employees handler")

	token := vars["greythrToken"]

	allEmployees, err := fetchAllEmployees(token, config, log)
	if err != nil {
		return "", fmt.Errorf("fetch employees: %w", err)
	}
	log.Info("fetched %d employees from GreytHR", len(allEmployees))

	empCodeToName := map[string]string{}
	empCodeToEmail := map[string]string{}
	empCodeToID := map[string]int{}
	empIDToCode := map[int]string{}
	activeEmpCodes := map[string]bool{}

	for _, emp := range allEmployees {
		if emp.Leftorg {
			continue
		}
		code := strings.TrimSpace(emp.EmployeeNo)
		if code == "" {
			continue
		}
		empCodeToName[code] = emp.Name
		empCodeToEmail[code] = emp.Email
		empCodeToID[code] = emp.EmployeeID
		empIDToCode[emp.EmployeeID] = code
		activeEmpCodes[code] = true
	}
	log.Info("active employees: %d", len(activeEmpCodes))

	outputEvent := map[string]interface{}{
		"employees": EmpData{
			ActiveEmpCodes: activeEmpCodes,
			EmpCodeToName:  empCodeToName,
			EmpCodeToEmail: empCodeToEmail,
			EmpCodeToID:    empCodeToID,
			EmpIDToCode:    empIDToCode,
		},
	}

	output, err := json.Marshal(outputEvent)
	if err != nil {
		log.Error("error marshalling result: %v", err)
		return "", err
	}

	return string(output), nil
}

func fetchAllEmployees(token string, config map[string]string, log *log.Logger) ([]EmployeeInfo, error) {
	var all []EmployeeInfo
	page := 0

	for {
		employees, pages, err := fetchEmployeesPage(token, config, page)
		if err != nil {
			return nil, err
		}
		all = append(all, employees...)
		log.Info("employees page %d: %d records", page, len(employees))

		if !pages.HasNext {
			break
		}
		page++
	}
	return all, nil
}

func fetchEmployeesPage(token string, config map[string]string, page int) ([]EmployeeInfo, PageInfo, error) {
	url := fmt.Sprintf("%s/employee/v2/employees?page=%d&size=100", config["baseURL"], page)
	body, err := greythrGet(url, token, config["x_greythr_domain"])
	if err != nil {
		return nil, PageInfo{}, fmt.Errorf("employees page %d: %w", page, err)
	}

	var resp EmployeesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, PageInfo{}, fmt.Errorf("unmarshal employees page %d: %w", page, err)
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
