package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/singtail/singtail/internal/daemon"
	"github.com/singtail/singtail/internal/syswin"
)

const (
	defaultListenAddr = "127.0.0.1:4189"
	defaultDataDir    = "./data"
	shutdownTimeout   = 5 * time.Second
)

func main() {
	listenAddr := flag.String("listen", defaultListenAddr, "HTTP listen address")
	dataDir := flag.String("data-dir", "", "directory for state and traffic database")
	webDir := flag.String("web-dir", "", "optional static frontend directory to serve")
	install := flag.Bool("install", false, "install background service")
	uninstall := flag.Bool("uninstall", false, "uninstall background service")
	demo := flag.Bool("demo", false, "enable demo mock data for UI preview")
	realProbe := flag.Bool("real-probe", true, "enable real network probing and sing-box runtime")
	flag.Parse()

	if *install && *uninstall {
		log.Fatal("cannot specify both -install and -uninstall")
	}
	if *install {
		if err := syswin.InstallService(""); err != nil {
			log.Fatalf("install service failed: %v", err)
		}
		fmt.Printf("Installed service %s (%s)\n", syswin.ServiceName, syswin.ServiceDisplayName)
		return
	}
	if *uninstall {
		if err := syswin.UninstallService(); err != nil {
			log.Fatalf("uninstall service failed: %v", err)
		}
		fmt.Printf("Uninstalled service %s\n", syswin.ServiceName)
		return
	}

	resolvedDataDir := resolveServiceDataDir(*dataDir, syswin.IsWindowsService())

	if syswin.IsWindowsService() {
		err := syswin.RunService(func(stopCh <-chan struct{}) error {
			return serve(stopCh, *listenAddr, resolvedDataDir, *webDir, *realProbe, *demo)
		})
		if err != nil {
			log.Fatalf("windows service failed: %v", err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := serve(ctx.Done(), *listenAddr, resolvedDataDir, *webDir, *realProbe, *demo); err != nil {
		log.Fatalf("singtail-service exited with error: %v", err)
	}
}

func resolveServiceDataDir(raw string, isService bool) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		if isService && !filepath.IsAbs(trimmed) {
			if pd := strings.TrimSpace(os.Getenv("ProgramData")); pd != "" {
				return filepath.Join(pd, "SingTail")
			}
			if exe, err := os.Executable(); err == nil {
				return filepath.Join(filepath.Dir(exe), trimmed)
			}
		}
		return filepath.Clean(trimmed)
	}

	switch runtime.GOOS {
	case "windows":
		if isService {
			if pd := strings.TrimSpace(os.Getenv("ProgramData")); pd != "" {
				return filepath.Join(pd, "SingTail")
			}
		}
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			portable := filepath.Join(exeDir, "data")
			if !isProgramFilesDir(exeDir) && isDirWritable(portable) {
				return portable
			}
		}
		if localAppData := strings.TrimSpace(os.Getenv("LocalAppData")); localAppData != "" {
			return filepath.Join(localAppData, "SingTail")
		}
		return defaultDataDir
	case "darwin":
		if os.Geteuid() == 0 {
			return "/Library/Application Support/SingTail"
		}
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			return filepath.Join(home, "Library", "Application Support", "SingTail")
		}
		return defaultDataDir
	default:
		if os.Geteuid() == 0 {
			return "/var/lib/singtail"
		}
		if cfgDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(cfgDir) != "" {
			return filepath.Join(cfgDir, "SingTail")
		}
		return defaultDataDir
	}
}

func isProgramFilesDir(dir string) bool {
	lower := strings.ToLower(filepath.Clean(dir))
	return strings.Contains(lower, `\program files`) || strings.Contains(lower, `/program files`)
}

func isDirWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	probe, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}

func serve(stopCh <-chan struct{}, listenAddr, dataDir, webDir string, realProbe, demo bool) error {
	var staticFS fs.FS
	if trimmedWeb := strings.TrimSpace(webDir); trimmedWeb != "" {
		info, err := os.Stat(trimmedWeb)
		if err != nil {
			return fmt.Errorf("stat -web-dir %q: %w", trimmedWeb, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("-web-dir %q is not a directory", trimmedWeb)
		}
		staticFS = os.DirFS(trimmedWeb)
	}

	srv, err := daemon.NewServerWithOptions(daemon.Options{
		DataDir:          dataDir,
		StaticFS:         staticFS,
		RealNetworkProbe: realProbe,
		DemoMode:         demo,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = srv.Close()
	}()

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", listenAddr, err)
	}

	httpSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("singtail-service listening on http://%s (data=%s, demo=%v, realProbe=%v)", ln.Addr().String(), dataDir, demo, realProbe)
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-stopCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		return <-errCh
	case err := <-errCh:
		return err
	}
}
