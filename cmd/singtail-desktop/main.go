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
	"sync"
	"sync/atomic"
	"time"

	"github.com/singtail/singtail/internal/contract"
	"github.com/singtail/singtail/internal/daemon"
	"github.com/singtail/singtail/internal/syswin"
)

//go:embed all:frontendDist
var embeddedFrontend embed.FS

const (
	defaultDaemonAddr    = "127.0.0.1:4189"
	probeTimeout         = 600 * time.Millisecond
	shutdownTimeout      = 5 * time.Second
	serviceReadyWait     = 10 * time.Second
	serviceReadyPollStep = 250 * time.Millisecond
)

var errRelaunchingElevated = errors.New("relaunching elevated desktop instance")

type daemonProbeInfo struct {
	reachable  bool
	buildTag   string
	privileged bool
}

type desktopRuntime struct {
	listenAddr string
	apiBaseURL string
	dataDir    string
	serviceDir string
	staticFS   fs.FS
	realProbe  bool
	demo       bool

	mu             sync.Mutex
	httpSrv        *http.Server
	embeddedDaemon *daemon.Server
	currentHandler atomic.Pointer[http.Handler]
}

func (m *desktopRuntime) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/api/privilege/elevate" {
		m.handleElevatePrivilege(w)
		return
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/tun" && m.needsPrivilegeBeforeTun(r) {
		m.handleTunWithAutoElevate(w, r)
		return
	}
	h := m.currentHandler.Load()
	if h == nil || *h == nil {
		http.Error(w, `{"error":"守护服务尚未就绪"}`, http.StatusServiceUnavailable)
		return
	}
	(*h).ServeHTTP(w, r)
}

