package main

import (
	"sort"
	"strings"
	"time"
)

type AttendanceRow struct {
	BadgeNumber  string
	EmployeeName string
	CheckTime    time.Time
}

type DayAttendance struct {
	Date            string   `json:"date"`
	Present         []string `json:"present"`
	PresentEmpNames []string `json:"presentEmpNames,omitempty"`
}

func ProcessAttendance(rows []AttendanceRow) []DayAttendance {
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
