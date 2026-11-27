package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/singtail/singtail/internal/contract"
)

func TestGenerateSingBoxConfigComplete(t *testing.T) {
	orgs := []contract.Organization{
		{ID: "org-personal", Name: "Personal Tailnet", StateDir: "data/ts-state-org-personal"},
		{ID: "org-studio", Name: "GolfJoy Studio", StateDir: "data/ts-state-org-studio"},
	}
	nodes := []contract.ExitNode{
		{ID: "ts-hk-cloud", OrgID: "org-personal", TailscaleIP: "100.91.77.63", Online: true, Healthy: true},
		{ID: "ts-us-macmini", OrgID: "org-studio", TailscaleIP: "100.123.114.28", Online: true, Healthy: true},
		{ID: "ts-sg-relay", OrgID: "org-studio", TailscaleIP: "100.123.45.90", Online: true, Healthy: true},
		{ID: "ts-offline", OrgID: "org-studio", TailscaleIP: "100.123.99.99", Online: false, Healthy: false},
	}
	rules := []contract.Rule{
		{ID: "r-1", Type: contract.RuleIPCIDR, Payload: "8.128.0.0/10", Action: contract.ActionDirect, Enabled: true},
		{ID: "r-2", Type: contract.RuleDomainSuffix, Payload: "netflix.com, nflxvideo.net", Action: contract.ActionFallbackProxy, Enabled: true},
		{ID: "r-3", Type: contract.RuleSet, Payload: "geosite-category-ads-all", Action: contract.ActionReject, Enabled: true},
		{ID: "r-4", Type: contract.RuleDomainKeyword, Payload: "disabled-domain", Action: contract.ActionDirect, Enabled: false},
		{ID: "r-final", Type: contract.RuleFinal, Payload: "默认兜底", Action: contract.ActionExit, Enabled: true},
	}
	providers := []contract.RuleProvider{
		{Tag: "geosite-cn", URL: "https://example.com/geosite-cn.srs"},
		{Tag: "geosite-category-ads-all", URL: "https://example.com/ads.srs"},
	}
	settings := contract.SystemSettings{
		FallbackProxyEnabled: true,
		FallbackProxyAddr:    "127.0.0.1:7897",
	}

	raw, err := GenerateSingBoxConfigWithOptions(orgs, nodes, "ts-sg-relay", contract.ModeRule, rules, providers, settings, ConfigOptions{
		TargetGOOS: "windows",
	})
	if err != nil {
		t.Fatalf("GenerateSingBoxConfigWithOptions failed: %v", err)
	}

	if !strings.Contains(string(raw), "\n  \"") {
		t.Fatal("expected indented JSON output")
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Verify experimental.clash_api.external_controller defaults to 127.0.0.1:19090
	exp := parsed["experimental"].(map[string]any)
	clashAPI := exp["clash_api"].(map[string]any)
	if clashAPI["external_controller"] != DefaultClashAPIAddr {
		t.Fatalf("expected external_controller %s, got %v", DefaultClashAPIAddr, clashAPI["external_controller"])
	}

	// Verify inbounds: tun + mixed
	inbounds := parsed["inbounds"].([]any)
	if len(inbounds) != 2 {
		t.Fatalf("expected 2 inbounds, got %d", len(inbounds))
	}
	tun := inbounds[0].(map[string]any)
	if tun["interface_name"] != "singbox" || int(tun["mtu"].(float64)) != 1280 {
		t.Fatalf("unexpected tun inbound: %+v", tun)
	}
	if !containsString(tun["address"].([]any), "172.19.0.1/30") {
		t.Fatalf("missing tun address: %+v", tun["address"])
	}
	if !containsString(tun["route_exclude_address"].([]any), "100.64.0.0/10") {
		t.Fatalf("missing 100.64.0.0/10 exclusion: %+v", tun["route_exclude_address"])
	}
	mixed := inbounds[1].(map[string]any)
	if mixed["listen"] != "127.0.0.1" || int(mixed["listen_port"].(float64)) != 7899 {
		t.Fatalf("unexpected mixed inbound: %+v", mixed)
	}

	// Verify DNS
	dns := parsed["dns"].(map[string]any)
	if dns["strategy"] != "ipv4_only" {
		t.Fatalf("expected strategy ipv4_only, got %v", dns["strategy"])
	}
	dnsServers := dns["servers"].([]any)
	if len(dnsServers) != 2 {
		t.Fatalf("expected 2 dns servers, got %d", len(dnsServers))
	}
	dnsRaw := string(raw)
	if !strings.Contains(dnsRaw, `"223.5.5.5"`) || !strings.Contains(dnsRaw, `"1.1.1.1"`) || !strings.Contains(dnsRaw, `"tailscale.com"`) || !strings.Contains(dnsRaw, `"tailscale.io"`) {
		t.Fatalf("missing expected DNS servers or tailscale bypass domains")
	}

	// Verify 1 endpoint per organization and active node IP selection
	endpoints := parsed["endpoints"].([]any)
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 org endpoints, got %d", len(endpoints))
	}
	ep0 := endpoints[0].(map[string]any)
	ep1 := endpoints[1].(map[string]any)
	if ep0["tag"] != "ep-org-personal" || ep0["exit_node"] != "100.91.77.63" {
		t.Fatalf("unexpected first endpoint: %+v", ep0)
	}
	if ep1["tag"] != "ep-org-studio" || ep1["type"] != "tailscale" || ep1["state_directory"] != "data/ts-state-org-studio" || ep1["exit_node"] != "100.123.45.90" {
		t.Fatalf("unexpected studio endpoint: %+v", ep1)
	}

	// Verify outbounds: selector + direct + fallback-proxy
	outbounds := parsed["outbounds"].([]any)
	if len(outbounds) != 3 {
		t.Fatalf("expected 3 outbounds, got %d", len(outbounds))
	}
	sel := outbounds[0].(map[string]any)
	if sel["tag"] != "默认出口" || sel["default"] != "ep-org-studio" {
		t.Fatalf("unexpected selector outbound: %+v", sel)
	}
	selOutbounds := sel["outbounds"].([]any)
	if !containsString(selOutbounds, "ep-org-personal") || !containsString(selOutbounds, "ep-org-studio") || !containsString(selOutbounds, "direct") {
		t.Fatalf("unexpected selector outbounds: %+v", selOutbounds)
	}
	fb := outbounds[2].(map[string]any)
	if fb["tag"] != "fallback-proxy" || fb["server"] != "127.0.0.1" || int(fb["server_port"].(float64)) != 7897 {
		t.Fatalf("unexpected fallback-proxy outbound: %+v", fb)
	}

	// Verify built-in process bypass and enabled rules
	route := parsed["route"].(map[string]any)
	routeJSON, _ := json.Marshal(route)
	routeStr := string(routeJSON)
	for _, needle := range []string{
		"tailscaled.exe",
		"verge-mihomo.exe",
		"clash-verge.exe",
		"IPNExtension",
		"8.128.0.0/10",
		"netflix.com",
		"fallback-proxy",
		"hijack-dns",
		"sniff",
	} {
		if !strings.Contains(routeStr, needle) {
			t.Fatalf("route missing %q: %s", needle, routeStr)
		}
	}
	if strings.Contains(routeStr, "disabled-domain") {
		t.Fatal("disabled rule should not be emitted")
	}
}

