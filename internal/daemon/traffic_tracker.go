package daemon

import (
	"math"

	"github.com/singtail/singtail/internal/contract"
	"github.com/singtail/singtail/internal/engine"
)

const exitActivityHoldTicks = 5

type connCounter struct {
	down int64
	up   int64
}

// ExitOrgUsage represents attributed upload and download byte increments for one Tailscale organization.
type ExitOrgUsage struct {
	OrgID     string
	DownBytes int64
	UpBytes   int64
}

// ExitTrafficDelta holds total and per-organization exit-node byte increments for one sample interval.
type ExitTrafficDelta struct {
	DownBytes int64
	UpBytes   int64
	ByOrg     []ExitOrgUsage
}

// TrafficDeltaTracker isolates exit-node traffic deltas from direct, fallback-proxy, and reject traffic.
type TrafficDeltaTracker struct {
	initialized       bool
	lastDownloadTotal int64
	lastUploadTotal   int64
	lastConns         map[string]connCounter

	recentDownShare float64
	recentUpShare   float64
	recentDownHold  int
	recentUpHold    int
}

// Reset clears accumulated counter history when TUN stops or restarts.
func (t *TrafficDeltaTracker) Reset() {
	t.initialized = false
	t.lastDownloadTotal = 0
	t.lastUploadTotal = 0
	t.lastConns = nil
	t.recentDownShare = 0
	t.recentUpShare = 0
	t.recentDownHold = 0
	t.recentUpHold = 0
}

// Compute returns exit-node-only byte deltas between the previous snapshot and snap.
func (t *TrafficDeltaTracker) Compute(mode contract.RoutingMode, defaultOrgID string, snap engine.ClashConnectionsSnapshot) ExitTrafficDelta {
	prevConns := t.lastConns
	downReset := !t.initialized || snap.DownloadTotal < t.lastDownloadTotal
	upReset := !t.initialized || snap.UploadTotal < t.lastUploadTotal

	var globalDownDelta, globalUpDelta int64
	if downReset {
		globalDownDelta = maxInt64(0, snap.DownloadTotal)
	} else {
		globalDownDelta = snap.DownloadTotal - t.lastDownloadTotal
	}
	if upReset {
		globalUpDelta = maxInt64(0, snap.UploadTotal)
	} else {
		globalUpDelta = snap.UploadTotal - t.lastUploadTotal
	}

	t.initialized = true
	t.lastDownloadTotal = snap.DownloadTotal
	t.lastUploadTotal = snap.UploadTotal

	if downReset && upReset {
		prevConns = nil
	}

	nextConns := make(map[string]connCounter, len(snap.ConnTraffic))
	var (
		trackedExitDown, trackedExitUp       int64
		trackedNonExitDown, trackedNonExitUp int64
		activeExitConns, activeNonExitConns  int
	)
	orgDown := make(map[string]int64)
	orgUp := make(map[string]int64)
	orgOrder := make([]string, 0, 2)
	ensureOrg := func(id string) {
		if id == "" {
			id = defaultOrgID
		}
		if _, exists := orgDown[id]; !exists {
			if _, upExists := orgUp[id]; !upExists {
				orgOrder = append(orgOrder, id)
			}
		}
	}

	for _, c := range snap.ConnTraffic {
		var prev connCounter
		if prevConns != nil && c.ID != "" {
			prev = prevConns[c.ID]
		}
		dd := deltaCounter(c.Download, prev.down, downReset)
		du := deltaCounter(c.Upload, prev.up, upReset)
		if c.ID != "" {
			nextConns[c.ID] = connCounter{down: c.Download, up: c.Upload}
		}

		if c.IsExitNode() {
			activeExitConns++
			trackedExitDown += dd
			trackedExitUp += du
			oid := c.ExitOrgID()
			if oid == "" {
				oid = defaultOrgID
			}
			ensureOrg(oid)
			orgDown[oid] += dd
			orgUp[oid] += du
		} else {
			activeNonExitConns++
			trackedNonExitDown += dd
			trackedNonExitUp += du
		}
	}
	t.lastConns = nextConns

	if mode == contract.ModeDirect {
		t.recentDownShare = 0
		t.recentUpShare = 0
		t.recentDownHold = 0
		t.recentUpHold = 0
		return ExitTrafficDelta{}
	}

	untrackedDown := maxInt64(0, globalDownDelta-(trackedExitDown+trackedNonExitDown))
	untrackedUp := maxInt64(0, globalUpDelta-(trackedExitUp+trackedNonExitUp))

	untrackedExitDown := t.estimateUntrackedExit(
		mode,
		untrackedDown,
		trackedExitDown,
		trackedNonExitDown,
		activeExitConns,
		activeNonExitConns,
		&t.recentDownShare,
		&t.recentDownHold,
	)
	untrackedExitUp := t.estimateUntrackedExit(
		mode,
		untrackedUp,
		trackedExitUp,
		trackedNonExitUp,
		activeExitConns,
		activeNonExitConns,
		&t.recentUpShare,
		&t.recentUpHold,
	)

	totalExitDown := clampInt64(trackedExitDown+untrackedExitDown, 0, globalDownDelta)
	totalExitUp := clampInt64(trackedExitUp+untrackedExitUp, 0, globalUpDelta)

	if totalExitDown > trackedExitDown || totalExitUp > trackedExitUp {
		targetOrg := dominantOrg(orgOrder, orgDown, orgUp, defaultOrgID)
		ensureOrg(targetOrg)
		orgDown[targetOrg] += maxInt64(0, totalExitDown-trackedExitDown)
		orgUp[targetOrg] += maxInt64(0, totalExitUp-trackedExitUp)
	}

	byOrg := make([]ExitOrgUsage, 0, len(orgOrder))
	for _, oid := range orgOrder {
		d, u := orgDown[oid], orgUp[oid]
		if d > 0 || u > 0 {
			byOrg = append(byOrg, ExitOrgUsage{
				OrgID:     oid,
				DownBytes: d,
				UpBytes:   u,
			})
		}
	}

	return ExitTrafficDelta{
		DownBytes: totalExitDown,
		UpBytes:   totalExitUp,
		ByOrg:     byOrg,
	}
}

