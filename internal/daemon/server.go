// Package daemon provides the local HTTP control gateway and static UI hosting for SingTail.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/singtail/singtail/internal/configstore"
	"github.com/singtail/singtail/internal/contract"
	"github.com/singtail/singtail/internal/engine"
	"github.com/singtail/singtail/internal/rules"
	"github.com/singtail/singtail/internal/stats"
	"github.com/singtail/singtail/internal/syswin"
	"github.com/singtail/singtail/internal/tailnet"
)

const (
	// DaemonBuildTag identifies the active daemon feature generation for desktop mismatch detection.
	DaemonBuildTag = "20261128-stats-priv-fix"

	defaultDataDir     = "./data"
	defaultMixedProxy  = engine.DefaultMixedProxy
	maxJSONBodyBytes   = 2 << 20
	sampleInterval     = 1 * time.Second
	bytesPerMB         = 1024 * 1024
	exitSwitchTimeout  = 15 * time.Second
	runtimeStartTimout = 50 * time.Second
	providerFetchLimit = 16 << 20
)

// Options configures how a daemon Server initializes storage, tailnet discovery, and sing-box runtime.
type Options struct {
	DataDir          string
	StaticFS         fs.FS
	RealNetworkProbe bool
	DemoMode         bool
	ClashAPIAddr     string
}

// Server exposes the SingTail REST control API and optional embedded frontend assets.
type Server struct {
	dataDir          string
	staticFS         fs.FS
	realNetworkProbe bool
	demoMode         bool

	coreAvailable bool
	coreVersion   string

	cfgStore     *configstore.Store
	stats        *stats.Store
	tailnet      *tailnet.Coordinator
	rules        *rules.Manager
	ctrl         *engine.Controller
	rt           *engine.Runtime
	clashSampler *engine.ClashSampler

	mux        *http.ServeMux
	httpClient *http.Client

	mu       sync.RWMutex
	settings contract.SystemSettings

	clashTotalsMu  sync.Mutex
	trafficTracker TrafficDeltaTracker
	lastSampleAt   time.Time

	tunMu  sync.Mutex
	saveMu sync.Mutex

	samplerSeq atomic.Int64
	stopCh     chan struct{}
	wg         sync.WaitGroup
	closeOnce  sync.Once
}

// NewServer initializes the daemon in production zero-mock mode using default options.
func NewServer(dataDir string, staticFS fs.FS, realNetworkProbe bool) (*Server, error) {
	return NewServerWithOptions(Options{
		DataDir:          dataDir,
		StaticFS:         staticFS,
		RealNetworkProbe: realNetworkProbe,
		DemoMode:         false,
	})
}

// NewServerWithOptions initializes persistent storage, domain managers, optional demo seeding,
// sing-box runtime supervision, background telemetry sampling, and HTTP routes.
func NewServerWithOptions(opts Options) (*Server, error) {
	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	dataDir = filepath.Clean(dataDir)

	_ = engine.SeedBundledRules(dataDir)

	statePath := filepath.Join(dataDir, "state.json")
	_, statErr := os.Stat(statePath)
	isFreshState := errors.Is(statErr, os.ErrNotExist)

	cfgStore := configstore.New(statePath)
	persisted, err := cfgStore.LoadOrDefault()
	if err != nil {
		return nil, fmt.Errorf("daemon: load config state: %w", err)
	}

	statsStore, err := stats.Open(filepath.Join(dataDir, "traffic.db"))
	if err != nil {
		return nil, fmt.Errorf("daemon: open traffic stats: %w", err)
	}

	coord := tailnet.NewCoordinator(dataDir, persisted.Orgs, persisted.ExitNodes, opts.DemoMode)
	orgs := coord.ListOrgs()
	nodes := coord.ListExitNodes()

	var rulesMgr *rules.Manager
	if isFreshState {
		rulesMgr = rules.NewManager(nil, nil)
	} else {
		rulesMgr = rules.NewManager(persisted.CustomRules, persisted.Providers)
	}

	activeExitID := resolveInitialExitID(persisted.ActiveExitID, nodes)
	prober := engine.NewDefaultProber(opts.RealNetworkProbe)
	ctrl := engine.NewController(prober, persisted.TunEnabled, persisted.RoutingMode, activeExitID, opts.DemoMode)

	if opts.DemoMode {
		if err := statsStore.SeedDemoIfEmpty(time.Now(), orgs, nodes); err != nil {
			_ = statsStore.Close()
			return nil, fmt.Errorf("daemon: seed demo stats: %w", err)
		}
	}

	coreAvailable, coreVersion, _ := engine.CoreStatus()
	rt := engine.NewRuntime(opts.ClashAPIAddr)

	s := &Server{
		dataDir:          dataDir,
		staticFS:         opts.StaticFS,
		realNetworkProbe: opts.RealNetworkProbe,
		demoMode:         opts.DemoMode,
		coreAvailable:    coreAvailable,
		coreVersion:      coreVersion,
		cfgStore:         cfgStore,
		stats:            statsStore,
		tailnet:          coord,
		rules:            rulesMgr,
		ctrl:             ctrl,
		rt:               rt,
		clashSampler:     engine.NewClashSampler(),
		settings:         persisted.Settings,
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		stopCh:           make(chan struct{}),
	}

	if s.shouldManageSingBox() && persisted.TunEnabled && hasAnyOnlineNode(orgs, nodes) {
		if err := s.applySingBoxRuntime(context.Background()); err != nil {
			s.ctrl.SetTun(false)
			s.ctrl.SetBanner(&contract.BannerNotice{
				Kind: "rollback",
				Text: err.Error(),
			})
		}
	}

	s.mux = s.buildRouter()

	s.wg.Add(1)
	go s.runTelemetrySampler()

	return s, nil
}

