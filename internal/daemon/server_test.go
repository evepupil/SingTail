package daemon

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/singtail/singtail/internal/contract"
)

func doReq(t *testing.T, handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request %s %s: %v", method, target, err)
		}
		rBody = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, target, rBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeSnapshot(t *testing.T, rec *httptest.ResponseRecorder) contract.DashboardSnapshot {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}
	var snap contract.DashboardSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode snapshot: %v (body: %s)", err, rec.Body.String())
	}
	return snap
}

func TestDaemonSnapshotPeriodsAndPersistence(t *testing.T) {
	dataDir := t.TempDir()
	srv, err := NewServer(dataDir, nil, false)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	periods := []contract.TrafficPeriodKey{
		contract.PeriodLive,
		contract.PeriodDay,
		contract.PeriodWeek,
		contract.PeriodMonth30,
		contract.PeriodMonthly,
	}
	for _, p := range periods {
		rec := doReq(t, srv, http.MethodGet, "/api/snapshot?period="+string(p), nil)
		snap := decodeSnapshot(t, rec)
		if snap.Traffic.Period != p {
			t.Fatalf("expected period %s, got %s", p, snap.Traffic.Period)
		}
		if snap.Traffic.TotalGB <= 0 {
			t.Fatalf("expected seeded positive traffic for period %s, got %v", p, snap.Traffic.TotalGB)
		}
		if len(snap.Orgs) < 2 || len(snap.ExitNodes) < 5 {
			t.Fatalf("expected seeded orgs/nodes, got orgs=%d nodes=%d", len(snap.Orgs), len(snap.ExitNodes))
		}
	}

	badPeriod := doReq(t, srv, http.MethodGet, "/api/snapshot?period=invalid", nil)
	if badPeriod.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid period, got %d", badPeriod.Code)
	}

	snap := decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/tun", contract.SetTunRequest{Enabled: false}))
	if snap.TunEnabled {
		t.Fatal("expected tunEnabled to be false")
	}

	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/mode", contract.SetModeRequest{Mode: contract.ModeGlobal}))
	if snap.RoutingMode != contract.ModeGlobal {
		t.Fatalf("expected routingMode global, got %s", snap.RoutingMode)
	}

	nextSettings := snap.Settings
	nextSettings.AutoVerifyIP = false
	nextSettings.FallbackProxyAddr = "127.0.0.1:7898"
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPut, "/api/settings", nextSettings))
	if snap.Settings.AutoVerifyIP || snap.Settings.FallbackProxyAddr != "127.0.0.1:7898" {
		t.Fatalf("settings update not reflected: %+v", snap.Settings)
	}

	if err := srv.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Re-open on same dataDir and verify persistence.
	reopened, err := NewServer(dataDir, nil, false)
	if err != nil {
		t.Fatalf("reopen NewServer failed: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	restored := decodeSnapshot(t, doReq(t, reopened, http.MethodGet, "/api/snapshot", nil))
	if restored.TunEnabled {
		t.Fatal("expected persisted tunEnabled=false")
	}
	if restored.RoutingMode != contract.ModeGlobal {
		t.Fatalf("expected persisted mode=global, got %s", restored.RoutingMode)
	}
	if restored.Settings.AutoVerifyIP || restored.Settings.FallbackProxyAddr != "127.0.0.1:7898" {
		t.Fatalf("expected persisted settings, got %+v", restored.Settings)
	}
}

func TestDaemonExitSwitchRollbackPingAndBanner(t *testing.T) {
	srv, err := NewServer(t.TempDir(), nil, false)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer func() { _ = srv.Close() }()

	// Switch to healthy Hong Kong node.
	snap := decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/exit/switch", contract.SwitchExitRequest{
		ExitID: "ts-hk-cloud",
	}))
	if snap.ActiveExitID != "ts-hk-cloud" {
		t.Fatalf("expected activeExitId ts-hk-cloud, got %s", snap.ActiveExitID)
	}
	if snap.BannerNotice != nil {
		t.Fatalf("expected no banner on healthy switch, got %+v", snap.BannerNotice)
	}

	// Switch to unhealthy node -> must return 200 with rollback banner and restore ts-hk-cloud.
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/exit/switch", contract.SwitchExitRequest{
		ExitID: "ts-us-broken",
	}))
	if snap.ActiveExitID != "ts-hk-cloud" {
		t.Fatalf("expected rollback to ts-hk-cloud, got %s", snap.ActiveExitID)
	}
	if snap.BannerNotice == nil || snap.BannerNotice.Kind != "rollback" {
		t.Fatalf("expected rollback banner, got %+v", snap.BannerNotice)
	}

	// Dismiss banner.
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/banner/dismiss", map[string]any{}))
	if snap.BannerNotice != nil {
		t.Fatalf("expected banner cleared, got %+v", snap.BannerNotice)
	}

	// Ping nodes.
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/exit/ping", map[string]any{}))
	if len(snap.ExitNodes) == 0 {
		t.Fatal("expected exit nodes after ping")
	}
}

