//go:build windows

package syswin

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	seeMaskNoCloseProcess = 0x00000040
	swHide                = 0
	swShowNormal          = 1
)

var (
	modShell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = modShell32.NewProc("ShellExecuteExW")
)

type shellExecuteInfoW struct {
	CbSize       uint32
	FMask        uint32
	Hwnd         windows.Handle
	LpVerb       *uint16
	LpFile       *uint16
	LpParameters *uint16
	LpDirectory  *uint16
	NShow        int32
	HInstApp     windows.Handle
	LpIDList     uintptr
	LpClass      *uint16
	HkeyClass    windows.Handle
	DwHotKey     uint32
	HIcon        windows.Handle
	HProcess     windows.Handle
}

// IsElevated reports whether the current process runs as Windows Administrator or LocalSystem service.
func IsElevated() bool {
	if IsWindowsService() {
		return true
	}
	return windows.GetCurrentProcessToken().IsElevated()
}

// InstallService registers SingTailService with automatic startup and crash recovery.
func InstallService(exePath string) error {
	return InstallServiceWithOptions(exePath, ServiceInstallOptions{})
}

// InstallServiceWithOptions registers or updates SingTailService with optional bound data directory and start mode.
func InstallServiceWithOptions(exePath string, opts ServiceInstallOptions) error {
	resolvedPath, err := resolveServiceExecutable(exePath)
	if err != nil {
		return err
	}

	args := buildServiceStartArgs(opts)
	startType := uint32(mgr.StartAutomatic)
	if opts.AutoStart != nil && !*opts.AutoStart {
		startType = mgr.StartManual
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
			cfg.BinaryPathName = buildWindowsCommandLine(resolvedPath, args...)
			cfg.DisplayName = ServiceDisplayName
			cfg.Description = ServiceDescription
			cfg.StartType = startType
			if updateErr := existing.UpdateConfig(cfg); updateErr != nil {
				return fmt.Errorf("update existing service config: %w", updateErr)
			}
		}
		return applyRecoveryPolicy(existing)
	}

	s, err := m.CreateService(ServiceName, resolvedPath, mgr.Config{
		DisplayName: ServiceDisplayName,
		Description: ServiceDescription,
		StartType:   startType,
	}, args...)
	if err != nil {
		return fmt.Errorf("create service %s: %w", ServiceName, err)
	}
	defer s.Close()

	return applyRecoveryPolicy(s)
}

func resolveServiceExecutable(exePath string) (string, error) {
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

func buildWindowsCommandLine(exePath string, args ...string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, syscall.EscapeArg(exePath))
	for _, arg := range args {
		parts = append(parts, syscall.EscapeArg(arg))
	}
	return strings.Join(parts, " ")
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

// QueryServiceStatus inspects SingTailService registration and runtime state using read-only SCM permissions.
func QueryServiceStatus() (ServiceStatusInfo, error) {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return ServiceStatusInfo{}, fmt.Errorf("open service control manager: %w", err)
	}
	defer func() { _ = windows.CloseServiceHandle(scm) }()

	namePtr, err := windows.UTF16PtrFromString(ServiceName)
	if err != nil {
		return ServiceStatusInfo{}, err
	}
	handle, err := windows.OpenService(scm, namePtr, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return ServiceStatusInfo{}, nil
		}
		return ServiceStatusInfo{}, fmt.Errorf("open service %s: %w", ServiceName, err)
	}
	defer func() { _ = windows.CloseServiceHandle(handle) }()

	info := ServiceStatusInfo{Installed: true}

	var ssp windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if err := windows.QueryServiceStatusEx(handle, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&ssp)), uint32(unsafe.Sizeof(ssp)), &needed); err == nil {
		info.Running = ssp.CurrentState == windows.SERVICE_RUNNING
	}

	var cfgNeeded uint32
	_ = windows.QueryServiceConfig(handle, nil, 0, &cfgNeeded)
	if cfgNeeded >= uint32(unsafe.Sizeof(windows.QUERY_SERVICE_CONFIG{})) {
		buf := make([]byte, cfgNeeded)
		cfg := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
		if err := windows.QueryServiceConfig(handle, cfg, cfgNeeded, &cfgNeeded); err == nil {
			info.AutoStart = cfg.StartType == windows.SERVICE_AUTO_START
			info.BinaryPath = windows.UTF16PtrToString(cfg.BinaryPathName)
		}
	}

	return info, nil
}

// StartInstalledService starts SingTailService if it is not already running.
func StartInstalledService() error {
	return StartOrRestartService(false)
}

