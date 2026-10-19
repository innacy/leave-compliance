package fetchfrommachine

import (
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AttendanceRow struct {
	BadgeNumber  string
	EmployeeName string
	CheckTime    time.Time
}

var (
	trRegex  = regexp.MustCompile(`(?is)<tr\b[^>]*>.*?</tr>`)
	divRegex = regexp.MustCompile(`(?is)<div\b[^>]*>([^<]*)</div>`)
	dateRegex = regexp.MustCompile(`^\d{2}-[A-Za-z]{3}-\d{4}$`)
)

var presentStatuses = map[string]bool{
	"Present":     true,
	"1/2Present":  true,
	"1/2 Present": true,
}

func ParseAttendance(html string) ([]AttendanceRow, error) {
	var rows []AttendanceRow
	var code, name string
	statusCount := map[string]int{}
	employees := 0

	for _, tr := range trRegex.FindAllString(html, -1) {
		cells := rowCells(tr)
		if len(cells) == 0 {
			continue
		}
		if containsCell(cells, "Emp Code:") {
			employees++
			code, name = employeeFromCells(cells)
			continue
		}
		if !dateRegex.MatchString(cells[0]) || code == "" {
			continue
		}
		status := statusIn(cells)
		if status == "" {
			continue
		}
		statusCount[status]++
		if !presentStatuses[status] {
			continue
		}
		checkTime, err := time.Parse("02-Jan-2006", cells[0])
		if err != nil {
			continue
		}
		rows = append(rows, AttendanceRow{
			BadgeNumber:  code,
			EmployeeName: name,
			CheckTime:    checkTime,
		})
	}

	log.Printf("attendance table: %d employees, statuses %s, present rows %d", employees, formatCounts(statusCount), len(rows))
	return rows, nil
}

func rowCells(tr string) []string {
	var cells []string
	for _, m := range divRegex.FindAllStringSubmatch(tr, -1) {
		text := strings.TrimSpace(m[1])
		if text == "" || text == "&nbsp;" {
			continue
		}
		cells = append(cells, text)
	}
	return cells
}

func containsCell(cells []string, want string) bool {
	for _, c := range cells {
		if c == want {
			return true
		}
	}
	return false
}

func employeeFromCells(cells []string) (code, name string) {
	for i, c := range cells {
		if c == "Emp Code:" && i+1 < len(cells) {
			code = cells[i+1]
		}
		if strings.HasPrefix(c, "Employee Name") && i+1 < len(cells) {
			name = cells[i+1]
		}
	}
	return code, name
}

func statusIn(cells []string) string {
	for _, c := range cells {
		if presentStatuses[c] || c == "Absent" || c == "WeeklyOff" || c == "On Leave" || c == "Holiday" || c == "Late Coming" || c == "Early Going" || c == "Short Worked" || c == "Extra Worked" || c == "Missed Out Punch" {
			return c
		}
	}
	return ""
}

func formatCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + strconv.Itoa(counts[k])
	}
	return strings.Join(parts, ", ")
}
