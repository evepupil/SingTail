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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	demo := flag.Bool("demo", false, "enable demo mock data for UI preview")
	realProbe := flag.Bool("real-probe", true, "enable real network probing and sing-box runtime")
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

	var httpSrv *http.Server
	var embeddedDaemon *daemon.Server
	var apiHandler http.Handler

	if !probeLocalDaemon(apiBaseURL) {
		embeddedDaemon, err = daemon.NewServerWithOptions(daemon.Options{
			DataDir:          resolvedDataDir,
			StaticFS:         staticFS,
			RealNetworkProbe: *realProbe,
			DemoMode:         *demo,
		})
		if err != nil {
			log.Fatalf("start embedded daemon: %v", err)
		}
		apiHandler = embeddedDaemon

		ln, listenErr := net.Listen("tcp", *listenAddr)
		if listenErr == nil {
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
		}
	} else {
		proxyHandler, proxyErr := buildAPIProxyHandler(apiBaseURL)
		if proxyErr != nil {
			log.Fatalf("create API proxy: %v", proxyErr)
		}
		apiHandler = proxyHandler
	}

	if runErr := runDesktopWindow(staticFS, apiHandler, apiBaseURL); runErr != nil {
		log.Printf("desktop window exited: %v", runErr)
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

	switch runtime.GOOS {
	case "windows":
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
		return "./data"
	case "darwin":
		if os.Geteuid() == 0 {
			return "/Library/Application Support/SingTail"
		}
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			return filepath.Join(home, "Library", "Application Support", "SingTail")
		}
		return "./data"
	default:
		if os.Geteuid() == 0 {
			return "/var/lib/singtail"
		}
		if cfgDir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(cfgDir) != "" {
			return filepath.Join(cfgDir, "SingTail")
		}
		return "./data"
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

func probeLocalDaemon(baseURL string) bool {
	client := &http.Client{Timeout: probeTimeout}
	snapResp, err := client.Get(baseURL + "/api/snapshot?period=live")
	if err != nil {
		return false
	}
	_ = snapResp.Body.Close()
	return snapResp.StatusCode == http.StatusOK
}

func buildAPIProxyHandler(apiBaseURL string) (http.Handler, error) {
	target, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Header.Set("Origin", apiBaseURL)
		if req.Method == http.MethodGet && req.URL.Path == "/api/snapshot" {
			req.Header.Del("Accept-Encoding")
		}
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.Request == nil || resp.Request.Method != http.MethodGet || resp.Request.URL.Path != "/api/snapshot" || resp.StatusCode != http.StatusOK {
			return nil
		}
		if resp.Header.Get("X-SingTail-Build") == daemon.DaemonBuildTag {
			return nil
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		var snap contract.DashboardSnapshot
		if json.Unmarshal(body, &snap) == nil && snap.BannerNotice == nil {
			snap.BannerNotice = &contract.BannerNotice{
				Kind: "rollback",
				Text: "后台系统服务仍是旧版本，请以管理员身份运行新包里的 install-service.bat 升级服务",
			}
			if updated, marshalErr := json.Marshal(snap); marshalErr == nil {
				updated = append(updated, '\n')
				body = updated
			}
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}
	return proxy, nil
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

func buildLiveTrayActions(apiBaseURL string) []syswin.TrayAction {
	snap, err := fetchLiveSnapshot(apiBaseURL)
	if err != nil {
		return []syswin.TrayAction{
			{
				Label: "切换 TUN 开关",
				OnClick: func() {
					_ = toggleTunViaAPI(apiBaseURL)
				},
			},
		}
	}

	tunLabel := "开启 TUN 虚拟网卡"
	if snap.TunEnabled {
		tunLabel = "TUN 已开启"
	}
	nextTun := !snap.TunEnabled

	actions := []syswin.TrayAction{
		{
			Label:   tunLabel,
			Checked: snap.TunEnabled,
			OnClick: func() {
				_ = postJSONViaAPI(apiBaseURL+"/api/tun", contract.SetTunRequest{Enabled: nextTun})
			},
		},
		{Separator: true},
		{
			Label:   "模式：规则",
			Checked: snap.RoutingMode == contract.ModeRule,
			OnClick: func() {
				_ = postJSONViaAPI(apiBaseURL+"/api/mode", contract.SetModeRequest{Mode: contract.ModeRule})
			},
		},
		{
			Label:   "模式：全局",
			Checked: snap.RoutingMode == contract.ModeGlobal,
			OnClick: func() {
				_ = postJSONViaAPI(apiBaseURL+"/api/mode", contract.SetModeRequest{Mode: contract.ModeGlobal})
			},
		},
		{
			Label:   "模式：直连",
			Checked: snap.RoutingMode == contract.ModeDirect,
			OnClick: func() {
				_ = postJSONViaAPI(apiBaseURL+"/api/mode", contract.SetModeRequest{Mode: contract.ModeDirect})
			},
		},
	}

	if len(snap.ExitNodes) > 0 {
		actions = append(actions, syswin.TrayAction{Separator: true})
		for _, node := range snap.ExitNodes {
			nodeID := node.ID
			ipText := strings.TrimSpace(node.PublicIP)
			if ipText == "" || ipText == "-" {
				ipText = strings.TrimSpace(node.TailscaleIP)
			}
			label := "出口：" + node.Label
			if ipText != "" {
				label = fmt.Sprintf("出口：%s (%s)", node.Label, ipText)
			}
			actions = append(actions, syswin.TrayAction{
				Label:   label,
				Checked: nodeID == snap.ActiveExitID,
				OnClick: func() {
					_ = postJSONViaAPI(apiBaseURL+"/api/exit/switch", contract.SwitchExitRequest{ExitID: nodeID})
				},
			})
		}
	}

	return actions
}

func fetchLiveSnapshot(apiBaseURL string) (contract.DashboardSnapshot, error) {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(apiBaseURL + "/api/snapshot?period=live")
	if err != nil {
		return contract.DashboardSnapshot{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return contract.DashboardSnapshot{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return contract.DashboardSnapshot{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var snap contract.DashboardSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return contract.DashboardSnapshot{}, err
	}
	return snap, nil
}

func toggleTunViaAPI(apiBaseURL string) error {
	snap, err := fetchLiveSnapshot(apiBaseURL)
	if err != nil {
		return err
	}
	return postJSONViaAPI(apiBaseURL+"/api/tun", contract.SetTunRequest{Enabled: !snap.TunEnabled})
}

func postJSONViaAPI(endpoint string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("POST %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
