package rules

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/singtail/singtail/internal/contract"
)

func TestNewManagerDefaults(t *testing.T) {
	mgr := NewManager(nil, nil)

	all := mgr.ListRules()
	custom := mgr.CustomRules()

	if len(all) != len(custom)+4 {
		t.Fatalf("expected 4 locked rules + %d custom rules, got %d total", len(custom), len(all))
	}
	if len(custom) != 7 {
		t.Fatalf("expected 7 preset custom rules, got %d", len(custom))
	}

	// Verify head locked rules.
	wantHead1 := "tailscaled.exe, tailscale-ipn.exe, verge-mihomo.exe, clash-verge.exe, Tailscale, IPNExtension, tailscaled, verge-mihomo, clash-verge, mihomo"
	if !all[0].Locked || all[0].Type != contract.RuleProcessName || all[0].Action != contract.ActionDirect ||
		all[0].Payload != wantHead1 {
		t.Fatalf("unexpected head locked rule[0]: %+v", all[0])
	}
	for _, r := range all {
		if r.Hits != 0 {
			t.Fatalf("expected initial rule %s Hits=0, got %d", r.ID, r.Hits)
		}
	}
	if !all[1].Locked || all[1].Type != contract.RuleIPCIDR || all[1].Action != contract.ActionDirect ||
		!strings.Contains(all[1].Payload, "100.64.0.0/10") ||
		!strings.Contains(all[1].Payload, "192.168.0.0/16") ||
		!strings.Contains(all[1].Payload, "10.0.0.0/8") {
		t.Fatalf("unexpected head locked rule[1]: %+v", all[1])
	}

	// Verify tail locked rules.
	n := len(all)
	if !all[n-2].Locked || all[n-2].Type != contract.RuleSet || all[n-2].Action != contract.ActionDirect ||
		all[n-2].Payload != "geosite-cn, geoip-cn" {
		t.Fatalf("unexpected tail locked rule[n-2]: %+v", all[n-2])
	}
	if !all[n-1].Locked || all[n-1].Type != contract.RuleFinal || all[n-1].Action != contract.ActionExit ||
		all[n-1].Payload != "默认兜底" {
		t.Fatalf("unexpected tail locked rule[n-1]: %+v", all[n-1])
	}

	// Verify default providers.
	provs := mgr.ListProviders()
	if len(provs) != 7 {
		t.Fatalf("expected 7 default providers, got %d", len(provs))
	}
	wantCounts := map[string]int{
		"geoip-cn":                 9420,
		"geosite-cn":               68510,
		"geosite-geolocation-!cn":  36920,
		"geosite-openai":           -1,
		"geosite-google":           -1,
		"geosite-github":           -1,
		"geosite-category-ads-all": -1,
	}
	for _, p := range provs {
		expected, ok := wantCounts[p.Tag]
		if !ok {
			t.Fatalf("unexpected default provider tag: %s", p.Tag)
		}
		if expected > 0 && p.EntryCount != expected {
			t.Fatalf("provider %s entryCount = %d, want %d", p.Tag, p.EntryCount, expected)
		}
		delete(wantCounts, p.Tag)
	}
	if len(wantCounts) != 0 {
		t.Fatalf("missing default providers: %v", wantCounts)
	}
}

func TestNewManagerEmptySlicesAndDefensiveCopy(t *testing.T) {
	mgr := NewManager([]contract.Rule{}, []contract.RuleProvider{})
	if len(mgr.CustomRules()) != 0 {
		t.Fatalf("expected 0 custom rules when non-nil empty slice passed, got %d", len(mgr.CustomRules()))
	}
	if len(mgr.ListRules()) != 4 {
		t.Fatalf("expected 4 locked rules, got %d", len(mgr.ListRules()))
	}
	if len(mgr.ListProviders()) != 0 {
		t.Fatalf("expected 0 providers when non-nil empty slice passed, got %d", len(mgr.ListProviders()))
	}

	rulesCopy := mgr.ListRules()
	rulesCopy[0].Payload = "mutated"
	if mgr.ListRules()[0].Payload == "mutated" {
		t.Fatal("ListRules did not return a defensive copy")
	}
}

