package stats

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/singtail/singtail/internal/contract"
)

// QueryReport aggregates time-series and per-node/per-org traffic breakdowns for the requested granularity.
func (s *Store) QueryReport(now time.Time, period contract.TrafficPeriodKey, orgs []contract.Organization, nodes []contract.ExitNode) (contract.TrafficReport, error) {
	if s == nil || s.db == nil {
		return contract.TrafficReport{}, fmt.Errorf("stats: store is closed")
	}

	liveDown, liveUp := s.LiveHistory()
	report := contract.TrafficReport{
		Period:          period,
		Labels:          make([]string, 0),
		DownSeries:      make([]float64, 0),
		UpSeries:        make([]float64, 0),
		LiveDownHistory: liveDown,
		LiveUpHistory:   liveUp,
		ByNode:          make([]contract.TrafficBreakdownItem, 0),
		ByOrg:           make([]contract.TrafficBreakdownItem, 0),
	}

	var (
		totalDown int64
		totalUp   int64
		nodeAgg   map[string]*bytePair
		orgAgg    map[string]*bytePair
		err       error
	)

	switch period {
	case contract.PeriodLive:
		report.Label = "实时"
		totalDown, totalUp, nodeAgg, orgAgg, err = s.queryDayTotals(now)
	case contract.PeriodDay:
		report.Label = "今日"
		report.Labels, report.DownSeries, report.UpSeries, totalDown, totalUp, nodeAgg, orgAgg, err = s.queryDaySeries(now)
	case contract.PeriodWeek:
		report.Label = "近 7 天"
		report.Labels, report.DownSeries, report.UpSeries, totalDown, totalUp, nodeAgg, orgAgg, err = s.queryWeekSeries(now)
	case contract.PeriodMonth30:
		report.Label = "近 30 天"
		report.Labels, report.DownSeries, report.UpSeries, totalDown, totalUp, nodeAgg, orgAgg, err = s.queryMonth30Series(now)
	case contract.PeriodMonthly:
		report.Label = "按月"
		report.Labels, report.DownSeries, report.UpSeries, totalDown, totalUp, nodeAgg, orgAgg, err = s.queryMonthlySeries(now)
	default:
		return contract.TrafficReport{}, fmt.Errorf("stats: unsupported traffic period %q", period)
	}
	if err != nil {
		return contract.TrafficReport{}, err
	}

	totalBytes := totalDown + totalUp
	report.TotalDownGB = bytesToGB(totalDown)
	report.TotalUpGB = bytesToGB(totalUp)
	report.TotalGB = bytesToGB(totalBytes)
	report.ByNode, report.ByOrg = buildBreakdowns(totalBytes, nodeAgg, orgAgg, orgs, nodes)
	return report, nil
}

func (s *Store) queryDayTotals(now time.Time) (int64, int64, map[string]*bytePair, map[string]*bytePair, error) {
	today := now.Format("2006-01-02")
	rows, err := s.db.Query(`SELECT org_id, node_id, down_bytes, up_bytes FROM traffic_daily WHERE day_key = ?;`, today)
	if err != nil {
		return 0, 0, nil, nil, fmt.Errorf("stats: query today totals: %w", err)
	}
	defer func() { _ = rows.Close() }()

	nodeAgg := make(map[string]*bytePair)
	orgAgg := make(map[string]*bytePair)
	var totalDown, totalUp int64

	for rows.Next() {
		var orgID, nodeID string
		var down, up int64
		if err := rows.Scan(&orgID, &nodeID, &down, &up); err != nil {
			return 0, 0, nil, nil, fmt.Errorf("stats: scan today totals: %w", err)
		}
		totalDown += down
		totalUp += up
		addAgg(nodeAgg, nodeID, down, up)
		addAgg(orgAgg, orgID, down, up)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, nil, nil, fmt.Errorf("stats: iterate today totals: %w", err)
	}
	return totalDown, totalUp, nodeAgg, orgAgg, nil
}