// ServeHTTP handles incoming HTTP requests, enforcing loopback origin checks and JSON content types on API endpoints.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
		if !isAllowedOrigin(r.Header.Get("Origin")) {
			writeJSONError(w, http.StatusForbidden, "forbidden origin")
			return
		}
		if (r.Method == http.MethodPost || r.Method == http.MethodPut) && hasRequestBody(r) {
			ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
			if !strings.HasPrefix(ct, "application/json") {
				writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must start with application/json")
				return
			}
		}
	}
	s.mux.ServeHTTP(w, r)
}

// Handler returns the HTTP handler for the daemon server.
func (s *Server) Handler() http.Handler {
	return s
}

// Close gracefully stops the background telemetry goroutine, sing-box child process, and SQLite storage.
func (s *Server) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.stopCh)
		s.wg.Wait()
		if s.rt != nil {
			_ = s.rt.Stop()
		}
		if s.stats != nil {
			closeErr = s.stats.Close()
		}
	})
	return closeErr
}

func (s *Server) buildRouter() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/snapshot", s.handleGetSnapshot)
	mux.HandleFunc("POST /api/tun", s.handleSetTun)
	mux.HandleFunc("POST /api/mode", s.handleSetMode)
	mux.HandleFunc("POST /api/exit/switch", s.handleSwitchExit)
	mux.HandleFunc("POST /api/exit/ping", s.handlePingExits)
	mux.HandleFunc("POST /api/banner/dismiss", s.handleDismissBanner)

	mux.HandleFunc("POST /api/rules", s.handleAddRule)
	mux.HandleFunc("POST /api/rules/toggle", s.handleToggleRule)
	mux.HandleFunc("POST /api/rules/move", s.handleMoveRule)
	mux.HandleFunc("DELETE /api/rules", s.handleDeleteRule)
	mux.HandleFunc("POST /api/rules/import", s.handleImportRules)
	mux.HandleFunc("POST /api/connections/quick-rule", s.handleQuickRule)

	mux.HandleFunc("POST /api/providers", s.handleAddProvider)
	mux.HandleFunc("POST /api/providers/refresh", s.handleRefreshProviders)

	mux.HandleFunc("POST /api/orgs", s.handleAddOrg)
	mux.HandleFunc("DELETE /api/orgs", s.handleRemoveOrg)
	mux.HandleFunc("POST /api/orgs/sync", s.handleSyncOrg)
	mux.HandleFunc("POST /api/orgs/login/start", s.handleStartOrgLogin)
	mux.HandleFunc("GET /api/orgs/login/poll", s.handlePollOrgLogin)
	mux.HandleFunc("POST /api/orgs/import-dir", s.handleImportOrgDir)

	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/export", s.handleExportConfig)
	mux.HandleFunc("POST /api/browser/open", s.handleOpenBrowser)

	mux.HandleFunc("/api/", s.handleAPINotFound)
	mux.HandleFunc("/", s.handleStatic)

	return mux
}

func (s *Server) shouldManageSingBox() bool {
	return !s.demoMode && s.realNetworkProbe
}

func (s *Server) applySingBoxRuntime(ctx context.Context) error {
	_, _, activeExitID, _, _ := s.ctrl.State()
	return s.applySingBoxRuntimeWithExit(ctx, activeExitID)
}

