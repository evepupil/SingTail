//go:build !windows

package syswin

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// TrayAction represents a clickable menu entry or visual separator in the system tray popup menu.
type TrayAction struct {
	Label     string
	Checked   bool
	Separator bool
	OnClick   func()
}

// TrayConfig configures the system tray icon and menu callbacks.
type TrayConfig struct {
	Tooltip      string
	OnOpen       func()
	OnToggle     func()
	OnQuit       func()
	Actions      []TrayAction
	BuildActions func() []TrayAction
}

// OpenBrowser opens an http or https URL using the host operating system's default browser.
func OpenBrowser(rawURL string) error {
	validated, err := validateHTTPURL(rawURL)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", validated).Start()
	default:
		return exec.Command("xdg-open", validated).Start()
	}
}

func validateHTTPURL(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", errors.New("browser url cannot be empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid browser url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("unsupported browser url scheme %q", parsed.Scheme)
	}
	return parsed.String(), nil
}

// RunTray blocks until stopCh is closed on non-Windows hosts where no Win32 notification area exists.
func RunTray(_ TrayConfig, stopCh <-chan struct{}) error {
	if stopCh == nil {
		return nil
	}
	<-stopCh
	return nil
}
