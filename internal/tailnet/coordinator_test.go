package tailnet

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/singtail/singtail/internal/contract"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/types/key"
)

func TestNewCoordinatorZeroMockAndDemoModes(t *testing.T) {
	t.Run("production mode starts empty when initialOrgs is empty", func(t *testing.T) {
		coord := NewCoordinator("data", nil, nil, false)
		orgs := coord.ListOrgs()
		nodes := coord.ListExitNodes()
		if orgs == nil || len(orgs) != 0 {
			t.Fatalf("expected non-nil empty orgs, got %#v", orgs)
		}
		if nodes == nil || len(nodes) != 0 {
			t.Fatalf("expected non-nil empty nodes, got %#v", nodes)
		}
	})

	t.Run("production mode restores persisted orgs and nodes verbatim", func(t *testing.T) {
		inOrgs := []contract.Organization{
			{ID: "org-1", Name: "Work", Account: "a@b.com", LocalTailscaleIP: "100.64.0.1"},
		}
		inNodes := []contract.ExitNode{
			{ID: "node-1", OrgID: "org-1", OrgName: "Work", Hostname: "gw-1", TailscaleIP: "100.64.0.2", PublicIP: "1.2.3.4", Online: true, Healthy: true, LatencyMs: 25},
		}
		coord := NewCoordinator("data", inOrgs, inNodes, false)
		orgs := coord.ListOrgs()
		nodes := coord.ListExitNodes()
		if len(orgs) != 1 || orgs[0].StateDir != "data/ts-state-org-1" {
			t.Fatalf("unexpected restored orgs: %+v", orgs)
		}
		if len(nodes) != 1 || nodes[0] != inNodes[0] {
			t.Fatalf("unexpected restored nodes: %+v", nodes)
		}
	})

	t.Run("demo mode loads demo orgs and nodes when initialOrgs is empty", func(t *testing.T) {
		coord := NewCoordinator("data", nil, nil, true)

		orgs := coord.ListOrgs()
		if len(orgs) != 2 {
			t.Fatalf("expected 2 demo orgs, got %d", len(orgs))
		}
		if orgs[0].ID != "org-personal" || orgs[0].StateDir != "data/ts-state-org-personal" {
			t.Fatalf("unexpected first org: %+v", orgs[0])
		}
		if orgs[1].ID != "org-studio" || orgs[1].StateDir != "data/ts-state-org-studio" {
			t.Fatalf("unexpected second org: %+v", orgs[1])
		}

		nodes := coord.ListExitNodes()
		if len(nodes) != 5 {
			t.Fatalf("expected 5 demo nodes, got %d", len(nodes))
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
				t.Fatalf("missing demo node %s", tc.id)
			}
			if node.PublicIP != tc.pubIP || node.Healthy != tc.healthy {
				t.Fatalf("unexpected node %s: %+v", tc.id, node)
			}
		}
	})
}