func (s *Server) applySingBoxRuntimeWithExit(ctx context.Context, exitID string) error {
	tunEnabled, _, _, _, _ := s.ctrl.State()
	return s.applySingBoxRuntimeWithTun(ctx, exitID, tunEnabled)
}

func (s *Server) applySingBoxRuntimeWithTun(ctx context.Context, exitID string, tunEnabled bool) error {
	if !s.shouldManageSingBox() || s.rt == nil {
		return nil
	}
	_, mode, _, _, _ := s.ctrl.State()
	s.resetClashTotals()
	return s.rt.ApplyRuntime(ctx, engine.RuntimeApplyRequest{
		DataDir:      s.dataDir,
		TunEnabled:   tunEnabled,
		Orgs:         s.tailnet.ListOrgs(),
		Nodes:        s.tailnet.ListExitNodes(),
		ActiveExitID: exitID,
		Mode:         mode,
		Rules:        s.rules.ListRules(),
		Providers:    s.rules.ListProviders(),
		Settings:     s.getSettings(),
	})
}

func (s *Server) resetClashTotals() {
	s.clashTotalsMu.Lock()
	s.trafficTracker.Reset()
	s.lastSampleAt = time.Time{}
	s.clashTotalsMu.Unlock()
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-SingTail-Build", DaemonBuildTag)
	periodParam := strings.TrimSpace(r.URL.Query().Get("period"))
	period := contract.PeriodLive
	if periodParam != "" {
		period = contract.TrafficPeriodKey(periodParam)
	}
	s.writeSnapshot(w, period)
}

