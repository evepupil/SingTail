package rules

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/singtail/singtail/internal/contract"
)

const (
	headLockedCount = 2
	tailLockedCount = 2
)

var (
	ErrRuleNotFound      = errors.New("rule not found")
	ErrRuleLocked        = errors.New("locked built-in rule cannot be modified")
	ErrRuleOutOfBounds   = errors.New("cannot move rule into locked boundary")
	ErrInvalidRuleType   = errors.New("invalid rule type")
	ErrInvalidRuleAction = errors.New("invalid rule action")
	ErrEmptyPayload      = errors.New("rule payload must not be empty")
	ErrInvalidDirection  = errors.New("move direction must be -1 or 1")
)

// Manager manages ordered routing rules and .srs rule-set providers safely across goroutines.
type Manager struct {
	mu        sync.RWMutex
	rules     []contract.Rule
	providers []contract.RuleProvider
	seq       uint64
}

// NewManager initializes a Manager with fixed head/tail locked rules, user custom rules, and rule providers.
// Passing nil for customRules or providers populates built-in defaults; passing a non-nil empty slice starts empty.
func NewManager(customRules []contract.Rule, providers []contract.RuleProvider) *Manager {
	m := &Manager{}

	head := defaultHeadLockedRules()
	tail := defaultTailLockedRules()

	var middle []contract.Rule
	if customRules == nil {
		middle = defaultCustomRules()
	} else {
		middle = make([]contract.Rule, 0, len(customRules))
		for _, r := range customRules {
			copied := r
			copied.Locked = false
			middle = append(middle, copied)
		}
	}

	m.rules = make([]contract.Rule, 0, len(head)+len(middle)+len(tail))
	m.rules = append(m.rules, head...)
	m.rules = append(m.rules, middle...)
	m.rules = append(m.rules, tail...)

	if providers == nil {
		m.providers = defaultProviders()
	} else {
		m.providers = make([]contract.RuleProvider, len(providers))
		copy(m.providers, providers)
	}

	// Fill missing IDs after all explicit rule and provider IDs are registered to prevent collisions.
	for i := range m.rules {
		if strings.TrimSpace(m.rules[i].ID) == "" {
			m.rules[i].ID = m.nextIDLocked("rule")
		}
	}
	for i := range m.providers {
		if strings.TrimSpace(m.providers[i].ID) == "" {
			m.providers[i].ID = m.nextIDLocked("prov")
		}
	}

	return m
}

func defaultHeadLockedRules() []contract.Rule {
	return []contract.Rule{
		{
			ID:      "builtin-head-1",
			Type:    contract.RuleProcessName,
			Payload: "tailscaled.exe, tailscale-ipn.exe, verge-mihomo.exe, clash-verge.exe, Tailscale, IPNExtension, tailscaled, verge-mihomo, clash-verge, mihomo",
			Action:  contract.ActionDirect,
			Hits:    0,
			Enabled: true,
			Locked:  true,
		},
		{
			ID:      "builtin-head-2",
			Type:    contract.RuleIPCIDR,
			Payload: "100.64.0.0/10, 192.168.0.0/16, 10.0.0.0/8",
			Action:  contract.ActionDirect,
			Hits:    0,
			Enabled: true,
			Locked:  true,
		},
	}
}

func defaultTailLockedRules() []contract.Rule {
	return []contract.Rule{
		{
			ID:      "builtin-tail-1",
			Type:    contract.RuleSet,
			Payload: "geosite-cn, geoip-cn",
			Action:  contract.ActionDirect,
			Hits:    0,
			Enabled: true,
			Locked:  true,
		},
		{
			ID:      "builtin-tail-2",
			Type:    contract.RuleFinal,
			Payload: "默认兜底",
			Action:  contract.ActionExit,
			Hits:    0,
			Enabled: true,
			Locked:  true,
		},
	}
}

func defaultCustomRules() []contract.Rule {
	return []contract.Rule{
		{
			ID:      "preset-1",
			Type:    contract.RuleProcessName,
			Payload: "YuanShen.exe, HYP.exe",
			Action:  contract.ActionDirect,
			Hits:    0,
			Enabled: true,
		},
		{
			ID:      "preset-2",
			Type:    contract.RuleProcessName,
			Payload: "cloudflared.exe",
			Action:  contract.ActionDirect,
			Hits:    0,
			Enabled: true,
		},
		{
			ID:      "preset-3",
			Type:    contract.RuleIPCIDR,
			Payload: "8.128.0.0/10",
			Action:  contract.ActionDirect,
			Hits:    0,
			Enabled: true,
		},
		{
			ID:      "preset-4",
			Type:    contract.RuleDomainSuffix,
			Payload: "chatgpt.com, openai.com, claude.ai",
			Action:  contract.ActionExit,
			Hits:    0,
			Enabled: true,
		},
		{
			ID:      "preset-5",
			Type:    contract.RuleDomainSuffix,
			Payload: "netflix.com, nflxvideo.net",
			Action:  contract.ActionFallbackProxy,
			Hits:    0,
			Enabled: true,
		},
		{
			ID:      "preset-6",
			Type:    contract.RuleSet,
			Payload: "geosite-google",
			Action:  contract.ActionExit,
			Hits:    0,
			Enabled: true,
		},
		{
			ID:      "preset-7",
			Type:    contract.RuleSet,
			Payload: "geosite-category-ads-all",
			Action:  contract.ActionReject,
			Hits:    0,
			Enabled: true,
		},
	}
}

