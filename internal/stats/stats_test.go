package stats_test

import (
	"database/sql"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/singtail/singtail/internal/contract"
	"github.com/singtail/singtail/internal/stats"
)

func testOrgsAndNodes() ([]contract.Organization, []contract.ExitNode) {
	orgs := []contract.Organization{
		{ID: "org-work", Name: "Acme Corp", Account: "alice@acme.io"},
		{ID: "org-home", Name: "Home Lab", Account: "alice@gmail.com"},
	}
	nodes := []contract.ExitNode{
		{ID: "node-us", OrgID: "org-work", OrgName: "Acme Corp", Hostname: "us-west", Label: "美国节点"},
		{ID: "node-hk", OrgID: "org-work", OrgName: "Acme Corp", Hostname: "hk-gw", Label: "香港网关"},
		{ID: "node-jp", OrgID: "org-home", OrgName: "Home Lab", Hostname: "jp-tokyo", Label: "东京节点"},
	}
	return orgs, nodes
}

func TestOpenAndLiveHistory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "traffic.db")
	st, err := stats.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	down, up := st.LiveHistory()
	if len(down) != 36 || len(up) != 36 {
		t.Fatalf("expected 36 live points, got down=%d up=%d", len(down), len(up))
	}
	for i := range down {
		if down[i] <= 0 || up[i] <= 0 {
			t.Fatalf("expected non-zero default wave at index %d, got down=%v up=%v", i, down[i], up[i])
		}
	}

	base := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 40; i++ {
		downBytes := int64((i + 1) * 1024 * 1024 / 2) // 0.5 MB increments
		upBytes := int64(256 * 1024)                  // 0.25 MB -> rounded 0.3
		if err := st.RecordTick(base.Add(time.Duration(i)*time.Second), "org-work", "node-us", downBytes, upBytes); err != nil {
			t.Fatalf("RecordTick %d failed: %v", i, err)
		}
	}

	down, up = st.LiveHistory()
	if len(down) != 36 || len(up) != 36 {
		t.Fatalf("expected 36 points after ticks, got down=%d up=%d", len(down), len(up))
	}
	if down[35] != 20.0 {
		t.Fatalf("expected latest down rate 20.0 MB/s, got %v", down[35])
	}
	if up[35] != 0.3 {
		t.Fatalf("expected latest up rate 0.3 MB/s, got %v", up[35])
	}
}

func TestRetention60HoursPreservesRollups(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "retention.db")
	st, err := stats.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	start := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	var expectedDown, expectedUp int64

	// Inject 61 hourly samples spanning [0h .. 60h] across two orgs/nodes.
	for h := 0; h <= 60; h++ {
		ts := start.Add(time.Duration(h) * time.Hour)
		d1 := int64(100+h) * 1024 * 1024
		u1 := int64(20+h) * 1024 * 1024
		d2 := int64(50+h) * 1024 * 1024
		u2 := int64(10+h) * 1024 * 1024

		if err := st.RecordTick(ts, "org-work", "node-us", d1, u1); err != nil {
			t.Fatalf("RecordTick work h=%d failed: %v", h, err)
		}
		if err := st.RecordTick(ts, "org-home", "node-jp", d2, u2); err != nil {
			t.Fatalf("RecordTick home h=%d failed: %v", h, err)
		}
		expectedDown += d1 + d2
		expectedUp += u1 + u2
	}
	_ = st.Close()

	// Inspect SQLite tables directly to verify retention cutoff and lossless rollups.
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer func() { _ = rawDB.Close() }()

	latest := start.Add(60 * time.Hour)
	cutoffMinute := latest.Add(-48*time.Hour).Unix() / 60

	var expiredCount, retainedCount int
	if err := rawDB.QueryRow(`SELECT COUNT(*) FROM traffic_samples WHERE bucket_minute < ?;`, cutoffMinute).Scan(&expiredCount); err != nil {
		t.Fatalf("query expired samples: %v", err)
	}
	if err := rawDB.QueryRow(`SELECT COUNT(*) FROM traffic_samples WHERE bucket_minute >= ?;`, cutoffMinute).Scan(&retainedCount); err != nil {
		t.Fatalf("query retained samples: %v", err)
	}
	if expiredCount != 0 {
		t.Fatalf("expected 0 samples older than 48h, got %d", expiredCount)
	}
	if retainedCount == 0 {
		t.Fatalf("expected retained samples within 48h, got 0")
	}

	var dailyDown, dailyUp int64
	if err := rawDB.QueryRow(`SELECT COALESCE(SUM(down_bytes), 0), COALESCE(SUM(up_bytes), 0) FROM traffic_daily;`).Scan(&dailyDown, &dailyUp); err != nil {
		t.Fatalf("sum traffic_daily: %v", err)
	}
	if dailyDown != expectedDown || dailyUp != expectedUp {
		t.Fatalf("daily rollup mismatch: got (%d, %d), want (%d, %d)", dailyDown, dailyUp, expectedDown, expectedUp)
	}

	var monthlyDown, monthlyUp int64
	if err := rawDB.QueryRow(`SELECT COALESCE(SUM(down_bytes), 0), COALESCE(SUM(up_bytes), 0) FROM traffic_monthly;`).Scan(&monthlyDown, &monthlyUp); err != nil {
		t.Fatalf("sum traffic_monthly: %v", err)
	}
	if monthlyDown != expectedDown || monthlyUp != expectedUp {
		t.Fatalf("monthly rollup mismatch: got (%d, %d), want (%d, %d)", monthlyDown, monthlyUp, expectedDown, expectedUp)
	}
}

