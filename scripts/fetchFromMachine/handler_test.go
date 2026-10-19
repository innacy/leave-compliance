package fetchfrommachine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecute_Success(t *testing.T) {
	esslServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/iclock/Default.aspx" && r.Method == http.MethodGet:
			http.SetCookie(w, &http.Cookie{Name: "ASP.NET_SessionId", Value: "test"})
			w.Write([]byte(`<input type="hidden" name="__VIEWSTATE" value="loginVS"/><input type="hidden" name="__VIEWSTATEGENERATOR" value="loginGEN"/>`))
		case r.URL.Path == "/iclock/Default.aspx":
			http.SetCookie(w, &http.Cookie{Name: "ASP.NET_SessionId", Value: "test"})
			w.Write([]byte(`<html><title>Home</title></html>`))
		case r.URL.Path == "/iclock/Reports/DailyAttendanceReport.aspx" && r.Method == http.MethodGet:
			w.Write([]byte(`<input name="__VIEWSTATE" value="vs123"/><input name="__VIEWSTATEGENERATOR" value="vsg456"/>`))
		case r.URL.Path == "/iclock/Reports/DailyAttendanceReport.aspx" && r.Method == http.MethodPost:
			w.Write([]byte(`1|#||4|1000|updatePanel|ReportViewer1_ctl09_ReportArea|
				<DIV>Emp Code:</DIV></TD><TD><DIV>1</DIV></TD>
				<DIV>Employee Name :</DIV></TD><TD><DIV>Test</DIV></TD>
				<TR VALIGN="top"><TD><DIV>01-Oct-2026</DIV></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD></TD><TD><DIV>Present</DIV></TD></TR>
			`))
		}
	}))
	defer esslServer.Close()

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