func TestGenerateSingBoxConfigDarwinAndRuleSetFiltering(t *testing.T) {
	rulesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rulesDir, "geosite-cn.srs"), []byte("SRS_MOCK_BYTES"), 0o644); err != nil {
		t.Fatalf("write mock srs: %v", err)
	}

	orgs := []contract.Organization{{ID: "org-1", StateDir: "data/ts-state-org-1"}}
	nodes := []contract.ExitNode{{ID: "ts-1", OrgID: "org-1", TailscaleIP: "100.64.0.10", Online: true}}
	providers := []contract.RuleProvider{
		{Tag: "geosite-cn", URL: "https://example.com/geosite-cn.srs"},
		{Tag: "geosite-missing", URL: "https://example.com/missing.srs"},
	}
	rules := []contract.Rule{
		{ID: "r-present", Type: contract.RuleSet, Payload: "geosite-cn, geosite-missing", Action: contract.ActionDirect, Enabled: true},
		{ID: "r-absent", Type: contract.RuleSet, Payload: "geosite-missing", Action: contract.ActionReject, Enabled: true},
	}

	raw, err := GenerateSingBoxConfigWithOptions(orgs, nodes, "ts-1", contract.ModeRule, rules, providers, contract.SystemSettings{}, ConfigOptions{
		RulesDir:   rulesDir,
		TargetGOOS: "darwin",
	})
	if err != nil {
		t.Fatalf("GenerateSingBoxConfigWithOptions failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	tun := parsed["inbounds"].([]any)[0].(map[string]any)
	if _, hasInterfaceName := tun["interface_name"]; hasInterfaceName {
		t.Fatalf("darwin tun inbound must not specify interface_name, got %v", tun["interface_name"])
	}

	route := parsed["route"].(map[string]any)
	ruleSets := route["rule_set"].([]any)
	if len(ruleSets) != 1 {
		t.Fatalf("expected 1 existing local rule_set, got %d", len(ruleSets))
	}
	rs0 := ruleSets[0].(map[string]any)
	if rs0["tag"] != "geosite-cn" || rs0["type"] != "local" {
		t.Fatalf("unexpected local rule_set: %+v", rs0)
	}

	routeJSON, _ := json.Marshal(route["rules"])
	if strings.Contains(string(routeJSON), "geosite-missing") {
		t.Fatalf("missing rule_set tag leaked into route rules: %s", string(routeJSON))
	}
}

func TestGenerateSingBoxConfigModesAndDisabledFallback(t *testing.T) {
	orgs := []contract.Organization{{ID: "org-1", StateDir: "data/ts-state-org-1"}}
	nodes := []contract.ExitNode{{ID: "ts-1", OrgID: "org-1", TailscaleIP: "100.64.0.10", Online: true}}

	raw, err := GenerateSingBoxConfig(orgs, nodes, "ts-1", contract.ModeDirect, nil, nil, contract.SystemSettings{FallbackProxyEnabled: false})
	if err != nil {
		t.Fatalf("GenerateSingBoxConfig failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	outbounds := parsed["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("expected 2 outbounds when fallback disabled, got %d", len(outbounds))
	}
	route := parsed["route"].(map[string]any)
	if route["final"] != "direct" {
		t.Fatalf("expected direct final in ModeDirect, got %v", route["final"])
	}
}

func TestGenerateSingBoxConfigFinalRejectAndHTTPSFallbackTLS(t *testing.T) {
	orgs := []contract.Organization{{ID: "org-1", StateDir: "data/ts-state-org-1"}}
	nodes := []contract.ExitNode{{ID: "ts-1", OrgID: "org-1", TailscaleIP: "100.64.0.10", Online: true}}
	rules := []contract.Rule{
		{ID: "r-final", Type: contract.RuleFinal, Payload: "默认兜底", Action: contract.ActionReject, Enabled: true},
		{ID: "r-allow", Type: contract.RuleDomainSuffix, Payload: "example.com", Action: contract.ActionDirect, Enabled: true},
	}
	settings := contract.SystemSettings{
		FallbackProxyEnabled: true,
		FallbackProxyAddr:    "https://proxy.example.com:8443",
	}

	raw, err := GenerateSingBoxConfig(orgs, nodes, "ts-1", contract.ModeRule, rules, nil, settings)
	if err != nil {
		t.Fatalf("GenerateSingBoxConfig failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	outbounds := parsed["outbounds"].([]any)
	fb := outbounds[2].(map[string]any)
	if fb["type"] != "http" || fb["server"] != "proxy.example.com" || int(fb["server_port"].(float64)) != 8443 {
		t.Fatalf("unexpected https fallback-proxy outbound: %+v", fb)
	}
	tlsCfg, ok := fb["tls"].(map[string]any)
	if !ok || tlsCfg["enabled"] != true {
		t.Fatalf("expected tls.enabled=true for https fallback proxy, got %+v", fb["tls"])
	}

	route := parsed["route"].(map[string]any)
	routeRules := route["rules"].([]any)
	lastRule := routeRules[len(routeRules)-1].(map[string]any)
	if len(lastRule) != 1 || lastRule["action"] != "reject" {
		t.Fatalf("expected trailing catch-all reject rule for RuleFinal ActionReject, got %+v", lastRule)
	}
}

func TestFetchClashConnectionsParsingAndDeltaRate(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" {
			http.NotFound(w, r)
			return
		}
		callCount++
		dl := int64(10240)
		if callCount >= 2 {
			dl = 10240 + 20480
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"downloadTotal": 307200,
			"uploadTotal": 10240,
			"connections": [
				{
					"id": "conn-1",
					"metadata": {
						"network": "tcp",
						"destinationIP": "140.82.114.4",
						"destinationPort": "443",
						"host": "github.com",
						"processPath": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
					},
					"upload": 1024,
					"download": ` + jsonNumber(dl) + `,
					"start": "2026-11-27T10:00:00Z",
					"chains": ["默认出口", "ep-org-studio"],
					"rule": "domain_suffix",
					"rulePayload": "github.com"
				},
				{
					"id": "conn-2",
					"metadata": {
						"network": "udp",
						"destinationIP": "223.5.5.5",
						"destinationPort": 53,
						"host": "",
						"process": "curl"
					},
					"upload": 100,
					"download": 400,
					"start": "2026-11-27T10:00:01Z",
					"chains": ["direct"],
					"rule": "rule_set",
					"rulePayload": "geosite-cn"
				}
			]
		}`))
	}))
	defer srv.Close()

	sampler := NewClashSampler()
	snap1, err := sampler.Fetch(srv.URL)
	if err != nil {
		t.Fatalf("first Fetch failed: %v", err)
	}
	if snap1.DownloadTotal != 307200 || snap1.UploadTotal != 10240 || len(snap1.Connections) != 2 {
		t.Fatalf("unexpected first snapshot: %+v", snap1)
	}
	c0 := snap1.Connections[0]
	if c0.Process != "chrome.exe" || c0.Host != "github.com" || c0.Port != 443 || c0.Network != "TCP" {
		t.Fatalf("unexpected parsed conn-1: %+v", c0)
	}
	if c0.RouteAction != contract.ActionExit || c0.MatchedRule != "DOMAIN · github.com" {
		t.Fatalf("unexpected conn-1 routing: %+v", c0)
	}
	c1 := snap1.Connections[1]
	if c1.Host != "223.5.5.5" || c1.Port != 53 || c1.Network != "UDP" || c1.RouteAction != contract.ActionDirect || c1.MatchedRule != "geosite-cn" {
		t.Fatalf("unexpected parsed conn-2: %+v", c1)
	}

	time.Sleep(30 * time.Millisecond)
	snap2, err := sampler.Fetch(srv.URL)
	if err != nil {
		t.Fatalf("second Fetch failed: %v", err)
	}
	if snap2.Connections[0].DownKBps <= 0 {
		t.Fatalf("expected positive DownKBps delta on second poll, got %v", snap2.Connections[0].DownKBps)
	}
}

func jsonNumber(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

type blockingProber struct {
	started chan struct{}
	release chan struct{}
	ip      string
	err     error
}

func (b *blockingProber) ProbePublicIP(ctx context.Context, _ string, _ contract.ExitNode) (string, error) {
	if b.started != nil {
		close(b.started)
	}
	if b.release != nil {
		select {
		case <-b.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return b.ip, b.err
}

func TestSwitchExitSuccessAndVerifyingState(t *testing.T) {
	prober := &blockingProber{
		started: make(chan struct{}),
		release: make(chan struct{}),
		ip:      "43.153.82.19",
	}
	ctrl := NewController(prober, true, contract.ModeRule, "ts-us-macmini", false)
	allNodes := []contract.ExitNode{
		{ID: "ts-us-macmini", Label: "洛杉矶 Mac mini", PublicIP: "68.225.41.109", Online: true, Healthy: true},
		{ID: "ts-hk-cloud", Label: "香港轻量云", PublicIP: "43.153.82.19", Online: true, Healthy: true},
	}

	done := make(chan struct{})
	var gotID string
	var gotIP string
	var gotNotice *contract.BannerNotice
	var gotErr error
	go func() {
		defer close(done)
		gotID, gotIP, gotNotice, gotErr = ctrl.SwitchExitAndProbe(context.Background(), allNodes[1], allNodes, true, "127.0.0.1:7899")
	}()

	<-prober.started
	_, _, activeDuring, verifyingDuring, noticeDuring := ctrl.State()
	if activeDuring != "ts-us-macmini" || verifyingDuring != "ts-hk-cloud" {
		t.Fatalf("unexpected state during verification: active=%s verifying=%s", activeDuring, verifyingDuring)
	}
	if noticeDuring == nil || noticeDuring.Kind != "verifying" {
		t.Fatalf("expected verifying notice during probe, got %+v", noticeDuring)
	}

	close(prober.release)
	<-done

	if gotErr != nil || gotID != "ts-hk-cloud" || gotIP != "43.153.82.19" || gotNotice != nil {
		t.Fatalf("unexpected switch result: id=%s ip=%s notice=%+v err=%v", gotID, gotIP, gotNotice, gotErr)
	}

	_, _, activeAfter, verifyingAfter, noticeAfter := ctrl.State()
	if activeAfter != "ts-hk-cloud" || verifyingAfter != "" || noticeAfter != nil {
		t.Fatalf("unexpected state after switch: active=%s verifying=%s notice=%+v", activeAfter, verifyingAfter, noticeAfter)
	}
}

func TestSwitchExitTimeoutRollback(t *testing.T) {
	prober := NewDefaultProber(false)
	ctrl := NewController(prober, true, contract.ModeRule, "ts-hk-cloud", false)

	allNodes := []contract.ExitNode{
		{ID: "ts-hk-cloud", Label: "香港轻量云", PublicIP: "43.153.82.19", Online: true, Healthy: true},
		{ID: "ts-us-broken", Label: "圣何塞节点", PublicIP: "203.0.113.88", Online: true, Healthy: false},
	}

	activeID, notice, err := ctrl.SwitchExit(context.Background(), allNodes[1], allNodes, true, "127.0.0.1:7899")
	if err == nil {
		t.Fatal("expected error when switching to unhealthy node")
	}
	if activeID != "ts-hk-cloud" {
		t.Fatalf("expected rollback to ts-hk-cloud, got %s", activeID)
	}
	expectedText := "圣何塞节点 连通核验超时，已回滚至 香港轻量云 (43.153.82.19)"
	if notice == nil || notice.Kind != "rollback" || notice.Text != expectedText {
		t.Fatalf("unexpected rollback notice: %+v", notice)
	}

	ctrl.ClearBanner()
	_, _, _, _, cleared := ctrl.State()
	if cleared != nil {
		t.Fatalf("expected cleared banner, got %+v", cleared)
	}
}

func TestSwitchExitSkipVerifyAndContextTimeout(t *testing.T) {
	prober := &blockingProber{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     errors.New("unreachable"),
	}
	ctrl := NewController(prober, false, contract.ModeRule, "ts-hk-cloud", false)
	allNodes := []contract.ExitNode{
		{ID: "ts-hk-cloud", Label: "香港轻量云", PublicIP: "43.153.82.19", Online: true, Healthy: true},
		{ID: "ts-jp-tokyo", Label: "东京软路由", PublicIP: "172.104.92.77", Online: true, Healthy: true},
	}

	activeID, notice, err := ctrl.SwitchExit(context.Background(), allNodes[1], allNodes, false, "")
	if err != nil || activeID != "ts-jp-tokyo" || notice != nil {
		t.Fatalf("unexpected skip-verify result: id=%s notice=%+v err=%v", activeID, notice, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	rolledID, rbNotice, rbErr := ctrl.SwitchExit(ctx, allNodes[0], allNodes, true, "")
	if rbErr == nil || rolledID != "ts-jp-tokyo" || rbNotice == nil || rbNotice.Kind != "rollback" {
		t.Fatalf("expected timeout rollback, got id=%s notice=%+v err=%v", rolledID, rbNotice, rbErr)
	}
}

func TestConnectionsProductionEmptyVsDemoMode(t *testing.T) {
	prodCtrl := NewController(nil, false, contract.ModeRule, "", false)
	if got := prodCtrl.ListConnections(contract.ExitNode{}); len(got) != 0 {
		t.Fatalf("expected 0 connections in production mode, got %d", len(got))
	}

	demoCtrl := NewController(nil, true, contract.ModeRule, "ts-us-macmini", true)
	conns := demoCtrl.ListConnections(contract.ExitNode{ID: "ts-hk-cloud", LatencyMs: 34})

	seen := make(map[string]contract.LiveConnection, len(conns))
	for _, c := range conns {
		seen[c.Process] = c
	}
	for _, proc := range []string{"Cursor.exe", "chrome.exe", "YuanShen.exe", "Code.exe", "verge-mihomo.exe"} {
		if _, ok := seen[proc]; !ok {
			t.Fatalf("missing expected connection for %s", proc)
		}
	}
	mihomo := seen["verge-mihomo.exe"]
	if mihomo.MatchedRule != "PROCESS · verge-mihomo.exe" || mihomo.RouteAction != contract.ActionDirect {
		t.Fatalf("unexpected verge-mihomo.exe connection: %+v", mihomo)
	}

	demoCtrl.ApplyQuickRuleToConn("c-101", "DOMAIN · api.anthropic.com", contract.ActionDirect)
	updated := demoCtrl.ListConnections(contract.ExitNode{})
	for _, c := range updated {
		if c.ID == "c-101" {
			if c.MatchedRule != "DOMAIN · api.anthropic.com" || c.RouteAction != contract.ActionDirect {
				t.Fatalf("quick rule not applied: %+v", c)
			}
			return
		}
	}
	t.Fatal("c-101 not found")
}

func TestControllerConcurrency(t *testing.T) {
	ctrl := NewController(NewDefaultProber(false), true, contract.ModeRule, "ts-hk-cloud", true)
	nodes := []contract.ExitNode{
		{ID: "ts-hk-cloud", Label: "香港轻量云", PublicIP: "43.153.82.19", LatencyMs: 34, Online: true, Healthy: true},
		{ID: "ts-jp-tokyo", Label: "东京软路由", PublicIP: "172.104.92.77", LatencyMs: 62, Online: true, Healthy: true},
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctrl.SetTun(idx%2 == 0)
			ctrl.SetMode(contract.ModeRule)
			_, _, _, _, _ = ctrl.State()
			_ = ctrl.ListConnections(nodes[idx%2])
			ctrl.ApplyQuickRuleToConn("c-102", "FINAL", contract.ActionExit)
			_, _, _ = ctrl.SwitchExit(context.Background(), nodes[idx%2], nodes, true, "127.0.0.1:7899")
		}(i)
	}
	wg.Wait()
}

func containsString(items []any, target string) bool {
	for _, item := range items {
		if s, ok := item.(string); ok && s == target {
			return true
		}
	}
	return false
}