func TestQueryPeriodsAndBreakdowns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "query.db")
	st, err := stats.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	orgs, nodes := testOrgsAndNodes()
	now := time.Date(2026, 11, 27, 15, 30, 0, 0, time.UTC)
	const gb = int64(1024 * 1024 * 1024)

	// Record historical ticks inside the 12-month, 30-day, 7-day, and today windows.
	ticks := []struct {
		ts     time.Time
		orgID  string
		nodeID string
		down   int64
		up     int64
	}{
		// 5 months ago
		{now.AddDate(0, -5, -1), "org-work", "node-us", 20 * gb, 4 * gb},
		{now.AddDate(0, -5, -1), "org-home", "node-jp", 10 * gb, 2 * gb},
		// 20 days ago (inside month30 and current month)
		{now.AddDate(0, 0, -20), "org-work", "node-hk", 8 * gb, 2 * gb},
		// 3 days ago (inside week, month30, monthly)
		{now.AddDate(0, 0, -3), "org-work", "node-us", 6 * gb, 1 * gb},
		{now.AddDate(0, 0, -3), "org-home", "node-jp", 3 * gb, 1 * gb},
		// Today at 09:00 and 14:00 (inside live, day, week, month30, monthly)
		{time.Date(2026, 11, 27, 9, 10, 0, 0, time.UTC), "org-work", "node-us", 4 * gb, 1 * gb},
		{time.Date(2026, 11, 27, 14, 20, 0, 0, time.UTC), "org-work", "node-hk", 2 * gb, 1 * gb},
		{time.Date(2026, 11, 27, 14, 30, 0, 0, time.UTC), "org-home", "node-jp", 1 * gb, 1 * gb},
	}

	for _, tk := range ticks {
		if err := st.RecordTick(tk.ts, tk.orgID, tk.nodeID, tk.down, tk.up); err != nil {
			t.Fatalf("RecordTick failed: %v", err)
		}
	}

	// Also inject a tick 13 months ago that must be excluded from all reports.
	if err := st.RecordTick(now.AddDate(0, -13, 0), "org-work", "node-us", 99*gb, 9*gb); err != nil {
		t.Fatalf("RecordTick old failed: %v", err)
	}

	expectedTotalsGB := map[contract.TrafficPeriodKey]float64{
		contract.PeriodLive:    10, // (4+1)+(2+1)+(1+1) = 10 GB
		contract.PeriodDay:     10, // same day total
		contract.PeriodWeek:    21, // 10 + (6+1)+(3+1) = 21 GB
		contract.PeriodMonth30: 31, // 21 + (8+2) = 31 GB
		contract.PeriodMonthly: 67, // 31 + (20+4)+(10+2) = 67 GB
	}

	for _, period := range []contract.TrafficPeriodKey{
		contract.PeriodLive,
		contract.PeriodDay,
		contract.PeriodWeek,
		contract.PeriodMonth30,
		contract.PeriodMonthly,
	} {
		rep, err := st.QueryReport(now, period, orgs, nodes)
		if err != nil {
			t.Fatalf("QueryReport(%s) failed: %v", period, err)
		}

		wantGB := expectedTotalsGB[period]
		if math.Abs(rep.TotalGB-wantGB) > 1e-9 {
			t.Fatalf("period %s TotalGB = %v, want %v", period, rep.TotalGB, wantGB)
		}
		if math.Abs((rep.TotalDownGB+rep.TotalUpGB)-rep.TotalGB) > 1e-9 {
			t.Fatalf("period %s down+up != total", period)
		}

		switch period {
		case contract.PeriodDay:
			if len(rep.Labels) != 12 {
				t.Fatalf("day labels len = %d, want 12", len(rep.Labels))
			}
		case contract.PeriodWeek:
			if len(rep.Labels) != 7 || rep.Labels[6] != "今日" {
				t.Fatalf("unexpected week labels: %v", rep.Labels)
			}
		case contract.PeriodMonth30:
			if len(rep.Labels) < 8 || len(rep.Labels) > 10 {
				t.Fatalf("month30 labels len = %d, want 8..10", len(rep.Labels))
			}
		case contract.PeriodMonthly:
			if len(rep.Labels) != 12 {
				t.Fatalf("monthly labels len = %d, want 12", len(rep.Labels))
			}
		}

		if period != contract.PeriodLive {
			var seriesDown, seriesUp float64
			for i := range rep.DownSeries {
				seriesDown += rep.DownSeries[i]
				seriesUp += rep.UpSeries[i]
			}
			if math.Abs(seriesDown-rep.TotalDownGB) > 1e-9 || math.Abs(seriesUp-rep.TotalUpGB) > 1e-9 {
				t.Fatalf("period %s series sum (%v, %v) != totals (%v, %v)", period, seriesDown, seriesUp, rep.TotalDownGB, rep.TotalUpGB)
			}
		}

		assertBreakdownValid(t, string(period), "ByNode", rep.ByNode, rep.TotalGB)
		assertBreakdownValid(t, string(period), "ByOrg", rep.ByOrg, rep.TotalGB)
	}

	dayRep, _ := st.QueryReport(now, contract.PeriodDay, orgs, nodes)
	if dayRep.ByNode[0].SubLabel != "Acme Corp" {
		t.Fatalf("expected ByNode[0].SubLabel 'Acme Corp', got %q", dayRep.ByNode[0].SubLabel)
	}
}

