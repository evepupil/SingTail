//go:build darwin

package syswin

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

const (
	// ServiceName is the logical service identifier shared across platforms.
	ServiceName = "SingTailService"
	// ServiceDisplayName is the human-readable service title.
	ServiceDisplayName = "SingTail Core Service"
	// ServiceDescription explains the background service purpose.
	ServiceDescription = "SingTail background daemon managing multi-tailnet exit nodes and sing-box TUN routing."

	// LaunchDaemonLabel is the macOS launchd service label.
	LaunchDaemonLabel = "io.singtail.service"
	// LaunchDaemonPlistPath is the system LaunchDaemon plist destination on macOS.
	LaunchDaemonPlistPath = "/Library/LaunchDaemons/io.singtail.service.plist"
	darwinServiceDataDir  = "/Library/Application Support/SingTail"
	darwinServiceLogDir   = "/Library/Logs/SingTail"
)

// InstallService installs and starts the SingTail LaunchDaemon on macOS,
// prompting for administrator privileges via osascript when not running as root.
func InstallService(exePath string) error {
	resolvedPath, err := resolveDarwinServiceExecutable(exePath)
	if err != nil {
		return err
	}

	plistContent := buildLaunchDaemonPlist(resolvedPath, darwinServiceDataDir, darwinServiceLogDir)
	tmpFile, err := os.CreateTemp("", "io.singtail.service.*.plist")
	if err != nil {
		return fmt.Errorf("create temp plist: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.WriteString(plistContent); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp plist: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp plist: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod temp plist: %w", err)
	}

	script := strings.Join([]string{
		fmt.Sprintf("mkdir -p %s %s %s",
			shellQuote("/Library/LaunchDaemons"),
			shellQuote(darwinServiceLogDir),
			shellQuote(darwinServiceDataDir),
		),
		fmt.Sprintf("cp %s %s", shellQuote(tmpPath), shellQuote(LaunchDaemonPlistPath)),
		fmt.Sprintf("chown root:wheel %s", shellQuote(LaunchDaemonPlistPath)),
		fmt.Sprintf("chmod 644 %s", shellQuote(LaunchDaemonPlistPath)),
		fmt.Sprintf("launchctl bootout system/%s >/dev/null 2>&1 || true", LaunchDaemonLabel),
		fmt.Sprintf("launchctl bootstrap system %s", shellQuote(LaunchDaemonPlistPath)),
		fmt.Sprintf("launchctl kickstart -k system/%s >/dev/null 2>&1 || true", LaunchDaemonLabel),
	}, " && ")

	if err := runShellWithPrivileges(script); err != nil {
		return fmt.Errorf("安装 macOS 后台服务失败: %w", err)
	}
	return nil
}

// UninstallService stops and removes the SingTail LaunchDaemon from macOS.
func UninstallService() error {
	script := strings.Join([]string{
		fmt.Sprintf("launchctl bootout system/%s >/dev/null 2>&1 || true", LaunchDaemonLabel),
		fmt.Sprintf("rm -f %s", shellQuote(LaunchDaemonPlistPath)),
	}, " && ")

	if err := runShellWithPrivileges(script); err != nil {
		return fmt.Errorf("卸载 macOS 后台服务失败: %w", err)
	}
	return nil
}

// IsWindowsService always returns false on macOS.
func IsWindowsService() bool {
	return false
}

// RunService invokes runFn directly on macOS and closes stopCh on SIGINT or SIGTERM.
func RunService(runFn func(stopCh <-chan struct{}) error) error {
	if runFn == nil {
		return nil
	}

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	closeStop := func() {
		stopOnce.Do(func() { close(stopCh) })
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	doneCh := make(chan struct{})

	defer func() {
		signal.Stop(sigCh)
		close(doneCh)
	}()

	go func() {
		select {
		case <-sigCh:
			closeStop()
		case <-doneCh:
		}
	}()

	return runFn(stopCh)
}

func resolveDarwinServiceExecutable(exePath string) (string, error) {
	if strings.TrimSpace(exePath) == "" {
		self, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve executable path: %w", err)
		}
		exePath = self
	}
	abs, err := filepath.Abs(exePath)
	if err != nil {
		return "", fmt.Errorf("resolve absolute service path: %w", err)
	}
	return abs, nil
}

func buildLaunchDaemonPlist(exePath, dataDir, logDir string) string {
	outLog := filepath.Join(logDir, "service.log")
	errLog := filepath.Join(logDir, "service.err.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>-data-dir</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, xmlEscape(LaunchDaemonLabel), xmlEscape(exePath), xmlEscape(dataDir), xmlEscape(outLog), xmlEscape(errLog))
}

func runShellWithPrivileges(script string) error {
	var out bytes.Buffer
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command("/bin/sh", "-c", script)
	} else {
		appleScript := fmt.Sprintf(`do shell script %q with administrator privileges`, script)
		cmd = exec.Command("osascript", "-e", appleScript)
	}
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
