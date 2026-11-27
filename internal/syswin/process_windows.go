//go:build windows

package syswin

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"

	"github.com/singtail/singtail/internal/contract"
	"golang.org/x/sys/windows"
)

// ListRunningApps enumerates active user-mode .exe processes on Windows without CGO.
func ListRunningApps() ([]contract.RunningApp, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("create process snapshot: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, fmt.Errorf("read first process: %w", err)
	}

	seen := make(map[string]contract.RunningApp)
	for {
		if entry.ProcessID > 4 {
			exeName := windows.UTF16ToString(entry.ExeFile[:])
			if strings.HasSuffix(strings.ToLower(exeName), ".exe") {
				if fullPath, ok := queryProcessPath(entry.ProcessID); ok && !isSystemBinaryPath(fullPath) {
					key := strings.ToLower(exeName)
					if _, exists := seen[key]; !exists {
						seen[key] = contract.RunningApp{
							Process: filepath.Base(fullPath),
							Path:    fullPath,
						}
					}
				}
			}
		}

		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				break
			}
			return nil, fmt.Errorf("iterate processes: %w", err)
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

func queryProcessPath(pid uint32) (string, bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:size]), true
}

func isSystemBinaryPath(fullPath string) bool {
	lower := strings.ToLower(filepath.ToSlash(fullPath))
	return strings.Contains(lower, "/windows/system32/") ||
		strings.Contains(lower, "/windows/syswow64/") ||
		strings.Contains(lower, "/windows/winsxs/")
}
