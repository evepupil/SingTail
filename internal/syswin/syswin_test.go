package syswin

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestListRunningApps(t *testing.T) {
	apps, err := ListRunningApps()
	if err != nil {
		t.Fatalf("ListRunningApps() error = %v", err)
	}
	if len(apps) == 0 {
		t.Fatal("ListRunningApps() returned empty list")
	}

	seen := make(map[string]struct{}, len(apps))
	for _, app := range apps {
		if strings.TrimSpace(app.Process) == "" {
			t.Fatalf("app has empty Process name: %+v", app)
		}
		if strings.TrimSpace(app.Path) == "" {
			t.Fatalf("app %q has empty Path", app.Process)
		}
		if runtime.GOOS != "windows" && strings.Contains(app.Path, `C:\`) {
			t.Fatalf("non-Windows host must not return fake Windows path: %+v", app)
		}
		key := strings.ToLower(app.Process)
		if _, dup := seen[key]; dup {
			t.Fatalf("duplicate process entry %q", app.Process)
		}
		seen[key] = struct{}{}
	}
}

func TestServiceStubAndConstants(t *testing.T) {
	if IsWindowsService() {
		t.Fatal("expected IsWindowsService() == false during go test")
	}
	if ServiceName != "SingTailService" {
		t.Fatalf("unexpected ServiceName = %q", ServiceName)
	}

	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		if err := InstallService("/tmp/singtail-service"); err != nil {
			t.Fatalf("InstallService stub error = %v", err)
		}
		if err := InstallServiceWithOptions("/tmp/singtail-service", ServiceInstallOptions{DataDir: "/tmp/data"}); err != nil {
			t.Fatalf("InstallServiceWithOptions stub error = %v", err)
		}
		if err := UninstallService(); err != nil {
			t.Fatalf("UninstallService stub error = %v", err)
		}
	}

	if _, err := QueryServiceStatus(); err != nil && runtime.GOOS != "windows" {
		t.Fatalf("QueryServiceStatus error = %v", err)
	}
}

func TestBuildServiceStartArgs(t *testing.T) {
	if args := buildServiceStartArgs(ServiceInstallOptions{}); args != nil {
		t.Fatalf("expected nil args for empty options, got %v", args)
	}
	args := buildServiceStartArgs(ServiceInstallOptions{DataDir: filepath.Join(".", "data")})
	if len(args) != 2 || args[0] != "-data-dir" || !filepath.IsAbs(args[1]) {
		t.Fatalf("unexpected service start args: %v", args)
	}

	if runtime.GOOS != "windows" {
		called := false
		wantErr := errors.New("boom")
		err := RunService(func(stopCh <-chan struct{}) error {
			called = true
			if stopCh == nil {
				t.Fatal("expected non-nil stopCh")
			}
			return wantErr
		})
		if !called {
			t.Fatal("expected RunService to invoke runFn on non-Windows hosts")
		}
		if !errors.Is(err, wantErr) {
			t.Fatalf("RunService() error = %v, want %v", err, wantErr)
		}
	}
}

func TestOpenBrowserRejectsInvalidURLs(t *testing.T) {
	invalidCases := []string{
		"",
		"   ",
		"javascript:alert(1)",
		"file:///etc/passwd",
		"http://",
		"not-a-url",
	}
	for _, raw := range invalidCases {
		if err := OpenBrowser(raw); err == nil {
			t.Errorf("OpenBrowser(%q) expected error, got nil", raw)
		}
	}

	got, err := validateHTTPURL("http://127.0.0.1:4189")
	if err != nil {
		t.Fatalf("validateHTTPURL valid http error = %v", err)
	}
	if got != "http://127.0.0.1:4189" {
		t.Fatalf("validateHTTPURL got %q", got)
	}
}

func TestRunTrayStopsOnChannelClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Win32 tray message loop requires desktop session")
	}

	stopCh := make(chan struct{})
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- RunTray(TrayConfig{Tooltip: "SingTail"}, stopCh)
	}()

	close(stopCh)
	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("RunTray() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunTray() did not exit after stopCh closed")
	}
}
