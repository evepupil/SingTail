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
	defaultDataDir     = "./data"
	defaultMixedProxy  = "127.0.0.1:7899"
	maxJSONBodyBytes   = 2 << 20
	sampleInterval     = 2 * time.Second
	bytesPerMB         = 1024 * 1024
	exitSwitchTimeout  = 5 * time.Second
	providerFetchLimit = 16 << 20
)

// Server exposes the SingTail REST control API and optional embedded frontend assets.
type Server struct {
	dataDir          string
	staticFS         fs.FS
	realNetworkProbe bool

	cfgStore *configstore.Store
	stats    *stats.Store
	tailnet  *tailnet.Coordinator
	rules    *rules.Manager
	ctrl     *engine.Controller

	mux        *http.ServeMux
	httpClient *http.Client

	mu       sync.RWMutex
	settings contract.SystemSettings

	saveMu sync.Mutex

	samplerSeq atomic.Int64
	stopCh     chan struct{}
	wg         sync.WaitGroup
	closeOnce  sync.Once
}

// NewServer initializes persistent storage, domain managers, demo traffic seeding,
// background telemetry sampling, and HTTP routes.
func NewServer(dataDir string, staticFS fs.FS, realNetworkProbe bool) (*Server, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = defaultDataDir
	}
	dataDir = filepath.Clean(dataDir)

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

	coord := tailnet.NewCoordinator(dataDir, persisted.Orgs)
	orgs := coord.ListOrgs()
	nodes := coord.ListExitNodes()

	var rulesMgr *rules.Manager
	if isFreshState {
		rulesMgr = rules.NewManager(nil, nil)
	} else {
		rulesMgr = rules.NewManager(persisted.CustomRules, persisted.Providers)
	}

	activeExitID := resolveInitialExitID(persisted.ActiveExitID, nodes)
	prober := engine.NewDefaultProber(realNetworkProbe)
	ctrl := engine.NewController(prober, persisted.TunEnabled, persisted.RoutingMode, activeExitID)

	if err := statsStore.SeedDemoIfEmpty(time.Now(), orgs, nodes); err != nil {
		_ = statsStore.Close()
		return nil, fmt.Errorf("daemon: seed demo stats: %w", err)
	}

	s := &Server{
		dataDir:          dataDir,
		staticFS:         staticFS,
		realNetworkProbe: realNetworkProbe,
		cfgStore:         cfgStore,
		stats:            statsStore,
		tailnet:          coord,
		rules:            rulesMgr,
		ctrl:             ctrl,
		settings:         persisted.Settings,
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		stopCh:           make(chan struct{}),
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

// Close gracefully stops the background telemetry goroutine and closes SQLite storage.
func (s *Server) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.stopCh)
		s.wg.Wait()
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
	mux.HandleFunc("POST /api/orgs/sync", s.handleSyncOrg)

	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("GET /api/export", s.handleExportConfig)

	mux.HandleFunc("/api/", s.handleAPINotFound)
	mux.HandleFunc("/", s.handleStatic)

	return mux
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
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

	s.ctrl.SetTun(req.Enabled)
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

	settings := s.getSettings()
	ctx, cancel := context.WithTimeout(r.Context(), exitSwitchTimeout)
	defer cancel()

	// SwitchExit updates activeExitID or records a rollback BannerNotice on probe failure;
	// either outcome returns 200 with the latest DashboardSnapshot so the UI renders the banner.
	_, _, _ = s.ctrl.SwitchExit(ctx, target, nodes, settings.AutoVerifyIP, defaultMixedProxy)

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

	if err := s.rules.RefreshAllProviders(r.Context(), fetcher); err != nil {
		_ = s.saveState()
		writeJSONError(w, http.StatusBadGateway, err.Error())
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

	if err := s.tailnet.SyncOrg(req.OrgID); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
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
	if _, err := engine.GenerateSingBoxConfig(
		s.tailnet.ListOrgs(),
		s.tailnet.ListExitNodes(),
		activeExitID,
		mode,
		s.rules.ListRules(),
		s.rules.ListProviders(),
		next,
	); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.mu.Lock()
	s.settings = next
	s.mu.Unlock()

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
			tunEnabled, _, activeExitID, _, _ := s.ctrl.State()
			if !tunEnabled {
				continue
			}

			nodes := s.tailnet.ListExitNodes()
			active, ok := findExitNodeByID(nodes, activeExitID)
			if !ok && len(nodes) > 0 {
				active = nodes[0]
			}
			if active.ID == "" {
				continue
			}

			seq := float64(s.samplerSeq.Add(1))
			downMB := math.Max(0.4, 2.2+1.2*math.Sin(seq*0.45)+0.4*math.Cos(seq*0.8))
			upMB := math.Max(0.1, 0.35+0.18*math.Sin(seq*0.45+0.6))
			downBytes := int64(math.Round(downMB * float64(bytesPerMB)))
			upBytes := int64(math.Round(upMB * float64(bytesPerMB)))

			_ = s.stats.RecordTick(now, active.OrgID, active.ID, downBytes, upBytes)
		}
	}
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
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
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