func TestRuleCRUDAndBoundaries(t *testing.T) {
	mgr := NewManager([]contract.Rule{}, []contract.RuleProvider{})

	if _, err := mgr.AddRule(contract.AddRuleRequest{
		Type:    contract.RuleDomainSuffix,
		Payload: "   ",
		Action:  contract.ActionExit,
	}); err == nil {
		t.Fatal("expected error for empty payload")
	}

	if _, err := mgr.AddRule(contract.AddRuleRequest{
		Type:    contract.RuleFinal,
		Payload: "anything",
		Action:  contract.ActionExit,
	}); err == nil {
		t.Fatal("expected error for FINAL user rule type")
	}

	if _, err := mgr.AddRule(contract.AddRuleRequest{
		Type:    contract.RuleDomainSuffix,
		Payload: "example.com",
		Action:  contract.RuleAction("INVALID"),
	}); err == nil {
		t.Fatal("expected error for invalid action")
	}

	r1, err := mgr.AddRule(contract.AddRuleRequest{
		Type:    contract.RuleDomainSuffix,
		Payload: "first.com",
		Action:  contract.ActionExit,
	})
	if err != nil {
		t.Fatalf("AddRule r1 failed: %v", err)
	}
	r2, err := mgr.AddRule(contract.AddRuleRequest{
		Type:    contract.RuleProcessName,
		Payload: "second.exe",
		Action:  contract.ActionDirect,
	})
	if err != nil {
		t.Fatalf("AddRule r2 failed: %v", err)
	}
	if r1.ID == "" || r2.ID == "" || r1.ID == r2.ID {
		t.Fatalf("expected distinct non-empty IDs, got %q and %q", r1.ID, r2.ID)
	}

	// r2 should be inserted at index 2 (top of custom zone).
	all := mgr.ListRules()
	if all[2].ID != r2.ID || all[3].ID != r1.ID {
		t.Fatalf("expected r2 at index 2 and r1 at index 3, got %s and %s", all[2].ID, all[3].ID)
	}

	// Boundary moves into locked zones must fail.
	if err := mgr.MoveRule(r2.ID, -1); !errors.Is(err, ErrRuleOutOfBounds) {
		t.Fatalf("expected ErrRuleOutOfBounds moving top custom rule up, got %v", err)
	}
	if err := mgr.MoveRule(r1.ID, +1); !errors.Is(err, ErrRuleOutOfBounds) {
		t.Fatalf("expected ErrRuleOutOfBounds moving bottom custom rule down, got %v", err)
	}
	if err := mgr.MoveRule(r1.ID, 0); !errors.Is(err, ErrInvalidDirection) {
		t.Fatalf("expected ErrInvalidDirection for direction 0, got %v", err)
	}

	// Valid move swaps r2 and r1.
	if err := mgr.MoveRule(r2.ID, +1); err != nil {
		t.Fatalf("MoveRule down failed: %v", err)
	}
	custom := mgr.CustomRules()
	if custom[0].ID != r1.ID || custom[1].ID != r2.ID {
		t.Fatalf("unexpected custom order after move: %+v", custom)
	}

	// Toggle custom rule.
	if err := mgr.ToggleRule(r1.ID); err != nil {
		t.Fatalf("ToggleRule failed: %v", err)
	}
	if mgr.CustomRules()[0].Enabled {
		t.Fatal("expected r1 Enabled to become false after toggle")
	}

	// Locked rules cannot be toggled, moved, or deleted.
	lockedIDs := []string{all[0].ID, all[1].ID, all[len(all)-2].ID, all[len(all)-1].ID}
	for _, id := range lockedIDs {
		if err := mgr.ToggleRule(id); !errors.Is(err, ErrRuleLocked) {
			t.Fatalf("ToggleRule(%s) = %v, want ErrRuleLocked", id, err)
		}
		if err := mgr.MoveRule(id, -1); !errors.Is(err, ErrRuleLocked) {
			t.Fatalf("MoveRule(%s) = %v, want ErrRuleLocked", id, err)
		}
		if err := mgr.DeleteRule(id); !errors.Is(err, ErrRuleLocked) {
			t.Fatalf("DeleteRule(%s) = %v, want ErrRuleLocked", id, err)
		}
	}

	// Unknown IDs return ErrRuleNotFound.
	if err := mgr.DeleteRule("non-existent"); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("DeleteRule missing = %v, want ErrRuleNotFound", err)
	}

	// Delete custom rule.
	if err := mgr.DeleteRule(r1.ID); err != nil {
		t.Fatalf("DeleteRule(%s) failed: %v", r1.ID, err)
	}
	if len(mgr.CustomRules()) != 1 || mgr.CustomRules()[0].ID != r2.ID {
		t.Fatalf("unexpected custom rules after delete: %+v", mgr.CustomRules())
	}
}

