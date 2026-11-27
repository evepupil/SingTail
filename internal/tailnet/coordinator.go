// Package tailnet manages multi-organization Tailscale identity state and exit node discovery.
package tailnet

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/singtail/singtail/internal/contract"
)

const pingDialTimeout = 1500 * time.Millisecond

// Coordinator manages authenticated Tailscale organizations and their discovered exit nodes.
type Coordinator struct {
	mu        sync.RWMutex
	stateRoot string
	demoMode  bool
	orgs      []contract.Organization
	nodes     []contract.ExitNode
	sessions  map[string]contract.OrgLoginSession
	seq       int
}

// NewCoordinator constructs a Coordinator with per-organization state directories.
// In production mode (demoMode == false), empty initialOrgs yields empty slices.
// Demo organizations and nodes are loaded only when demoMode == true and initialOrgs is empty.
func NewCoordinator(stateRoot string, initialOrgs []contract.Organization, initialNodes []contract.ExitNode, demoMode bool) *Coordinator {
	c := &Coordinator{
		stateRoot: stateRoot,
		demoMode:  demoMode,
		orgs:      make([]contract.Organization, 0),
		nodes:     make([]contract.ExitNode, 0),
		sessions:  make(map[string]contract.OrgLoginSession),
	}

	if len(initialOrgs) == 0 {
		if demoMode {
			c.orgs = defaultDemoOrgs(stateRoot)
			c.nodes = defaultDemoNodes()
		}
		return c
	}

	c.orgs = make([]contract.Organization, len(initialOrgs))
	for i, org := range initialOrgs {
		if org.StateDir == "" && org.ID != "" {
			org.StateDir = orgStateDir(stateRoot, org.ID)
		}
		c.orgs[i] = org
	}

	if len(initialNodes) > 0 {
		c.nodes = make([]contract.ExitNode, len(initialNodes))
		copy(c.nodes, initialNodes)
	}

	return c
}

func orgStateDir(stateRoot, orgID string) string {
	return filepath.ToSlash(filepath.Join(stateRoot, "ts-state-"+orgID))
}

func defaultDemoOrgs(stateRoot string) []contract.Organization {
	return []contract.Organization{
		{
			ID:               "org-personal",
			Name:             "Personal Tailnet",
			Account:          "alex.dev@gmail.com",
			LocalTailscaleIP: "100.98.210.14",
			StateDir:         orgStateDir(stateRoot, "org-personal"),
			LastSync:         "刚刚",
		},
		{
			ID:               "org-studio",
			Name:             "GolfJoy Studio",
			Account:          "ops@golfjoy.io",
			LocalTailscaleIP: "100.123.88.42",
			StateDir:         orgStateDir(stateRoot, "org-studio"),
			LastSync:         "1 分钟前",
		},
	}
}

func defaultDemoNodes() []contract.ExitNode {
	return []contract.ExitNode{
		{
			ID:          "ts-us-macmini",
			OrgID:       "org-studio",
			OrgName:     "GolfJoy Studio",
			Hostname:    "us-la-macmini-m2",
			Label:       "洛杉矶 Mac mini",
			TailscaleIP: "100.123.114.28",
			PublicIP:    "68.225.41.109",
			Region:      "US · Los Angeles",
			RelayMode:   "直连",
			LatencyMs:   138,
			Online:      true,
			Healthy:     true,
		},
		{
			ID:          "ts-hk-cloud",
			OrgID:       "org-personal",
			OrgName:     "Personal Tailnet",
			Hostname:    "hk-edge-cvm-01",
			Label:       "香港轻量云",
			TailscaleIP: "100.91.77.63",
			PublicIP:    "43.153.82.19",
			Region:      "HK · Hong Kong",
			RelayMode:   "直连",
			LatencyMs:   34,
			Online:      true,
			Healthy:     true,
		},
		{
			ID:          "ts-jp-tokyo",
			OrgID:       "org-personal",
			OrgName:     "Personal Tailnet",
			Hostname:    "jp-tyo-router",
			Label:       "东京软路由",
			TailscaleIP: "100.91.104.12",
			PublicIP:    "172.104.92.77",
			Region:      "JP · Tokyo",
			RelayMode:   "直连",
			LatencyMs:   62,
			Online:      true,
			Healthy:     true,
		},
		{
			ID:          "ts-sg-relay",
			OrgID:       "org-studio",
			OrgName:     "GolfJoy Studio",
			Hostname:    "sg-build-gateway",
			Label:       "新加坡网关",
			TailscaleIP: "100.123.45.90",
			PublicIP:    "18.141.209.54",
			Region:      "SG · Singapore",
			RelayMode:   "中继",
			LatencyMs:   89,
			Online:      true,
			Healthy:     true,
		},
		{
			ID:          "ts-us-broken",
			OrgID:       "org-personal",
			OrgName:     "Personal Tailnet",
			Hostname:    "us-sjc-sandbox",
			Label:       "圣何塞节点",
			TailscaleIP: "100.91.201.99",
			PublicIP:    "203.0.113.88",
			Region:      "US · San Jose",
			RelayMode:   "中继",
			LatencyMs:   285,
			Online:      true,
			Healthy:     false,
		},
	}
}

