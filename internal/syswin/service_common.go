package syswin

import (
	"path/filepath"
	"strings"
)

const (
	// ServiceName is the Windows SCM service identifier for SingTail's background daemon.
	ServiceName = "SingTailService"
	// ServiceDisplayName is shown in Windows Services (services.msc).
	ServiceDisplayName = "SingTail Core Service"
	// ServiceDescription explains the background service purpose in services.msc.
	ServiceDescription = "SingTail background daemon managing multi-tailnet exit nodes and sing-box TUN routing."
)

// ServiceInstallOptions controls optional data directory binding and startup behavior when installing SingTailService.
type ServiceInstallOptions struct {
	DataDir   string
	AutoStart *bool
}

// ServiceStatusInfo reports whether the background service is registered and actively running.
type ServiceStatusInfo struct {
	Installed  bool
	Running    bool
	AutoStart  bool
	BinaryPath string
}

func buildServiceStartArgs(opts ServiceInstallOptions) []string {
	trimmedDir := strings.TrimSpace(opts.DataDir)
	if trimmedDir == "" {
		return nil
	}
	absDir, err := filepath.Abs(trimmedDir)
	if err != nil {
		absDir = filepath.Clean(trimmedDir)
	}
	return []string{"-data-dir", absDir}
}