func TestExtractOrgAndNodesFromStatus(t *testing.T) {
	k1 := key.NewNode().Public()
	k2 := key.NewNode().Public()
	k3 := key.NewNode().Public()

	st := &ipnstate.Status{
		BackendState: "Running",
		TailscaleIPs: []netip.Addr{
			netip.MustParseAddr("fd7a:115c:a1e0::1"),
			netip.MustParseAddr("100.88.1.10"),
		},
		CurrentTailnet: &ipnstate.TailnetStatus{Name: "acme.ts.net"},
		Self: &ipnstate.PeerStatus{
			UserID:  tailcfg.UserID(42),
			DNSName: "self.acme.ts.net.",
		},
		User: map[tailcfg.UserID]tailcfg.UserProfile{
			tailcfg.UserID(42): {LoginName: "alice@acme.io"},
		},
		Peer: map[key.NodePublic]*ipnstate.PeerStatus{
			k1: {
				ID:             "node-exit-1",
				HostName:       "hk-gateway",
				DNSName:        "hk-gateway.acme.ts.net.",
				TailscaleIPs:   []netip.Addr{netip.MustParseAddr("fd7a:115c:a1e0::2"), netip.MustParseAddr("100.88.1.20")},
				CurAddr:        "43.153.82.19:41641",
				Relay:          "hkg",
				Online:         true,
				ExitNodeOption: true,
				Location:       &tailcfg.Location{CountryCode: "HK", City: "Hong Kong"},
			},
			k2: {
				ID:             "node-exit-2",
				HostName:       "",
				DNSName:        "sg-relay.acme.ts.net.",
				TailscaleIPs:   []netip.Addr{netip.MustParseAddr("100.88.1.30")},
				CurAddr:        "",
				Relay:          "sin",
				Online:         false,
				ExitNodeOption: true,
			},
			k3: {
				ID:             "node-client-only",
				HostName:       "laptop",
				TailscaleIPs:   []netip.Addr{netip.MustParseAddr("100.88.1.40")},
				Online:         true,
				ExitNodeOption: false,
			},
		},
	}

	org, nodes := extractOrgAndNodesFromStatus("org-acme", "/tmp/ts-state-org-acme", "", "", st)
	if org.Name != "acme.ts.net" || org.Account != "alice@acme.io" || org.LocalTailscaleIP != "100.88.1.10" {
		t.Fatalf("unexpected extracted org: %+v", org)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 exit nodes (non-exit peer filtered out), got %d: %+v", len(nodes), nodes)
	}

	if nodes[0].Hostname != "hk-gateway" ||
		nodes[0].Label != "Hong Kong · hk-gateway" ||
		nodes[0].TailscaleIP != "100.88.1.20" ||
		nodes[0].PublicIP != "" ||
		nodes[0].Region != "HK · Hong Kong" ||
		nodes[0].RelayMode != "直连" ||
		!nodes[0].Online || !nodes[0].Healthy || nodes[0].LatencyMs != 0 {
		t.Fatalf("unexpected first extracted exit node: %+v", nodes[0])
	}

	if nodes[1].Hostname != "sg-relay" ||
		nodes[1].Label != "sg-relay" ||
		nodes[1].TailscaleIP != "100.88.1.30" ||
		nodes[1].Region != "SIN" ||
		nodes[1].RelayMode != "中继" ||
		nodes[1].Online || nodes[1].Healthy {
		t.Fatalf("unexpected second extracted exit node: %+v", nodes[1])
	}

	orgAlias, _ := extractOrgAndNodesFromStatus("org-acme", "/tmp/ts-state-org-acme", "My Custom Alias", "https://headscale.example.com", st)
	if orgAlias.Name != "My Custom Alias" || orgAlias.ControlURL != "https://headscale.example.com" {
		t.Fatalf("expected alias and controlURL preserved, got %+v", orgAlias)
	}
}

func TestAddOrgRemoveOrgAndUpdateNodePublicIP(t *testing.T) {
	stateRoot := t.TempDir()
	coord := NewCoordinator(stateRoot, nil, nil, false)

	if _, _, err := coord.AddOrg(contract.AddOrgRequest{Name: "   "}); err == nil {
		t.Fatal("expected error for empty org name")
	}

	org, node, err := coord.AddOrg(contract.AddOrgRequest{
		Name:         "Acme Labs",
		Account:      "ops@acme.dev",
		NodeHostname: "acme-tyo-01",
		PublicIP:     "",
		Region:       "JP · Tokyo",
	})
	if err != nil {
		t.Fatalf("AddOrg failed: %v", err)
	}

	if !strings.HasSuffix(org.StateDir, "ts-state-"+org.ID) {
		t.Fatalf("unexpected StateDir: %s", org.StateDir)
	}
	if err := os.MkdirAll(filepath.FromSlash(org.StateDir), 0o700); err != nil {
		t.Fatalf("mkdir org stateDir: %v", err)
	}

	coord.UpdateNodePublicIP(node.ID, "198.51.100.42")
	nodes := coord.ListExitNodes()
	if len(nodes) != 1 || nodes[0].PublicIP != "198.51.100.42" {
		t.Fatalf("expected PublicIP updated, got %+v", nodes)
	}

	if err := coord.RemoveOrg("non-existent"); err == nil {
		t.Fatal("expected error removing unknown org")
	}
	if err := coord.RemoveOrg(org.ID); err != nil {
		t.Fatalf("RemoveOrg failed: %v", err)
	}
	if len(coord.ListOrgs()) != 0 || len(coord.ListExitNodes()) != 0 {
		t.Fatalf("expected empty orgs and nodes after RemoveOrg")
	}
	if _, err := os.Stat(filepath.FromSlash(org.StateDir)); !os.IsNotExist(err) {
		t.Fatalf("expected org stateDir removed, stat err=%v", err)
	}
}

