package tailnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/singtail/singtail/internal/contract"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
)

const (
	loginSessionTimeout = 5 * time.Minute
	importStateTimeout  = 15 * time.Second
	refreshTsnetTimeout = 12 * time.Second
	errNotAuthenticated = "该目录尚未完成 Tailscale 登录授权"
)

// StartLogin initiates an interactive tsnet browser OAuth flow for a new organization
// and returns an initial OrgLoginSession handle for polling via PollLogin.
func (c *Coordinator) StartLogin(ctx context.Context, req contract.StartOrgLoginRequest) (contract.OrgLoginSession, error) {
	if ctx != nil && ctx.Err() != nil {
		return contract.OrgLoginSession{}, ctx.Err()
	}

	c.mu.Lock()
	c.seq++
	seq := c.seq
	slug := sanitizeSlug(req.Alias)
	if slug == "" {
		slug = fmt.Sprintf("tailnet-%d", seq)
	}
	orgID := fmt.Sprintf("org-%s", slug)
	for c.hasOrgIDLocked(orgID) {
		c.seq++
		orgID = fmt.Sprintf("org-%s-%d", slug, c.seq)
	}

	sessionID := fmt.Sprintf("login-%d-%d", time.Now().UnixNano(), seq)
	stateDir := filepath.Join(c.stateRoot, "ts-state-"+orgID)
	session := contract.OrgLoginSession{
		SessionID: sessionID,
		State:     "starting",
	}
	c.sessions[sessionID] = session
	c.mu.Unlock()

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		c.failLoginSession(sessionID, fmt.Errorf("创建组织状态目录失败: %w", err))
		return contract.OrgLoginSession{}, err
	}

	hostname := sanitizeHostname("singtail-" + orgID)
	go c.runLoginFlow(sessionID, orgID, stateDir, hostname, req)

	return session, nil
}

// PollLogin returns the current status snapshot of a login session started by StartLogin.
func (c *Coordinator) PollLogin(sessionID string) (contract.OrgLoginSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sess, ok := c.sessions[strings.TrimSpace(sessionID)]
	return sess, ok
}

// ImportStateDir imports an existing authenticated tsnet/tailscaled state directory into Coordinator,
// discovers all exit nodes offered by peers, and immediately closes the temporary tsnet instance.
func (c *Coordinator) ImportStateDir(ctx context.Context, req contract.ImportOrgStateDirRequest) (contract.Organization, []contract.ExitNode, error) {
	if ctx != nil && ctx.Err() != nil {
		return contract.Organization{}, nil, ctx.Err()
	}

	srcDir := strings.TrimSpace(req.SourceDir)
	if srcDir == "" {
		return contract.Organization{}, nil, errors.New("凭据目录路径不能为空")
	}
	info, err := os.Stat(srcDir)
	if err != nil {
		return contract.Organization{}, nil, fmt.Errorf("读取凭据目录失败: %w", err)
	}
	if !info.IsDir() {
		return contract.Organization{}, nil, errors.New("指定路径必须是目录")
	}

	c.mu.Lock()
	c.seq++
	seq := c.seq
	slug := sanitizeSlug(req.Alias)
	if slug == "" {
		slug = fmt.Sprintf("imported-%d", seq)
	}
	orgID := fmt.Sprintf("org-%s", slug)
	for c.hasOrgIDLocked(orgID) {
		c.seq++
		orgID = fmt.Sprintf("org-%s-%d", slug, c.seq)
	}
	c.mu.Unlock()

	destDir := filepath.Join(c.stateRoot, "ts-state-"+orgID)
	if err := copyStateDir(srcDir, destDir); err != nil {
		_ = os.RemoveAll(destDir)
		return contract.Organization{}, nil, fmt.Errorf("复制凭据目录失败: %w", err)
	}

	importCtx, cancel := context.WithTimeout(nonNilContext(ctx), importStateTimeout)
	defer cancel()

	st, org, nodes, err := readRunningStatusAndProbe(importCtx, destDir, "", sanitizeHostname("singtail-"+orgID), req.Alias, orgID, errNotAuthenticated)
	if err != nil {
		_ = os.RemoveAll(destDir)
		return contract.Organization{}, nil, err
	}
	_ = st

	c.mu.Lock()
	c.orgs = append(c.orgs, org)
	c.nodes = append(c.nodes, nodes...)
	c.mu.Unlock()

	outNodes := make([]contract.ExitNode, len(nodes))
	copy(outNodes, nodes)
	return org, outNodes, nil
}