func (s *Store) queryDaySeries(now time.Time) ([]string, []float64, []float64, int64, int64, map[string]*bytePair, map[string]*bytePair, error) {
	const bucketCount = 12
	labels := make([]string, bucketCount)
	downBuckets := make([]int64, bucketCount)
	upBuckets := make([]int64, bucketCount)
	for i := 0; i < bucketCount; i++ {
		labels[i] = fmt.Sprintf("%02d:00", i*2)
	}

	today := now.Format("2006-01-02")
	rows, err := s.db.Query(
		`SELECT hour_key, org_id, node_id, down_bytes, up_bytes FROM traffic_hourly WHERE hour_key >= ? AND hour_key <= ?;`,
		today+"T00", today+"T23",
	)
	if err != nil {
		return nil, nil, nil, 0, 0, nil, nil, fmt.Errorf("stats: query day series: %w", err)
	}
	defer func() { _ = rows.Close() }()

	nodeAgg := make(map[string]*bytePair)
	orgAgg := make(map[string]*bytePair)
	var totalDown, totalUp int64

	for rows.Next() {
		var hourKey, orgID, nodeID string
		var down, up int64
		if err := rows.Scan(&hourKey, &orgID, &nodeID, &down, &up); err != nil {
			return nil, nil, nil, 0, 0, nil, nil, fmt.Errorf("stats: scan day series: %w", err)
		}
		totalDown += down
		totalUp += up
		addAgg(nodeAgg, nodeID, down, up)
		addAgg(orgAgg, orgID, down, up)

		if len(hourKey) >= 13 {
			if h, convErr := strconv.Atoi(hourKey[11:13]); convErr == nil && h >= 0 && h < 24 {
				idx := h / 2
				downBuckets[idx] += down
				upBuckets[idx] += up
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, 0, 0, nil, nil, fmt.Errorf("stats: iterate day series: %w", err)
	}

	downSeries, upSeries := bucketsToGBSeries(downBuckets, upBuckets)
	return labels, downSeries, upSeries, totalDown, totalUp, nodeAgg, orgAgg, nil
}

func (s *Store) queryWeekSeries(now time.Time) ([]string, []float64, []float64, int64, int64, map[string]*bytePair, map[string]*bytePair, error) {
	const days = 7
	labels := make([]string, days)
	dayIndex := make(map[string]int, days)
	downBuckets := make([]int64, days)
	upBuckets := make([]int64, days)

	var startKey, endKey string
	for i := 0; i < days; i++ {
		offset := days - 1 - i
		day := now.AddDate(0, 0, -offset)
		key := day.Format("2006-01-02")
		dayIndex[key] = i
		if offset == 0 {
			labels[i] = "今日"
		} else {
			labels[i] = day.Format("01/02")
		}
		if i == 0 {
			startKey = key
		}
		if i == days-1 {
			endKey = key
		}
	}

	totalDown, totalUp, nodeAgg, orgAgg, err := s.scanDailyRange(startKey, endKey, func(dayKey string, down, up int64) {
		if idx, ok := dayIndex[dayKey]; ok {
			downBuckets[idx] += down
			upBuckets[idx] += up
		}
	})
	if err != nil {
		return nil, nil, nil, 0, 0, nil, nil, err
	}

	downSeries, upSeries := bucketsToGBSeries(downBuckets, upBuckets)
	return labels, downSeries, upSeries, totalDown, totalUp, nodeAgg, orgAgg, nil
}

func (s *Store) queryMonth30Series(now time.Time) ([]string, []float64, []float64, int64, int64, map[string]*bytePair, map[string]*bytePair, error) {
	// Split 30 days into 8 contiguous buckets (3+3+4+4+4+4+4+4) so every day maps to one bar label.
	bucketSizes := []int{3, 3, 4, 4, 4, 4, 4, 4}
	labels := make([]string, len(bucketSizes))
	downBuckets := make([]int64, len(bucketSizes))
	upBuckets := make([]int64, len(bucketSizes))
	dayToBucket := make(map[string]int, 30)

	cursorOffset := 29
	var startKey, endKey string
	for bIdx, size := range bucketSizes {
		var bucketEnd time.Time
		for step := 0; step < size; step++ {
			day := now.AddDate(0, 0, -cursorOffset)
			key := day.Format("2006-01-02")
			dayToBucket[key] = bIdx
			if startKey == "" {
				startKey = key
			}
			endKey = key
			bucketEnd = day
			cursorOffset--
		}
		labels[bIdx] = bucketEnd.Format("01/02")
	}

	totalDown, totalUp, nodeAgg, orgAgg, err := s.scanDailyRange(startKey, endKey, func(dayKey string, down, up int64) {
		if idx, ok := dayToBucket[dayKey]; ok {
			downBuckets[idx] += down
			upBuckets[idx] += up
		}
	})
	if err != nil {
		return nil, nil, nil, 0, 0, nil, nil, err
	}

	downSeries, upSeries := bucketsToGBSeries(downBuckets, upBuckets)
	return labels, downSeries, upSeries, totalDown, totalUp, nodeAgg, orgAgg, nil
}

func (s *Store) queryMonthlySeries(now time.Time) ([]string, []float64, []float64, int64, int64, map[string]*bytePair, map[string]*bytePair, error) {
	const months = 12
	labels := make([]string, months)
	monthIndex := make(map[string]int, months)
	downBuckets := make([]int64, months)
	upBuckets := make([]int64, months)

	firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	var startKey, endKey string
	for i := 0; i < months; i++ {
		offset := months - 1 - i
		mTime := firstOfMonth.AddDate(0, -offset, 0)
		key := mTime.Format("2006-01")
		monthIndex[key] = i
		labels[i] = fmt.Sprintf("%d月", int(mTime.Month()))
		if i == 0 {
			startKey = key
		}
		if i == months-1 {
			endKey = key
		}
	}

	rows, err := s.db.Query(
		`SELECT month_key, org_id, node_id, down_bytes, up_bytes FROM traffic_monthly WHERE month_key >= ? AND month_key <= ?;`,
		startKey, endKey,
	)
	if err != nil {
		return nil, nil, nil, 0, 0, nil, nil, fmt.Errorf("stats: query monthly series: %w", err)
	}
	defer func() { _ = rows.Close() }()

	nodeAgg := make(map[string]*bytePair)
	orgAgg := make(map[string]*bytePair)
	var totalDown, totalUp int64

	for rows.Next() {
		var mKey, orgID, nodeID string
		var down, up int64
		if err := rows.Scan(&mKey, &orgID, &nodeID, &down, &up); err != nil {
			return nil, nil, nil, 0, 0, nil, nil, fmt.Errorf("stats: scan monthly series: %w", err)
		}
		totalDown += down
		totalUp += up
		addAgg(nodeAgg, nodeID, down, up)
		addAgg(orgAgg, orgID, down, up)
		if idx, ok := monthIndex[mKey]; ok {
			downBuckets[idx] += down
			upBuckets[idx] += up
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, 0, 0, nil, nil, fmt.Errorf("stats: iterate monthly series: %w", err)
	}

	downSeries, upSeries := bucketsToGBSeries(downBuckets, upBuckets)
	return labels, downSeries, upSeries, totalDown, totalUp, nodeAgg, orgAgg, nil
}

func (s *Store) scanDailyRange(startKey, endKey string, onRow func(dayKey string, down, up int64)) (int64, int64, map[string]*bytePair, map[string]*bytePair, error) {
	rows, err := s.db.Query(
		`SELECT day_key, org_id, node_id, down_bytes, up_bytes FROM traffic_daily WHERE day_key >= ? AND day_key <= ?;`,
		startKey, endKey,
	)
	if err != nil {
		return 0, 0, nil, nil, fmt.Errorf("stats: query daily range: %w", err)
	}
	defer func() { _ = rows.Close() }()

	nodeAgg := make(map[string]*bytePair)
	orgAgg := make(map[string]*bytePair)
	var totalDown, totalUp int64

	for rows.Next() {
		var dayKey, orgID, nodeID string
		var down, up int64
		if err := rows.Scan(&dayKey, &orgID, &nodeID, &down, &up); err != nil {
			return 0, 0, nil, nil, fmt.Errorf("stats: scan daily range: %w", err)
		}
		totalDown += down
		totalUp += up
		addAgg(nodeAgg, nodeID, down, up)
		addAgg(orgAgg, orgID, down, up)
		onRow(dayKey, down, up)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, nil, nil, fmt.Errorf("stats: iterate daily range: %w", err)
	}
	return totalDown, totalUp, nodeAgg, orgAgg, nil
}

func addAgg(m map[string]*bytePair, key string, down, up int64) {
	p, ok := m[key]
	if !ok {
		p = &bytePair{}
		m[key] = p
	}
	p.down += down
	p.up += up
}

func bucketsToGBSeries(downBuckets, upBuckets []int64) ([]float64, []float64) {
	downSeries := make([]float64, len(downBuckets))
	upSeries := make([]float64, len(upBuckets))
	for i := range downBuckets {
		downSeries[i] = bytesToGB(downBuckets[i])
		upSeries[i] = bytesToGB(upBuckets[i])
	}
	return downSeries, upSeries
}

func buildBreakdowns(totalBytes int64, nodeAgg, orgAgg map[string]*bytePair, orgs []contract.Organization, nodes []contract.ExitNode) ([]contract.TrafficBreakdownItem, []contract.TrafficBreakdownItem) {
	orgByID := make(map[string]contract.Organization, len(orgs))
	for _, o := range orgs {
		orgByID[o.ID] = o
	}

	nodeByID := make(map[string]contract.ExitNode, len(nodes))
	orgNameFallback := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nodeByID[n.ID] = n
		if n.OrgID != "" && n.OrgName != "" {
			orgNameFallback[n.OrgID] = n.OrgName
		}
	}

	byNode := make([]contract.TrafficBreakdownItem, 0, len(nodeAgg))
	for nodeID, bp := range nodeAgg {
		sumBytes := bp.down + bp.up
		name := nodeID
		subLabel := ""
		if n, ok := nodeByID[nodeID]; ok {
			if n.Label != "" {
				name = n.Label
			} else if n.Hostname != "" {
				name = n.Hostname
			}
			if n.OrgName != "" {
				subLabel = n.OrgName
			} else if o, ok := orgByID[n.OrgID]; ok {
				subLabel = o.Name
			}
		}
		byNode = append(byNode, contract.TrafficBreakdownItem{
			ID:        nodeID,
			Name:      name,
			SubLabel:  subLabel,
			DownBytes: bp.down,
			UpBytes:   bp.up,
			TotalGB:   bytesToGB(sumBytes),
			Percent:   calcPercent(sumBytes, totalBytes),
		})
	}
	sortBreakdown(byNode)

	byOrg := make([]contract.TrafficBreakdownItem, 0, len(orgAgg))
	for orgID, bp := range orgAgg {
		sumBytes := bp.down + bp.up
		name := orgID
		subLabel := ""
		if o, ok := orgByID[orgID]; ok {
			if o.Name != "" {
				name = o.Name
			}
			subLabel = o.Account
		} else if fallback, ok := orgNameFallback[orgID]; ok {
			name = fallback
		}
		byOrg = append(byOrg, contract.TrafficBreakdownItem{
			ID:        orgID,
			Name:      name,
			SubLabel:  subLabel,
			DownBytes: bp.down,
			UpBytes:   bp.up,
			TotalGB:   bytesToGB(sumBytes),
			Percent:   calcPercent(sumBytes, totalBytes),
		})
	}
	sortBreakdown(byOrg)

	return byNode, byOrg
}

func calcPercent(part, total int64) float64 {
	if total <= 0 || part <= 0 {
		return 0
	}
	return round1(float64(part) / float64(total) * 100)
}

func sortBreakdown(items []contract.TrafficBreakdownItem) {
	sort.Slice(items, func(i, j int) bool {
		ti := items[i].DownBytes + items[i].UpBytes
		tj := items[j].DownBytes + items[j].UpBytes
		if ti != tj {
			return ti > tj
		}
		return items[i].ID < items[j].ID
	})
}
