package configstore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/singtail/singtail/internal/configstore"
	"github.com/singtail/singtail/internal/contract"
)

func TestLoadOrDefaultWhenMissing(t *testing.T) {
	stPath := filepath.Join(t.TempDir(), "missing", "settings.json")
	store := configstore.New(stPath)

	got, err := store.LoadOrDefault()
	if err != nil {
		t.Fatalf("LoadOrDefault failed: %v", err)
	}
	if got.TunEnabled {
		t.Fatalf("expected default TunEnabled=false")
	}
	if got.RoutingMode != contract.ModeRule {
		t.Fatalf("expected default RoutingMode=%q, got %q", contract.ModeRule, got.RoutingMode)
	}
	if got.Settings.FallbackProxyEnabled || got.Settings.FallbackProxyAddr != "127.0.0.1:7897" {
		t.Fatalf("unexpected fallback defaults: %+v", got.Settings)
	}
	if !got.Settings.AutoStartService || !got.Settings.AutoVerifyIP || got.Settings.AutoUpdateRulesDays != 14 {
		t.Fatalf("unexpected system settings defaults: %+v", got.Settings)
	}
	if got.Orgs == nil || got.ExitNodes == nil || got.CustomRules == nil || got.Providers == nil {
		t.Fatalf("expected non-nil empty slices in default state")
	}
	if len(got.Orgs) != 0 || len(got.ExitNodes) != 0 {
		t.Fatalf("expected empty Orgs and ExitNodes in default state, got orgs=%d nodes=%d", len(got.Orgs), len(got.ExitNodes))
	}
}