func TestPingNodesProductionAndDemo(t *testing.T) {
	t.Run("production mode does not fabricate random latencies", func(t *testing.T) {
		coord := NewCoordinator("data",
			[]contract.Organization{{ID: "org-1", Name: "Work"}},
			[]contract.ExitNode{
				{ID: "n-online", OrgID: "org-1", TailscaleIP: "127.0.0.1", Online: true, Healthy: true, LatencyMs: 42},
				{ID: "n-offline", OrgID: "org-1", TailscaleIP: "127.0.0.1", Online: false, Healthy: false, LatencyMs: 0},
			},
			false,
		)

		got := coord.PingNodes(context.Background())
		if len(got) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(got))
		}
		if got[1].LatencyMs != 0 {
			t.Fatalf("expected offline node latency preserved as 0, got %d", got[1].LatencyMs)
		}
	})

	t.Run("demo mode simulates latency range", func(t *testing.T) {
		coord := NewCoordinator("data", nil, nil, true)

		if err := coord.SyncOrg("non-existent"); err == nil {
			t.Fatal("expected error when syncing unknown org")
		}
		if err := coord.SyncOrg("org-studio"); err != nil {
			t.Fatalf("SyncOrg failed: %v", err)
		}

		for round := 0; round < 5; round++ {
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
	})
}

func TestImportStateDirValidationAndUnauthenticatedDir(t *testing.T) {
	stateRoot := t.TempDir()
	coord := NewCoordinator(stateRoot, nil, nil, false)

	if _, _, err := coord.ImportStateDir(context.Background(), contract.ImportOrgStateDirRequest{
		SourceDir: filepath.Join(stateRoot, "does-not-exist"),
	}); err == nil {
		t.Fatal("expected error for missing source directory")
	}

	filePath := filepath.Join(stateRoot, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if _, _, err := coord.ImportStateDir(context.Background(), contract.ImportOrgStateDirRequest{
		SourceDir: filePath,
	}); err == nil {
		t.Fatal("expected error when source path is a file")
	}

	emptySrc := filepath.Join(stateRoot, "empty-ts-state")
	if err := os.MkdirAll(emptySrc, 0o700); err != nil {
		t.Fatalf("mkdir emptySrc: %v", err)
	}
	_, _, err := coord.ImportStateDir(context.Background(), contract.ImportOrgStateDirRequest{
		SourceDir: emptySrc,
		Alias:     "Empty Test",
	})
	if err == nil || !strings.Contains(err.Error(), "该目录尚未完成 Tailscale 登录授权") {
		t.Fatalf("expected unauthenticated dir error, got %v", err)
	}
	if len(coord.ListOrgs()) != 0 {
		t.Fatalf("expected no org registered on failed import")
	}
}

func TestStartLoginValidationAndPoll(t *testing.T) {
	coord := NewCoordinator(t.TempDir(), nil, nil, false)

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coord.StartLogin(cancelledCtx, contract.StartOrgLoginRequest{Alias: "Cancelled"}); err == nil {
		t.Fatal("expected error when StartLogin receives cancelled context")
	}

	if _, ok := coord.PollLogin("missing-session"); ok {
		t.Fatal("expected PollLogin to return false for unknown session")
	}
}

func TestRefreshOrgFromTsnetValidation(t *testing.T) {
	coord := NewCoordinator(t.TempDir(), nil, nil, false)
	if _, err := coord.RefreshOrgFromTsnet(context.Background(), "missing"); err == nil {
		t.Fatal("expected error refreshing unknown org")
	}
}

func TestCoordinatorConcurrentAccess(t *testing.T) {
	coord := NewCoordinator(t.TempDir(), nil, nil, false)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = coord.ListOrgs()
			_ = coord.ListExitNodes()
			_ = coord.PingNodes(context.Background())
			if idx%2 == 0 {
				_, node, err := coord.AddOrg(contract.AddOrgRequest{Name: "Concurrent Org"})
				if err == nil {
					coord.UpdateNodePublicIP(node.ID, "203.0.113.10")
				}
			} else {
				_ = coord.SyncOrg("")
			}
		}(i)
	}
	wg.Wait()
}
