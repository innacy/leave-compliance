package fetchfrommachine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestESSLClient_Login_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "Default.aspx") {
			t.Errorf("expected path containing Default.aspx, got %s", r.URL.Path)
		}
		http.SetCookie(w, &http.Cookie{
			Name:  "ASP.NET_SessionId",
			Value: "testsession123",
		})
		if r.Method == http.MethodGet {
			w.Write([]byte(`<input type="hidden" name="__VIEWSTATE" value="freshVS" /><input type="hidden" name="__VIEWSTATEGENERATOR" value="GEN1" />`))
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		r.ParseForm()
		if r.FormValue("__VIEWSTATE") != "freshVS" {
			t.Errorf("viewstate = %q, want fresh value from the login page", r.FormValue("__VIEWSTATE"))
		}
		if r.FormValue("StaffloginDialog$txt_LoginName") != "testuser" {
			t.Errorf("unexpected username: %s", r.FormValue("StaffloginDialog$txt_LoginName"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><title>Home</title></html>`))
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

func TestNormalizeBase64(t *testing.T) {
	cases := map[string]string{
		"j8JTOGSIQb6qUwJ1Q3vucQ==":     "j8JTOGSIQb6qUwJ1Q3vucQ==",
		"j8JTOGSIQb6qUwJ1Q3vucQ":       "j8JTOGSIQb6qUwJ1Q3vucQ==",
		"j8JTOGSIQb6qUwJ1Q3vucQ%3D%3D": "j8JTOGSIQb6qUwJ1Q3vucQ==",
	}
	for in, want := range cases {
		if got := normalizeBase64(in); got != want {
			t.Errorf("normalizeBase64(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestESSLClient_Login_StillOnLoginPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "ASP.NET_SessionId", Value: "sess"})
		if r.Method == http.MethodGet {
			w.Write([]byte(`<input type="hidden" name="__VIEWSTATE" value="vs" /><input type="hidden" name="__VIEWSTATEGENERATOR" value="gen" />`))
			return
		}
		w.Write([]byte(`<input name="StaffloginDialog$txt_LoginName" />`))
	}))
	defer server.Close()

	client := NewESSLClient(&Config{ESSLURL: server.URL, ESSLUsername: "essl", ESSLPasswordEncrypted: "x", ESSLEncryptionKey: "k"})
	if err := client.Login(); err == nil {
		t.Fatal("Login() should fail when the login form is returned")
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

func TestAuthCookieSentToReportDespiteNarrowPath(t *testing.T) {
	var reportCookie string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/iclock/Default.aspx" && r.Method == http.MethodGet:
			w.Write([]byte(`<input type="hidden" name="__VIEWSTATE" value="vs"/><input type="hidden" name="__VIEWSTATEGENERATOR" value="gen"/>`))
		case r.URL.Path == "/iclock/Default.aspx":
			http.SetCookie(w, &http.Cookie{Name: "ASP.NET_SessionId", Value: "sess", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "AuthToken", Value: "tok", Path: "/iclock/Main.aspx"})
			w.Write([]byte(`<html>home</html>`))
		case r.URL.Path == "/iclock/Reports/DailyAttendanceReport.aspx":
			reportCookie = r.Header.Get("Cookie")
			w.Write([]byte(`<input type="hidden" name="__VIEWSTATE" value="rvs"/><input type="hidden" name="__VIEWSTATEGENERATOR" value="rgen"/>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewESSLClient(&Config{ESSLURL: server.URL, ESSLUsername: "essl", ESSLPasswordEncrypted: "pw", ESSLEncryptionKey: "key"})
	if err := client.Login(); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if _, _, err := client.FetchViewState(); err != nil {
		t.Fatalf("FetchViewState() error = %v", err)
	}
	if !strings.Contains(reportCookie, "AuthToken=tok") {
		t.Fatalf("report cookie header = %q, want AuthToken", reportCookie)
	}
}

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

func TestESSLClient_FetchReport_LoadsViewerAfterShell(t *testing.T) {
	var sawAsync bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.FormValue("__EVENTTARGET") == "ReportViewer1$ctl09$Reserved_AsyncLoadTarget" {
			sawAsync = true
			if r.Header.Get("X-MicrosoftAjax") != "Delta=true" {
				t.Errorf("missing ajax header")
			}
			if r.FormValue("__VIEWSTATE") != "shellVS" {
				t.Errorf("async viewstate = %q", r.FormValue("__VIEWSTATE"))
			}
			w.Write([]byte(`1|#||4|20|updatePanel|ReportViewer1_ctl09_ReportArea|<DIV>Emp Code:</DIV></TD><TD><DIV>9</DIV>`))
			return
		}
		w.Write([]byte(`<html><input type="hidden" name="__VIEWSTATE" id="__VIEWSTATE" value="shellVS" /><input type="hidden" name="__VIEWSTATEGENERATOR" id="__VIEWSTATEGENERATOR" value="GEN" /><div id="ReportViewer1_ctl09_ReportArea"></div></html>`))
	}))
	defer server.Close()

	client := NewESSLClient(&Config{ESSLURL: server.URL})
	client.viewState = "old"
	client.viewStateGenerator = "oldgen"
	html, err := client.FetchReport(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchReport() error = %v", err)
	}
	if !sawAsync {
		t.Fatal("expected a second request to load the report viewer")
	}
	if !strings.Contains(html, "Emp Code") {
		t.Fatalf("html = %q", html)
	}
}

func TestESSLClient_FetchReport_AllPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.FormValue("__EVENTTARGET") {
		case "ReportViewer1$ctl09$Reserved_AsyncLoadTarget":
			w.Write([]byte(`updatePanel|ReportViewer1_ctl09_ReportArea|<TR><TD><DIV>Emp Code:</DIV></TD><TD><DIV>1</DIV></TD></TR>|0|hiddenField|__VIEWSTATE|page1vs|8|hiddenField|__VIEWSTATEGENERATOR|GEN|ToolBarUpdate:{'CurrentPage':1,'TotalPages':2,'IsEstimatePageCount':false}`))
		case "ReportViewer1$ctl05$ctl00$Next$ctl00":
			if r.FormValue("__VIEWSTATE") != "page1vs" {
				t.Errorf("next viewstate = %q", r.FormValue("__VIEWSTATE"))
			}
			w.Write([]byte(`updatePanel|ReportViewer1_ctl09_ReportArea|<TR><TD><DIV>Emp Code:</DIV></TD><TD><DIV>2</DIV></TD></TR>|0|hiddenField|__VIEWSTATE|page2vs|8|hiddenField|__VIEWSTATEGENERATOR|GEN|ToolBarUpdate:{'CurrentPage':2,'TotalPages':2,'IsEstimatePageCount':false}`))
		default:
			w.Write([]byte(`<input type="hidden" name="__VIEWSTATE" value="shellVS" /><input type="hidden" name="__VIEWSTATEGENERATOR" value="GEN" />`))
		}
	}))
	defer server.Close()

	client := NewESSLClient(&Config{ESSLURL: server.URL})
	client.viewState = "shellVS"
	client.viewStateGenerator = "GEN"
	html, err := client.FetchReport(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("FetchReport() error = %v", err)
	}
	if !strings.Contains(html, ">1</DIV>") || !strings.Contains(html, ">2</DIV>") {
		t.Fatalf("expected both pages, got %s", html)
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