// ListRules returns a snapshot copy of all rules (head locked, user custom, tail locked).
func (m *Manager) ListRules() []contract.Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]contract.Rule, len(m.rules))
	copy(out, m.rules)
	return out
}

// CustomRules returns a snapshot copy of user-editable rules between the head and tail locked zones.
func (m *Manager) CustomRules() []contract.Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.rules) <= headLockedCount+tailLockedCount {
		return []contract.Rule{}
	}
	middle := m.rules[headLockedCount : len(m.rules)-tailLockedCount]
	out := make([]contract.Rule, len(middle))
	copy(out, middle)
	return out
}

// AddRule validates and inserts a new custom rule right after the head locked rules (index 2).
func (m *Manager) AddRule(req contract.AddRuleRequest) (contract.Rule, error) {
	ruleType := contract.RuleType(strings.ToUpper(strings.TrimSpace(string(req.Type))))
	if !isValidUserRuleType(ruleType) {
		return contract.Rule{}, fmt.Errorf("%w: %q", ErrInvalidRuleType, req.Type)
	}

	payload := strings.TrimSpace(req.Payload)
	if payload == "" {
		return contract.Rule{}, ErrEmptyPayload
	}

	action := contract.RuleAction(strings.ToUpper(strings.TrimSpace(string(req.Action))))
	if !isValidRuleAction(action) {
		return contract.Rule{}, fmt.Errorf("%w: %q", ErrInvalidRuleAction, req.Action)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	rule := contract.Rule{
		ID:      m.nextIDLocked("rule"),
		Type:    ruleType,
		Payload: payload,
		Action:  action,
		Hits:    0,
		Enabled: true,
		Locked:  false,
	}
	m.insertCustomRulesTopLocked([]contract.Rule{rule})
	return rule, nil
}

// ToggleRule flips the Enabled state of an unlocked custom rule.
func (m *Manager) ToggleRule(ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.findRuleIndexLocked(ruleID)
	if idx < 0 {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleID)
	}
	if m.rules[idx].Locked {
		return fmt.Errorf("%w: %s", ErrRuleLocked, ruleID)
	}
	m.rules[idx].Enabled = !m.rules[idx].Enabled
	return nil
}

// MoveRule moves an unlocked custom rule by direction (-1 up, +1 down) within the custom zone.
func (m *Manager) MoveRule(ruleID string, direction int) error {
	if direction != -1 && direction != 1 {
		return fmt.Errorf("%w: got %d", ErrInvalidDirection, direction)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.findRuleIndexLocked(ruleID)
	if idx < 0 {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleID)
	}
	if m.rules[idx].Locked {
		return fmt.Errorf("%w: %s", ErrRuleLocked, ruleID)
	}

	target := idx + direction
	if target < headLockedCount || target >= len(m.rules)-tailLockedCount {
		return fmt.Errorf("%w: index %d", ErrRuleOutOfBounds, target)
	}

	m.rules[idx], m.rules[target] = m.rules[target], m.rules[idx]
	return nil
}

// DeleteRule removes an unlocked custom rule by ID.
func (m *Manager) DeleteRule(ruleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.findRuleIndexLocked(ruleID)
	if idx < 0 {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleID)
	}
	if m.rules[idx].Locked {
		return fmt.Errorf("%w: %s", ErrRuleLocked, ruleID)
	}

	m.rules = append(m.rules[:idx], m.rules[idx+1:]...)
	return nil
}

func (m *Manager) findRuleIndexLocked(ruleID string) int {
	for i, r := range m.rules {
		if r.ID == ruleID {
			return i
		}
	}
	return -1
}

func (m *Manager) insertCustomRulesTopLocked(items []contract.Rule) {
	if len(items) == 0 {
		return
	}
	updated := make([]contract.Rule, 0, len(m.rules)+len(items))
	updated = append(updated, m.rules[:headLockedCount]...)
	updated = append(updated, items...)
	updated = append(updated, m.rules[headLockedCount:]...)
	m.rules = updated
}

func (m *Manager) nextIDLocked(prefix string) string {
	for {
		m.seq++
		candidate := fmt.Sprintf("%s-%d", prefix, m.seq)
		if !m.idExistsLocked(candidate) {
			return candidate
		}
	}
}

func (m *Manager) idExistsLocked(id string) bool {
	for _, r := range m.rules {
		if r.ID == id {
			return true
		}
	}
	for _, p := range m.providers {
		if p.ID == id {
			return true
		}
	}
	return false
}

func isValidUserRuleType(t contract.RuleType) bool {
	switch t {
	case contract.RuleDomainSuffix,
		contract.RuleDomainKeyword,
		contract.RuleProcessName,
		contract.RuleIPCIDR,
		contract.RuleSet:
		return true
	default:
		return false
	}
}

func isValidRuleAction(a contract.RuleAction) bool {
	switch a {
	case contract.ActionExit,
		contract.ActionDirect,
		contract.ActionFallbackProxy,
		contract.ActionReject:
		return true
	default:
		return false
	}
}

func normalizeImportAction(raw string) (contract.RuleAction, bool) {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	if upper == "PROXY" {
		return contract.ActionExit, true
	}
	action := contract.RuleAction(upper)
	return action, isValidRuleAction(action)
}