func (m *desktopRuntime) needsPrivilegeBeforeTun(r *http.Request) bool {
	if runtime.GOOS != "windows" || m.demo || !m.realProbe || syswin.IsElevated() {
		return false
	}
	m.mu.Lock()
	hasEmbedded := m.embeddedDaemon != nil
	m.mu.Unlock()
	if !hasEmbedded {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		return false
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	var req contract.SetTunRequest
	if json.Unmarshal(body, &req) != nil {
		return false
	}
	return req.Enabled
}

func (m *desktopRuntime) handleElevatePrivilege(w http.ResponseWriter) {
	if err := m.elevateRuntime(); err != nil {
		if errors.Is(err, errRelaunchingElevated) {
			writeDesktopJSON(w, http.StatusOK, map[string]any{"relaunching": true})
			go func() {
				time.Sleep(150 * time.Millisecond)
				os.Exit(0)
			}()
			return
		}
		writeDesktopJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	h := m.currentHandler.Load()
	if h == nil || *h == nil {
		writeDesktopJSONError(w, http.StatusServiceUnavailable, "后台服务连接失败")
		return
	}
	req, err := http.NewRequest(http.MethodGet, "/api/snapshot?period=live", nil)
	if err != nil {
		writeDesktopJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	(*h).ServeHTTP(w, req)
}

func (m *desktopRuntime) handleTunWithAutoElevate(w http.ResponseWriter, r *http.Request) {
	if err := m.elevateRuntime(); err != nil {
		if errors.Is(err, errRelaunchingElevated) {
			writeDesktopJSONError(w, http.StatusConflict, "正在以管理员权限重启，请在新窗口中开启 TUN")
			go func() {
				time.Sleep(150 * time.Millisecond)
				os.Exit(0)
			}()
			return
		}
		writeDesktopJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	h := m.currentHandler.Load()
	if h == nil || *h == nil {
		writeDesktopJSONError(w, http.StatusServiceUnavailable, "后台服务尚未就绪")
		return
	}
	(*h).ServeHTTP(w, r)
}

func (m *desktopRuntime) bootstrap(allowElevate bool) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.demo && m.realProbe {
		info := probeDaemonState(m.apiBaseURL)
		serviceExe := findSiblingServiceExe()

		if info.reachable && info.privileged && (info.buildTag == daemon.DaemonBuildTag || serviceExe == "") {
			if err := m.attachProxyLocked(); err != nil {
				return false, err
			}
			return false, nil
		}

		if runtime.GOOS == "windows" {
			if info.reachable && info.buildTag != daemon.DaemonBuildTag && serviceExe != "" {
				if svcInfo, err := syswin.QueryServiceStatus(); err == nil && svcInfo.Installed && allowElevate {
					cancelled, setupErr := m.setupAndStartServiceLocked(serviceExe)
					if !cancelled && setupErr == nil && waitForDaemonReady(m.apiBaseURL, serviceReadyWait) {
						if err := m.attachProxyLocked(); err != nil {
							return false, err
						}
						return false, nil
					}
				}
			}

			if !info.reachable {
				if syswin.IsElevated() {
					if serviceExe != "" {
						if setupErr := installAndStartServiceDirect(serviceExe, m.serviceDir); setupErr == nil && waitForDaemonReady(m.apiBaseURL, serviceReadyWait) {
							if err := m.attachProxyLocked(); err != nil {
								return false, err
							}
							return false, nil
						}
					}
				} else {
					svcInfo, _ := syswin.QueryServiceStatus()
					if svcInfo.Installed && !svcInfo.Running {
						if startErr := syswin.StartInstalledService(); startErr == nil && waitForDaemonReady(m.apiBaseURL, serviceReadyWait) {
							if err := m.attachProxyLocked(); err != nil {
								return false, err
							}
							return false, nil
						}
					}

					if allowElevate {
						if serviceExe != "" {
							cancelled, setupErr := m.setupAndStartServiceLocked(serviceExe)
							if !cancelled && setupErr == nil && waitForDaemonReady(m.apiBaseURL, serviceReadyWait) {
								if err := m.attachProxyLocked(); err != nil {
									return false, err
								}
								return false, nil
							}
						} else {
							cancelled, relaunchErr := syswin.RelaunchSelfElevated(buildRelaunchArgs(m.listenAddr, m.dataDir, m.demo, m.realProbe)...)
							if relaunchErr == nil && !cancelled {
								return true, nil
							}
						}
					}
				}
			}
		}

		if info.reachable {
			if err := m.attachProxyLocked(); err != nil {
				return false, err
			}
			return false, nil
		}
	} else if probeLocalDaemon(m.apiBaseURL) {
		if err := m.attachProxyLocked(); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := m.startEmbeddedLocked(); err != nil {
		return false, err
	}
	return false, nil
}

func (m *desktopRuntime) elevateRuntime() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if info := probeDaemonState(m.apiBaseURL); info.reachable && info.privileged && info.buildTag == daemon.DaemonBuildTag && m.embeddedDaemon == nil {
		return m.attachProxyLocked()
	}

	m.stopEmbeddedLocked()

	serviceExe := findSiblingServiceExe()
	if runtime.GOOS == "windows" && serviceExe != "" {
		var (
			cancelled bool
			setupErr  error
		)
		if syswin.IsElevated() {
			setupErr = installAndStartServiceDirect(serviceExe, m.serviceDir)
		} else {
			cancelled, setupErr = m.setupAndStartServiceLocked(serviceExe)
		}
		if !cancelled && setupErr == nil && waitForDaemonReady(m.apiBaseURL, serviceReadyWait) {
			return m.attachProxyLocked()
		}

		_ = m.startEmbeddedLocked()
		if cancelled {
			return errors.New("已取消管理员授权，开启虚拟网卡需要安装并启动系统后台服务")
		}
		if setupErr != nil {
			return fmt.Errorf("启动系统服务失败: %w", setupErr)
		}
		return errors.New("后台系统服务未响应控制端口")
	}

	if runtime.GOOS == "windows" && !syswin.IsElevated() {
		cancelled, err := syswin.RelaunchSelfElevated(buildRelaunchArgs(m.listenAddr, m.dataDir, m.demo, m.realProbe)...)
		if err == nil && !cancelled {
			return errRelaunchingElevated
		}
		_ = m.startEmbeddedLocked()
		if cancelled {
			return errors.New("已取消管理员授权")
		}
		return err
	}

	return m.startEmbeddedLocked()
}

func (m *desktopRuntime) setupAndStartServiceLocked(serviceExe string) (bool, error) {
	selfExe, err := os.Executable()
	if err != nil {
		return false, err
	}
	args := []string{
		"--elevated-setup-service",
		"--listen", m.listenAddr,
		"--service-exe", serviceExe,
	}
	if strings.TrimSpace(m.serviceDir) != "" {
		args = append(args, "--data-dir", m.serviceDir)
	}
	cancelled, exitCode, runErr := syswin.RunElevatedWait(selfExe, true, args...)
	if cancelled {
		return true, nil
	}
	if runErr != nil {
		return false, runErr
	}
	if exitCode != 0 {
		return false, fmt.Errorf("elevated service setup exited with code %d", exitCode)
	}
	return false, nil
}

func (m *desktopRuntime) attachProxyLocked() error {
	proxyHandler, err := buildAPIProxyHandler(m.apiBaseURL)
	if err != nil {
		return err
	}
	m.currentHandler.Store(&proxyHandler)
	return nil
}

func (m *desktopRuntime) startEmbeddedLocked() error {
	embedded, err := daemon.NewServerWithOptions(daemon.Options{
		DataDir:          m.dataDir,
		StaticFS:         m.staticFS,
		RealNetworkProbe: m.realProbe,
		DemoMode:         m.demo,
	})
	if err != nil {
		return fmt.Errorf("start embedded daemon: %w", err)
	}

	var handler http.Handler = embedded
	m.embeddedDaemon = embedded
	m.currentHandler.Store(&handler)

	ln, listenErr := net.Listen("tcp", m.listenAddr)
	if listenErr == nil {
		httpSrv := &http.Server{
			Addr:              m.listenAddr,
			Handler:           embedded,
			ReadHeaderTimeout: 10 * time.Second,
		}
		m.httpSrv = httpSrv
		go func() {
			if serveErr := httpSrv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				log.Printf("embedded daemon stopped: %v", serveErr)
			}
		}()
	}
	return nil
}

func (m *desktopRuntime) stopEmbeddedLocked() {
	if m.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		_ = m.httpSrv.Shutdown(ctx)
		cancel()
		m.httpSrv = nil
	}
	if m.embeddedDaemon != nil {
		_ = m.embeddedDaemon.Close()
		m.embeddedDaemon = nil
	}
}

func (m *desktopRuntime) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopEmbeddedLocked()
}