func assertBreakdownValid(t *testing.T, period, kind string, items []contract.TrafficBreakdownItem, totalGB float64) {
	t.Helper()
	if len(items) == 0 {
		t.Fatalf("%s %s is empty", period, kind)
	}
	var sumGB, sumPct float64
	for i, it := range items {
		sumGB += it.TotalGB
		sumPct += it.Percent
		if i > 0 && items[i-1].TotalGB < it.TotalGB {
			t.Fatalf("%s %s not sorted descending at index %d: %v < %v", period, kind, i, items[i-1].TotalGB, it.TotalGB)
		}
	}
	if math.Abs(sumGB-totalGB) > 1e-9 {
		t.Fatalf("%s %s sumGB=%v != totalGB=%v", period, kind, sumGB, totalGB)
	}
	if math.Abs(sumPct-100.0) > 0.3 {
		t.Fatalf("%s %s sumPercent=%v not ~100", period, kind, sumPct)
	}
}

func TestSeedDemoIfEmpty(t *testing.T) {
	st, err := stats.Open(filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	orgs, nodes := testOrgsAndNodes()
	now := time.Date(2026, 11, 27, 16, 0, 0, 0, time.UTC)

	if err := st.SeedDemoIfEmpty(now, orgs, nodes); err != nil {
		t.Fatalf("SeedDemoIfEmpty failed: %v", err)
	}

	day1, err := st.QueryReport(now, contract.PeriodDay, orgs, nodes)
	if err != nil {
		t.Fatalf("QueryReport day failed: %v", err)
	}
	week1, err := st.QueryReport(now, contract.PeriodWeek, orgs, nodes)
	if err != nil {
		t.Fatalf("QueryReport week failed: %v", err)
	}
	m30, err := st.QueryReport(now, contract.PeriodMonth30, orgs, nodes)
	if err != nil {
		t.Fatalf("QueryReport month30 failed: %v", err)
	}
	monthly1, err := st.QueryReport(now, contract.PeriodMonthly, orgs, nodes)
	if err != nil {
		t.Fatalf("QueryReport monthly failed: %v", err)
	}

	if day1.TotalGB <= 0 || week1.TotalGB <= day1.TotalGB || m30.TotalGB <= week1.TotalGB || monthly1.TotalGB <= m30.TotalGB {
		t.Fatalf("unexpected seeded progression: day=%v week=%v month30=%v monthly=%v",
			day1.TotalGB, week1.TotalGB, m30.TotalGB, monthly1.TotalGB)
	}

	// Second call must be idempotent.
	if err := st.SeedDemoIfEmpty(now, orgs, nodes); err != nil {
		t.Fatalf("second SeedDemoIfEmpty failed: %v", err)
	}
	monthly2, err := st.QueryReport(now, contract.PeriodMonthly, orgs, nodes)
	if err != nil {
		t.Fatalf("QueryReport monthly after second seed failed: %v", err)
	}
	if math.Abs(monthly1.TotalGB-monthly2.TotalGB) > 1e-9 {
		t.Fatalf("SeedDemoIfEmpty was not idempotent: %v vs %v", monthly1.TotalGB, monthly2.TotalGB)
	}
}

func TestConcurrentRecordAndQuery(t *testing.T) {
	st, err := stats.Open(filepath.Join(t.TempDir(), "race.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	orgs, nodes := testOrgsAndNodes()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 15; i++ {
				_ = st.RecordTick(now.Add(time.Duration(i)*time.Second), orgs[id%2].ID, nodes[id%3].ID, 1024*1024, 256*1024)
				_, _ = st.LiveHistory()
				_, _ = st.QueryReport(now, contract.PeriodDay, orgs, nodes)
			}
		}(g)
	}
	wg.Wait()
}