// ListOrgs returns a copy of all registered Tailscale organizations.
func (c *Coordinator) ListOrgs() []contract.Organization {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]contract.Organization, len(c.orgs))
	copy(out, c.orgs)
	return out
}

// ListExitNodes returns a copy of all discovered exit nodes across organizations.
func (c *Coordinator) ListExitNodes() []contract.ExitNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]contract.ExitNode, len(c.nodes))
	copy(out, c.nodes)
	return out
}

// RemoveOrg deletes the organization and its exit nodes, and removes its ts-state directory when present.
func (c *Coordinator) RemoveOrg(orgID string) error {
	target := strings.TrimSpace(orgID)
	if target == "" {
		return errors.New("组织 ID 不能为空")
	}

	c.mu.Lock()
	orgIdx := -1
	var stateDir string
	for i := range c.orgs {
		if c.orgs[i].ID == target {
			orgIdx = i
			stateDir = c.orgs[i].StateDir
			break
		}
	}
	if orgIdx < 0 {
		c.mu.Unlock()
		return fmt.Errorf("未找到组织: %s", target)
	}

	c.orgs = append(c.orgs[:orgIdx], c.orgs[orgIdx+1:]...)

	filtered := make([]contract.ExitNode, 0, len(c.nodes))
	for _, n := range c.nodes {
		if n.OrgID != target {
			filtered = append(filtered, n)
		}
	}
	c.nodes = filtered
	if stateDir == "" && c.stateRoot != "" {
		stateDir = filepath.Join(c.stateRoot, "ts-state-"+target)
	}
	c.mu.Unlock()

	if stateDir != "" {
		_ = os.RemoveAll(filepath.FromSlash(stateDir))
	}
	return nil
}

// UpdateNodePublicIP updates the verified public IP address for a discovered exit node.
func (c *Coordinator) UpdateNodePublicIP(nodeID, publicIP string) {
	target := strings.TrimSpace(nodeID)
	if target == "" {
		return
	}
	ip := strings.TrimSpace(publicIP)

	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.nodes {
		if c.nodes[i].ID == target {
			c.nodes[i].PublicIP = ip
			return
		}
	}
}

// AddOrg registers a new Tailscale organization with an isolated state directory
// and prepends its primary exit node to the exit node list.
func (c *Coordinator) AddOrg(req contract.AddOrgRequest) (contract.Organization, contract.ExitNode, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return contract.Organization{}, contract.ExitNode{}, errors.New("组织名称不能为空")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.seq++
	slug := sanitizeSlug(name)
	if slug == "" {
		slug = fmt.Sprintf("custom-%d", c.seq)
	}
	orgID := fmt.Sprintf("org-%s", slug)
	for c.hasOrgIDLocked(orgID) {
		c.seq++
		orgID = fmt.Sprintf("org-%s-%d", slug, c.seq)
	}

	account := strings.TrimSpace(req.Account)
	if account == "" {
		account = "member@tailnet.local"
	}

	hostOctet := 60 + (c.seq % 180)
	org := contract.Organization{
		ID:               orgID,
		Name:             name,
		Account:          account,
		LocalTailscaleIP: fmt.Sprintf("100.100.%d.10", hostOctet),
		StateDir:         orgStateDir(c.stateRoot, orgID),
		LastSync:         "刚刚",
	}

	hostname := strings.TrimSpace(req.NodeHostname)
	if hostname == "" {
		hostname = fmt.Sprintf("%s-exit", slug)
	}
	label := hostname
	if strings.TrimSpace(req.NodeHostname) == "" {
		label = name + " 出口"
	}

	publicIP := strings.TrimSpace(req.PublicIP)
	if publicIP == "" {
		publicIP = fmt.Sprintf("198.51.100.%d", 10+(c.seq%200))
	}

	region := strings.TrimSpace(req.Region)
	if region == "" {
		region = "Custom · Tailnet"
	}

	nodeID := fmt.Sprintf("ts-%s", slug)
	for c.hasNodeIDLocked(nodeID) {
		c.seq++
		nodeID = fmt.Sprintf("ts-%s-%d", slug, c.seq)
	}

	node := contract.ExitNode{
		ID:          nodeID,
		OrgID:       org.ID,
		OrgName:     org.Name,
		Hostname:    hostname,
		Label:       label,
		TailscaleIP: fmt.Sprintf("100.100.%d.20", hostOctet),
		PublicIP:    publicIP,
		Region:      region,
		RelayMode:   "直连",
		LatencyMs:   48,
		Online:      true,
		Healthy:     true,
	}

	c.orgs = append(c.orgs, org)
	c.nodes = append([]contract.ExitNode{node}, c.nodes...)
	return org, node, nil
}

