package syswin

import (
	"errors"
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

	if runtime.GOOS != "windows" {
		if err := InstallService("/tmp/singtail-service"); err != nil {
			t.Fatalf("InstallService stub error = %v", err)
		}
		if err := UninstallService(); err != nil {
			t.Fatalf("UninstallService stub error = %v", err)
		}

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
