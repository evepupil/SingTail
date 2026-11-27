package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/singtail/singtail/internal/contract"
)

// Controller manages sing-box runtime state, live connections, and exit-node switching with rollback.
type Controller struct {
	switchMu sync.Mutex

	mu           sync.RWMutex
	prober       IPProber
	demoMode     bool
	tunEnabled   bool
	mode         contract.RoutingMode
	activeExitID string
	verifyingID  string
	notice       *contract.BannerNotice
	connections  []contract.LiveConnection
}

// NewController creates a Controller initialized with runtime flags.
// Live connections start empty in production mode and use sample entries only when demoMode is true.
func NewController(
	prober IPProber,
	tunEnabled bool,
	mode contract.RoutingMode,
	activeExitID string,
	demoMode bool,
) *Controller {
	if prober == nil {
		prober = NewDefaultProber(false)
	}
	if mode == "" {
		mode = contract.ModeRule
	}

	conns := []contract.LiveConnection{}
	if demoMode {
		conns = defaultLiveConnections()
	}

	return &Controller{
		prober:       prober,
		demoMode:     demoMode,
		tunEnabled:   tunEnabled,
		mode:         mode,
		activeExitID: activeExitID,
		connections:  conns,
	}
}

// SetTun updates the TUN virtual interface enabled state.
func (ctrl *Controller) SetTun(enabled bool) {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	ctrl.tunEnabled = enabled
}

// SetMode updates the traffic routing mode.
func (ctrl *Controller) SetMode(mode contract.RoutingMode) {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	if mode != "" {
		ctrl.mode = mode
	}
}

// SetActiveExitID directly updates the active exit node ID without running a probe.
func (ctrl *Controller) SetActiveExitID(exitID string) {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	ctrl.activeExitID = exitID
}

// SetBanner records a banner notice for the UI.
func (ctrl *Controller) SetBanner(notice *contract.BannerNotice) {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	if notice == nil {
		ctrl.notice = nil
		return
	}
	cp := *notice
	ctrl.notice = &cp
}

// ClearBanner dismisses the active verification or rollback banner notice.
func (ctrl *Controller) ClearBanner() {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	ctrl.notice = nil
}

// SetConnections replaces the observed live connections list with a copy of conns.
func (ctrl *Controller) SetConnections(conns []contract.LiveConnection) {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	if len(conns) == 0 {
		ctrl.connections = []contract.LiveConnection{}
		return
	}
	cp := make([]contract.LiveConnection, len(conns))
	copy(cp, conns)
	ctrl.connections = cp
}

// State returns a thread-safe snapshot of the controller state.
func (ctrl *Controller) State() (tunEnabled bool, mode contract.RoutingMode, activeExitID string, verifyingID string, notice *contract.BannerNotice) {
	ctrl.mu.RLock()
	defer ctrl.mu.RUnlock()

	var noticeCopy *contract.BannerNotice
	if ctrl.notice != nil {
		n := *ctrl.notice
		noticeCopy = &n
	}
	return ctrl.tunEnabled, ctrl.mode, ctrl.activeExitID, ctrl.verifyingID, noticeCopy
}

// SwitchExit switches the active exit node and optionally verifies public IP egress.
// On verification failure or timeout, it automatically rolls back to the previous exit node
// and records a rollback BannerNotice.
func (ctrl *Controller) SwitchExit(
	ctx context.Context,
	target contract.ExitNode,
	allNodes []contract.ExitNode,
	verifyIP bool,
	proxyAddr string,
) (activeID string, notice *contract.BannerNotice, err error) {
	activeID, _, notice, err = ctrl.SwitchExitAndProbe(ctx, target, allNodes, verifyIP, proxyAddr)
	return activeID, notice, err
}

// SwitchExitAndProbe switches the active exit node, optionally verifies public IP egress,
// and returns the effective exit ID along with the probed public IP on success.
func (ctrl *Controller) SwitchExitAndProbe(
	ctx context.Context,
	target contract.ExitNode,
	allNodes []contract.ExitNode,
	verifyIP bool,
	proxyAddr string,
) (activeID string, probedIP string, notice *contract.BannerNotice, err error) {
	ctrl.switchMu.Lock()
	defer ctrl.switchMu.Unlock()

	targetLabel := target.Label
	if targetLabel == "" {
		targetLabel = target.ID
	}

	ctrl.mu.Lock()
	prevID := ctrl.activeExitID
	if !verifyIP {
		ctrl.activeExitID = target.ID
		ctrl.verifyingID = ""
		ctrl.notice = nil
		ctrl.mu.Unlock()
		return target.ID, "", nil, nil
	}
	ctrl.verifyingID = target.ID
	ctrl.notice = &contract.BannerNotice{
		Kind: "verifying",
		Text: fmt.Sprintf("正在核验 %s 出口连通性...", targetLabel),
	}
	prober := ctrl.prober
	ctrl.mu.Unlock()

	ip, probeErr := prober.ProbePublicIP(ctx, proxyAddr, target)
	if probeErr == nil {
		ctrl.mu.Lock()
		ctrl.activeExitID = target.ID
		ctrl.verifyingID = ""
		ctrl.notice = nil
		ctrl.mu.Unlock()
		return target.ID, ip, nil, nil
	}

	prevNode := resolvePrevNode(allNodes, prevID)
	rollbackNotice := contract.BannerNotice{
		Kind: "rollback",
		Text: fmt.Sprintf("%s 连通核验超时，已回滚至 %s (%s)", targetLabel, prevNode.Label, prevNode.PublicIP),
	}

	ctrl.mu.Lock()
	ctrl.activeExitID = prevID
	ctrl.verifyingID = ""
	ctrl.notice = &rollbackNotice
	ctrl.mu.Unlock()

	noticeCopy := rollbackNotice
	return prevID, "", &noticeCopy, probeErr
}