func TestDaemonRulesQuickRuleProvidersOrgsAndExport(t *testing.T) {
	dataDir := t.TempDir()
	srv, err := NewServer(dataDir, nil, false)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer func() { _ = srv.Close() }()

	initial := decodeSnapshot(t, doReq(t, srv, http.MethodGet, "/api/snapshot", nil))
	initialRuleCount := len(initial.Rules)

	// Add rule.
	snap := decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/rules", contract.AddRuleRequest{
		Type:    contract.RuleDomainSuffix,
		Payload: "example.org",
		Action:  contract.ActionDirect,
	}))
	if len(snap.Rules) != initialRuleCount+1 {
		t.Fatalf("expected %d rules, got %d", initialRuleCount+1, len(snap.Rules))
	}
	addedRule := snap.Rules[2]
	if addedRule.Payload != "example.org" || !addedRule.Enabled {
		t.Fatalf("unexpected added rule at index 2: %+v", addedRule)
	}

	// Toggle rule.
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/rules/toggle", contract.ToggleRuleRequest{
		RuleID: addedRule.ID,
	}))
	if snap.Rules[2].Enabled {
		t.Fatal("expected added rule to be disabled after toggle")
	}

	// Move rule down (+1) then back up (-1).
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/rules/move", contract.MoveRuleRequest{
		RuleID:    addedRule.ID,
		Direction: 1,
	}))
	if snap.Rules[3].ID != addedRule.ID {
		t.Fatalf("expected rule moved to index 3, got %s", snap.Rules[3].ID)
	}
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/rules/move", contract.MoveRuleRequest{
		RuleID:    addedRule.ID,
		Direction: -1,
	}))
	if snap.Rules[2].ID != addedRule.ID {
		t.Fatalf("expected rule moved back to index 2, got %s", snap.Rules[2].ID)
	}

	// Locked built-in rule cannot be deleted.
	delLocked := doReq(t, srv, http.MethodDelete, "/api/rules?id=builtin-head-1", nil)
	if delLocked.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 deleting locked rule, got %d", delLocked.Code)
	}

	// Delete custom rule.
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodDelete, "/api/rules?id="+addedRule.ID, nil))
	if len(snap.Rules) != initialRuleCount {
		t.Fatalf("expected %d rules after delete, got %d", initialRuleCount, len(snap.Rules))
	}

	// Import rules.
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/rules/import", contract.ImportRulesRequest{
		RawText: "rules:\n  - DOMAIN-SUFFIX,claude.ai,PROXY\n  - PROCESS-NAME,slack.exe,DIRECT\n",
	}))
	if len(snap.Rules) != initialRuleCount+2 {
		t.Fatalf("expected %d rules after import, got %d", initialRuleCount+2, len(snap.Rules))
	}

	// Quick rule from live connection.
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/connections/quick-rule", contract.QuickRouteRequest{
		ConnID:    "c-101",
		MatchKind: "domain",
		Action:    contract.ActionDirect,
	}))
	var c101 contract.LiveConnection
	for _, c := range snap.Connections {
		if c.ID == "c-101" {
			c101 = c
			break
		}
	}
	if c101.RouteAction != contract.ActionDirect || !strings.Contains(c101.MatchedRule, "api.anthropic.com") {
		t.Fatalf("quick rule did not update connection c-101: %+v", c101)
	}

	// Add & refresh provider.
	provCount := len(snap.Providers)
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/providers", contract.AddProviderRequest{
		Tag:        "geosite-custom",
		SourceName: "Local",
		URL:        "https://example.com/custom.srs",
	}))
	if len(snap.Providers) != provCount+1 {
		t.Fatalf("expected %d providers, got %d", provCount+1, len(snap.Providers))
	}
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/providers/refresh", map[string]any{}))
	if len(snap.Providers) != provCount+1 {
		t.Fatalf("unexpected provider count after refresh: %d", len(snap.Providers))
	}

	// Add & sync org.
	orgCount := len(snap.Orgs)
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/orgs", contract.AddOrgRequest{
		Name:         "Lab Tailnet",
		Account:      "lab@example.com",
		NodeHostname: "lab-exit-01",
		PublicIP:     "198.51.100.42",
		Region:       "JP · Osaka",
	}))
	if len(snap.Orgs) != orgCount+1 {
		t.Fatalf("expected %d orgs, got %d", orgCount+1, len(snap.Orgs))
	}
	newOrgID := snap.Orgs[len(snap.Orgs)-1].ID
	snap = decodeSnapshot(t, doReq(t, srv, http.MethodPost, "/api/orgs/sync", map[string]string{"orgId": newOrgID}))
	if snap.Orgs[len(snap.Orgs)-1].LastSync != "刚刚" {
		t.Fatalf("expected LastSync 刚刚, got %s", snap.Orgs[len(snap.Orgs)-1].LastSync)
	}

	// Sanitized export must exclude locked rules and org state.
	exportRec := doReq(t, srv, http.MethodGet, "/api/export", nil)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on export, got %d", exportRec.Code)
	}
	var exported contract.SanitizedExport
	if err := json.Unmarshal(exportRec.Body.Bytes(), &exported); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if len(exported.CustomRules) == 0 || len(exported.Providers) != len(snap.Providers) {
		t.Fatalf("unexpected sanitized export counts: rules=%d providers=%d", len(exported.CustomRules), len(exported.Providers))
	}
	rawExport := exportRec.Body.String()
	for _, secret := range []string{"ts-state", "alex.dev@gmail.com", "ops@golfjoy.io", "builtin-head-1"} {
		if strings.Contains(rawExport, secret) {
			t.Fatalf("sanitized export leaked %q: %s", secret, rawExport)
		}
	}
}

