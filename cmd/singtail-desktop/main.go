package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/singtail/singtail/internal/contract"
	"github.com/singtail/singtail/internal/daemon"
	"github.com/singtail/singtail/internal/syswin"
)

//go:embed all:frontendDist
var embeddedFrontend embed.FS

const (
	defaultDaemonAddr = "127.0.0.1:4189"
	probeTimeout      = 600 * time.Millisecond
	shutdownTimeout   = 5 * time.Second
)

func main() {
	listenAddr := flag.String("listen", defaultDaemonAddr, "local daemon address")
	dataDir := flag.String("data-dir", "", "directory for local state and traffic database")
	realProbe := flag.Bool("real-probe", false, "enable real HTTP egress probing and provider fetching")
	flag.Parse()

	resolvedDataDir := resolveDesktopDataDir(*dataDir)
	staticFS, err := fs.Sub(embeddedFrontend, "frontendDist")
	if err != nil {
		log.Fatalf("load embedded frontend: %v", err)
	}

	apiBaseURL, err := buildClientBaseURL(*listenAddr)
	if err != nil {
		log.Fatalf("%v", err)
	}
	uiURL := apiBaseURL

	var httpSrv *http.Server
	var embeddedDaemon *daemon.Server

	online, servesUI := probeLocalDaemon(apiBaseURL)
	if !online {
		embeddedDaemon, err = daemon.NewServer(resolvedDataDir, staticFS, *realProbe)
		if err != nil {
			log.Fatalf("start embedded daemon: %v", err)
		}
		ln, listenErr := net.Listen("tcp", *listenAddr)
		if listenErr != nil {
			_ = embeddedDaemon.Close()
			log.Fatalf("listen %s: %v", *listenAddr, listenErr)
		}
		httpSrv = &http.Server{
			Addr:              *listenAddr,
			Handler:           embeddedDaemon,
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			if serveErr := httpSrv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				log.Printf("embedded daemon stopped: %v", serveErr)
			}
		}()
	} else if !servesUI {
		// External headless daemon is online without static UI; host embedded UI on loopback and proxy /api.
		proxyHandler, proxyErr := buildUIProxyHandler(staticFS, apiBaseURL)
		if proxyErr != nil {
			log.Fatalf("create UI proxy: %v", proxyErr)
		}
		ln, listenErr := net.Listen("tcp", "127.0.0.1:0")
		if listenErr != nil {
			log.Fatalf("listen local UI proxy: %v", listenErr)
		}
		uiURL = "http://" + ln.Addr().String()
		httpSrv = &http.Server{
			Addr:              ln.Addr().String(),
			Handler:           proxyHandler,
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			if serveErr := httpSrv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				log.Printf("UI proxy stopped: %v", serveErr)
			}
		}()
	}

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	requestStop := func() {
		stopOnce.Do(func() { close(stopCh) })
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			requestStop()
		case <-stopCh:
		}
		signal.Stop(sigCh)
	}()

	openWindow := func() {
		if launchErr := openAppWindow(uiURL, resolvedDataDir); launchErr != nil {
			log.Printf("open desktop window failed: %v", launchErr)
		}
	}

	// Launch primary window immediately on startup.
	openWindow()

	trayCfg := syswin.TrayConfig{
		Tooltip: "SingTail — 多组织出口网关",
		OnOpen:  openWindow,
		OnToggle: func() {
			if err := toggleTunViaAPI(apiBaseURL); err != nil {
				log.Printf("toggle routing failed: %v", err)
			}
		},
		OnQuit: requestStop,
	}

	if trayErr := syswin.RunTray(trayCfg, stopCh); trayErr != nil {
		log.Printf("system tray exited: %v", trayErr)
		<-stopCh
	}

	if httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		_ = httpSrv.Shutdown(ctx)
		cancel()
	}
	if embeddedDaemon != nil {
		_ = embeddedDaemon.Close()
	}
}

func resolveDesktopDataDir(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		return filepath.Clean(trimmed)
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "data")
	}
	return "./data"
}

func probeLocalDaemon(baseURL string) (online bool, servesUI bool) {
	client := &http.Client{Timeout: probeTimeout}

	snapResp, err := client.Get(baseURL + "/api/snapshot?period=live")
	if err != nil {
		return false, false
	}
	_ = snapResp.Body.Close()
	if snapResp.StatusCode != http.StatusOK {
		return false, false
	}

	uiResp, err := client.Get(baseURL + "/")
	if err != nil {
		return true, false
	}
	_ = uiResp.Body.Close()
	return true, uiResp.StatusCode == http.StatusOK
}

func buildUIProxyHandler(staticFS fs.FS, apiBaseURL string) (http.Handler, error) {
	target, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cleanPath := path.Clean("/" + r.URL.Path)
		relPath := strings.TrimPrefix(cleanPath, "/")
		if relPath != "" && relPath != "index.html" {
			if info, statErr := fs.Stat(staticFS, relPath); statErr == nil && !info.IsDir() {
				http.FileServer(http.FS(staticFS)).ServeHTTP(w, r)
				return
			}
		}
		indexBytes, readErr := fs.ReadFile(staticFS, "index.html")
		if readErr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexBytes)
	})
	return mux, nil
}

func openAppWindow(targetURL, dataDir string) error {
	if runtime.GOOS == "windows" {
		if edgePath, ok := findEdgeExecutable(); ok {
			profileDir := filepath.Join(dataDir, "edge-app-profile")
			args := []string{
				fmt.Sprintf("--app=%s", targetURL),
				"--window-size=1360,860",
				fmt.Sprintf("--user-data-dir=%s", profileDir),
				"--no-first-run",
				"--no-default-browser-check",
			}
			cmd := exec.Command(edgePath, args...)
			if err := cmd.Start(); err == nil {
				go func() {
					_ = cmd.Wait()
				}()
				return nil
			}
		}
	}
	return syswin.OpenBrowser(targetURL)
}

func findEdgeExecutable() (string, bool) {
	if p, err := exec.LookPath("msedge.exe"); err == nil {
		return p, true
	}
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}
	for _, c := range candidates {
		if strings.TrimSpace(c) == "" {
			continue
		}
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, true
		}
	}
	return "", false
}

func buildClientBaseURL(listenAddr string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil {
		return "", fmt.Errorf("invalid -listen address %q: %w", listenAddr, err)
	}
	switch strings.TrimSpace(host) {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port), nil
}

func toggleTunViaAPI(apiBaseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(apiBaseURL + "/api/snapshot?period=live")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /api/snapshot returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var snap contract.DashboardSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return err
	}

	payload, err := json.Marshal(contract.SetTunRequest{Enabled: !snap.TunEnabled})
	if err != nil {
		return err
	}
	postResp, err := client.Post(apiBaseURL+"/api/tun", "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusOK {
		postBody, _ := io.ReadAll(io.LimitReader(postResp.Body, 1<<20))
		return fmt.Errorf("POST /api/tun returned HTTP %d: %s", postResp.StatusCode, strings.TrimSpace(string(postBody)))
	}
	return nil
}
