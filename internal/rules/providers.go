package rules

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/singtail/singtail/internal/contract"
)

const minSRSBytes = 8

var (
	ErrEmptyProviderTag = errors.New("provider tag must not be empty")
	ErrInvalidURL       = errors.New("provider URL is invalid")
	ErrDuplicateTag     = errors.New("provider tag already exists")
)

func defaultProviders() []contract.RuleProvider {
	now := time.Now().Format("01-02 15:04")
	return []contract.RuleProvider{
		{
			ID:         "prov-geoip-cn",
			Tag:        "geoip-cn",
			SourceName: "MetaCubeX",
			URL:        "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/cn.srs",
			EntryCount: 9420,
			UpdatedAt:  now,
		},
		{
			ID:         "prov-geosite-cn",
			Tag:        "geosite-cn",
			SourceName: "MetaCubeX",
			URL:        "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/cn.srs",
			EntryCount: 68510,
			UpdatedAt:  now,
		},
		{
			ID:         "prov-geosite-geolocation-not-cn",
			Tag:        "geosite-geolocation-!cn",
			SourceName: "MetaCubeX",
			URL:        "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/geolocation-!cn.srs",
			EntryCount: 36920,
			UpdatedAt:  now,
		},
		{
			ID:         "prov-geosite-openai",
			Tag:        "geosite-openai",
			SourceName: "MetaCubeX",
			URL:        "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/openai.srs",
			EntryCount: 180,
			UpdatedAt:  now,
		},
		{
			ID:         "prov-geosite-google",
			Tag:        "geosite-google",
			SourceName: "MetaCubeX",
			URL:        "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/google.srs",
			EntryCount: 840,
			UpdatedAt:  now,
		},
		{
			ID:         "prov-geosite-github",
			Tag:        "geosite-github",
			SourceName: "MetaCubeX",
			URL:        "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/github.srs",
			EntryCount: 160,
			UpdatedAt:  now,
		},
		{
			ID:         "prov-geosite-category-ads-all",
			Tag:        "geosite-category-ads-all",
			SourceName: "MetaCubeX",
			URL:        "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/category-ads-all.srs",
			EntryCount: 41200,
			UpdatedAt:  now,
		},
	}
}

// ListProviders returns a snapshot copy of all configured .srs rule providers.
func (m *Manager) ListProviders() []contract.RuleProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]contract.RuleProvider, len(m.providers))
	copy(out, m.providers)
	return out
}

// AddProvider validates and registers a new .srs rule-set provider.
func (m *Manager) AddProvider(req contract.AddProviderRequest) (contract.RuleProvider, error) {
	tag := strings.TrimSpace(req.Tag)
	if tag == "" {
		return contract.RuleProvider{}, ErrEmptyProviderTag
	}

	rawURL := strings.TrimSpace(req.URL)
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return contract.RuleProvider{}, fmt.Errorf("%w: %q", ErrInvalidURL, req.URL)
	}

	sourceName := strings.TrimSpace(req.SourceName)
	if sourceName == "" {
		sourceName = "Custom"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, existing := range m.providers {
		if strings.EqualFold(existing.Tag, tag) {
			return contract.RuleProvider{}, fmt.Errorf("%w: %s", ErrDuplicateTag, tag)
		}
	}

	provider := contract.RuleProvider{
		ID:         m.nextIDLocked("prov"),
		Tag:        tag,
		SourceName: sourceName,
		URL:        rawURL,
		EntryCount: 0,
		UpdatedAt:  time.Now().Format("01-02 15:04"),
	}
	m.providers = append(m.providers, provider)
	return provider, nil
}

// RefreshAllProviders updates provider timestamps or fetches and validates .srs payloads via fetcher.
func (m *Manager) RefreshAllProviders(ctx context.Context, fetcher func(ctx context.Context, url string) ([]byte, error)) error {
	return m.RefreshAllProvidersToDir(ctx, "", fetcher, nil)
}

// RefreshAllProvidersToDir fetches and validates .srs payloads, optionally persisting validated files
// atomically into rulesDir as <tag>.srs via a temporary <tag>.srs.tmp file.
func (m *Manager) RefreshAllProvidersToDir(ctx context.Context, rulesDir string, fetcher func(ctx context.Context, url string) ([]byte, error), validateFile func(tmpPath string) error) error {
	if ctx == nil {
		ctx = context.Background()
	}

	nowStr := time.Now().Format("01-02 15:04")

	if fetcher == nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		for i := range m.providers {
			m.providers[i].UpdatedAt = nowStr
		}
		return nil
	}

	trimmedDir := strings.TrimSpace(rulesDir)
	if trimmedDir != "" {
		if err := os.MkdirAll(trimmedDir, 0o755); err != nil {
			return fmt.Errorf("create rules directory: %w", err)
		}
	}

	m.mu.RLock()
	snapshot := make([]contract.RuleProvider, len(m.providers))
	copy(snapshot, m.providers)
	m.mu.RUnlock()

	succeeded := make(map[string]bool, len(snapshot))
	var errs []error

	for _, p := range snapshot {
		data, err := fetcher(ctx, p.URL)
		if err != nil {
			errs = append(errs, fmt.Errorf("provider %s fetch failed: %w", p.Tag, err))
			continue
		}
		if len(data) < minSRSBytes {
			errs = append(errs, fmt.Errorf("provider %s payload too short (%d bytes < %d)", p.Tag, len(data), minSRSBytes))
			continue
		}

		if trimmedDir != "" {
			if p.Tag == "" || strings.ContainsAny(p.Tag, "/\\") || strings.Contains(p.Tag, "..") {
				errs = append(errs, fmt.Errorf("provider %s has invalid tag for file path", p.Tag))
				continue
			}
			tmpPath := filepath.Join(trimmedDir, p.Tag+".srs.tmp")
			finalPath := filepath.Join(trimmedDir, p.Tag+".srs")
			if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
				errs = append(errs, fmt.Errorf("provider %s write temp file failed: %w", p.Tag, err))
				continue
			}
			if validateFile != nil {
				if err := validateFile(tmpPath); err != nil {
					_ = os.Remove(tmpPath)
					errs = append(errs, fmt.Errorf("provider %s validation failed: %w", p.Tag, err))
					continue
				}
			}
			if err := os.Rename(tmpPath, finalPath); err != nil {
				_ = os.Remove(tmpPath)
				errs = append(errs, fmt.Errorf("provider %s rename failed: %w", p.Tag, err))
				continue
			}
		}

		succeeded[p.ID] = true
	}

	if len(succeeded) > 0 {
		m.mu.Lock()
		for i := range m.providers {
			if succeeded[m.providers[i].ID] {
				m.providers[i].UpdatedAt = nowStr
			}
		}
		m.mu.Unlock()
	}

	return errors.Join(errs...)
}