func TestDaemonStaticSPAAndOriginProtection(t *testing.T) {
	staticFS := fstest.MapFS{
		"index.html":            {Data: []byte("<!doctype html><html><body>SingTail SPA</body></html>")},
		"assets/app.js":         {Data: []byte("console.log('singtail');")},
		"assets/styles/app.css": {Data: []byte("body{color:#fff}")},
	}

	srv, err := NewServer(t.TempDir(), staticFS, false)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer func() { _ = srv.Close() }()

	// Root serves index.html.
	rootRec := doReq(t, srv, http.MethodGet, "/", nil)
	if rootRec.Code != http.StatusOK || !strings.Contains(rootRec.Body.String(), "SingTail SPA") {
		t.Fatalf("unexpected root response: %d %s", rootRec.Code, rootRec.Body.String())
	}

	// Existing static asset serves directly.
	jsRec := doReq(t, srv, http.MethodGet, "/assets/app.js", nil)
	if jsRec.Code != http.StatusOK || !strings.Contains(jsRec.Body.String(), "console.log") {
		t.Fatalf("unexpected static js response: %d %s", jsRec.Code, jsRec.Body.String())
	}

	// Unknown non-API route falls back to SPA index.html.
	spaRec := doReq(t, srv, http.MethodGet, "/rules", nil)
	if spaRec.Code != http.StatusOK || !strings.Contains(spaRec.Body.String(), "SingTail SPA") {
		t.Fatalf("expected SPA fallback for /rules, got %d %s", spaRec.Code, spaRec.Body.String())
	}

	// Unknown /api/ route returns JSON 404 without SPA fallback.
	api404 := doReq(t, srv, http.MethodGet, "/api/unknown", nil)
	if api404.Code != http.StatusNotFound || !strings.Contains(api404.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected JSON 404 for unknown API, got %d %s", api404.Code, api404.Body.String())
	}

	// Cross-origin request from external site or sandboxed null origin is blocked.
	for _, blockedOrigin := range []string{"https://evil.example", "null"} {
		evilReq := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
		evilReq.Header.Set("Origin", blockedOrigin)
		evilRec := httptest.NewRecorder()
		srv.ServeHTTP(evilRec, evilReq)
		if evilRec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for origin %q, got %d", blockedOrigin, evilRec.Code)
		}
	}

	// POST/PUT with request body requires application/json Content-Type.
	plainPost := httptest.NewRequest(http.MethodPost, "/api/tun", strings.NewReader(`{"enabled":false}`))
	plainPost.Header.Set("Content-Type", "text/plain")
	plainRec := httptest.NewRecorder()
	srv.ServeHTTP(plainRec, plainPost)
	if plainRec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for text/plain POST body, got %d", plainRec.Code)
	}

	plainPut := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{}`))
	plainPutRec := httptest.NewRecorder()
	srv.ServeHTTP(plainPutRec, plainPut)
	if plainPutRec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for PUT body without application/json Content-Type, got %d", plainPutRec.Code)
	}

	// Loopback Vite dev origin is allowed.
	devReq := httptest.NewRequest(http.MethodGet, "/api/snapshot", nil)
	devReq.Header.Set("Origin", "http://localhost:5173")
	devRec := httptest.NewRecorder()
	srv.ServeHTTP(devRec, devReq)
	if devRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for localhost origin, got %d", devRec.Code)
	}
}

func TestDaemonConcurrency(t *testing.T) {
	srv, err := NewServer(t.TempDir(), nil, false)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	defer func() { _ = srv.Close() }()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = doReq(t, srv, http.MethodGet, "/api/snapshot", nil)
			_ = doReq(t, srv, http.MethodPost, "/api/tun", contract.SetTunRequest{Enabled: idx%2 == 0})
			_ = doReq(t, srv, http.MethodPost, "/api/exit/ping", map[string]any{})
		}(i)
	}
	wg.Wait()
}
