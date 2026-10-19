package fetchfrommachine

import (
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ESSLClient struct {
	cfg                *Config
	httpClient         *http.Client
	viewState          string
	viewStateGenerator string
	reportPageHTML     string
	homeURL            string
}

func NewESSLClient(cfg *Config) *ESSLClient {
	jar, _ := cookiejar.New(nil)
	return &ESSLClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Jar:     jar,
			Transport: &cookiePathTransport{base: http.DefaultTransport},
		},
	}
}

// cookiePathTransport rewrites Set-Cookie Path to "/" so a cookie issued for
// /iclock/Main.aspx is still sent to /iclock/Reports/...
type cookiePathTransport struct {
	base http.RoundTripper
}

func (t *cookiePathTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}
	rawCookies := resp.Header.Values("Set-Cookie")
	if len(rawCookies) == 0 {
		return resp, nil
	}
	resp.Header.Del("Set-Cookie")
	for _, raw := range rawCookies {
		log.Printf("Set-Cookie %s", cookieSummary(raw))
		resp.Header.Add("Set-Cookie", forceCookiePath(raw, "/"))
	}
	return resp, nil
}

func cookieSummary(raw string) string {
	parts := strings.Split(raw, ";")
	name := strings.SplitN(strings.TrimSpace(parts[0]), "=", 2)[0]
	path := "(default)"
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(strings.ToLower(p), "path=") {
			path = p[len("path="):]
		}
	}
	return name + " path=" + path
}

func forceCookiePath(raw, path string) string {
	parts := strings.Split(raw, ";")
	found := false
	for i, p := range parts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(p)), "path=") {
			parts[i] = " Path=" + path
			found = true
		}
	}
	if !found {
		parts = append(parts, " Path="+path)
	}
	return strings.Join(parts, ";")
}

func (c *ESSLClient) Login() error {
	loginURL := c.cfg.ESSLURL + "/iclock/Default.aspx"

	pageHTML, err := c.get(loginURL)
	if err != nil {
		return fmt.Errorf("open login page: %w", err)
	}

	viewState, viewStateGen, err := extractViewState(pageHTML)
	if err != nil {
		return fmt.Errorf("login page: %w", err)
	}

	formData := url.Values{}
	formData.Set("__EVENTTARGET", "")
	formData.Set("__EVENTARGUMENT", "")
	formData.Set("__VIEWSTATE", viewState)
	formData.Set("__VIEWSTATEGENERATOR", viewStateGen)
	formData.Set("StaffloginDialog$txt_LoginName", c.cfg.ESSLUsername)
	formData.Set("StaffloginDialog$Txt_Password", normalizeBase64(c.cfg.ESSLPasswordEncrypted))
	formData.Set("StaffloginDialog$txtKey", c.cfg.ESSLEncryptionKey)
	formData.Set("StaffloginDialog$Btn_Ok", "Login")

	req, err := http.NewRequest(http.MethodPost, loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.cfg.ESSLURL)
	req.Header.Set("Referer", loginURL)
	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request to %s: %w", loginURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("login HTTP %d final URL %s, cookies: %s", resp.StatusCode, resp.Request.URL, cookieNames(c.httpClient.Jar, resp.Request.URL))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("login failed: HTTP %d from %s: %s", resp.StatusCode, resp.Request.URL, snippet(body))
	}

	page := string(body)
	if strings.Contains(page, "StaffloginDialog$txt_LoginName") || strings.Contains(page, "StaffloginDialog_txt_LoginName") {
		return fmt.Errorf("login rejected: still on the login page (%s). page says: %s", resp.Request.URL, snippet(body))
	}
	if strings.Contains(resp.Request.URL.Path, "LogOut.aspx") {
		return fmt.Errorf("login ended at logout page: %s", resp.Request.URL)
	}
	if !c.hasSessionCookie() {
		return fmt.Errorf("login failed: no session cookie (HTTP %d). page says: %s", resp.StatusCode, snippet(body))
	}

	c.homeURL = resp.Request.URL.String()
	c.loadFrames(resp.Request.URL, page, 0)

	return nil
}

var frameSrcRegex = regexp.MustCompile(`(?i)<(?:frame|iframe)\b[^>]*\bsrc\s*=\s*"([^"]+)"`)