func TestImportTextFormats(t *testing.T) {
	t.Run("JSON array", func(t *testing.T) {
		mgr := NewManager([]contract.Rule{}, []contract.RuleProvider{})
		count, err := mgr.ImportText(`[{"type":"DOMAIN-SUFFIX","payload":"a.com","action":"EXIT"},{"type":"DOMAIN-KEYWORD","payload":"b","action":"PROXY"}]`)
		if err != nil {
			t.Fatalf("ImportText JSON failed: %v", err)
		}
		if count != 2 {
			t.Fatalf("imported count = %d, want 2", count)
		}
		custom := mgr.CustomRules()
		if len(custom) != 2 || !custom[0].Enabled || custom[1].Action != contract.ActionExit {
			t.Fatalf("unexpected imported rules: %+v", custom)
		}
	})

	t.Run("Clash YAML and comma-separated mixed with comments", func(t *testing.T) {
		mgr := NewManager([]contract.Rule{
			{ID: "existing-1", Type: contract.RuleDomainSuffix, Payload: "old.com", Action: contract.ActionDirect, Enabled: true},
		}, []contract.RuleProvider{})

		raw := `
# Header comment
// Double slash comment
rules:
  - DOMAIN-SUFFIX,anthropic.com,PROXY
  - 'IP-CIDR,1.2.3.0/24,DIRECT,no-resolve'
PROCESS-NAME,steam.exe,DIRECT
RULE-SET,geosite-ads,REJECT
DOMAIN-KEYWORD,stream,FALLBACK_PROXY
[{"type":"DOMAIN-SUFFIX","payload":"mix.dev","action":"EXIT"}]
INVALID-TYPE,foo,DIRECT
`
		count, err := mgr.ImportText(raw)
		if err != nil {
			t.Fatalf("ImportText mixed failed: %v", err)
		}
		if count != 6 {
			t.Fatalf("imported count = %d, want 6", count)
		}

		all := mgr.ListRules()
		// Inserted at index 2..7, pushing existing-1 to index 8.
		if all[2].Payload != "anthropic.com" || all[2].Action != contract.ActionExit {
			t.Fatalf("unexpected rule at index 2: %+v", all[2])
		}
		if all[3].Payload != "1.2.3.0/24" || all[3].Action != contract.ActionDirect {
			t.Fatalf("unexpected rule at index 3: %+v", all[3])
		}
		if all[4].Type != contract.RuleProcessName || all[4].Payload != "steam.exe" {
			t.Fatalf("unexpected rule at index 4: %+v", all[4])
		}
		if all[5].Action != contract.ActionReject || all[6].Action != contract.ActionFallbackProxy {
			t.Fatalf("unexpected actions at index 5/6: %+v / %+v", all[5], all[6])
		}
		if all[7].Payload != "mix.dev" {
			t.Fatalf("unexpected rule at index 7: %+v", all[7])
		}
		if all[8].ID != "existing-1" {
			t.Fatalf("expected existing-1 pushed to index 8, got %+v", all[8])
		}
	})

	t.Run("Quoted lines with trailing inline comments", func(t *testing.T) {
		mgr := NewManager([]contract.Rule{}, []contract.RuleProvider{})
		raw := `
  - 'DOMAIN-SUFFIX,anthropic.com,PROXY' # allow
  - "IP-CIDR,1.2.3.0/24,DIRECT" // local
  - 'DOMAIN-KEYWORD,foo # bar,DIRECT' # trailing comment
`
		count, err := mgr.ImportText(raw)
		if err != nil {
			t.Fatalf("ImportText quoted+comment failed: %v", err)
		}
		if count != 3 {
			t.Fatalf("imported count = %d, want 3", count)
		}

		custom := mgr.CustomRules()
		if custom[0].Type != contract.RuleDomainSuffix || custom[0].Payload != "anthropic.com" || custom[0].Action != contract.ActionExit {
			t.Fatalf("unexpected custom[0]: %+v", custom[0])
		}
		if custom[1].Type != contract.RuleIPCIDR || custom[1].Payload != "1.2.3.0/24" || custom[1].Action != contract.ActionDirect {
			t.Fatalf("unexpected custom[1]: %+v", custom[1])
		}
		if custom[2].Type != contract.RuleDomainKeyword || custom[2].Payload != "foo # bar" || custom[2].Action != contract.ActionDirect {
			t.Fatalf("unexpected custom[2]: %+v", custom[2])
		}
	})

	t.Run("Zero valid rules returns error", func(t *testing.T) {
		mgr := NewManager([]contract.Rule{}, []contract.RuleProvider{})
		inputs := []string{
			"",
			"# only comments\n// nothing here\n",
			"FINAL,fallback,EXIT",
			"DOMAIN-SUFFIX,,EXIT",
			`[{"type":"UNKNOWN","payload":"a.com","action":"EXIT"}]`,
		}
		for _, in := range inputs {
			count, err := mgr.ImportText(in)
			if err == nil || count != 0 {
				t.Fatalf("ImportText(%q) = (%d, %v), want (0, error)", in, count, err)
			}
		}
	})
}

