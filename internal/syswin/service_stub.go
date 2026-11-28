//go:build !windows && !darwin

package syswin

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// IsElevated reports whether the current Unix process has root privileges.
func IsElevated() bool {
	return os.Geteuid() == 0
}

// InstallService is a safe no-op on generic Unix hosts.
func InstallService(_ string) error {
	return nil
}

// InstallServiceWithOptions is a safe no-op on generic Unix hosts.
func InstallServiceWithOptions(_ string, _ ServiceInstallOptions) error {
	return nil
}

// QueryServiceStatus returns an empty status on generic Unix hosts.
func QueryServiceStatus() (ServiceStatusInfo, error) {
	return ServiceStatusInfo{}, nil
}

// StartInstalledService is a safe no-op on generic Unix hosts.
func StartInstalledService() error {
	return nil
}

// StartOrRestartService is a safe no-op on generic Unix hosts.
func StartOrRestartService(_ bool) error {
	return nil
}

// SetServiceAutoStart is a safe no-op on generic Unix hosts.
func SetServiceAutoStart(_ bool) error {
	return nil
}

// RunElevatedWait is a safe no-op on generic Unix hosts.
func RunElevatedWait(_ string, _ bool, _ ...string) (bool, uint32, error) {
	return false, 0, nil
}

// RelaunchSelfElevated is a safe no-op on generic Unix hosts.
func RelaunchSelfElevated(_ ...string) (bool, error) {
	return false, nil
}

// UninstallService is a safe no-op on generic Unix hosts.
func UninstallService() error {
	return nil
}

// IsWindowsService always returns false on non-Windows hosts.
func IsWindowsService() bool {
	return false
}

// RunService invokes runFn directly on non-Windows hosts and closes stopCh on SIGINT or SIGTERM.
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
