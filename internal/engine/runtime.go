package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/singtail/singtail/internal/contract"
)

const (
	clashReadyTimeout  = 35 * time.Second
	clashPollInterval  = 150 * time.Millisecond
	clashConnTimeout   = 1200 * time.Millisecond
	singBoxCheckTimout = 10 * time.Second
	maxStderrBuffer    = 32 * 1024
)

// RuntimeApplyRequest carries the desired sing-box runtime inputs.
type RuntimeApplyRequest struct {
	DataDir      string
	TunEnabled   bool
	Orgs         []contract.Organization
	Nodes        []contract.ExitNode
	ActiveExitID string
	Mode         contract.RoutingMode
	Rules        []contract.Rule
	Providers    []contract.RuleProvider
	Settings     contract.SystemSettings
}

// ClashConnectionsSnapshot holds aggregated byte totals and active connections from Clash API.
type ClashConnectionsSnapshot struct {
	DownloadTotal int64
	UploadTotal   int64
	Connections   []contract.LiveConnection
}

// Runtime supervises the managed sing-box child process and Clash API communication.
type Runtime struct {
	mu           sync.Mutex
	clashAPIAddr string
	binFinder    func() string
	cmd          *exec.Cmd
	doneCh       chan struct{}
	exitMu       sync.Mutex
	exitErr      error
	stderrBuf    *tailBuffer
}

// NewRuntime creates a Runtime manager bound to clashAPIAddr (defaults to 127.0.0.1:19090).
func NewRuntime(clashAPIAddr string) *Runtime {
	addr := strings.TrimSpace(clashAPIAddr)
	if addr == "" {
		addr = DefaultClashAPIAddr
	}
	return &Runtime{
		clashAPIAddr: addr,
		binFinder:    FindSingBoxBinary,
	}
}

// ClashAPIAddr returns the configured host:port address for Clash API queries.
func (r *Runtime) ClashAPIAddr() string {
	if r == nil || strings.TrimSpace(r.clashAPIAddr) == "" {
		return DefaultClashAPIAddr
	}
	return r.clashAPIAddr
}

// IsRunning reports whether a managed sing-box subprocess is currently alive.
func (r *Runtime) IsRunning() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isRunningLocked()
}

func (r *Runtime) isRunningLocked() bool {
	if r.cmd == nil || r.cmd.Process == nil || r.doneCh == nil {
		return false
	}
	select {
	case <-r.doneCh:
		return false
	default:
		return true
	}
}

// Stop gracefully terminates the managed sing-box child process if running.
func (r *Runtime) Stop() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopLocked()
}

func (r *Runtime) stopLocked() error {
	if r.cmd == nil || r.cmd.Process == nil {
		r.cmd = nil
		r.doneCh = nil
		return nil
	}

	done := r.doneCh
	proc := r.cmd.Process

	if r.isRunningLocked() {
		if runtime.GOOS != "windows" {
			_ = proc.Signal(syscall.SIGTERM)
		} else {
			_ = proc.Kill()
		}

		if done != nil {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				_ = proc.Kill()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
				}
			}
		}
	}

	r.cmd = nil
	r.doneCh = nil
	return nil
}