// RefreshOrgFromTsnet starts a short-lived tsnet instance on the organization's stateDir,
// refreshes its exit node list while preserving known public IPs and latencies, and releases the directory lock.
func (c *Coordinator) RefreshOrgFromTsnet(ctx context.Context, orgID string) ([]contract.ExitNode, error) {
	target := strings.TrimSpace(orgID)
	if target == "" {
		return nil, errors.New("组织 ID 不能为空")
	}

	c.mu.RLock()
	var org contract.Organization
	found := false
	for _, item := range c.orgs {
		if item.ID == target {
			org = item
			found = true
			break
		}
	}
	c.mu.RUnlock()

	if !found {
		return nil, fmt.Errorf("未找到组织: %s", target)
	}

	stateDir := strings.TrimSpace(org.StateDir)
	if stateDir == "" {
		stateDir = filepath.Join(c.stateRoot, "ts-state-"+org.ID)
	} else {
		stateDir = filepath.FromSlash(stateDir)
	}
	info, err := os.Stat(stateDir)
	if err != nil {
		return nil, fmt.Errorf("读取组织状态目录失败: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("组织状态路径必须是目录")
	}

	refreshCtx, cancel := context.WithTimeout(nonNilContext(ctx), refreshTsnetTimeout)
	defer cancel()

	_, refreshedOrg, newNodes, err := readRunningStatusAndProbe(refreshCtx, stateDir, org.ControlURL, sanitizeHostname("singtail-"+org.ID), org.Name, org.ID, errNotAuthenticated)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	orgIdx := -1
	for i := range c.orgs {
		if c.orgs[i].ID == target {
			orgIdx = i
			break
		}
	}
	if orgIdx < 0 {
		return nil, fmt.Errorf("未找到组织: %s", target)
	}

	oldByID := make(map[string]contract.ExitNode)
	oldByTSIP := make(map[string]contract.ExitNode)
	keptOtherNodes := make([]contract.ExitNode, 0, len(c.nodes))
	for _, n := range c.nodes {
		if n.OrgID != target {
			keptOtherNodes = append(keptOtherNodes, n)
			continue
		}
		oldByID[n.ID] = n
		if n.TailscaleIP != "" {
			oldByTSIP[n.TailscaleIP] = n
		}
	}

	for i := range newNodes {
		prev, ok := oldByID[newNodes[i].ID]
		if !ok && newNodes[i].TailscaleIP != "" {
			prev, ok = oldByTSIP[newNodes[i].TailscaleIP]
		}
		if ok {
			if prev.PublicIP != "" {
				newNodes[i].PublicIP = prev.PublicIP
			}
			if prev.LatencyMs > 0 {
				newNodes[i].LatencyMs = prev.LatencyMs
			}
		}
	}

	c.orgs[orgIdx].LastSync = "刚刚"
	if refreshedOrg.Account != "" {
		c.orgs[orgIdx].Account = refreshedOrg.Account
	}
	if refreshedOrg.LocalTailscaleIP != "" {
		c.orgs[orgIdx].LocalTailscaleIP = refreshedOrg.LocalTailscaleIP
	}
	c.nodes = append(keptOtherNodes, newNodes...)

	out := make([]contract.ExitNode, len(newNodes))
	copy(out, newNodes)
	return out, nil
}

func (c *Coordinator) runLoginFlow(sessionID, orgID, stateDir, hostname string, req contract.StartOrgLoginRequest) {
	watchCtx, cancel := context.WithTimeout(context.Background(), loginSessionTimeout)
	defer cancel()

	srv := &tsnet.Server{
		Dir:        stateDir,
		Hostname:   hostname,
		ControlURL: strings.TrimSpace(req.ControlURL),
		Ephemeral:  false,
		Logf:       func(string, ...any) {},
	}

	srvClosed := false
	closeServer := func() {
		if !srvClosed {
			srvClosed = true
			_ = srv.Close()
		}
	}
	defer closeServer()

	if err := srv.Start(); err != nil {
		c.failLoginSession(sessionID, fmt.Errorf("启动 tsnet 登录实例失败: %w", err))
		_ = os.RemoveAll(stateDir)
		return
	}

	lc, err := srv.LocalClient()
	if err != nil {
		c.failLoginSession(sessionID, fmt.Errorf("获取 tsnet LocalClient 失败: %w", err))
		_ = os.RemoveAll(stateDir)
		return
	}

	watcher, err := lc.WatchIPNBus(watchCtx, ipn.NotifyInitialState)
	if err != nil {
		c.failLoginSession(sessionID, fmt.Errorf("监听 tsnet 登录状态失败: %w", err))
		_ = os.RemoveAll(stateDir)
		return
	}
	defer func() { _ = watcher.Close() }()

	loginRequested := false
	for {
		n, err := watcher.Next()
		if err != nil {
			if st, stErr := lc.Status(watchCtx); stErr == nil && st != nil && st.BackendState == "Running" {
				org, nodes := extractOrgAndNodesFromStatus(orgID, stateDir, req.Alias, req.ControlURL, st)
				probeExitNodesViaTsnet(watchCtx, srv, lc, st, nodes)
				_ = watcher.Close()
				closeServer()
				c.completeLoginSession(sessionID, org, nodes)
				return
			}
			c.failLoginSession(sessionID, fmt.Errorf("等待 Tailscale 授权超时或中断: %w", err))
			closeServer()
			_ = os.RemoveAll(stateDir)
			return
		}

		if n.BrowseToURL != nil && strings.TrimSpace(*n.BrowseToURL) != "" {
			c.updateLoginAuthURL(sessionID, strings.TrimSpace(*n.BrowseToURL))
		}

		st, stErr := lc.Status(watchCtx)
		if stErr == nil && st != nil {
			if strings.TrimSpace(st.AuthURL) != "" {
				c.updateLoginAuthURL(sessionID, strings.TrimSpace(st.AuthURL))
			}
			if st.BackendState == "Running" {
				org, nodes := extractOrgAndNodesFromStatus(orgID, stateDir, req.Alias, req.ControlURL, st)
				probeExitNodesViaTsnet(watchCtx, srv, lc, st, nodes)
				_ = watcher.Close()
				closeServer()
				c.completeLoginSession(sessionID, org, nodes)
				return
			}
			if !loginRequested && (st.BackendState == "NeedsLogin" || st.BackendState == "NoState" || st.BackendState == "Stopped") {
				loginRequested = true
				_ = lc.StartLoginInteractive(watchCtx)
			}
			continue
		}

		if !loginRequested && n.State != nil && (*n.State == ipn.NeedsLogin || *n.State == ipn.NoState || *n.State == ipn.Stopped) {
			loginRequested = true
			_ = lc.StartLoginInteractive(watchCtx)
		}
	}
}

func (c *Coordinator) updateLoginAuthURL(sessionID, authURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	sess, ok := c.sessions[sessionID]
	if !ok || sess.State == "authenticated" || sess.State == "error" {
		return
	}
	sess.State = "auth_url"
	sess.AuthURL = authURL
	c.sessions[sessionID] = sess
}

func (c *Coordinator) completeLoginSession(sessionID string, org contract.Organization, nodes []contract.ExitNode) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.orgs = append(c.orgs, org)
	c.nodes = append(c.nodes, nodes...)

	sess := c.sessions[sessionID]
	sess.SessionID = sessionID
	sess.State = "authenticated"
	sess.Error = ""
	sess.OrgName = org.Name
	sess.NodeCount = len(nodes)
	c.sessions[sessionID] = sess
}