func (c *ESSLClient) loadFrames(pageURL *url.URL, pageHTML string, depth int) {
	if pageURL == nil || depth > 2 {
		return
	}
	seen := map[string]bool{}
	for _, m := range frameSrcRegex.FindAllStringSubmatch(pageHTML, -1) {
		ref, err := url.Parse(html.UnescapeString(m[1]))
		if err != nil || ref.Scheme == "javascript" {
			continue
		}
		abs := pageURL.ResolveReference(ref)
		if abs.Host != "" && pageURL.Host != "" && !strings.EqualFold(abs.Host, pageURL.Host) {
			continue
		}
		if strings.Contains(strings.ToLower(abs.Path), "logout") || seen[abs.String()] {
			continue
		}
		seen[abs.String()] = true
		log.Printf("loading frame %s", abs)
		body, err := c.get(abs.String())
		if err != nil {
			log.Printf("frame %s failed: %v", abs, err)
			continue
		}
		c.loadFrames(abs, body, depth+1)
	}
}

func (c *ESSLClient) hasSessionCookie() bool {
	u, err := url.Parse(c.cfg.ESSLURL + "/iclock/Default.aspx")
	if err != nil || c.httpClient.Jar == nil {
		return false
	}
	for _, ck := range c.httpClient.Jar.Cookies(u) {
		if ck.Name == "ASP.NET_SessionId" && ck.Value != "" {
			return true
		}
	}
	return false
}

