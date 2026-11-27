//go:build darwin

package syswin

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/singtail/singtail/internal/contract"
)

// ListRunningApps enumerates active processes on macOS via ps without synthetic stub entries.
func ListRunningApps() ([]contract.RunningApp, error) {
	out, err := exec.Command("ps", "-axo", "comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("list macOS processes via ps: %w", err)
	}
	return parsePSProcessLines(string(out)), nil
}

func parsePSProcessLines(raw string) []contract.RunningApp {
	seen := make(map[string]contract.RunningApp)
	for _, line := range strings.Split(raw, "\n") {
		fullPath := strings.TrimSpace(line)
		if fullPath == "" {
			continue
		}

		proc := filepath.Base(fullPath)
		if !strings.Contains(fullPath, "/") && strings.HasPrefix(proc, "-") && len(proc) > 1 {
			proc = proc[1:]
		}
		if proc == "" || proc == "." || proc == "/" {
			continue
		}

		key := strings.ToLower(proc)
		existing, exists := seen[key]
		if !exists || (!strings.Contains(existing.Path, "/Applications/") && strings.Contains(fullPath, "/Applications/")) {
			seen[key] = contract.RunningApp{
				Process: proc,
				Path:    fullPath,
			}
		}
	}

	apps := make([]contract.RunningApp, 0, len(seen))
	for _, app := range seen {
		apps = append(apps, app)
	}
	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Process) < strings.ToLower(apps[j].Process)
	})
	return apps
}