func (s *Server) handleSetTun(w http.ResponseWriter, r *http.Request) {
	var req contract.SetTunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.tunMu.Lock()
	defer s.tunMu.Unlock()

	if req.Enabled && s.shouldManageSingBox() {
		if !hasAnyOnlineNode(s.tailnet.ListOrgs(), s.tailnet.ListExitNodes()) {
			writeJSONError(w, http.StatusBadRequest, "请先挂载包含在线出口节点的 Tailscale 组织")
			return
		}
		if s.rt == nil || !s.rt.IsRunning() {
			if extPath := findExternalSingBoxProcess(); extPath != "" {
				writeJSONError(w, http.StatusConflict, fmt.Sprintf("检测到系统中已有外部 sing-box 正在运行（%s），请先停止旧进程再开启 TUN", extPath))
				return
			}
		}
	}

	if s.shouldManageSingBox() {
		_, mode, activeExitID, _, _ := s.ctrl.State()
		runCtx, cancel := context.WithTimeout(context.Background(), runtimeStartTimout)
		err := s.applySingBoxRuntimeWithTun(runCtx, activeExitID, req.Enabled)
		cancel()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.ctrl.SetTun(req.Enabled)
		if !req.Enabled {
			s.ctrl.SetConnections(nil)
			s.resetClashTotals()
		} else if s.getSettings().AutoVerifyIP && activeExitID != "" && mode != contract.ModeDirect {
			go s.probeAndSavePublicIPAsync(activeExitID)
		}
	} else {
		s.ctrl.SetTun(req.Enabled)
	}

	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleSetMode(w http.ResponseWriter, r *http.Request) {
	var req contract.SetModeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch req.Mode {
	case contract.ModeRule, contract.ModeGlobal, contract.ModeDirect:
	default:
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("不支持的分流模式: %q", req.Mode))
		return
	}

	s.ctrl.SetMode(req.Mode)
	if s.shouldManageSingBox() && s.rt != nil && s.rt.IsRunning() {
		if err := s.rt.PatchMode(r.Context(), req.Mode); err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleSwitchExit(w http.ResponseWriter, r *http.Request) {
	var req contract.SwitchExitRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	exitID := strings.TrimSpace(req.ExitID)
	if exitID == "" {
		writeJSONError(w, http.StatusBadRequest, "exitId 不能为空")
		return
	}

	nodes := s.tailnet.ListExitNodes()
	target, ok := findExitNodeByID(nodes, exitID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("未找到出口节点: %s", exitID))
		return
	}

	tunEnabled, _, prevExitID, _, _ := s.ctrl.State()
	if s.shouldManageSingBox() && tunEnabled {
		runCtx, runCancel := context.WithTimeout(context.Background(), runtimeStartTimout)
		err := s.applySingBoxRuntimeWithExit(runCtx, target.ID)
		runCancel()
		if err != nil {
			_ = s.applySingBoxRuntimeWithExit(context.Background(), prevExitID)
			s.ctrl.SetBanner(&contract.BannerNotice{
				Kind: "rollback",
				Text: fmt.Sprintf("切换出口启动内核失败: %v", err),
			})
			_ = s.saveState()
			s.writeSnapshot(w, contract.PeriodLive)
			return
		}
	}

	settings := s.getSettings()
	_, mode, _, _, _ := s.ctrl.State()
	verifyIP := settings.AutoVerifyIP && (!s.shouldManageSingBox() || (tunEnabled && mode != contract.ModeDirect))

	ctx, cancel := context.WithTimeout(r.Context(), exitSwitchTimeout)
	defer cancel()

	// SwitchExitAndProbe updates activeExitID or records a rollback BannerNotice on probe failure;
	// either outcome returns 200 with the latest DashboardSnapshot so the UI renders the banner.
	activeID, probedIP, _, probeErr := s.ctrl.SwitchExitAndProbe(ctx, target, nodes, verifyIP, defaultMixedProxy)
	if probeErr == nil && probedIP != "" {
		s.tailnet.UpdateNodePublicIP(activeID, probedIP)
	}
	if probeErr != nil && s.shouldManageSingBox() && tunEnabled {
		_ = s.applySingBoxRuntimeWithExit(context.Background(), activeID)
	}

	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handlePingExits(w http.ResponseWriter, r *http.Request) {
	s.tailnet.PingNodes(r.Context())
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleDismissBanner(w http.ResponseWriter, _ *http.Request) {
	s.ctrl.ClearBanner()
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleAddRule(w http.ResponseWriter, r *http.Request) {
	var req contract.AddRuleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := s.rules.AddRule(req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleToggleRule(w http.ResponseWriter, r *http.Request) {
	var req contract.ToggleRuleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.rules.ToggleRule(strings.TrimSpace(req.RuleID)); err != nil {
		writeRuleError(w, err)
		return
	}
	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleMoveRule(w http.ResponseWriter, r *http.Request) {
	var req contract.MoveRuleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.rules.MoveRule(strings.TrimSpace(req.RuleID), req.Direction); err != nil {
		writeRuleError(w, err)
		return
	}
	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少规则 id 参数")
		return
	}

	if err := s.rules.DeleteRule(id); err != nil {
		writeRuleError(w, err)
		return
	}
	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleImportRules(w http.ResponseWriter, r *http.Request) {
	var req contract.ImportRulesRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := s.rules.ImportText(req.RawText); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleQuickRule(w http.ResponseWriter, r *http.Request) {
	var req contract.QuickRouteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	connID := strings.TrimSpace(req.ConnID)
	if connID == "" {
		writeJSONError(w, http.StatusBadRequest, "connId 不能为空")
		return
	}

	_, _, activeExitID, _, _ := s.ctrl.State()
	activeExit, _ := findExitNodeByID(s.tailnet.ListExitNodes(), activeExitID)
	conns := s.ctrl.ListConnections(activeExit)

	var targetConn contract.LiveConnection
	found := false
	for _, c := range conns {
		if c.ID == connID {
			targetConn = c
			found = true
			break
		}
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("未找到连接: %s", connID))
		return
	}

	var ruleType contract.RuleType
	var payload string
	var matchedLabel string

	switch strings.ToLower(strings.TrimSpace(req.MatchKind)) {
	case "domain":
		ruleType = contract.RuleDomainSuffix
		payload = strings.TrimSpace(targetConn.Host)
		if payload == "" {
			writeJSONError(w, http.StatusBadRequest, "连接缺少目标域名")
			return
		}
		matchedLabel = "DOMAIN · " + payload
	case "process":
		ruleType = contract.RuleProcessName
		payload = strings.TrimSpace(targetConn.Process)
		if payload == "" {
			writeJSONError(w, http.StatusBadRequest, "连接缺少进程名")
			return
		}
		matchedLabel = "PROCESS · " + payload
	default:
		writeJSONError(w, http.StatusBadRequest, "matchKind 必须为 domain 或 process")
		return
	}

	added, err := s.rules.AddRule(contract.AddRuleRequest{
		Type:    ruleType,
		Payload: payload,
		Action:  req.Action,
	})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.ctrl.ApplyQuickRuleToConn(targetConn.ID, matchedLabel, added.Action)
	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleAddProvider(w http.ResponseWriter, r *http.Request) {
	var req contract.AddProviderRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := s.rules.AddProvider(req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleRefreshProviders(w http.ResponseWriter, r *http.Request) {
	var fetcher func(ctx context.Context, rawURL string) ([]byte, error)
	if s.realNetworkProbe {
		fetcher = s.fetchProviderPayload
	}

	rulesDir := filepath.Join(s.dataDir, "rules")
	if err := s.rules.RefreshAllProvidersToDir(r.Context(), rulesDir, fetcher, nil); err != nil {
		_ = s.saveState()
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) fetchProviderPayload(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, providerFetchLimit))
}

func (s *Server) handleAddOrg(w http.ResponseWriter, r *http.Request) {
	var req contract.AddOrgRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, _, err := s.tailnet.AddOrg(req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.ensureActiveExitSelected()
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleStartOrgLogin(w http.ResponseWriter, r *http.Request) {
	var req contract.StartOrgLoginRequest
	if err := decodeOptionalJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess, err := s.tailnet.StartLogin(r.Context(), req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handlePollOrgLogin(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	if sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少 sessionId 参数")
		return
	}

	sess, ok := s.tailnet.PollLogin(sessionID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, fmt.Sprintf("未找到登录会话: %s", sessionID))
		return
	}

	if sess.State == "authenticated" {
		s.ensureActiveExitSelected()
		if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.saveState(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleImportOrgDir(w http.ResponseWriter, r *http.Request) {
	var req contract.ImportOrgStateDirRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, _, err := s.tailnet.ImportStateDir(r.Context(), req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.ensureActiveExitSelected()
	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleRemoveOrg(w http.ResponseWriter, r *http.Request) {
	orgID := strings.TrimSpace(r.URL.Query().Get("id"))
	if orgID == "" {
		writeJSONError(w, http.StatusBadRequest, "缺少组织 id 参数")
		return
	}

	_, _, activeExitID, _, _ := s.ctrl.State()
	prevNodes := s.tailnet.ListExitNodes()
	activeBelongsToRemoved := false
	for _, n := range prevNodes {
		if n.ID == activeExitID && n.OrgID == orgID {
			activeBelongsToRemoved = true
			break
		}
	}

	wasRunning := s.shouldManageSingBox() && s.rt != nil && s.rt.IsRunning()
	if wasRunning {
		_ = s.rt.Stop()
	}

	if err := s.tailnet.RemoveOrg(orgID); err != nil {
		if wasRunning {
			_ = s.applySingBoxRuntime(context.Background())
		}
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}

	remaining := s.tailnet.ListExitNodes()
	if activeBelongsToRemoved || activeExitID == "" {
		nextID := resolveInitialExitID("", remaining)
		s.ctrl.SetActiveExitID(nextID)
		if nextID == "" {
			s.ctrl.SetTun(false)
			s.ctrl.SetConnections(nil)
		}
	} else if len(remaining) == 0 {
		s.ctrl.SetTun(false)
		s.ctrl.SetConnections(nil)
	}

	if s.shouldManageSingBox() {
		if err := s.applySingBoxRuntime(r.Context()); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleSyncOrg(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrgID string `json:"orgId"`
	}
	if err := decodeOptionalJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.demoMode {
		if err := s.tailnet.SyncOrg(req.OrgID); err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		if err := s.saveState(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.writeSnapshot(w, contract.PeriodLive)
		return
	}

	targetID := strings.TrimSpace(req.OrgID)
	var orgIDs []string
	if targetID != "" {
		orgIDs = []string{targetID}
	} else {
		for _, org := range s.tailnet.ListOrgs() {
			orgIDs = append(orgIDs, org.ID)
		}
	}

	tunEnabled, _, _, _, _ := s.ctrl.State()
	wasRunning := s.shouldManageSingBox() && s.rt != nil && s.rt.IsRunning()
	if wasRunning {
		_ = s.rt.Stop()
	}

	for _, id := range orgIDs {
		if _, err := s.tailnet.RefreshOrgFromTsnet(r.Context(), id); err != nil {
			if wasRunning && tunEnabled {
				_ = s.applySingBoxRuntime(context.Background())
			}
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
	}

	s.ensureActiveExitSelected()
	if s.shouldManageSingBox() && tunEnabled {
		if err := s.applySingBoxRuntime(r.Context()); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var next contract.SystemSettings
	if err := decodeJSON(r, &next); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if next.AutoUpdateRulesDays < 0 {
		writeJSONError(w, http.StatusBadRequest, "规则更新周期不能为负数")
		return
	}

	_, mode, activeExitID, _, _ := s.ctrl.State()
	if _, err := engine.GenerateSingBoxConfigWithOptions(
		s.tailnet.ListOrgs(),
		s.tailnet.ListExitNodes(),
		activeExitID,
		mode,
		s.rules.ListRules(),
		s.rules.ListProviders(),
		next,
		engine.ConfigOptions{RulesDir: filepath.Join(s.dataDir, "rules")},
	); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.mu.Lock()
	s.settings = next
	s.mu.Unlock()

	if s.shouldManageSingBox() && (syswin.IsWindowsService() || syswin.IsElevated()) {
		if err := syswin.SetServiceAutoStart(next.AutoStartService); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("同步系统服务自启状态失败: %v", err))
			return
		}
	}

	if err := s.reloadRuntimeIfRunning(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.saveState(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSnapshot(w, contract.PeriodLive)
}

func (s *Server) handleExportConfig(w http.ResponseWriter, _ *http.Request) {
	exported := configstore.BuildSanitizedExport(
		s.rules.ListRules(),
		s.rules.ListProviders(),
		s.getSettings(),
		time.Now(),
	)
	writeJSON(w, http.StatusOK, exported)
}

func (s *Server) handleOpenBrowser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := syswin.OpenBrowser(req.URL); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) probeAndSavePublicIPAsync(exitID string) {
	tunEnabled, mode, activeExitID, _, _ := s.ctrl.State()
	if !tunEnabled || mode == contract.ModeDirect || activeExitID != exitID {
		return
	}
	nodes := s.tailnet.ListExitNodes()
	target, ok := findExitNodeByID(nodes, exitID)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), exitSwitchTimeout)
	defer cancel()

	if s.shouldManageSingBox() && (s.rt == nil || !s.rt.IsExitRouteActive(ctx)) {
		return
	}

	prober := engine.NewDefaultProber(s.realNetworkProbe)
	ip, err := prober.ProbePublicIP(ctx, defaultMixedProxy, target)
	if err != nil || strings.TrimSpace(ip) == "" {
		return
	}

	tunAfter, modeAfter, activeAfter, _, _ := s.ctrl.State()
	if !tunAfter || modeAfter == contract.ModeDirect || activeAfter != exitID {
		return
	}
	s.tailnet.UpdateNodePublicIP(exitID, ip)
	_ = s.saveState()
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, _ *http.Request) {
	writeJSONError(w, http.StatusNotFound, "API endpoint not found")
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if s.staticFS == nil {
		http.NotFound(w, r)
		return
	}

	cleanPath := path.Clean("/" + r.URL.Path)
	relPath := strings.TrimPrefix(cleanPath, "/")

	if relPath != "" && relPath != "index.html" {
		if info, err := fs.Stat(s.staticFS, relPath); err == nil && !info.IsDir() {
			http.FileServer(http.FS(s.staticFS)).ServeHTTP(w, r)
			return
		}
	}

	indexBytes, err := fs.ReadFile(s.staticFS, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexBytes)
}

func (s *Server) reloadRuntimeIfRunning(ctx context.Context) error {
	if !s.shouldManageSingBox() {
		return nil
	}
	tunEnabled, _, _, _, _ := s.ctrl.State()
	if !tunEnabled {
		return nil
	}
	return s.applySingBoxRuntime(ctx)
}

func (s *Server) ensureActiveExitSelected() {
	_, _, activeExitID, _, _ := s.ctrl.State()
	nodes := s.tailnet.ListExitNodes()
	if activeExitID != "" {
		if _, ok := findExitNodeByID(nodes, activeExitID); ok {
			return
		}
	}
	s.ctrl.SetActiveExitID(resolveInitialExitID("", nodes))
}

func (s *Server) writeSnapshot(w http.ResponseWriter, period contract.TrafficPeriodKey) {
	snap, err := s.buildSnapshot(period)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) buildSnapshot(period contract.TrafficPeriodKey) (contract.DashboardSnapshot, error) {
	if period == "" {
		period = contract.PeriodLive
	}

	tunEnabled, mode, activeExitID, verifyingID, banner := s.ctrl.State()
	orgs := s.tailnet.ListOrgs()
	nodes := s.tailnet.ListExitNodes()
	activeExit, _ := findExitNodeByID(nodes, activeExitID)

	ruleList := s.rules.ListRules()
	providers := s.rules.ListProviders()
	conns := s.ctrl.ListConnections(activeExit)

	runningApps, err := syswin.ListRunningApps()
	if err != nil || runningApps == nil {
		runningApps = []contract.RunningApp{}
	}

	settings := s.getSettings()
	traffic, err := s.stats.QueryReport(time.Now(), period, orgs, nodes)
	if err != nil {
		return contract.DashboardSnapshot{}, err
	}

	return contract.DashboardSnapshot{
		TunEnabled:      tunEnabled,
		Privileged:      !s.shouldManageSingBox() || syswin.IsElevated() || syswin.IsWindowsService(),
		CoreAvailable:   s.coreAvailable,
		CoreVersion:     s.coreVersion,
		RoutingMode:     mode,
		ActiveExitID:    activeExitID,
		VerifyingExitID: verifyingID,
		BannerNotice:    banner,
		Orgs:            nonNilOrgs(orgs),
		ExitNodes:       nonNilNodes(nodes),
		Rules:           nonNilRules(ruleList),
		Providers:       nonNilProviders(providers),
		Connections:     nonNilConnections(conns),
		RunningApps:     runningApps,
		Settings:        settings,
		Traffic:         traffic,
	}, nil
}

func (s *Server) getSettings() contract.SystemSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *Server) saveState() error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	tunEnabled, mode, activeExitID, _, _ := s.ctrl.State()
	persisted := configstore.PersistedState{
		TunEnabled:   tunEnabled,
		RoutingMode:  mode,
		ActiveExitID: activeExitID,
		Orgs:         s.tailnet.ListOrgs(),
		ExitNodes:    s.tailnet.ListExitNodes(),
		CustomRules:  s.rules.CustomRules(),
		Providers:    s.rules.ListProviders(),
		Settings:     s.getSettings(),
	}
	return s.cfgStore.Save(persisted)
}

func (s *Server) runTelemetrySampler() {
	defer s.wg.Done()

	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			if s.demoMode {
				s.sampleDemoTick(now)
				continue
			}
			s.sampleRealTick(now)
		}
	}
}

func (s *Server) sampleDemoTick(now time.Time) {
	tunEnabled, _, activeExitID, _, _ := s.ctrl.State()
	if !tunEnabled {
		return
	}

	nodes := s.tailnet.ListExitNodes()
	active, ok := findExitNodeByID(nodes, activeExitID)
	if !ok && len(nodes) > 0 {
		active = nodes[0]
	}
	if active.ID == "" {
		return
	}

	seq := float64(s.samplerSeq.Add(1))
	downMB := math.Max(0.4, 2.2+1.2*math.Sin(seq*0.45)+0.4*math.Cos(seq*0.8))
	upMB := math.Max(0.1, 0.35+0.18*math.Sin(seq*0.45+0.6))
	downBytes := int64(math.Round(downMB * float64(bytesPerMB)))
	upBytes := int64(math.Round(upMB * float64(bytesPerMB)))

	_ = s.stats.RecordSample(now, sampleInterval, active.OrgID, active.ID, downBytes, upBytes)
}

func (s *Server) sampleRealTick(now time.Time) {
	tunEnabled, mode, activeExitID, _, _ := s.ctrl.State()
	nodes := s.tailnet.ListExitNodes()
	active, ok := findExitNodeByID(nodes, activeExitID)
	if !ok && len(nodes) > 0 {
		active = nodes[0]
	}

	if !tunEnabled || s.rt == nil || !s.rt.IsRunning() {
		s.ctrl.SetConnections(nil)
		s.resetClashTotals()
		s.stats.PushIdleLivePoint()
		return
	}

	snap, err := s.clashSampler.Fetch(s.rt.ClashAPIAddr())
	if err != nil {
		return
	}
	s.ctrl.SetConnections(snap.Connections)

	if active.ID == "" {
		s.stats.PushIdleLivePoint()
		return
	}

	s.clashTotalsMu.Lock()
	elapsed := sampleInterval
	if !s.lastSampleAt.IsZero() {
		if delta := now.Sub(s.lastSampleAt); delta >= 200*time.Millisecond && delta <= 10*time.Second {
			elapsed = delta
		}
	}
	s.lastSampleAt = now
	delta := s.trafficTracker.Compute(mode, active.OrgID, snap)
	s.clashTotalsMu.Unlock()

	entries := make([]stats.NodeTrafficEntry, 0, len(delta.ByOrg))
	for _, usage := range delta.ByOrg {
		orgID := usage.OrgID
		nodeID := resolveNodeIDForOrg(orgID, active, nodes)
		if nodeID == "" {
			orgID = active.OrgID
			nodeID = active.ID
		}
		entries = append(entries, stats.NodeTrafficEntry{
			OrgID:     orgID,
			NodeID:    nodeID,
			DownBytes: usage.DownBytes,
			UpBytes:   usage.UpBytes,
		})
	}

	_ = s.stats.RecordTrafficSample(now, elapsed, delta.DownBytes, delta.UpBytes, entries)
}

func resolveNodeIDForOrg(orgID string, active contract.ExitNode, nodes []contract.ExitNode) string {
	if orgID == "" || orgID == active.OrgID {
		return active.ID
	}
	for _, n := range nodes {
		if n.OrgID == orgID && n.Online {
			return n.ID
		}
	}
	for _, n := range nodes {
		if n.OrgID == orgID {
			return n.ID
		}
	}
	return ""
}

func findExternalSingBoxProcess() string {
	apps, err := syswin.ListRunningApps()
	if err != nil || len(apps) == 0 {
		return ""
	}
	ownBin := strings.ToLower(filepath.Clean(engine.FindSingBoxBinary()))
	for _, app := range apps {
		name := strings.ToLower(strings.TrimSpace(app.Process))
		if name != "sing-box.exe" && name != "sing-box" {
			continue
		}
		cleanPath := filepath.Clean(strings.TrimSpace(app.Path))
		if ownBin != "" && cleanPath != "." && strings.EqualFold(cleanPath, ownBin) {
			continue
		}
		if cleanPath != "" && cleanPath != "." {
			return cleanPath
		}
		return app.Process
	}
	return ""
}

func hasAnyOnlineNode(orgs []contract.Organization, nodes []contract.ExitNode) bool {
	if len(orgs) == 0 || len(nodes) == 0 {
		return false
	}
	orgIDs := make(map[string]bool, len(orgs))
	for _, org := range orgs {
		if org.ID != "" {
			orgIDs[org.ID] = true
		}
	}
	for _, n := range nodes {
		if n.Online && strings.TrimSpace(n.TailscaleIP) != "" && orgIDs[n.OrgID] {
			return true
		}
	}
	return false
}

func resolveInitialExitID(savedID string, nodes []contract.ExitNode) string {
	if savedID != "" {
		if _, ok := findExitNodeByID(nodes, savedID); ok {
			return savedID
		}
	}
	for _, n := range nodes {
		if n.Online && n.Healthy {
			return n.ID
		}
	}
	if len(nodes) > 0 {
		return nodes[0].ID
	}
	return ""
}

func findExitNodeByID(nodes []contract.ExitNode, id string) (contract.ExitNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return contract.ExitNode{}, false
}

func hasRequestBody(r *http.Request) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return false
	}
	return r.ContentLength != 0
}

func isAllowedOrigin(origin string) bool {
	trimmed := strings.TrimSpace(origin)
	if trimmed == "" {
		return true
	}
	if strings.EqualFold(trimmed, "null") {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	if strings.EqualFold(parsed.Scheme, "wails") {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "wails" || host == "wails.localhost" || strings.HasSuffix(host, ".wails.localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

func decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("请求体不能为空")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodyBytes))
	if err != nil {
		return fmt.Errorf("读取请求体失败: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return errors.New("请求体不能为空")
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("JSON 格式错误: %w", err)
	}
	return nil
}

func decodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBodyBytes))
	if err != nil {
		return fmt.Errorf("读取请求体失败: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("JSON 格式错误: %w", err)
	}
	return nil
}

func writeRuleError(w http.ResponseWriter, err error) {
	if errors.Is(err, rules.ErrRuleNotFound) {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSONError(w, http.StatusBadRequest, err.Error())
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"internal encode error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte("\n"))
}

func nonNilOrgs(items []contract.Organization) []contract.Organization {
	if items == nil {
		return []contract.Organization{}
	}
	return items
}

func nonNilNodes(items []contract.ExitNode) []contract.ExitNode {
	if items == nil {
		return []contract.ExitNode{}
	}
	return items
}

func nonNilRules(items []contract.Rule) []contract.Rule {
	if items == nil {
		return []contract.Rule{}
	}
	return items
}

func nonNilProviders(items []contract.RuleProvider) []contract.RuleProvider {
	if items == nil {
		return []contract.RuleProvider{}
	}
	return items
}

func nonNilConnections(items []contract.LiveConnection) []contract.LiveConnection {
	if items == nil {
		return []contract.LiveConnection{}
	}
	return items
}