// normalizeBase64 accepts the password as copied from a curl (URL-encoded, e.g. "%3D%3D")
// or with its "=" padding stripped, and returns valid padded Base64.
func normalizeBase64(s string) string {
	s = strings.TrimSpace(s)
	if decoded, err := url.QueryUnescape(s); err == nil {
		s = decoded
	}
	if rem := len(s) % 4; rem != 0 {
		s += strings.Repeat("=", 4-rem)
	}
	return s
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	s = strings.Join(strings.Fields(s), " ")
	const max = 300
	if len(s) > max {
		return s[:max] + "..."
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}

const browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

var (
	viewStateRegex    = regexp.MustCompile(`name="__VIEWSTATE"[^>]*value="([^"]*)"`)
	viewStateGenRegex = regexp.MustCompile(`name="__VIEWSTATEGENERATOR"[^>]*value="([^"]*)"`)
)

var (
	deltaViewStateRegex    = regexp.MustCompile(`hiddenField\|__VIEWSTATE\|([^|]*)`)
	deltaViewStateGenRegex = regexp.MustCompile(`hiddenField\|__VIEWSTATEGENERATOR\|([^|]*)`)
	toolbarUpdateRegex = regexp.MustCompile(`(?i)CurrentPage['"]?\s*[:=]\s*['"]?(\d+).*?TotalPages['"]?\s*[:=]\s*['"]?(\d+)`)
)

func extractViewState(pageHTML string) (string, string, error) {
	vsMatch := viewStateRegex.FindStringSubmatch(pageHTML)
	if len(vsMatch) < 2 {
		vsMatch = deltaViewStateRegex.FindStringSubmatch(pageHTML)
	}
	if len(vsMatch) < 2 {
		return "", "", fmt.Errorf("__VIEWSTATE not found. page says: %s", snippet([]byte(pageHTML)))
	}
	vsgMatch := viewStateGenRegex.FindStringSubmatch(pageHTML)
	if len(vsgMatch) < 2 {
		vsgMatch = deltaViewStateGenRegex.FindStringSubmatch(pageHTML)
	}
	if len(vsgMatch) < 2 {
		return "", "", fmt.Errorf("__VIEWSTATEGENERATOR not found. page says: %s", snippet([]byte(pageHTML)))
	}
	return vsMatch[1], vsgMatch[1], nil
}

func pageCounts(pageHTML string) (current, total int) {
	normalized := strings.NewReplacer("&#39;", "'", "&quot;", `"`).Replace(pageHTML)
	m := toolbarUpdateRegex.FindStringSubmatch(normalized)
	if len(m) < 3 {
		return 0, 0
	}
	current, _ = strconv.Atoi(m[1])
	total, _ = strconv.Atoi(m[2])
	return current, total
}

func pageFingerprint(pageHTML string) string {
	var b strings.Builder
	for _, tr := range trRegex.FindAllString(pageHTML, -1) {
		cells := rowCells(tr)
		if len(cells) == 0 {
			continue
		}
		if containsCell(cells, "Emp Code:") || dateRegex.MatchString(cells[0]) {
			b.WriteString(strings.Join(cells, "|"))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func cookieNames(jar http.CookieJar, u *url.URL) string {
	if jar == nil || u == nil {
		return "(none)"
	}
	cookies := jar.Cookies(u)
	if len(cookies) == 0 {
		return "(none)"
	}
	names := make([]string, len(cookies))
	for i, ck := range cookies {
		names[i] = ck.Name
	}
	return strings.Join(names, ", ")
}

func (c *ESSLClient) get(rawURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, resp.Request.URL, snippet(body))
	}
	return string(body), nil
}

func (c *ESSLClient) FetchViewState() (string, string, error) {
	reportURL := c.cfg.ESSLURL + "/iclock/Reports/DailyAttendanceReport.aspx"

	req, err := http.NewRequest(http.MethodGet, reportURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("create viewstate request: %w", err)
	}
	req.Header.Set("User-Agent", browserUserAgent)
	if c.homeURL != "" {
		req.Header.Set("Referer", c.homeURL)
	}
	if u, err := url.Parse(reportURL); err == nil {
		log.Printf("cookies for report page: %s", cookieNames(c.httpClient.Jar, u))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("viewstate request to %s: %w", reportURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read viewstate response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("viewstate page HTTP %d: %s", resp.StatusCode, snippet(body))
	}

	page := string(body)
	log.Printf("report page HTTP %d final URL %s (%d bytes)", resp.StatusCode, resp.Request.URL, len(body))
	if strings.Contains(resp.Request.URL.Path, "LogOut.aspx") || strings.Contains(page, "StaffloginDialog") {
		return "", "", fmt.Errorf("not logged in: opening the report redirected to %s. page says: %s", resp.Request.URL, snippet(body))
	}

	vs, vsg, err := extractViewState(page)
	if err != nil {
		return "", "", fmt.Errorf("report page (HTTP %d, %s): %w", resp.StatusCode, resp.Request.URL, err)
	}

	c.viewState = vs
	c.viewStateGenerator = vsg
	c.reportPageHTML = page

	return c.viewState, c.viewStateGenerator, nil
}

var inputTagRegex = regexp.MustCompile(`(?i)<input\b[^>]*>`)
var inputAttrRegex = regexp.MustCompile(`(?i)\b(type|name|value)\s*=\s*"([^"]*)"`)

func hiddenFields(pageHTML string) url.Values {
	vals := url.Values{}
	for _, tag := range inputTagRegex.FindAllString(pageHTML, -1) {
		attrs := map[string]string{}
		for _, m := range inputAttrRegex.FindAllStringSubmatch(tag, -1) {
			attrs[strings.ToLower(m[1])] = html.UnescapeString(m[2])
		}
		name := attrs["name"]
		if name == "" {
			continue
		}
		if t := attrs["type"]; t != "" && !strings.EqualFold(t, "hidden") {
			continue
		}
		vals.Set(name, attrs["value"])
	}
	return vals
}

func (c *ESSLClient) FetchReport(fromDate, toDate time.Time) (string, error) {
	reportURL := c.cfg.ESSLURL + "/iclock/Reports/DailyAttendanceReport.aspx"

	formData := hiddenFields(c.reportPageHTML)
	formData.Set("__EVENTTARGET", "ReportProtoType$btn_GenerateReport")
	formData.Set("__EVENTARGUMENT", "")
	formData.Set("__VIEWSTATE", c.viewState)
	formData.Set("__VIEWSTATEGENERATOR", c.viewStateGenerator)

	formData.Set("ReportProtoType$Drp_FromDateDD", fmt.Sprintf("%d", fromDate.Day()))
	formData.Set("ReportProtoType$Drp_FromDateMM", fmt.Sprintf("%d", int(fromDate.Month())))
	formData.Set("ReportProtoType$Drp_FromDateYYYY", fmt.Sprintf("%d", fromDate.Year()))
	formData.Set("ReportProtoType$Drp_ToDateDD", fmt.Sprintf("%d", toDate.Day()))
	formData.Set("ReportProtoType$Drp_ToDateMM", fmt.Sprintf("%d", int(toDate.Month())))
	formData.Set("ReportProtoType$Drp_ToDateYYYY", fmt.Sprintf("%d", toDate.Year()))

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
	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("report request to %s: %w", reportURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read report response: %w", err)
	}
	log.Printf("report HTTP %d from %s (%d bytes)", resp.StatusCode, resp.Request.URL, len(body))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("report HTTP %d: %s", resp.StatusCode, snippet(body))
	}

	page := string(body)
	if !strings.Contains(page, "Emp Code") && !strings.Contains(page, "updatePanel|ReportViewer1") {
		log.Println("report shell returned without attendance rows, loading report viewer content...")
		loaded, err := c.loadReportContent(page)
		if err != nil {
			return "", err
		}
		page = loaded
	}

	return c.collectReportPages(page)
}

func (c *ESSLClient) collectReportPages(first string) (string, error) {
	pages := []string{first}
	seen := map[string]bool{}
	if fp := pageFingerprint(first); fp != "" {
		seen[fp] = true
	}
	current, total := pageCounts(first)
	if total > 0 {
		log.Printf("report page %d of %d", current, total)
	} else {
		log.Printf("report page 1, page count was not in the response; stepping with Next until the page repeats")
	}
	latest := first
	for len(pages) < 200 {
		if total > 0 && current >= total {
			break
		}
		log.Printf("loading report page %d...", len(pages)+1)
		next, err := c.postReportEvent(latest, "ReportViewer1$ctl05$ctl00$Next$ctl00")
		if err != nil {
			log.Printf("stopped after %d report page(s): %v", len(pages), err)
			break
		}
		if !strings.Contains(next, "Emp Code") {
			loaded, loadErr := c.loadReportContent(next)
			if loadErr != nil || !strings.Contains(loaded, "Emp Code") {
				log.Printf("no further report page after %d", len(pages))
				break
			}
			next = loaded
		}
		fp := pageFingerprint(next)
		if fp == "" || seen[fp] {
			log.Printf("report page repeated after %d page(s), stopping", len(pages))
			break
		}
		seen[fp] = true
		nextCurrent, nextTotal := pageCounts(next)
		if nextTotal > total {
			total = nextTotal
		}
		if nextCurrent > 0 {
			current = nextCurrent
		}
		if total > 0 {
			log.Printf("report page %d of %d", current, total)
		} else {
			log.Printf("report page %d", len(pages)+1)
		}
		pages = append(pages, next)
		latest = next
		if total > 0 && current >= total {
			break
		}
	}
	log.Printf("collected %d report page(s)", len(pages))
	return strings.Join(pages, "\n"), nil
}

func (c *ESSLClient) postReportEvent(pageHTML, eventTarget string) (string, error) {
	viewState, viewStateGen, err := extractViewState(pageHTML)
	if err != nil {
		return "", err
	}
	reportURL := c.cfg.ESSLURL + "/iclock/Reports/DailyAttendanceReport.aspx"
	formData := hiddenFields(c.reportPageHTML)
	formData.Set("ScriptManager1", "ReportViewer1$ctl09$ReportArea|"+eventTarget)
	formData.Set("__EVENTTARGET", eventTarget)
	formData.Set("__EVENTARGUMENT", "")
	formData.Set("__VIEWSTATE", viewState)
	formData.Set("__VIEWSTATEGENERATOR", viewStateGen)
	formData.Set("__ASYNCPOST", "true")

	req, err := http.NewRequest(http.MethodPost, reportURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("create report event request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.cfg.ESSLURL)
	req.Header.Set("Referer", reportURL)
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("X-MicrosoftAjax", "Delta=true")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("report event %s: %w", eventTarget, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read report event response: %w", err)
	}
	log.Printf("report event %s HTTP %d (%d bytes)", eventTarget, resp.StatusCode, len(body))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("report event HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	return string(body), nil
}

func (c *ESSLClient) loadReportContent(pageHTML string) (string, error) {
	body, err := c.postReportEvent(pageHTML, "ReportViewer1$ctl09$Reserved_AsyncLoadTarget")
	if err != nil {
		return "", fmt.Errorf("report content: %w", err)
	}
	return body, nil
}
