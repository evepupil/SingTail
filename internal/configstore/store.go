package configstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/singtail/singtail/internal/contract"
)

const exportFormatVersion = "1"

// PersistedState stores the full local SingTail daemon state on disk.
type PersistedState struct {
	TunEnabled   bool                    `json:"tunEnabled"`
	RoutingMode  contract.RoutingMode    `json:"routingMode"`
	ActiveExitID string                  `json:"activeExitId"`
	Orgs         []contract.Organization `json:"orgs"`
	CustomRules  []contract.Rule         `json:"customRules"`
	Providers    []contract.RuleProvider `json:"providers"`
	Settings     contract.SystemSettings `json:"settings"`
}

// Store provides atomic JSON persistence for PersistedState.
type Store struct {
	filePath string
	mu       sync.Mutex
}

// New creates a Store backed by the given filePath.
func New(filePath string) *Store {
	return &Store{filePath: filePath}
}

func defaultState() PersistedState {
	return PersistedState{
		TunEnabled:  true,
		RoutingMode: contract.ModeRule,
		Orgs:        make([]contract.Organization, 0),
		CustomRules: make([]contract.Rule, 0),
		Providers:   make([]contract.RuleProvider, 0),
		Settings: contract.SystemSettings{
			AutoStartService:     true,
			AutoVerifyIP:         true,
			FallbackProxyEnabled: true,
			FallbackProxyAddr:    "127.0.0.1:7897",
			AutoUpdateRulesDays:  14,
		},
	}
}

func normalizeSlices(st *PersistedState) {
	if st.Orgs == nil {
		st.Orgs = make([]contract.Organization, 0)
	}
	if st.CustomRules == nil {
		st.CustomRules = make([]contract.Rule, 0)
	}
	if st.Providers == nil {
		st.Providers = make([]contract.RuleProvider, 0)
	}
}

// LoadOrDefault reads the persisted state from disk, or returns default settings if the file does not exist.
func (s *Store) LoadOrDefault() (PersistedState, error) {
	if s == nil || strings.TrimSpace(s.filePath) == "" {
		return PersistedState{}, fmt.Errorf("configstore: file path must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultState(), nil
		}
		return PersistedState{}, fmt.Errorf("configstore: read state file: %w", err)
	}

	st := defaultState()
	if err := json.Unmarshal(data, &st); err != nil {
		return PersistedState{}, fmt.Errorf("configstore: decode state file: %w", err)
	}
	normalizeSlices(&st)
	return st, nil
}

// Save atomically writes the state to disk via a temporary file followed by rename.
func (s *Store) Save(st PersistedState) error {
	if s == nil || strings.TrimSpace(s.filePath) == "" {
		return fmt.Errorf("configstore: file path must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeSlices(&st)
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("configstore: create parent directory: %w", err)
	}

	payload, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("configstore: encode state file: %w", err)
	}
	payload = append(payload, '\n')

	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o600); err != nil {
		return fmt.Errorf("configstore: write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("configstore: rename temp file: %w", err)
	}
	return nil
}

// BuildSanitizedExport builds a shareable configuration snapshot with locked rules, organizations, state directories, and credentials stripped.
func BuildSanitizedExport(rules []contract.Rule, providers []contract.RuleProvider, settings contract.SystemSettings, now time.Time) contract.SanitizedExport {
	customRules := make([]contract.SanitizedRule, 0, len(rules))
	for _, r := range rules {
		if r.Locked {
			continue
		}
		customRules = append(customRules, contract.SanitizedRule{
			Type:    r.Type,
			Payload: r.Payload,
			Action:  r.Action,
			Enabled: r.Enabled,
		})
	}

	sanitizedProviders := make([]contract.SanitizedProvider, 0, len(providers))
	for _, p := range providers {
		sanitizedProviders = append(sanitizedProviders, contract.SanitizedProvider{
			Tag:        p.Tag,
			SourceName: p.SourceName,
			URL:        p.URL,
		})
	}

	return contract.SanitizedExport{
		Version:     exportFormatVersion,
		ExportedAt:  now.UTC().Format(time.RFC3339),
		CustomRules: customRules,
		Providers:   sanitizedProviders,
		Settings:    settings,
	}
}