func (c *Coordinator) failLoginSession(sessionID string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	sess := c.sessions[sessionID]
	sess.SessionID = sessionID
	sess.State = "error"
	if err != nil {
		sess.Error = err.Error()
	}
	c.sessions[sessionID] = sess
}

func readRunningStatusAndProbe(ctx context.Context, stateDir, controlURL, hostname, alias, orgID, unauthMsg string) (*ipnstate.Status, contract.Organization, []contract.ExitNode, error) {
	srv := &tsnet.Server{
		Dir:        stateDir,
		Hostname:   hostname,
		ControlURL: strings.TrimSpace(controlURL),
		Ephemeral:  false,
		Logf:       func(string, ...any) {},
	}
	if err := srv.Start(); err != nil {
		_ = srv.Close()
		return nil, contract.Organization{}, nil, fmt.Errorf("启动 tsnet 实例失败: %w", err)
	}
	defer func() { _ = srv.Close() }()

	lc, err := srv.LocalClient()
	if err != nil {
		return nil, contract.Organization{}, nil, fmt.Errorf("获取 tsnet LocalClient 失败: %w", err)
	}

	watcher, err := lc.WatchIPNBus(ctx, ipn.NotifyInitialState)
	if err != nil {
		return nil, contract.Organization{}, nil, fmt.Errorf("监听 tsnet 状态失败: %w", err)
	}
	defer func() { _ = watcher.Close() }()

	for {
		n, err := watcher.Next()
		if err != nil {
			if st, stErr := lc.Status(ctx); stErr == nil && st != nil && st.BackendState == "Running" {
				org, nodes := extractOrgAndNodesFromStatus(orgID, stateDir, alias, controlURL, st)
				probeExitNodesViaTsnet(ctx, srv, lc, st, nodes)
				return st, org, nodes, nil
			}
			if ctx.Err() != nil {
				return nil, contract.Organization{}, nil, errors.New(unauthMsg)
			}
			return nil, contract.Organization{}, nil, fmt.Errorf("读取 tsnet 状态失败: %w", err)
		}

		if st, stErr := lc.Status(ctx); stErr == nil && st != nil {
			switch st.BackendState {
			case "Running":
				org, nodes := extractOrgAndNodesFromStatus(orgID, stateDir, alias, controlURL, st)
				probeExitNodesViaTsnet(ctx, srv, lc, st, nodes)
				return st, org, nodes, nil
			case "NeedsLogin", "NeedsMachineAuth", "Stopped":
				return nil, contract.Organization{}, nil, errors.New(unauthMsg)
			}
		}

		if n.State != nil {
			switch *n.State {
			case ipn.NeedsLogin, ipn.NeedsMachineAuth, ipn.Stopped:
				return nil, contract.Organization{}, nil, errors.New(unauthMsg)
			}
		}
	}
}