// ApplyRuntime stops sing-box when TUN is disabled or no online exit node exists,
// or validates and (re)starts sing-box with the generated runtime configuration.
func (r *Runtime) ApplyRuntime(ctx context.Context, req RuntimeApplyRequest) error {
	if r == nil {
		return errors.New("runtime manager is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !req.TunEnabled || !hasOnlineExitNode(req.Orgs, req.Nodes) {
		return r.stopLocked()
	}
	verifyStartupExit := !r.isRunningLocked() && req.Mode != contract.ModeDirect

	finder := r.binFinder
	if finder == nil {
		finder = FindSingBoxBinary
	}
	binPath := finder()
	if binPath == "" {
		return errors.New("未找到 sing-box 内核可执行文件，请确认已内置 bin/sing-box")
	}

	dataDir := strings.TrimSpace(req.DataDir)
	if dataDir == "" {
		dataDir = "./data"
	}
	dataDir = filepath.Clean(dataDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("创建运行目录失败: %w", err)
	}

	_ = SeedBundledRules(dataDir)
	rulesDir := filepath.Join(dataDir, "rules")

	cfgBytes, err := GenerateSingBoxConfigWithOptions(
		req.Orgs,
		req.Nodes,
		req.ActiveExitID,
		req.Mode,
		req.Rules,
		req.Providers,
		req.Settings,
		ConfigOptions{
			DataDir:      dataDir,
			RulesDir:     rulesDir,
			ClashAPIAddr: r.ClashAPIAddr(),
			TargetGOOS:   runtime.GOOS,
		},
	)
	if err != nil {
		return fmt.Errorf("生成 sing-box 配置失败: %w", err)
	}

	cfgPath := filepath.Join(dataDir, "sing-box-runtime.json")
	if err := writeFileAtomic(cfgPath, append(cfgBytes, '\n')); err != nil {
		return fmt.Errorf("写入 sing-box 配置失败: %w", err)
	}

	checkCtx, checkCancel := context.WithTimeout(ctx, singBoxCheckTimout)
	checkCmd := exec.CommandContext(checkCtx, binPath, "check", "-D", dataDir, "-c", cfgPath)
	checkOut, checkErr := checkCmd.CombinedOutput()
	checkCancel()
	if checkErr != nil {
		msg := tailOutputLines(string(checkOut), 6)
		if msg == "" {
			msg = checkErr.Error()
		}
		return fmt.Errorf("sing-box 配置校验失败: %s", msg)
	}

	if err := r.stopLocked(); err != nil {
		return fmt.Errorf("停止旧 sing-box 进程失败: %w", err)
	}

	stderrBuf := newTailBuffer(maxStderrBuffer)
	runCmd := exec.Command(binPath, "run", "-D", dataDir, "-c", cfgPath)
	runCmd.Stdout = stderrBuf
	runCmd.Stderr = stderrBuf

	if err := runCmd.Start(); err != nil {
		return fmt.Errorf("启动 sing-box 进程失败: %w", err)
	}

	doneCh := make(chan struct{})
	r.exitMu.Lock()
	r.exitErr = nil
	r.exitMu.Unlock()
	go func() {
		waitErr := runCmd.Wait()
		r.exitMu.Lock()
		r.exitErr = waitErr
		r.exitMu.Unlock()
		close(doneCh)
	}()

	r.cmd = runCmd
	r.doneCh = doneCh
	r.stderrBuf = stderrBuf

	logPath := filepath.Join(dataDir, "sing-box.log")
	readyCtx, readyCancel := context.WithTimeout(ctx, clashReadyTimeout)
	defer readyCancel()
	if err := r.waitForClashReady(readyCtx, doneCh, stderrBuf, logPath); err != nil {
		_ = r.stopLocked()
		return err
	}
	if verifyStartupExit {
		if err := waitForExitReady(readyCtx, buildClashBaseURL(r.ClashAPIAddr()), doneCh); err != nil {
			_ = r.stopLocked()
			return err
		}
	}
	return nil
}

func readLogFileTail(logPath string, maxLines int) string {
	if strings.TrimSpace(logPath) == "" {
		return ""
	}
	data, err := os.ReadFile(logPath)
	if err != nil || len(data) == 0 {
		return ""
	}
	if len(data) > maxStderrBuffer {
		data = data[len(data)-maxStderrBuffer:]
	}
	return tailOutputLines(string(data), maxLines)
}

func (r *Runtime) waitForClashReady(ctx context.Context, doneCh <-chan struct{}, stderrBuf *tailBuffer, logPath string) error {
	ticker := time.NewTicker(clashPollInterval)
	defer ticker.Stop()

	versionURL := buildClashBaseURL(r.ClashAPIAddr()) + "/version"
	client := &http.Client{Timeout: 500 * time.Millisecond}

	for {
		if checkClashVersion(ctx, client, versionURL) {
			return nil
		}

		select {
		case <-doneCh:
			r.exitMu.Lock()
			exitErr := r.exitErr
			r.exitMu.Unlock()
			detail := tailOutputLines(stderrBuf.String(), 6)
			if detail == "" {
				detail = readLogFileTail(logPath, 6)
			}
			if detail == "" && exitErr != nil {
				detail = exitErr.Error()
			}
			if looksLikePrivilegeError(detail) {
				return fmt.Errorf("启动虚拟网卡失败（需要管理员/系统服务权限）: %s", detail)
			}
			return fmt.Errorf("sing-box 启动后异常退出: %s", detail)
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ctx.Err()
			}
			detail := tailOutputLines(stderrBuf.String(), 6)
			if detail == "" {
				detail = readLogFileTail(logPath, 6)
			}
			if detail != "" {
				if looksLikePrivilegeError(detail) {
					return fmt.Errorf("启动虚拟网卡失败（需要管理员/系统服务权限）: %s", detail)
				}
				return fmt.Errorf("等待 sing-box 启动超时: %s", detail)
			}
			return errors.New("等待 sing-box 启动超时")
		case <-ticker.C:
		}
	}
}

