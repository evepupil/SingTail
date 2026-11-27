package tailnet

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/singtail/singtail/internal/contract"
)

func TestNewCoordinatorDefaults(t *testing.T) {
	coord := NewCoordinator("data", nil)

	orgs := coord.ListOrgs()
	if len(orgs) != 2 {
		t.Fatalf("expected 2 default orgs, got %d", len(orgs))
	}
	if orgs[0].ID != "org-personal" || orgs[0].StateDir != "data/ts-state-org-personal" {
		t.Fatalf("unexpected first org: %+v", orgs[0])
	}
	if orgs[1].ID != "org-studio" || orgs[1].StateDir != "data/ts-state-org-studio" {
		t.Fatalf("unexpected second org: %+v", orgs[1])
	}

	nodes := coord.ListExitNodes()
	if len(nodes) != 5 {
		t.Fatalf("expected 5 default nodes, got %d", len(nodes))
	}

	byID := make(map[string]contract.ExitNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	checks := []struct {
		id      string
		pubIP   string
		healthy bool
	}{
		{"ts-us-macmini", "68.225.41.109", true},
		{"ts-hk-cloud", "43.153.82.19", true},
		{"ts-jp-tokyo", "172.104.92.77", true},
		{"ts-sg-relay", "18.141.209.54", true},
		{"ts-us-broken", "203.0.113.88", false},
	}
	for _, tc := range checks {
		node, ok := byID[tc.id]
		if !ok {
			t.Fatalf("missing default node %s", tc.id)
		}
		if node.PublicIP != tc.pubIP || node.Healthy != tc.healthy {
			t.Fatalf("unexpected node %s: %+v", tc.id, node)
		}
	}
}

func TestAddOrgPrependsNodeAndAssignsStateDir(t *testing.T) {
	coord := NewCoordinator("/var/lib/singtail", nil)

	if _, _, err := coord.AddOrg(contract.AddOrgRequest{Name: "   "}); err == nil {
		t.Fatal("expected error for empty org name")
	}

	org, node, err := coord.AddOrg(contract.AddOrgRequest{
		Name:         "Acme Labs",
		Account:      "ops@acme.dev",
		NodeHostname: "acme-tyo-01",
		PublicIP:     "198.51.100.42",
		Region:       "JP · Tokyo",
	})
	if err != nil {
		t.Fatalf("AddOrg failed: %v", err)
	}

	if !strings.HasPrefix(org.StateDir, "/var/lib/singtail/ts-state-"+org.ID) {
		t.Fatalf("unexpected StateDir: %s", org.StateDir)
	}
	if node.OrgID != org.ID || node.PublicIP != "198.51.100.42" || !node.Healthy || !node.Online {
		t.Fatalf("unexpected created node: %+v", node)
	}

	nodes := coord.ListExitNodes()
	if len(nodes) != 6 {
		t.Fatalf("expected 6 nodes after AddOrg, got %d", len(nodes))
	}
	if nodes[0].ID != node.ID {
		t.Fatalf("expected new node %s at index 0, got %s", node.ID, nodes[0].ID)
	}
}

func TestSyncOrgAndPingNodes(t *testing.T) {
	coord := NewCoordinator("data", nil)

	if err := coord.SyncOrg("non-existent"); err == nil {
		t.Fatal("expected error when syncing unknown org")
	}
	if err := coord.SyncOrg("org-studio"); err != nil {
		t.Fatalf("SyncOrg failed: %v", err)
	}
	for _, org := range coord.ListOrgs() {
		if org.ID == "org-studio" && org.LastSync != "刚刚" {
			t.Fatalf("expected org-studio LastSync updated, got %q", org.LastSync)
		}
	}

	for round := 0; round < 10; round++ {
		nodes := coord.PingNodes(context.Background())
		for _, n := range nodes {
			if !n.Healthy {
				if n.LatencyMs != 299 {
					t.Fatalf("expected unhealthy node %s at 299ms, got %d", n.ID, n.LatencyMs)
				}
				continue
			}
			if n.LatencyMs < 18 {
				t.Fatalf("expected healthy node %s latency >= 18ms, got %d", n.ID, n.LatencyMs)
			}
		}
	}
}

func TestCoordinatorConcurrentAccess(t *testing.T) {
	coord := NewCoordinator("data", nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = coord.ListOrgs()
			_ = coord.ListExitNodes()
			_ = coord.PingNodes(context.Background())
			if idx%2 == 0 {
				_, _, _ = coord.AddOrg(contract.AddOrgRequest{Name: "Concurrent Org"})
			} else {
				_ = coord.SyncOrg("")
			}
		}(i)
	}
	wg.Wait()
}