func TestSaveAndLoadAtomic(t *testing.T) {
	dir := t.TempDir()
	stPath := filepath.Join(dir, "nested", "cfg", "settings.json")
	store := configstore.New(stPath)

	want := configstore.PersistedState{
		TunEnabled:   false,
		RoutingMode:  contract.ModeGlobal,
		ActiveExitID: "ts-hk-cloud",
		Orgs: []contract.Organization{
			{
				ID:               "org-work",
				Name:             "Acme Corp",
				Account:          "alice@acme.io",
				LocalTailscaleIP: "100.64.0.12",
				StateDir:         `C:\ProgramData\SingTail\ts-state\org-work`,
				LastSync:         "2026-11-27T12:00:00Z",
			},
		},
		ExitNodes: []contract.ExitNode{
			{
				ID:          "ts-hk-cloud",
				OrgID:       "org-work",
				OrgName:     "Acme Corp",
				Hostname:    "hk-cloud",
				Label:       "Hong Kong · hk-cloud",
				TailscaleIP: "100.64.0.99",
				PublicIP:    "43.153.82.19",
				Region:      "HK · Hong Kong",
				RelayMode:   "直连",
				LatencyMs:   35,
				Online:      true,
				Healthy:     true,
			},
		},
		CustomRules: []contract.Rule{
			{
				ID:      "r-custom-1",
				Type:    contract.RuleDomainSuffix,
				Payload: "openai.com",
				Action:  contract.ActionExit,
				Hits:    42,
				Enabled: true,
				Locked:  false,
			},
		},
		Providers: []contract.RuleProvider{
			{
				ID:         "p-1",
				Tag:        "geosite-cn",
				SourceName: "SagerNet",
				URL:        "https://example.com/geosite-cn.srs",
				EntryCount: 1200,
				UpdatedAt:  "2026-11-27T10:00:00Z",
			},
		},
		Settings: contract.SystemSettings{
			AutoStartService:     false,
			AutoVerifyIP:         true,
			FallbackProxyEnabled: true,
			FallbackProxyAddr:    "127.0.0.1:7890",
			AutoUpdateRulesDays:  7,
		},
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(stPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed after atomic rename, stat err=%v", err)
	}

	got, err := store.LoadOrDefault()
	if err != nil {
		t.Fatalf("LoadOrDefault after Save failed: %v", err)
	}
	if got.TunEnabled != want.TunEnabled || got.RoutingMode != want.RoutingMode || got.ActiveExitID != want.ActiveExitID {
		t.Fatalf("state header mismatch: got %+v, want %+v", got, want)
	}
	if len(got.Orgs) != 1 || got.Orgs[0].StateDir != want.Orgs[0].StateDir {
		t.Fatalf("orgs mismatch: got %+v", got.Orgs)
	}
	if len(got.ExitNodes) != 1 || got.ExitNodes[0] != want.ExitNodes[0] {
		t.Fatalf("exitNodes mismatch: got %+v, want %+v", got.ExitNodes, want.ExitNodes)
	}
	if len(got.CustomRules) != 1 || got.CustomRules[0].Payload != "openai.com" {
		t.Fatalf("rules mismatch: got %+v", got.CustomRules)
	}
	if len(got.Providers) != 1 || got.Providers[0].Tag != "geosite-cn" {
		t.Fatalf("providers mismatch: got %+v", got.Providers)
	}
	if got.Settings != want.Settings {
		t.Fatalf("settings mismatch: got %+v, want %+v", got.Settings, want.Settings)
	}
}

func TestBuildSanitizedExport(t *testing.T) {
	rules := []contract.Rule{
		{
			ID:      "r-locked-1",
			Type:    contract.RuleProcessName,
			Payload: "tailscaled.exe",
			Action:  contract.ActionDirect,
			Hits:    999,
			Enabled: true,
			Locked:  true,
		},
		{
			ID:      "r-locked-2",
			Type:    contract.RuleIPCIDR,
			Payload: "100.64.0.0/10",
			Action:  contract.ActionDirect,
			Hits:    120,
			Enabled: true,
			Locked:  true,
		},
		{
			ID:      "r-user-1",
			Type:    contract.RuleDomainSuffix,
			Payload: "anthropic.com",
			Action:  contract.ActionExit,
			Hits:    88,
			Enabled: true,
			Locked:  false,
		},
	}
	providers := []contract.RuleProvider{
		{
			ID:         "prov-1",
			Tag:        "geoip-cn",
			SourceName: "Official",
			URL:        "https://example.com/geoip-cn.srs",
			EntryCount: 5000,
			UpdatedAt:  "2026-11-27T08:00:00Z",
		},
	}
	settings := contract.SystemSettings{
		AutoStartService:     true,
		AutoVerifyIP:         true,
		FallbackProxyEnabled: true,
		FallbackProxyAddr:    "127.0.0.1:7897",
		AutoUpdateRulesDays:  14,
	}

	now := time.Date(2026, 11, 27, 12, 30, 0, 0, time.UTC)
	exported := configstore.BuildSanitizedExport(rules, providers, settings, now)

	if len(exported.CustomRules) != 1 {
		t.Fatalf("expected 1 custom rule (locked rules filtered out), got %d", len(exported.CustomRules))
	}
	if exported.CustomRules[0].Payload != "anthropic.com" {
		t.Fatalf("unexpected exported rule: %+v", exported.CustomRules[0])
	}
	if len(exported.Providers) != 1 || exported.Providers[0].Tag != "geoip-cn" {
		t.Fatalf("unexpected exported providers: %+v", exported.Providers)
	}
	if exported.ExportedAt != "2026-11-27T12:30:00Z" {
		t.Fatalf("unexpected ExportedAt: %q", exported.ExportedAt)
	}

	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	jsonText := string(raw)

	forbidden := []string{
		"stateDir",
		"ts-state",
		"orgs",
		"tailscaled.exe",
		"100.64.0.0/10",
		"locked",
		"hits",
		"entryCount",
		"authKey",
	}
	for _, needle := range forbidden {
		if strings.Contains(jsonText, needle) {
			t.Fatalf("sanitized export JSON unexpectedly contains %q: %s", needle, jsonText)
		}
	}
}

func TestConcurrentSaveAndLoad(t *testing.T) {
	store := configstore.New(filepath.Join(t.TempDir(), "concurrent.json"))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				st := configstore.PersistedState{
					TunEnabled:   (n+j)%2 == 0,
					RoutingMode:  contract.ModeRule,
					ActiveExitID: "ts-us",
				}
				if err := store.Save(st); err != nil {
					t.Errorf("concurrent Save failed: %v", err)
				}
				if _, err := store.LoadOrDefault(); err != nil {
					t.Errorf("concurrent LoadOrDefault failed: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
}