func TestProvidersAndRefresh(t *testing.T) {
	t.Run("AddProvider validation and addition", func(t *testing.T) {
		mgr := NewManager(nil, []contract.RuleProvider{})
		if _, err := mgr.AddProvider(contract.AddProviderRequest{
			Tag: "", URL: "https://example.com/a.srs",
		}); err == nil {
			t.Fatal("expected error for empty tag")
		}
		if _, err := mgr.AddProvider(contract.AddProviderRequest{
			Tag: "bad-url", URL: "not-a-url",
		}); err == nil {
			t.Fatal("expected error for invalid URL")
		}

		p, err := mgr.AddProvider(contract.AddProviderRequest{
			Tag:        "custom-set",
			SourceName: "MyRepo",
			URL:        "https://example.com/custom.srs",
		})
		if err != nil {
			t.Fatalf("AddProvider failed: %v", err)
		}
		if p.ID == "" || p.Tag != "custom-set" || len(mgr.ListProviders()) != 1 {
			t.Fatalf("unexpected provider added: %+v", p)
		}

		if _, err := mgr.AddProvider(contract.AddProviderRequest{
			Tag:        "custom-set",
			SourceName: "Dup",
			URL:        "https://example.com/dup.srs",
		}); !errors.Is(err, ErrDuplicateTag) {
			t.Fatalf("expected ErrDuplicateTag, got %v", err)
		}
	})

	t.Run("RefreshAllProviders nil fetcher updates timestamps", func(t *testing.T) {
		mgr := NewManager(nil, []contract.RuleProvider{
			{ID: "p1", Tag: "t1", URL: "https://example.com/1.srs", UpdatedAt: "old-1"},
			{ID: "p2", Tag: "t2", URL: "https://example.com/2.srs", UpdatedAt: "old-2"},
		})

		if err := mgr.RefreshAllProviders(context.Background(), nil); err != nil {
			t.Fatalf("RefreshAllProviders(nil) failed: %v", err)
		}

		tsPattern := regexp.MustCompile(`^\d{2}-\d{2} \d{2}:\d{2}$`)
		for _, p := range mgr.ListProviders() {
			if !tsPattern.MatchString(p.UpdatedAt) {
				t.Fatalf("provider %s UpdatedAt = %q, want MM-DD HH:MM", p.Tag, p.UpdatedAt)
			}
		}
	})

	t.Run("RefreshAllProviders preserves failed items and updates healthy items", func(t *testing.T) {
		mgr := NewManager(nil, []contract.RuleProvider{
			{ID: "p-ok", Tag: "ok-set", URL: "https://example.com/ok.srs", UpdatedAt: "old-ok"},
			{ID: "p-short", Tag: "short-set", URL: "https://example.com/short.srs", UpdatedAt: "old-short"},
			{ID: "p-err", Tag: "err-set", URL: "https://example.com/err.srs", UpdatedAt: "old-err"},
		})

		err := mgr.RefreshAllProviders(context.Background(), func(_ context.Context, rawURL string) ([]byte, error) {
			switch rawURL {
			case "https://example.com/ok.srs":
				return []byte("SRS-VALID-PAYLOAD"), nil
			case "https://example.com/short.srs":
				return []byte("short"), nil // < 8 bytes
			default:
				return nil, errors.New("network timeout")
			}
		})
		if err == nil {
			t.Fatal("expected aggregated error when some providers fail")
		}
		if !strings.Contains(err.Error(), "short-set") || !strings.Contains(err.Error(), "err-set") {
			t.Fatalf("expected aggregated error to mention failed providers, got: %v", err)
		}

		provs := mgr.ListProviders()
		if provs[0].UpdatedAt == "old-ok" {
			t.Fatal("expected healthy provider UpdatedAt to be refreshed")
		}
		if provs[1].UpdatedAt != "old-short" {
			t.Fatalf("short payload provider UpdatedAt = %q, want preserved %q", provs[1].UpdatedAt, "old-short")
		}
		if provs[2].UpdatedAt != "old-err" {
			t.Fatalf("errored provider UpdatedAt = %q, want preserved %q", provs[2].UpdatedAt, "old-err")
		}
	})

	t.Run("RefreshAllProvidersToDir atomic write and validation", func(t *testing.T) {
		rulesDir := filepath.Join(t.TempDir(), "rules")
		if err := os.MkdirAll(rulesDir, 0o755); err != nil {
			t.Fatalf("mkdir rulesDir: %v", err)
		}
		existingBadFile := filepath.Join(rulesDir, "bad-set.srs")
		if err := os.WriteFile(existingBadFile, []byte("PRIOR-GOOD-SRS"), 0o644); err != nil {
			t.Fatalf("seed existing bad-set.srs: %v", err)
		}

		mgr := NewManager(nil, []contract.RuleProvider{
			{ID: "p-good", Tag: "good-set", URL: "https://example.com/good.srs", UpdatedAt: "old-good"},
			{ID: "p-bad", Tag: "bad-set", URL: "https://example.com/bad.srs", UpdatedAt: "old-bad"},
		})

		err := mgr.RefreshAllProvidersToDir(
			context.Background(),
			rulesDir,
			func(_ context.Context, rawURL string) ([]byte, error) {
				if strings.HasSuffix(rawURL, "good.srs") {
					return []byte("SRS-GOOD-DATA"), nil
				}
				return []byte("SRS-INVALID-DATA"), nil
			},
			func(tmpPath string) error {
				raw, err := os.ReadFile(tmpPath)
				if err != nil {
					return err
				}
				if strings.Contains(string(raw), "INVALID") {
					return errors.New("corrupt srs header")
				}
				return nil
			},
		)
		if err == nil || !strings.Contains(err.Error(), "bad-set") {
			t.Fatalf("expected validation error for bad-set, got %v", err)
		}

		goodBytes, err := os.ReadFile(filepath.Join(rulesDir, "good-set.srs"))
		if err != nil || string(goodBytes) != "SRS-GOOD-DATA" {
			t.Fatalf("expected good-set.srs written atomically, got %q err=%v", string(goodBytes), err)
		}
		if _, err := os.Stat(filepath.Join(rulesDir, "good-set.srs.tmp")); !os.IsNotExist(err) {
			t.Fatalf("expected good-set.srs.tmp removed, stat err=%v", err)
		}

		badBytes, err := os.ReadFile(existingBadFile)
		if err != nil || string(badBytes) != "PRIOR-GOOD-SRS" {
			t.Fatalf("expected prior bad-set.srs preserved on validation failure, got %q err=%v", string(badBytes), err)
		}
		if _, err := os.Stat(filepath.Join(rulesDir, "bad-set.srs.tmp")); !os.IsNotExist(err) {
			t.Fatalf("expected bad-set.srs.tmp removed after validation failure, stat err=%v", err)
		}

		provs := mgr.ListProviders()
		if provs[0].UpdatedAt == "old-good" || provs[1].UpdatedAt != "old-bad" {
			t.Fatalf("unexpected provider timestamps: %+v", provs)
		}
	})
}

