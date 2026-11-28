package daemon

import (
	"testing"

	"github.com/singtail/singtail/internal/contract"
	"github.com/singtail/singtail/internal/engine"
)

func TestTrafficDeltaTrackerExcludesDirectAndSeparatesExit(t *testing.T) {
	var tracker TrafficDeltaTracker

	snap1 := engine.ClashConnectionsSnapshot{
		DownloadTotal: 1500,
		UploadTotal:   600,
		ConnTraffic: []engine.ClashConnTraffic{
			{ID: "exit-1", Download: 1000, Upload: 300, Chains: []string{"ep-org-1", "默认出口"}, RouteAction: contract.ActionExit},
			{ID: "cn-1", Download: 500, Upload: 300, Chains: []string{"direct"}, RouteAction: contract.ActionDirect},
		},
	}

	d1 := tracker.Compute(contract.ModeRule, "org-1", snap1)
	if d1.DownBytes != 1000 || d1.UpBytes != 300 {
		t.Fatalf("first tick mismatch: got down=%d up=%d, want down=1000 up=300", d1.DownBytes, d1.UpBytes)
	}

	// Simulate pure direct download burst while exit stays idle.
	snap2 := engine.ClashConnectionsSnapshot{
		DownloadTotal: 10_001_500,
		UploadTotal:   100_600,
		ConnTraffic: []engine.ClashConnTraffic{
			{ID: "exit-1", Download: 1000, Upload: 300, Chains: []string{"ep-org-1", "默认出口"}, RouteAction: contract.ActionExit},
			{ID: "cn-1", Download: 10_000_500, Upload: 100_300, Chains: []string{"direct"}, RouteAction: contract.ActionDirect},
		},
	}
	d2 := tracker.Compute(contract.ModeRule, "org-1", snap2)
	if d2.DownBytes != 0 || d2.UpBytes != 0 {
		t.Fatalf("expected direct-only delta to attribute 0 exit bytes, got down=%d up=%d", d2.DownBytes, d2.UpBytes)
	}

	// Direct mode must attribute 0 even if global counters advance.
	snap3 := engine.ClashConnectionsSnapshot{
		DownloadTotal: 12_001_500,
		UploadTotal:   200_600,
	}
	d3 := tracker.Compute(contract.ModeDirect, "org-1", snap3)
	if d3.DownBytes != 0 || d3.UpBytes != 0 {
		t.Fatalf("expected direct mode to attribute 0 exit bytes, got down=%d up=%d", d3.DownBytes, d3.UpBytes)
	}
}

func TestTrafficDeltaTrackerHandlesShortLivedExitAndReset(t *testing.T) {
	var tracker TrafficDeltaTracker

	snap1 := engine.ClashConnectionsSnapshot{
		DownloadTotal: 2000,
		UploadTotal:   500,
		ConnTraffic: []engine.ClashConnTraffic{
			{ID: "exit-1", Download: 2000, Upload: 500, Chains: []string{"ep-org-1", "默认出口"}, RouteAction: contract.ActionExit},
		},
	}
	_ = tracker.Compute(contract.ModeRule, "org-1", snap1)

	// Next tick: exit-1 grew by 1000 and another short exit connection closed after transferring 3000 bytes.
	snap2 := engine.ClashConnectionsSnapshot{
		DownloadTotal: 6000,
		UploadTotal:   1500,
		ConnTraffic: []engine.ClashConnTraffic{
			{ID: "exit-1", Download: 3000, Upload: 800, Chains: []string{"ep-org-1", "默认出口"}, RouteAction: contract.ActionExit},
		},
	}
	d2 := tracker.Compute(contract.ModeRule, "org-1", snap2)
	if d2.DownBytes != 4000 || d2.UpBytes != 1000 {
		t.Fatalf("expected short-lived exit burst inclusion: got down=%d up=%d, want 4000/1000", d2.DownBytes, d2.UpBytes)
	}

	// Simulate sing-box restart (global counters dropped below previous totals).
	snapReset := engine.ClashConnectionsSnapshot{
		DownloadTotal: 800,
		UploadTotal:   200,
		ConnTraffic: []engine.ClashConnTraffic{
			{ID: "exit-new", Download: 800, Upload: 200, Chains: []string{"ep-org-2", "默认出口"}, RouteAction: contract.ActionExit},
		},
	}
	dReset := tracker.Compute(contract.ModeRule, "org-1", snapReset)
	if dReset.DownBytes != 800 || dReset.UpBytes != 200 {
		t.Fatalf("restart delta mismatch: got down=%d up=%d", dReset.DownBytes, dReset.UpBytes)
	}
	if len(dReset.ByOrg) != 1 || dReset.ByOrg[0].OrgID != "org-2" {
		t.Fatalf("expected org-2 attribution from chain, got %+v", dReset.ByOrg)
	}
}