// probeExitNodesViaTsnet routes short-lived userspace HTTP requests through up to 4 online exit nodes
// to automatically populate each node's real public egress IP and round-trip latency right after authorization.
func probeExitNodesViaTsnet(parentCtx context.Context, srv *tsnet.Server, lc *local.Client, st *ipnstate.Status, nodes []contract.ExitNode) {
	if srv == nil || lc == nil || st == nil || len(nodes) == 0 {
		return
	}

	peerIDByIP := make(map[string]tailcfg.StableNodeID)
	peerIDByHost := make(map[string]tailcfg.StableNodeID)
	for _, peer := range st.Peer {
		if peer == nil || !peer.ExitNodeOption || !peer.Online || peer.ID.IsZero() {
			continue
		}
		for _, ip := range peer.TailscaleIPs {
			peerIDByIP[ip.String()] = peer.ID
		}
		if h := strings.ToLower(strings.TrimSpace(peer.HostName)); h != "" {
			peerIDByHost[h] = peer.ID
		}
	}
	if len(peerIDByIP) == 0 && len(peerIDByHost) == 0 {
		return
	}

	defer func() {
		clearCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = lc.EditPrefs(clearCtx, &ipn.MaskedPrefs{
			Prefs:         ipn.Prefs{ExitNodeID: ""},
			ExitNodeIDSet: true,
		})
	}()

	budgetCtx, cancelBudget := context.WithTimeout(nonNilContext(parentCtx), 8*time.Second)
	defer cancelBudget()

	client := srv.HTTPClient()
	client.Timeout = 2800 * time.Millisecond

	probed := 0
	for i := range nodes {
		if probed >= 4 || budgetCtx.Err() != nil {
			break
		}
		if !nodes[i].Online || strings.TrimSpace(nodes[i].PublicIP) != "" {
			continue
		}

		stableID := peerIDByIP[nodes[i].TailscaleIP]
		if stableID.IsZero() {
			stableID = peerIDByHost[strings.ToLower(strings.TrimSpace(nodes[i].Hostname))]
		}
		if stableID.IsZero() {
			continue
		}

		probed++
		if _, err := lc.EditPrefs(budgetCtx, &ipn.MaskedPrefs{
			Prefs:         ipn.Prefs{ExitNodeID: stableID},
			ExitNodeIDSet: true,
		}); err != nil {
			continue
		}

		ip, rttMs := fetchPublicIPViaClient(budgetCtx, client)
		if ip != "" {
			nodes[i].PublicIP = ip
			if nodes[i].LatencyMs <= 0 && rttMs > 0 {
				nodes[i].LatencyMs = rttMs
			}
		}
	}
}

func fetchPublicIPViaClient(ctx context.Context, client *http.Client) (string, int) {
	endpoints := []string{
		"https://api.ipify.org?format=text",
		"https://checkip.amazonaws.com",
	}
	for _, ep := range endpoints {
		if ctx.Err() != nil {
			return "", 0
		}
		start := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		candidate := strings.TrimSpace(string(body))
		if parsed := net.ParseIP(candidate); parsed != nil && !parsed.IsUnspecified() && !parsed.IsLoopback() {
			ms := int(time.Since(start).Milliseconds())
			if ms < 1 {
				ms = 1
			}
			return parsed.String(), ms
		}
	}
	return "", 0
}