func checkClashVersion(ctx context.Context, client *http.Client, versionURL string) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 450*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, versionURL, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// PatchMode updates the active Clash routing mode via PATCH /configs when sing-box is running.
func (r *Runtime) PatchMode(ctx context.Context, mode contract.RoutingMode) error {
	if r == nil || !r.IsRunning() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	clashMode := "Rule"
	switch mode {
	case contract.ModeGlobal:
		clashMode = "Global"
	case contract.ModeDirect:
		clashMode = "Direct"
	case contract.ModeRule:
		clashMode = "Rule"
	}

	payload, err := json.Marshal(map[string]string{"mode": clashMode})
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	endpoint := buildClashBaseURL(r.ClashAPIAddr()) + "/configs"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("同步 Clash 路由模式失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("Clash PATCH /configs 返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// FindSingBoxBinary locates the bundled or system sing-box executable.
func FindSingBoxBinary() string {
	binName := "sing-box"
	if runtime.GOOS == "windows" {
		binName = "sing-box.exe"
	}

	candidates := make([]string, 0, 6)
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "bin", binName),
			filepath.Join(exeDir, binName),
			filepath.Join(exeDir, "..", "Resources", "bin", binName),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "bin", binName))
	}

	for _, candidate := range candidates {
		if isExecutableFile(candidate) {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}

	if lookedUp, err := exec.LookPath(binName); err == nil && isExecutableFile(lookedUp) {
		return lookedUp
	}
	return ""
}

// CoreStatus checks whether sing-box is available and parses its version string.
func CoreStatus() (available bool, version string, binPath string) {
	binPath = FindSingBoxBinary()
	if binPath == "" {
		return false, "", ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, binPath, "version").CombinedOutput()
	if err != nil {
		return true, "", binPath
	}

	firstLine := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	const prefix = "sing-box version "
	if strings.HasPrefix(strings.ToLower(firstLine), prefix) {
		version = strings.TrimSpace(firstLine[len(prefix):])
	} else {
		version = firstLine
	}
	return true, version, binPath
}

// SeedBundledRules copies bundled .srs files into <dataDir>/rules if missing locally.
func SeedBundledRules(dataDir string) error {
	if strings.TrimSpace(dataDir) == "" {
		return nil
	}
	targetDir := filepath.Join(filepath.Clean(dataDir), "rules")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create rules dir: %w", err)
	}

	sourceDirs := make([]string, 0, 2)
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		sourceDirs = append(sourceDirs,
			filepath.Join(exeDir, "rules"),
			filepath.Join(exeDir, "..", "Resources", "rules"),
		)
	}

	for _, srcDir := range sourceDirs {
		entries, err := os.ReadDir(srcDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".srs") {
				continue
			}
			dstPath := filepath.Join(targetDir, entry.Name())
			if _, statErr := os.Stat(dstPath); statErr == nil {
				continue
			}
			srcPath := filepath.Join(srcDir, entry.Name())
			data, readErr := os.ReadFile(srcPath)
			if readErr != nil {
				continue
			}
			if writeErr := writeFileAtomic(dstPath, data); writeErr != nil {
				return writeErr
			}
		}
	}
	return nil
}