// ListConnections returns a snapshot of observed active connections, updating
// exit-routed RTT estimates from the current active exit node when available.
func (ctrl *Controller) ListConnections(activeExit contract.ExitNode) []contract.LiveConnection {
	ctrl.mu.RLock()
	defer ctrl.mu.RUnlock()

	out := make([]contract.LiveConnection, len(ctrl.connections))
	for i, conn := range ctrl.connections {
		if conn.RouteAction == contract.ActionExit && activeExit.LatencyMs > 0 {
			conn.RttMs = activeExit.LatencyMs
		}
		out[i] = conn
	}
	return out
}

// ApplyQuickRuleToConn updates the matched rule label and route action for a connection.
func (ctrl *Controller) ApplyQuickRuleToConn(connID, matchedRule string, action contract.RuleAction) {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()

	for i := range ctrl.connections {
		if ctrl.connections[i].ID == connID {
			ctrl.connections[i].MatchedRule = matchedRule
			ctrl.connections[i].RouteAction = action
			return
		}
	}
}

func resolvePrevNode(allNodes []contract.ExitNode, prevID string) contract.ExitNode {
	for _, n := range allNodes {
		if n.ID == prevID {
			if n.Label == "" {
				n.Label = n.ID
			}
			if n.PublicIP == "" {
				n.PublicIP = "-"
			}
			return n
		}
	}
	label := prevID
	if label == "" {
		label = "原出口"
	}
	return contract.ExitNode{
		ID:       prevID,
		Label:    label,
		PublicIP: "-",
	}
}

func defaultLiveConnections() []contract.LiveConnection {
	return []contract.LiveConnection{
		{
			ID:          "c-101",
			Process:     "Cursor.exe",
			Host:        "api.anthropic.com",
			DstIP:       "160.79.104.25",
			Port:        443,
			Network:     "TCP",
			MatchedRule: "FINAL",
			RouteAction: contract.ActionExit,
			RttMs:       142,
			DownKBps:    420.5,
			TotalBytes:  "3.8 MB",
			StartedAt:   "10:56:12",
		},
		{
			ID:          "c-102",
			Process:     "chrome.exe",
			Host:        "github.com",
			DstIP:       "140.82.114.4",
			Port:        443,
			Network:     "QUIC",
			MatchedRule: "FINAL",
			RouteAction: contract.ActionExit,
			RttMs:       138,
			DownKBps:    1840.0,
			TotalBytes:  "14.2 MB",
			StartedAt:   "10:55:48",
		},
		{
			ID:          "c-103",
			Process:     "YuanShen.exe",
			Host:        "cn-gf01-prd.yuanshen.com",
			DstIP:       "8.134.91.50",
			Port:        22102,
			Network:     "UDP",
			MatchedRule: "PROCESS · YuanShen.exe",
			RouteAction: contract.ActionDirect,
			RttMs:       24,
			DownKBps:    28.4,
			TotalBytes:  "9.1 MB",
			StartedAt:   "10:31:05",
		},
		{
			ID:          "c-104",
			Process:     "chrome.exe",
			Host:        "www.bilibili.com",
			DstIP:       "119.96.135.82",
			Port:        443,
			Network:     "TCP",
			MatchedRule: "geosite-cn",
			RouteAction: contract.ActionDirect,
			RttMs:       19,
			DownKBps:    3120.8,
			TotalBytes:  "68.4 MB",
			StartedAt:   "10:52:19",
		},
		{
			ID:          "c-105",
			Process:     "Code.exe",
			Host:        "chatgpt.com",
			DstIP:       "172.64.155.209",
			Port:        443,
			Network:     "TCP",
			MatchedRule: "chatgpt.com",
			RouteAction: contract.ActionExit,
			RttMs:       140,
			DownKBps:    196.2,
			TotalBytes:  "2.1 MB",
			StartedAt:   "10:54:03",
		},
		{
			ID:          "c-106",
			Process:     "verge-mihomo.exe",
			Host:        "hk-bgp-02.airport-node.net",
			DstIP:       "103.214.16.88",
			Port:        8443,
			Network:     "TCP",
			MatchedRule: "PROCESS · verge-mihomo.exe",
			RouteAction: contract.ActionDirect,
			RttMs:       31,
			DownKBps:    2490.0,
			TotalBytes:  "42.0 MB",
			StartedAt:   "10:49:33",
		},
		{
			ID:          "c-107",
			Process:     "chrome.exe",
			Host:        "secure-glimr.doubleclick.net",
			DstIP:       "0.0.0.0",
			Port:        443,
			Network:     "TCP",
			MatchedRule: "geosite-category-ads-all",
			RouteAction: contract.ActionReject,
			RttMs:       0,
			DownKBps:    0,
			TotalBytes:  "0 B",
			StartedAt:   "10:57:02",
		},
		{
			ID:          "c-108",
			Process:     "chrome.exe",
			Host:        "www.netflix.com",
			DstIP:       "52.26.71.19",
			Port:        443,
			Network:     "TCP",
			MatchedRule: "netflix.com",
			RouteAction: contract.ActionFallbackProxy,
			RttMs:       56,
			DownKBps:    2490.0,
			TotalBytes:  "42.0 MB",
			StartedAt:   "10:49:33",
		},
	}
}
