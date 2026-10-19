package fetchfrommachine

import (
	"testing"
	"time"
)

func TestParseAttendance_Success(t *testing.T) {
	html := `1|#||4|70333|updatePanel|ReportViewer1_ctl09_ReportArea|<div>
	<TR VALIGN="top"><TD><DIV>Emp Code:</DIV></TD><TD><DIV>1</DIV></TD><TD><DIV>Employee Name :</DIV></TD><TD><DIV>Test Employee1</DIV></TD></TR>
	<TR VALIGN="top"><TD><DIV>08-Sep-2026</DIV></TD><TD><DIV>NS</DIV></TD><TD><DIV>10:00</DIV></TD><TD><DIV>Present</DIV></TD></TR>
	<TR VALIGN="top"><TD><DIV>09-Sep-2026</DIV></TD><TD><DIV>NS</DIV></TD><TD><DIV>Absent</DIV></TD></TR>
	<TR VALIGN="top"><TD><DIV>Emp Code:</DIV>
</TD><TD><DIV>2</DIV></TD><TD><DIV>Employee Name :</DIV></TD><TD><DIV>VISHNU</DIV></TD></TR>
	<TR VALIGN="top"><TD><DIV>08-Sep-2026</DIV></TD><TD><DIV>Present</DIV></TD></TR>
	<TR VALIGN="top"><TD><DIV>09-Sep-2026</DIV></TD><TD><DIV>1/2Present</DIV></TD></TR>
	</div>`

	rows, err := ParseAttendance(html)
	if err != nil {
		t.Fatalf("ParseAttendance() error = %v", err)
	}

	if len(rows) != 3 {
		t.Errorf("got %d rows, want 3", len(rows))
	}

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