type connSample struct {
	download  int64
	sampledAt time.Time
}

// ClashSampler polls Clash API /connections and computes per-connection download rates from deltas.
type ClashSampler struct {
	mu      sync.Mutex
	client  *http.Client
	samples map[string]connSample
}

// NewClashSampler constructs a ClashSampler with a 1.2s HTTP timeout.
func NewClashSampler() *ClashSampler {
	return &ClashSampler{
		client:  &http.Client{Timeout: clashConnTimeout},
		samples: make(map[string]connSample),
	}
}

var defaultClashSampler = NewClashSampler()

// FetchClashConnections queries Clash API /connections using the default sampler.
func FetchClashConnections(clashAPIAddr string) (ClashConnectionsSnapshot, error) {
	return defaultClashSampler.Fetch(clashAPIAddr)
}

type clashConnectionsPayload struct {
	DownloadTotal int64                 `json:"downloadTotal"`
	UploadTotal   int64                 `json:"uploadTotal"`
	Connections   []clashConnectionItem `json:"connections"`
}

type clashConnectionItem struct {
	ID          string            `json:"id"`
	Metadata    clashConnMetadata `json:"metadata"`
	Upload      int64             `json:"upload"`
	Download    int64             `json:"download"`
	Start       string            `json:"start"`
	Chains      []string          `json:"chains"`
	Rule        string            `json:"rule"`
	RulePayload string            `json:"rulePayload"`
}

type clashConnMetadata struct {
	Network         string          `json:"network"`
	Type            string          `json:"type"`
	SourceIP        string          `json:"sourceIP"`
	DestinationIP   string          `json:"destinationIP"`
	DestinationPort json.RawMessage `json:"destinationPort"`
	Host            string          `json:"host"`
	ProcessPath     string          `json:"processPath"`
	Process         string          `json:"process"`
}

