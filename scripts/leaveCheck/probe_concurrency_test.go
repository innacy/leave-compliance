package handler

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zinscky/log"
)

// TestProbeGreytHRConcurrency is a manual test you run against the real
// greytHR API to find the safe concurrency threshold. It is skipped by
// default; run it with:
//
//	GREYTHR_PROBE=1 GREYTHR_TOKEN=<token> GREYTHR_DOMAIN=<domain> \
//	GREYTHR_BASE=<base_url> GREYTHR_EMP_ID=<id> GREYTHR_MAX_CONCURRENT=20 \
//	  go test ./scripts/leaveCheck/ -run TestProbeGreytHRConcurrency -v -count=1
//
// It fires N concurrent requests for the same employee and reports how
// many succeeded vs timed out vs got 429. Increase GREYTHR_MAX_CONCURRENT
// until you start seeing failures.
func TestProbeGreytHRConcurrency(t *testing.T) {
	if os.Getenv("GREYTHR_PROBE") == "" {
		t.Skip("set GREYTHR_PROBE=1 to run this against real greytHR")
	}

	token := os.Getenv("GREYTHR_TOKEN")
	domain := os.Getenv("GREYTHR_DOMAIN")
	base := os.Getenv("GREYTHR_BASE")
	empIDStr := os.Getenv("GREYTHR_EMP_ID")
	maxStr := os.Getenv("GREYTHR_MAX_CONCURRENT")

	if token == "" || domain == "" || base == "" || empIDStr == "" {
		t.Fatal("missing required env vars: GREYTHR_TOKEN, GREYTHR_DOMAIN, GREYTHR_BASE, GREYTHR_EMP_ID")
	}

	empID, _ := strconv.Atoi(empIDStr)
	maxConcurrent := 10
	if maxStr != "" {
		maxConcurrent, _ = strconv.Atoi(maxStr)
	}

	lg := log.New(0, "probe")
	config := map[string]string{"baseURL": base, "x_greythr_domain": domain}
	end := time.Now().Format("2006-01-02")
	start := time.Now().AddDate(0, -1, 0).Format("2006-01-02")

	// Restore production backoff for this test.
	origBackoff := leaveFetchBaseBackoff
	leaveFetchBaseBackoff = 2 * time.Second
	defer func() { leaveFetchBaseBackoff = origBackoff }()

	for concurrency := 1; concurrency <= maxConcurrent; concurrency++ {
		var ok, fail atomic.Int32
		var wg sync.WaitGroup
		t0 := time.Now()

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := fetchEmployeeLeaveTransactions(token, config, empID, start, end, lg)
				if err != nil {
					fail.Add(1)
				} else {
					ok.Add(1)
				}
			}()
		}
		wg.Wait()

		dur := time.Since(t0)
		t.Logf("concurrency=%2d  ok=%2d  fail=%2d  time=%s",
			concurrency, ok.Load(), fail.Load(), dur.Round(time.Millisecond))

		if fail.Load() > 0 {
			t.Logf("⚠ failures started at concurrency=%d — safe limit is %d", concurrency, concurrency-1)
			break
		}

		// Brief pause between rounds to avoid rate-limit bleed.
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("Set leaveFetchConcurrency to the highest concurrency with 0 failures.")
}