func main() {
	listenAddr := flag.String("listen", defaultDaemonAddr, "local daemon address")
	dataDir := flag.String("data-dir", "", "directory for local state and traffic database")
	demo := flag.Bool("demo", false, "enable demo mock data for UI preview")
	realProbe := flag.Bool("real-probe", true, "enable real network probing and sing-box runtime")
	noElevate := flag.Bool("no-elevate", false, "skip automatic UAC elevation on startup")
	elevatedSetupService := flag.Bool("elevated-setup-service", false, "internal helper to install and start SingTailService")
	serviceExeFlag := flag.String("service-exe", "", "path to singtail-service executable")
	flag.Parse()

	apiBaseURL, err := buildClientBaseURL(*listenAddr)
	if err != nil {
		log.Fatalf("%v", err)
	}

	if *elevatedSetupService {
		svcExe := strings.TrimSpace(*serviceExeFlag)
		if svcExe == "" {
			svcExe = findSiblingServiceExe()
		}
		if svcExe == "" {
			log.Fatal("singtail-service executable not found")
		}
		if err := installAndStartServiceDirect(svcExe, strings.TrimSpace(*dataDir)); err != nil {
			log.Fatalf("elevated setup service failed: %v", err)
		}
		if !waitForDaemonReady(apiBaseURL, serviceReadyWait) {
			log.Fatal("SingTailService did not answer in time after start")
		}
		return
	}

	resolvedDataDir := resolveDesktopDataDir(*dataDir)
	resolvedServiceDir := resolveServiceBindDataDir(*dataDir)
	staticFS, err := fs.Sub(embeddedFrontend, "frontendDist")
	if err != nil {
		log.Fatalf("load embedded frontend: %v", err)
	}

	mgr := &desktopRuntime{
		listenAddr: *listenAddr,
		apiBaseURL: apiBaseURL,
		dataDir:    resolvedDataDir,
		serviceDir: resolvedServiceDir,
		staticFS:   staticFS,
		realProbe:  *realProbe,
		demo:       *demo,
	}

	relaunched, err := mgr.bootstrap(!*noElevate)
	if err != nil {
		log.Fatalf("%v", err)
	}
	if relaunched {
		return
	}
	defer mgr.close()

	if runErr := runDesktopWindow(staticFS, mgr, apiBaseURL); runErr != nil {
		log.Printf("desktop window exited: %v", runErr)
	}
}

