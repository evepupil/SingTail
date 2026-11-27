//go:build !windows && !darwin

package syswin

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const (
	// ServiceName is the Windows SCM service identifier for SingTail's background daemon.
	ServiceName = "SingTailService"
	// ServiceDisplayName is shown in Windows Services (services.msc).
	ServiceDisplayName = "SingTail Core Service"
	// ServiceDescription explains the background service purpose in services.msc.
	ServiceDescription = "SingTail background daemon managing multi-tailnet exit nodes and sing-box TUN routing."
)

// InstallService is a safe no-op on generic Unix hosts.
func InstallService(_ string) error {
	return nil
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
