//go:build windows

package syswin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	// ServiceName is the Windows SCM service identifier for SingTail's background daemon.
	ServiceName = "SingTailService"
	// ServiceDisplayName is shown in Windows Services (services.msc).
	ServiceDisplayName = "SingTail Core Service"
	// ServiceDescription explains the background service purpose in services.msc.
	ServiceDescription = "SingTail background daemon managing multi-tailnet exit nodes and sing-box TUN routing."
)

// InstallService registers SingTailService with automatic startup and crash recovery.
func InstallService(exePath string) error {
	resolvedPath, err := resolveServiceExecutable(exePath)
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect service control manager: %w", err)
	}
	defer m.Disconnect()

	if existing, err := m.OpenService(ServiceName); err == nil {
		defer existing.Close()
		cfg, cfgErr := existing.Config()
		if cfgErr == nil {
			cfg.BinaryPathName = syscall.EscapeArg(resolvedPath)
			cfg.DisplayName = ServiceDisplayName
			cfg.Description = ServiceDescription
			cfg.StartType = mgr.StartAutomatic
			if updateErr := existing.UpdateConfig(cfg); updateErr != nil {
				return fmt.Errorf("update existing service config: %w", updateErr)
			}
		}
		return applyRecoveryPolicy(existing)
	}

	s, err := m.CreateService(ServiceName, resolvedPath, mgr.Config{
		DisplayName: ServiceDisplayName,
		Description: ServiceDescription,
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		return fmt.Errorf("create service %s: %w", ServiceName, err)
	}
	defer s.Close()

	return applyRecoveryPolicy(s)
}

func resolveServiceExecutable(exePath string) (string, error) {
	if exePath == "" {
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

func applyRecoveryPolicy(s *mgr.Service) error {
	actions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}
	if err := s.SetRecoveryActions(actions, 86400); err != nil {
		return fmt.Errorf("configure service recovery policy: %w", err)
	}
	return nil
}

// UninstallService stops SingTailService if running and removes it from Windows SCM.
func UninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect service control manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return fmt.Errorf("open service %s: %w", ServiceName, err)
	}
	defer s.Close()

	if status, err := s.Query(); err == nil && status.State != svc.Stopped {
		if _, stopErr := s.Control(svc.Stop); stopErr == nil {
			waitServiceStopped(s, 10*time.Second)
		}
	}

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service %s: %w", ServiceName, err)
	}
	return nil
}

func waitServiceStopped(s *mgr.Service, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil || status.State == svc.Stopped {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// IsWindowsService reports whether the current process was launched by the Windows Service Control Manager.
func IsWindowsService() bool {
	ok, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return ok
}

// RunService runs runFn inside a Windows SCM service lifecycle, signaling stopCh on Stop or Shutdown.
func RunService(runFn func(stopCh <-chan struct{}) error) error {
	if runFn == nil {
		return nil
	}
	return svc.Run(ServiceName, &serviceHandler{runFn: runFn})
}

type serviceHandler struct {
	runFn func(stopCh <-chan struct{}) error
}

func (h *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const acceptedCmds = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	closeStop := func() {
		stopOnce.Do(func() { close(stopCh) })
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.runFn(stopCh)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: acceptedCmds}

	for {
		select {
		case err := <-errCh:
			if err != nil {
				return false, 1
			}
			return false, 0
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				changes <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				closeStop()
				select {
				case err := <-errCh:
					if err != nil {
						return false, 1
					}
					return false, 0
				case <-time.After(15 * time.Second):
					return false, 0
				}
			}
		}
	}
}
