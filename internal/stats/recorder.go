package stats

import (
	"database/sql"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/singtail/singtail/internal/contract"
)

var liveInitialized sync.Map

func ensureLiveZeroLocked(s *Store) {
	if s == nil || s.db == nil {
		return
	}
	if _, ok := liveInitialized.Load(s.db); ok {
		return
	}
	for i := range s.liveDown {
		s.liveDown[i] = 0
	}
	for i := range s.liveUp {
		s.liveUp[i] = 0
	}
	markLiveInitialized(s)
}

func markLiveInitialized(s *Store) {
	if s == nil || s.db == nil {
		return
	}
	if _, loaded := liveInitialized.LoadOrStore(s.db, struct{}{}); !loaded {
		runtime.AddCleanup(s, func(db *sql.DB) {
			liveInitialized.Delete(db)
		}, s.db)
	}
}

type orgNodeKey struct {
	orgID  string
	nodeID string
}

type bytePair struct {
	down int64
	up   int64
}

// LiveHistory returns a snapshot of the 36-point download and upload rate history in MB/s.
func (s *Store) LiveHistory() (downMBps []float64, upMBps []float64) {
	if s == nil {
		return make([]float64, liveHistoryLen), make([]float64, liveHistoryLen)
	}
	if s.db != nil {
		if _, ok := liveInitialized.Load(s.db); !ok {
			s.mu.Lock()
			ensureLiveZeroLocked(s)
			s.mu.Unlock()
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	down := make([]float64, len(s.liveDown))
	copy(down, s.liveDown)
	up := make([]float64, len(s.liveUp))
	copy(up, s.liveUp)
	return down, up
}

// RecordTick records one traffic delta into the live ring buffer and atomically updates SQLite rollup tables.
func (s *Store) RecordTick(ts time.Time, orgID, nodeID string, downBytes, upBytes int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("stats: store is closed")
	}
	if downBytes < 0 {
		downBytes = 0
	}
	if upBytes < 0 {
		upBytes = 0
	}

	downMB := round1(float64(downBytes) / float64(bytesPerMB))
	upMB := round1(float64(upBytes) / float64(bytesPerMB))
	s.pushLivePoint(downMB, upMB)

	bucketMinute := ts.Unix() / 60
	cutoffMinute := ts.Add(-48*time.Hour).Unix() / 60
	hourKey := ts.Format("2006-01-02T15")
	dayKey := ts.Format("2006-01-02")
	monthKey := ts.Format("2006-01")

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("stats: begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const upsertSample = `
		INSERT INTO traffic_samples (bucket_minute, org_id, node_id, down_bytes, up_bytes)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(bucket_minute, org_id, node_id) DO UPDATE SET
			down_bytes = down_bytes + excluded.down_bytes,
			up_bytes = up_bytes + excluded.up_bytes;`
	if _, err := tx.Exec(upsertSample, bucketMinute, orgID, nodeID, downBytes, upBytes); err != nil {
		return fmt.Errorf("stats: upsert traffic_samples: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM traffic_samples WHERE bucket_minute < ?;`, cutoffMinute); err != nil {
		return fmt.Errorf("stats: prune traffic_samples: %w", err)
	}

	const upsertHourly = `
		INSERT INTO traffic_hourly (hour_key, org_id, node_id, down_bytes, up_bytes)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(hour_key, org_id, node_id) DO UPDATE SET
			down_bytes = down_bytes + excluded.down_bytes,
			up_bytes = up_bytes + excluded.up_bytes;`
	if _, err := tx.Exec(upsertHourly, hourKey, orgID, nodeID, downBytes, upBytes); err != nil {
		return fmt.Errorf("stats: upsert traffic_hourly: %w", err)
	}

	const upsertDaily = `
		INSERT INTO traffic_daily (day_key, org_id, node_id, down_bytes, up_bytes)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(day_key, org_id, node_id) DO UPDATE SET
			down_bytes = down_bytes + excluded.down_bytes,
			up_bytes = up_bytes + excluded.up_bytes;`
	if _, err := tx.Exec(upsertDaily, dayKey, orgID, nodeID, downBytes, upBytes); err != nil {
		return fmt.Errorf("stats: upsert traffic_daily: %w", err)
	}

	const upsertMonthly = `
		INSERT INTO traffic_monthly (month_key, org_id, node_id, down_bytes, up_bytes)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(month_key, org_id, node_id) DO UPDATE SET
			down_bytes = down_bytes + excluded.down_bytes,
			up_bytes = up_bytes + excluded.up_bytes;`
	if _, err := tx.Exec(upsertMonthly, monthKey, orgID, nodeID, downBytes, upBytes); err != nil {
		return fmt.Errorf("stats: upsert traffic_monthly: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("stats: commit tick tx: %w", err)
	}
	return nil
}

func (s *Store) pushLivePoint(downMB, upMB float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ensureLiveZeroLocked(s)

	if len(s.liveDown) < liveHistoryLen {
		s.liveDown = append(s.liveDown, downMB)
	} else {
		copy(s.liveDown, s.liveDown[1:])
		s.liveDown[liveHistoryLen-1] = downMB
	}

	if len(s.liveUp) < liveHistoryLen {
		s.liveUp = append(s.liveUp, upMB)
	} else {
		copy(s.liveUp, s.liveUp[1:])
		s.liveUp[liveHistoryLen-1] = upMB
	}
}

// SeedDemoIfEmpty populates realistic historical rows for the past 12 months, 30 days, and today when traffic_monthly is empty.
func (s *Store) SeedDemoIfEmpty(now time.Time, orgs []contract.Organization, nodes []contract.ExitNode) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("stats: store is closed")
	}
	markLiveInitialized(s)

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM traffic_monthly;`).Scan(&count); err != nil {
		return fmt.Errorf("stats: check traffic_monthly count: %w", err)
	}
	if count > 0 || len(nodes) == 0 {
		return nil
	}

	fallbackOrgID := ""
	if len(orgs) > 0 {
		fallbackOrgID = orgs[0].ID
	}

	weights := make([]float64, len(nodes))
	var weightSum float64
	for i := range nodes {
		w := 1.0 / float64(i+1)
		weights[i] = w
		weightSum += w
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("stats: begin seed tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmtHourly, err := tx.Prepare(`INSERT OR REPLACE INTO traffic_hourly (hour_key, org_id, node_id, down_bytes, up_bytes) VALUES (?, ?, ?, ?, ?);`)
	if err != nil {
		return fmt.Errorf("stats: prepare hourly seed: %w", err)
	}
	defer func() { _ = stmtHourly.Close() }()

	stmtDaily, err := tx.Prepare(`INSERT OR REPLACE INTO traffic_daily (day_key, org_id, node_id, down_bytes, up_bytes) VALUES (?, ?, ?, ?, ?);`)
	if err != nil {
		return fmt.Errorf("stats: prepare daily seed: %w", err)
	}
	defer func() { _ = stmtDaily.Close() }()

	stmtMonthly, err := tx.Prepare(`INSERT OR REPLACE INTO traffic_monthly (month_key, org_id, node_id, down_bytes, up_bytes) VALUES (?, ?, ?, ?, ?);`)
	if err != nil {
		return fmt.Errorf("stats: prepare monthly seed: %w", err)
	}
	defer func() { _ = stmtMonthly.Close() }()

	resolveOrg := func(n contract.ExitNode) string {
		if n.OrgID != "" {
			return n.OrgID
		}
		return fallbackOrgID
	}

	todayKey := now.Format("2006-01-02")
	todaySum := make(map[orgNodeKey]bytePair)
	monthlySum := make(map[string]map[orgNodeKey]bytePair)
	monthlyDaysCovered := make(map[string]int)

	addMonthly := func(mKey string, k orgNodeKey, down, up int64) {
		bucket, ok := monthlySum[mKey]
		if !ok {
			bucket = make(map[orgNodeKey]bytePair)
			monthlySum[mKey] = bucket
		}
		cur := bucket[k]
		cur.down += down
		cur.up += up
		bucket[k] = cur
	}

	// Seed today's two-hour buckets up to now.Hour() so PeriodDay and PeriodLive share consistent totals.
	diurnal := []float64{0.55, 0.30, 0.20, 0.25, 0.85, 2.20, 1.80, 2.05, 2.45, 1.90, 1.75, 1.35}
	for h := 0; h <= now.Hour(); h += 2 {
		slot := h / 2
		hourKey := fmt.Sprintf("%sT%02d", todayKey, h)
		slotTotalDown := int64(diurnal[slot] * float64(bytesPerGB))
		slotTotalUp := int64(diurnal[slot] * 0.14 * float64(bytesPerGB))

		for i, n := range nodes {
			ratio := weights[i] / weightSum
			down := int64(math.Round(float64(slotTotalDown) * ratio))
			up := int64(math.Round(float64(slotTotalUp) * ratio))
			orgID := resolveOrg(n)
			if _, err := stmtHourly.Exec(hourKey, orgID, n.ID, down, up); err != nil {
				return fmt.Errorf("stats: insert hourly seed: %w", err)
			}
			k := orgNodeKey{orgID: orgID, nodeID: n.ID}
			cur := todaySum[k]
			cur.down += down
			cur.up += up
			todaySum[k] = cur
		}
	}

	thisMonthKey := now.Format("2006-01")
	monthlyDaysCovered[thisMonthKey]++
	for k, v := range todaySum {
		if _, err := stmtDaily.Exec(todayKey, k.orgID, k.nodeID, v.down, v.up); err != nil {
			return fmt.Errorf("stats: insert today daily seed: %w", err)
		}
		addMonthly(thisMonthKey, k, v.down, v.up)
	}

	// Seed the preceding 29 days into traffic_daily and roll them into traffic_monthly.
	for d := 1; d < 30; d++ {
		day := now.AddDate(0, 0, -d)
		dayKey := day.Format("2006-01-02")
		mKey := day.Format("2006-01")
		monthlyDaysCovered[mKey]++

		baseGB := 11.5 + 3.2*math.Sin(float64(d)*0.65) + 1.4*math.Cos(float64(d)*1.1)
		dayTotalDown := int64(baseGB * float64(bytesPerGB))
		dayTotalUp := int64(baseGB * 0.135 * float64(bytesPerGB))

		for i, n := range nodes {
			ratio := weights[i] / weightSum
			down := int64(math.Round(float64(dayTotalDown) * ratio))
			up := int64(math.Round(float64(dayTotalUp) * ratio))
			orgID := resolveOrg(n)
			if _, err := stmtDaily.Exec(dayKey, orgID, n.ID, down, up); err != nil {
				return fmt.Errorf("stats: insert daily seed: %w", err)
			}
			addMonthly(mKey, orgNodeKey{orgID: orgID, nodeID: n.ID}, down, up)
		}
	}

	// Ensure all 12 natural months ending at now have full-month demo totals.
	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	for m := 0; m < 12; m++ {
		mTime := firstOfMonth.AddDate(0, -m, 0)
		mKey := mTime.Format("2006-01")
		if m > 0 {
			covered := monthlyDaysCovered[mKey]
			remainingDays := 30 - covered
			if remainingDays > 0 {
				monthFactor := 10.5 + 2.8*math.Sin(float64(m)*0.7)
				extraDown := int64(float64(remainingDays) * monthFactor * float64(bytesPerGB))
				extraUp := int64(float64(remainingDays) * monthFactor * 0.135 * float64(bytesPerGB))
				for i, n := range nodes {
					ratio := weights[i] / weightSum
					down := int64(math.Round(float64(extraDown) * ratio))
					up := int64(math.Round(float64(extraUp) * ratio))
					addMonthly(mKey, orgNodeKey{orgID: resolveOrg(n), nodeID: n.ID}, down, up)
				}
			}
		}

		for k, v := range monthlySum[mKey] {
			if _, err := stmtMonthly.Exec(mKey, k.orgID, k.nodeID, v.down, v.up); err != nil {
				return fmt.Errorf("stats: insert monthly seed: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("stats: commit seed tx: %w", err)
	}
	return nil
}
