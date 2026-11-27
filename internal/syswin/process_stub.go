//go:build !windows && !darwin

package syswin

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/singtail/singtail/internal/contract"
)

// ListRunningApps enumerates active OS processes via ps on Unix hosts.
func ListRunningApps() ([]contract.RunningApp, error) {
	out, err := exec.Command("ps", "-eo", "comm=").Output()
	if err != nil {
		return nil, fmt.Errorf("list processes via ps: %w", err)
	}

	seen := make(map[string]contract.RunningApp)
	for _, line := range strings.Split(string(out), "\n") {
		rawPath := strings.TrimSpace(line)
		if rawPath == "" {
			continue
		}
		proc := filepath.Base(rawPath)
		if proc == "" || proc == "." || proc == "/" {
			continue
		}
		key := strings.ToLower(proc)
		if _, exists := seen[key]; !exists {
			seen[key] = contract.RunningApp{
				Process: proc,
				Path:    rawPath,
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
	return apps, nil
}