// StartOrRestartService ensures SingTailService is running, optionally restarting an already-running instance.
func StartOrRestartService(restartIfRunning bool) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect service control manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("open service %s: %w", ServiceName, err)
	}
	defer s.Close()

	status, err := s.Query()
	if err == nil {
		switch status.State {
		case svc.Running:
			if !restartIfRunning {
				return nil
			}
			if _, stopErr := s.Control(svc.Stop); stopErr != nil {
				return fmt.Errorf("stop service %s: %w", ServiceName, stopErr)
			}
			waitServiceStopped(s, 12*time.Second)
		case svc.StartPending:
			if !restartIfRunning {
				return waitServiceRunning(s, 12*time.Second)
			}
			waitServiceRunning(s, 8*time.Second)
			if _, stopErr := s.Control(svc.Stop); stopErr != nil {
				return fmt.Errorf("stop pending service %s: %w", ServiceName, stopErr)
			}
			waitServiceStopped(s, 12*time.Second)
		case svc.StopPending:
			waitServiceStopped(s, 12*time.Second)
		}
	}

	if err := s.Start(); err != nil {
		return fmt.Errorf("start service %s: %w", ServiceName, err)
	}
	return waitServiceRunning(s, 12*time.Second)
}

// SetServiceAutoStart updates the SCM startup mode of SingTailService when registered.
func SetServiceAutoStart(autoStart bool) error {
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

	cfg, err := s.Config()
	if err != nil {
		return fmt.Errorf("read service config: %w", err)
	}
	if autoStart {
		cfg.StartType = mgr.StartAutomatic
	} else {
		cfg.StartType = mgr.StartManual
	}
	if err := s.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("update service start type: %w", err)
	}
	return nil
}

// RunElevatedWait launches exePath with UAC elevation, waits for completion, and reports cancellation or exit code.
func RunElevatedWait(exePath string, hideWindow bool, args ...string) (cancelled bool, exitCode uint32, err error) {
	return shellExecuteElevated(exePath, hideWindow, true, args...)
}

// RelaunchSelfElevated launches a new elevated instance of the current executable without waiting for exit.
func RelaunchSelfElevated(args ...string) (cancelled bool, err error) {
	self, resolveErr := resolveServiceExecutable("")
	if resolveErr != nil {
		return false, resolveErr
	}
	cancelled, _, err = shellExecuteElevated(self, false, false, args...)
	return cancelled, err
}

func shellExecuteElevated(exePath string, hideWindow, waitExit bool, args ...string) (cancelled bool, exitCode uint32, err error) {
	resolved, resolveErr := resolveServiceExecutable(exePath)
	if resolveErr != nil {
		return false, 0, resolveErr
	}

	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return false, 0, err
	}
	filePtr, err := windows.UTF16PtrFromString(resolved)
	if err != nil {
		return false, 0, err
	}

	var paramsPtr *uint16
	if len(args) > 0 {
		escaped := make([]string, len(args))
		for i, a := range args {
			escaped[i] = syscall.EscapeArg(a)
		}
		paramsPtr, err = windows.UTF16PtrFromString(strings.Join(escaped, " "))
		if err != nil {
			return false, 0, err
		}
	}

	dirPtr, err := windows.UTF16PtrFromString(filepath.Dir(resolved))
	if err != nil {
		return false, 0, err
	}

	show := int32(swShowNormal)
	if hideWindow {
		show = swHide
	}

	sei := shellExecuteInfoW{
		CbSize:       uint32(unsafe.Sizeof(shellExecuteInfoW{})),
		FMask:        seeMaskNoCloseProcess,
		LpVerb:       verbPtr,
		LpFile:       filePtr,
		LpParameters: paramsPtr,
		LpDirectory:  dirPtr,
		NShow:        show,
	}

	r1, _, lastErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if r1 == 0 {
		if errors.Is(lastErr, windows.ERROR_CANCELLED) {
			return true, 0, nil
		}
		return false, 0, fmt.Errorf("elevated launch failed: %w", lastErr)
	}
	if sei.HProcess != 0 {
		defer func() { _ = windows.CloseHandle(sei.HProcess) }()
		if waitExit {
			waitRes, waitErr := windows.WaitForSingleObject(sei.HProcess, 35000)
			if waitErr != nil {
				return false, 0, fmt.Errorf("wait elevated process: %w", waitErr)
			}
			if waitRes == uint32(windows.WAIT_TIMEOUT) {
				return false, 0, errors.New("timed out waiting for elevated helper")
			}
			if err := windows.GetExitCodeProcess(sei.HProcess, &exitCode); err != nil {
				return false, 0, fmt.Errorf("read elevated helper exit code: %w", err)
			}
		}
	}
	return false, exitCode, nil
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

func waitServiceRunning(s *mgr.Service, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("query service status: %w", err)
		}
		if status.State == svc.Running {
			return nil
		}
		if status.State == svc.Stopped {
			return errors.New("service stopped immediately after start")
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("timed out waiting for service to reach Running state")
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