func extractOrgAndNodesFromStatus(orgID, stateDir, alias, controlURL string, st *ipnstate.Status) (contract.Organization, []contract.ExitNode) {
	orgName := strings.TrimSpace(alias)
	if orgName == "" && st != nil && st.CurrentTailnet != nil {
		orgName = strings.TrimSpace(st.CurrentTailnet.Name)
	}
	if orgName == "" {
		orgName = "Tailscale Org"
	}

	var account string
	if st != nil && st.Self != nil && st.User != nil {
		if profile, ok := st.User[st.Self.UserID]; ok {
			account = strings.TrimSpace(profile.LoginName)
		}
	}

	var localIP string
	if st != nil {
		for _, ip := range st.TailscaleIPs {
			if ip.Is4() {
				localIP = ip.String()
				break
			}
		}
		if localIP == "" && len(st.TailscaleIPs) > 0 {
			localIP = st.TailscaleIPs[0].String()
		}
	}

	org := contract.Organization{
		ID:               orgID,
		Name:             orgName,
		Account:          account,
		LocalTailscaleIP: localIP,
		ControlURL:       strings.TrimSpace(controlURL),
		StateDir:         filepath.ToSlash(stateDir),
		LastSync:         "刚刚",
	}

	nodes := make([]contract.ExitNode, 0)
	if st == nil {
		return org, nodes
	}

	for pubKey, peer := range st.Peer {
		if peer == nil || !peer.ExitNodeOption {
			continue
		}

		hostname := strings.TrimSpace(peer.HostName)
		if hostname == "" && peer.DNSName != "" {
			hostname = strings.Split(strings.TrimSuffix(peer.DNSName, "."), ".")[0]
		}
		if hostname == "" {
			hostname = "exit-node"
		}

		label := hostname
		if peer.Location != nil && strings.TrimSpace(peer.Location.City) != "" {
			label = fmt.Sprintf("%s · %s", strings.TrimSpace(peer.Location.City), hostname)
		}

		var tsIP string
		for _, addr := range peer.TailscaleIPs {
			if addr.Is4() {
				tsIP = addr.String()
				break
			}
		}
		if tsIP == "" && len(peer.TailscaleIPs) > 0 {
			tsIP = peer.TailscaleIPs[0].String()
		}

		region := resolvePeerRegion(peer)
		relayMode := "中继"
		if strings.TrimSpace(peer.CurAddr) != "" {
			relayMode = "直连"
		}

		peerSuffix := sanitizeSlug(string(peer.ID))
		if peerSuffix == "" {
			peerSuffix = sanitizeSlug(hostname)
		}
		if peerSuffix == "" {
			peerSuffix = sanitizeSlug(pubKey.ShortString())
		}
		nodeID := fmt.Sprintf("ts-%s-%s", orgID, peerSuffix)

		nodes = append(nodes, contract.ExitNode{
			ID:          nodeID,
			OrgID:       org.ID,
			OrgName:     org.Name,
			Hostname:    hostname,
			Label:       label,
			TailscaleIP: tsIP,
			PublicIP:    "",
			Region:      region,
			RelayMode:   relayMode,
			LatencyMs:   0,
			Online:      peer.Online,
			Healthy:     peer.Online,
		})
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Hostname != nodes[j].Hostname {
			return nodes[i].Hostname < nodes[j].Hostname
		}
		return nodes[i].ID < nodes[j].ID
	})

	return org, nodes
}

func resolvePeerRegion(peer *ipnstate.PeerStatus) string {
	if peer.Location != nil {
		cc := strings.TrimSpace(peer.Location.CountryCode)
		city := strings.TrimSpace(peer.Location.City)
		switch {
		case cc != "" && city != "":
			return cc + " · " + city
		case cc != "":
			return cc
		case city != "":
			return city
		}
	}
	if relay := strings.TrimSpace(peer.Relay); relay != "" {
		return strings.ToUpper(relay)
	}
	return "Tailnet"
}

func copyStateDir(srcDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		destPath := filepath.Join(destDir, entry.Name())
		if entry.IsDir() {
			if err := copyStateDir(srcPath, destPath); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := copyFile(srcPath, destPath, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(srcPath, destPath string, perm os.FileMode) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	if perm == 0 {
		perm = 0o600
	}
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func sanitizeHostname(raw string) string {
	slug := sanitizeSlug(raw)
	if slug == "" {
		return "singtail-client"
	}
	if len(slug) > 63 {
		slug = strings.TrimRight(slug[:63], "-")
	}
	if slug == "" {
		return "singtail-client"
	}
	return slug
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