func (t *TrafficDeltaTracker) estimateUntrackedExit(
	mode contract.RoutingMode,
	untracked int64,
	trackedExit int64,
	trackedNonExit int64,
	activeExitConns int,
	activeNonExitConns int,
	recentShare *float64,
	recentHold *int,
) int64 {
	trackedTotal := trackedExit + trackedNonExit
	if trackedTotal > 0 {
		share := float64(trackedExit) / float64(trackedTotal)
		if trackedExit > 0 {
			*recentShare = share
			*recentHold = exitActivityHoldTicks
		} else if *recentHold > 0 {
			*recentHold--
			if *recentHold == 0 {
				*recentShare = 0
			}
		}
		if untracked <= 0 {
			return 0
		}
		return int64(math.Round(float64(untracked) * share))
	}

	if untracked <= 0 {
		if *recentHold > 0 {
			*recentHold--
			if *recentHold == 0 {
				*recentShare = 0
			}
		}
		return 0
	}

	if mode == contract.ModeGlobal {
		if activeNonExitConns > 0 && activeExitConns == 0 {
			return 0
		}
		return untracked
	}

	if *recentHold > 0 && *recentShare > 0 {
		*recentHold--
		share := *recentShare
		if *recentHold == 0 {
			*recentShare = 0
		}
		return int64(math.Round(float64(untracked) * share))
	}

	if activeExitConns > 0 {
		share := float64(activeExitConns) / float64(activeExitConns+activeNonExitConns)
		return int64(math.Round(float64(untracked) * share))
	}
	return 0
}

func deltaCounter(curr, prev int64, reset bool) int64 {
	if reset || curr < prev {
		return maxInt64(0, curr)
	}
	return curr - prev
}

func dominantOrg(order []string, down, up map[string]int64, fallback string) string {
	bestOrg := fallback
	var bestBytes int64 = -1
	for _, oid := range order {
		sum := down[oid] + up[oid]
		if sum > bestBytes {
			bestBytes = sum
			bestOrg = oid
		}
	}
	if bestOrg == "" && len(order) > 0 {
		return order[0]
	}
	return bestOrg
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func clampInt64(v, minV, maxV int64) int64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