func installAndStartServiceDirect(serviceExe, dataDir string) error {
	if err := syswin.InstallServiceWithOptions(serviceExe, syswin.ServiceInstallOptions{DataDir: dataDir}); err != nil {
		return err
	}
	return syswin.StartOrRestartService(true)
}

func findSiblingServiceExe() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	name := "singtail-service"
	if runtime.GOOS == "windows" {
		name = "singtail-service.exe"
	}
	candidate := filepath.Join(filepath.Dir(exe), name)
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return ""
	}
	return candidate
}

func resolveServiceBindDataDir(explicitFlag string) string {
	trimmed := strings.TrimSpace(explicitFlag)
	if trimmed != "" {
		abs, err := filepath.Abs(trimmed)
		if err == nil {
			return abs
		}
		return filepath.Clean(trimmed)
	}
	if runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			portable := filepath.Join(exeDir, "data")
			if !isProgramFilesDir(exeDir) && isDirWritable(portable) {
				return portable
			}
		}
	}
	return ""
}

func buildRelaunchArgs(listenAddr, dataDir string, demo, realProbe bool) []string {
	args := []string{"--no-elevate", "--listen", listenAddr}
	if strings.TrimSpace(dataDir) != "" {
		args = append(args, "--data-dir", dataDir)
	}
	if demo {
		args = append(args, "--demo")
	}
	if !realProbe {
		args = append(args, "--real-probe=false")
	}
	return args
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
	return probeDaemonState(baseURL).reachable
}

func probeDaemonState(baseURL string) daemonProbeInfo {
	client := &http.Client{Timeout: probeTimeout}
	snapResp, err := client.Get(baseURL + "/api/snapshot?period=live")
	if err != nil {
		return daemonProbeInfo{}
	}
	defer func() { _ = snapResp.Body.Close() }()
	if snapResp.StatusCode != http.StatusOK {
		return daemonProbeInfo{}
	}

	body, err := io.ReadAll(io.LimitReader(snapResp.Body, 4<<20))
	if err != nil {
		return daemonProbeInfo{
			reachable: true,
			buildTag:  snapResp.Header.Get("X-SingTail-Build"),
		}
	}
	var snap contract.DashboardSnapshot
	privileged := true
	if json.Unmarshal(body, &snap) == nil {
		privileged = snap.Privileged
	}
	return daemonProbeInfo{
		reachable:  true,
		buildTag:   snapResp.Header.Get("X-SingTail-Build"),
		privileged: privileged,
	}
}

func waitForDaemonReady(baseURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if probeLocalDaemon(baseURL) {
			return true
		}
		time.Sleep(serviceReadyPollStep)
	}
	return false
}

func writeDesktopJSON(w http.ResponseWriter, status int, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, `{"error":"internal encode error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
	_, _ = w.Write([]byte("\n"))
}

func writeDesktopJSONError(w http.ResponseWriter, status int, msg string) {
	writeDesktopJSON(w, status, map[string]string{"error": msg})
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
				Text: "后台系统服务仍是旧版本，请点击重新授权或以管理员身份运行新包里的 install-service.bat 升级服务",
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
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("POST %s returned HTTP %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