// SyncOrg refreshes the synchronization timestamp for the target organization,
// or for all organizations when orgID is empty.
func (c *Coordinator) SyncOrg(orgID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	target := strings.TrimSpace(orgID)
	if target == "" {
		for i := range c.orgs {
			c.orgs[i].LastSync = "刚刚"
		}
		return nil
	}

	for i := range c.orgs {
		if c.orgs[i].ID == target {
			c.orgs[i].LastSync = "刚刚"
			return nil
		}
	}
	return fmt.Errorf("未找到组织: %s", target)
}

// PingNodes refreshes latency metrics for all nodes.
// In demoMode, it simulates latency fluctuation. In production mode, it measures real TCP dial
// duration against each online node's TailscaleIP:80 and preserves existing values when unreachable.
func (c *Coordinator) PingNodes(ctx context.Context) []contract.ExitNode {
	if ctx != nil && ctx.Err() != nil {
		return c.ListExitNodes()
	}

	if c.demoMode {
		return c.pingDemoNodes()
	}

	c.mu.RLock()
	snapshot := make([]contract.ExitNode, len(c.nodes))
	copy(snapshot, c.nodes)
	c.mu.RUnlock()

	if len(snapshot) == 0 {
		return []contract.ExitNode{}
	}

	if ctx == nil {
		ctx = context.Background()
	}

	measured := make(map[string]int, len(snapshot))
	var measureMu sync.Mutex
	var wg sync.WaitGroup
	dialer := net.Dialer{Timeout: pingDialTimeout}

	for _, node := range snapshot {
		if !node.Online || strings.TrimSpace(node.TailscaleIP) == "" {
			continue
		}
		wg.Add(1)
		go func(n contract.ExitNode) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, pingDialTimeout)
			defer cancel()

			start := time.Now()
			conn, err := dialer.DialContext(probeCtx, "tcp", net.JoinHostPort(strings.TrimSpace(n.TailscaleIP), "80"))
			if err != nil {
				return
			}
			_ = conn.Close()
			ms := int(time.Since(start).Milliseconds())
			if ms < 1 {
				ms = 1
			}
			measureMu.Lock()
			measured[n.ID] = ms
			measureMu.Unlock()
		}(node)
	}
	wg.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.nodes {
		if ms, ok := measured[c.nodes[i].ID]; ok {
			c.nodes[i].LatencyMs = ms
		}
	}

	out := make([]contract.ExitNode, len(c.nodes))
	copy(out, c.nodes)
	return out
}

func (c *Coordinator) pingDemoNodes() []contract.ExitNode {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.nodes {
		if !c.nodes[i].Healthy || !c.nodes[i].Online {
			c.nodes[i].LatencyMs = 299
			continue
		}
		delta := rand.IntN(9) - 4 // [-4, +4]
		next := c.nodes[i].LatencyMs + delta
		if next < 18 {
			next = 18
		}
		if next > 240 {
			next = 240
		}
		c.nodes[i].LatencyMs = next
	}

	out := make([]contract.ExitNode, len(c.nodes))
	copy(out, c.nodes)
	return out
}

func (c *Coordinator) hasOrgIDLocked(id string) bool {
	for _, org := range c.orgs {
		if org.ID == id {
			return true
		}
	}
	return false
}

func (c *Coordinator) hasNodeIDLocked(id string) bool {
	for _, node := range c.nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func sanitizeSlug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == ' ':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