func TestManagerConcurrency(t *testing.T) {
	mgr := NewManager(nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				r, err := mgr.AddRule(contract.AddRuleRequest{
					Type:    contract.RuleDomainSuffix,
					Payload: "conc.example.com",
					Action:  contract.ActionExit,
				})
				if err == nil {
					_ = mgr.ToggleRule(r.ID)
					_ = mgr.MoveRule(r.ID, +1)
					_ = mgr.DeleteRule(r.ID)
				}
				_ = mgr.ListRules()
				_ = mgr.CustomRules()
				_, _ = mgr.ImportText("DOMAIN-SUFFIX,imp.com,PROXY")
				_ = mgr.ListProviders()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 10; j++ {
			_ = mgr.RefreshAllProviders(context.Background(), func(_ context.Context, _ string) ([]byte, error) {
				return []byte("0123456789"), nil
			})
		}
	}()

	wg.Wait()
}

func TestNewManagerAvoidsExplicitIDCollisions(t *testing.T) {
	mgr := NewManager(
		[]contract.Rule{
			{ID: "", Type: contract.RuleDomainSuffix, Payload: "first.com", Action: contract.ActionExit, Enabled: true},
			{ID: "rule-1", Type: contract.RuleDomainSuffix, Payload: "second.com", Action: contract.ActionDirect, Enabled: true},
			{ID: "", Type: contract.RuleDomainSuffix, Payload: "third.com", Action: contract.ActionExit, Enabled: true},
		},
		[]contract.RuleProvider{
			{ID: "", Tag: "t1", URL: "https://example.com/1.srs"},
			{ID: "prov-1", Tag: "t2", URL: "https://example.com/2.srs"},
			{ID: "", Tag: "t3", URL: "https://example.com/3.srs"},
		},
	)

	seenRuleIDs := make(map[string]bool)
	for _, r := range mgr.ListRules() {
		if r.ID == "" {
			t.Fatalf("found empty rule ID: %+v", r)
		}
		if seenRuleIDs[r.ID] {
			t.Fatalf("duplicate rule ID generated: %s", r.ID)
		}
		seenRuleIDs[r.ID] = true
	}
	if !seenRuleIDs["rule-1"] {
		t.Fatal("expected explicit rule-1 ID to be preserved")
	}

	seenProvIDs := make(map[string]bool)
	for _, p := range mgr.ListProviders() {
		if p.ID == "" {
			t.Fatalf("found empty provider ID: %+v", p)
		}
		if seenProvIDs[p.ID] {
			t.Fatalf("duplicate provider ID generated: %s", p.ID)
		}
		seenProvIDs[p.ID] = true
	}
	if !seenProvIDs["prov-1"] {
		t.Fatal("expected explicit prov-1 ID to be preserved")
	}
}