// Fetch queries GET /connections on clashAPIAddr and converts active sessions into contract.LiveConnection.
func (s *ClashSampler) Fetch(clashAPIAddr string) (ClashConnectionsSnapshot, error) {
	if s == nil {
		return ClashConnectionsSnapshot{}, errors.New("clash sampler is nil")
	}

	endpoint := buildClashBaseURL(clashAPIAddr) + "/connections"
	ctx, cancel := context.WithTimeout(context.Background(), clashConnTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ClashConnectionsSnapshot{}, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return ClashConnectionsSnapshot{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ClashConnectionsSnapshot{}, fmt.Errorf("clash /connections returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return ClashConnectionsSnapshot{}, err
	}

	var raw clashConnectionsPayload
	if err := json.Unmarshal(body, &raw); err != nil {
		return ClashConnectionsSnapshot{}, fmt.Errorf("decode clash /connections: %w", err)
	}

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	nextSamples := make(map[string]connSample, len(raw.Connections))
	out := make([]contract.LiveConnection, 0, len(raw.Connections))

	for _, item := range raw.Connections {
		proc := baseProcessName(item.Metadata.ProcessPath)
		if proc == "" {
			proc = strings.TrimSpace(item.Metadata.Process)
		}
		if proc == "" {
			proc = "system"
		}

		host := strings.TrimSpace(item.Metadata.Host)
		dstIP := strings.TrimSpace(item.Metadata.DestinationIP)
		if host == "" {
			host = dstIP
		}

		port := parsePort(item.Metadata.DestinationPort)
		network := strings.ToUpper(strings.TrimSpace(item.Metadata.Network))
		if network == "" {
			network = "TCP"
		}

		downKBps := 0.0
		if prev, ok := s.samples[item.ID]; ok {
			elapsed := now.Sub(prev.sampledAt).Seconds()
			if elapsed > 0 && item.Download >= prev.download {
				downKBps = math.Round((float64(item.Download-prev.download)/1024.0/elapsed)*10) / 10
			}
		}
		if item.ID != "" {
			nextSamples[item.ID] = connSample{
				download:  item.Download,
				sampledAt: now,
			}
		}

		out = append(out, contract.LiveConnection{
			ID:          item.ID,
			Process:     proc,
			Host:        host,
			DstIP:       dstIP,
			Port:        port,
			Network:     network,
			MatchedRule: formatMatchedRule(item.Rule, item.RulePayload),
			RouteAction: inferRouteAction(item.Chains),
			DownKBps:    downKBps,
			TotalBytes:  formatByteCount(item.Download + item.Upload),
			StartedAt:   formatConnStartTime(item.Start),
		})
	}

	s.samples = nextSamples

	return ClashConnectionsSnapshot{
		DownloadTotal: raw.DownloadTotal,
		UploadTotal:   raw.UploadTotal,
		Connections:   out,
	}, nil
}

func buildClashBaseURL(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		trimmed = DefaultClashAPIAddr
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return strings.TrimRight(trimmed, "/")
	}
	return "http://" + strings.TrimRight(trimmed, "/")
}

func hasOnlineExitNode(orgs []contract.Organization, nodes []contract.ExitNode) bool {
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

func isExecutableFile(p string) bool {
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return false
	}
	return true
}

func writeFileAtomic(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func baseProcessName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	idxSlash := strings.LastIndexAny(trimmed, `/\`)
	if idxSlash >= 0 && idxSlash < len(trimmed)-1 {
		return trimmed[idxSlash+1:]
	}
	return trimmed
}

func parsePort(raw json.RawMessage) int {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0
	}
	trimmed = strings.Trim(trimmed, `"`)
	port, err := strconv.Atoi(trimmed)
	if err != nil || port < 0 {
		return 0
	}
	return port
}

func formatMatchedRule(rule, payload string) string {
	r := strings.TrimSpace(rule)
	p := strings.TrimSpace(payload)
	if r == "" && p == "" {
		return "FINAL"
	}
	if strings.EqualFold(r, "final") {
		return "FINAL"
	}
	if p == "" {
		return r
	}
	switch strings.ToLower(r) {
	case "rule_set", "ruleset":
		return p
	case "process_name", "process":
		return "PROCESS · " + p
	case "domain_suffix", "domain", "domain_keyword":
		return "DOMAIN · " + p
	default:
		return r + " · " + p
	}
}

func inferRouteAction(chains []string) contract.RuleAction {
	for _, c := range chains {
		lower := strings.ToLower(strings.TrimSpace(c))
		switch {
		case lower == "reject" || lower == "block":
			return contract.ActionReject
		case lower == directTag:
			return contract.ActionDirect
		case lower == fallbackProxyTag:
			return contract.ActionFallbackProxy
		}
	}
	return contract.ActionExit
}

func formatByteCount(total int64) string {
	if total <= 0 {
		return "0 B"
	}
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case total >= gb:
		return fmt.Sprintf("%.1f GB", float64(total)/float64(gb))
	case total >= mb:
		return fmt.Sprintf("%.1f MB", float64(total)/float64(mb))
	case total >= kb:
		return fmt.Sprintf("%.1f KB", float64(total)/float64(kb))
	default:
		return fmt.Sprintf("%d B", total)
	}
}

func formatConnStartTime(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed.Local().Format("15:04:05")
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.Local().Format("15:04:05")
	}
	return trimmed
}

func tailOutputLines(raw string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			cleaned = append(cleaned, t)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) > maxLines {
		cleaned = cleaned[len(cleaned)-maxLines:]
	}
	return strings.Join(cleaned, " | ")
}

func looksLikePrivilegeError(msg string) bool {
	lower := strings.ToLower(msg)
	for _, kw := range []string{
		"operation not permitted",
		"permission denied",
		"access is denied",
		"administrator",
		"elevated",
		"privilege",
		"wintun",
		"utun",
		"create tun",
		"configure tun",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func newTailBuffer(maxBytes int) *tailBuffer {
	return &tailBuffer{max: maxBytes}
}

func (tb *tailBuffer) Write(p []byte) (int, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.buf = append(tb.buf, p...)
	if len(tb.buf) > tb.max {
		tb.buf = append([]byte(nil), tb.buf[len(tb.buf)-tb.max:]...)
	}
	return len(p), nil
}

func (tb *tailBuffer) String() string {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return string(tb.buf)
}
