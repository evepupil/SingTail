package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/singtail/singtail/internal/contract"
)

const (
	defaultExitTag      = "默认出口"
	directTag           = "direct"
	fallbackProxyTag    = "fallback-proxy"
	dnsCNTag            = "dns-cn"
	dnsProxyTag         = "dns-proxy"
	defaultMixedPort    = 7899
	defaultTunMTU       = 1280
	defaultTunCIDR      = "172.19.0.1/30"
	tailscaleCGNATCIDR  = "100.64.0.0/10"
	DefaultClashAPIAddr = "127.0.0.1:19090"
)

// ConfigOptions controls platform-specific and runtime paths for sing-box config generation.
type ConfigOptions struct {
	RulesDir     string
	ClashAPIAddr string
	TargetGOOS   string
}

type singBoxConfig struct {
	Log          singBoxLog           `json:"log"`
	Experimental *singBoxExperimental `json:"experimental,omitempty"`
	DNS          singBoxDNS           `json:"dns"`
	Inbounds     []map[string]any     `json:"inbounds"`
	Endpoints    []map[string]any     `json:"endpoints"`
	Outbounds    []map[string]any     `json:"outbounds"`
	Route        singBoxRoute         `json:"route"`
}

type singBoxExperimental struct {
	ClashAPI *singBoxClashAPI `json:"clash_api,omitempty"`
}

type singBoxClashAPI struct {
	ExternalController string `json:"external_controller"`
}

type singBoxLog struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type singBoxDNS struct {
	Servers  []map[string]any `json:"servers"`
	Rules    []map[string]any `json:"rules"`
	Final    string           `json:"final"`
	Strategy string           `json:"strategy"`
}

type singBoxRoute struct {
	Rules                 []map[string]any `json:"rules"`
	RuleSet               []map[string]any `json:"rule_set,omitempty"`
	Final                 string           `json:"final"`
	AutoDetectInterface   bool             `json:"auto_detect_interface"`
	DefaultDomainResolver string           `json:"default_domain_resolver,omitempty"`
}

// GenerateSingBoxConfig builds an indented sing-box JSON configuration with default options.
func GenerateSingBoxConfig(
	orgs []contract.Organization,
	nodes []contract.ExitNode,
	activeExitID string,
	mode contract.RoutingMode,
	rules []contract.Rule,
	providers []contract.RuleProvider,
	settings contract.SystemSettings,
) ([]byte, error) {
	return GenerateSingBoxConfigWithOptions(
		orgs,
		nodes,
		activeExitID,
		mode,
		rules,
		providers,
		settings,
		ConfigOptions{},
	)
}

// GenerateSingBoxConfigWithOptions builds an indented sing-box JSON configuration incorporating
// per-organization Tailscale endpoints, Clash API control, platform TUN compatibility,
// anti-pollution DNS, optional fallback proxy, and validated routing rules.
func GenerateSingBoxConfigWithOptions(
	orgs []contract.Organization,
	nodes []contract.ExitNode,
	activeExitID string,
	mode contract.RoutingMode,
	rules []contract.Rule,
	providers []contract.RuleProvider,
	settings contract.SystemSettings,
	opts ConfigOptions,
) ([]byte, error) {
	targetGOOS := strings.TrimSpace(opts.TargetGOOS)
	if targetGOOS == "" {
		targetGOOS = runtime.GOOS
	}
	clashAddr := strings.TrimSpace(opts.ClashAPIAddr)
	if clashAddr == "" {
		clashAddr = DefaultClashAPIAddr
	}

	endpoints, epTags, activeOrgID := buildOrgEndpoints(orgs, nodes, activeExitID)

	selectorOutbounds := make([]string, 0, len(epTags)+1)
	selectorOutbounds = append(selectorOutbounds, epTags...)
	selectorOutbounds = append(selectorOutbounds, directTag)

	selectedDefault := selectorOutbounds[0]
	if activeOrgID != "" {
		wantedTag := "ep-" + activeOrgID
		for _, tag := range epTags {
			if tag == wantedTag {
				selectedDefault = wantedTag
				break
			}
		}
	}

	outbounds := []map[string]any{
		{
			"type":      "selector",
			"tag":       defaultExitTag,
			"outbounds": selectorOutbounds,
			"default":   selectedDefault,
		},
		{
			"type": "direct",
			"tag":  directTag,
		},
	}

	if settings.FallbackProxyEnabled {
		proxyOutbound, err := buildFallbackProxyOutbound(settings.FallbackProxyAddr)
		if err != nil {
			return nil, err
		}
		outbounds = append(outbounds, proxyOutbound)
	}

	tunInbound := map[string]any{
		"type":                  "tun",
		"tag":                   "tun-in",
		"address":               []string{defaultTunCIDR},
		"mtu":                   defaultTunMTU,
		"auto_route":            true,
		"strict_route":          true,
		"route_exclude_address": []string{tailscaleCGNATCIDR},
	}
	if targetGOOS != "darwin" {
		tunInbound["interface_name"] = "singbox"
	}

	inbounds := []map[string]any{
		tunInbound,
		{
			"type":        "mixed",
			"tag":         "mixed-in",
			"listen":      "127.0.0.1",
			"listen_port": defaultMixedPort,
		},
	}

	ruleSets, availableRuleSets := buildRuleSets(providers, strings.TrimSpace(opts.RulesDir))

	dnsFinal := dnsProxyTag
	if mode == contract.ModeDirect {
		dnsFinal = dnsCNTag
	}
	dnsRules := []map[string]any{
		{
			"domain_suffix": []string{"tailscale.com", "tailscale.io"},
			"server":        dnsCNTag,
		},
	}
	if availableRuleSets == nil || availableRuleSets["geosite-cn"] {
		if hasRuleSetTag(ruleSets, "geosite-cn") {
			dnsRules = append(dnsRules, map[string]any{
				"rule_set": []string{"geosite-cn"},
				"server":   dnsCNTag,
			})
		}
	}

	dnsCfg := singBoxDNS{
		Servers: []map[string]any{
			{
				"tag":    dnsCNTag,
				"type":   "udp",
				"server": "223.5.5.5",
			},
			{
				"tag":    dnsProxyTag,
				"type":   "https",
				"server": "1.1.1.1",
				"detour": defaultExitTag,
			},
		},
		Rules:    dnsRules,
		Final:    dnsFinal,
		Strategy: "ipv4_only",
	}

	routeRules, finalOutbound := buildRouteRules(mode, rules, settings.FallbackProxyEnabled, availableRuleSets)

	cfg := singBoxConfig{
		Log: singBoxLog{
			Level:     "info",
			Timestamp: true,
		},
		Experimental: &singBoxExperimental{
			ClashAPI: &singBoxClashAPI{
				ExternalController: clashAddr,
			},
		},
		DNS:       dnsCfg,
		Inbounds:  inbounds,
		Endpoints: endpoints,
		Outbounds: outbounds,
		Route: singBoxRoute{
			Rules:                 routeRules,
			RuleSet:               ruleSets,
			Final:                 finalOutbound,
			AutoDetectInterface:   true,
			DefaultDomainResolver: dnsCNTag,
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func buildOrgEndpoints(
	orgs []contract.Organization,
	nodes []contract.ExitNode,
	activeExitID string,
) (endpoints []map[string]any, epTags []string, activeOrgID string) {
	orgMap := make(map[string]contract.Organization, len(orgs))
	orgOrder := make([]string, 0, len(orgs))
	seenOrg := make(map[string]bool, len(orgs))

	for _, org := range orgs {
		if org.ID == "" {
			continue
		}
		orgMap[org.ID] = org
		if !seenOrg[org.ID] {
			seenOrg[org.ID] = true
			orgOrder = append(orgOrder, org.ID)
		}
	}

	nodesByOrg := make(map[string][]contract.ExitNode)
	for _, node := range nodes {
		if node.ID == activeExitID && node.Online && strings.TrimSpace(node.TailscaleIP) != "" {
			activeOrgID = node.OrgID
		}
		if !node.Online || strings.TrimSpace(node.TailscaleIP) == "" {
			continue
		}
		if node.OrgID != "" && !seenOrg[node.OrgID] {
			seenOrg[node.OrgID] = true
			orgOrder = append(orgOrder, node.OrgID)
		}
		nodesByOrg[node.OrgID] = append(nodesByOrg[node.OrgID], node)
	}

	endpoints = make([]map[string]any, 0, len(orgOrder))
	epTags = make([]string, 0, len(orgOrder))

	// Emit one tailscale endpoint per organization to avoid state_directory lock contention.
	for _, orgID := range orgOrder {
		orgNodes := nodesByOrg[orgID]
		if len(orgNodes) == 0 {
			continue
		}

		chosen := orgNodes[0]
		if orgID == activeOrgID {
			for _, candidate := range orgNodes {
				if candidate.ID == activeExitID {
					chosen = candidate
					break
				}
			}
		}

		org := orgMap[orgID]
		stateDir := strings.TrimSpace(org.StateDir)
		if stateDir == "" {
			stateDir = filepath.ToSlash(filepath.Join("data", "ts-state-"+orgID))
		}

		epTag := "ep-" + orgID
		ep := map[string]any{
			"type":                       "tailscale",
			"tag":                        epTag,
			"state_directory":            stateDir,
			"exit_node":                  chosen.TailscaleIP,
			"exit_node_allow_lan_access": true,
		}
		if cu := strings.TrimSpace(org.ControlURL); cu != "" {
			ep["control_url"] = cu
		}
		endpoints = append(endpoints, ep)
		epTags = append(epTags, epTag)
	}

	return endpoints, epTags, activeOrgID
}

func buildFallbackProxyOutbound(rawAddr string) (map[string]any, error) {
	addr := strings.TrimSpace(rawAddr)
	if addr == "" {
		return nil, errors.New("fallback proxy address cannot be empty")
	}

	proxyType := "http"
	tlsEnabled := false
	switch {
	case strings.HasPrefix(addr, "socks5://"):
		proxyType = "socks"
		addr = strings.TrimPrefix(addr, "socks5://")
	case strings.HasPrefix(addr, "http://"):
		addr = strings.TrimPrefix(addr, "http://")
	case strings.HasPrefix(addr, "https://"):
		tlsEnabled = true
		addr = strings.TrimPrefix(addr, "https://")
	}
	addr = strings.TrimSuffix(addr, "/")

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid fallback proxy address %q: %w", rawAddr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid fallback proxy port %q", portStr)
	}
	if host == "" {
		host = "127.0.0.1"
	}

	outbound := map[string]any{
		"type":        proxyType,
		"tag":         fallbackProxyTag,
		"server":      host,
		"server_port": port,
	}
	if tlsEnabled {
		outbound["tls"] = map[string]any{"enabled": true}
	}
	return outbound, nil
}

func buildRuleSets(providers []contract.RuleProvider, rulesDir string) ([]map[string]any, map[string]bool) {
	if len(providers) == 0 {
		return nil, nil
	}

	var available map[string]bool
	if rulesDir != "" {
		available = make(map[string]bool, len(providers))
	}

	out := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		tag := strings.TrimSpace(p.Tag)
		rawURL := strings.TrimSpace(p.URL)
		if tag == "" {
			continue
		}

		if rulesDir != "" {
			localPath := filepath.Join(rulesDir, tag+".srs")
			info, err := os.Stat(localPath)
			if err != nil || info.IsDir() || info.Size() == 0 {
				continue
			}
			available[tag] = true
			out = append(out, map[string]any{
				"tag":    tag,
				"type":   "local",
				"format": "binary",
				"path":   filepath.ToSlash(localPath),
			})
			continue
		}

		if rawURL == "" {
			continue
		}
		out = append(out, map[string]any{
			"tag":             tag,
			"type":            "remote",
			"format":          "binary",
			"url":             rawURL,
			"download_detour": defaultExitTag,
		})
	}
	return out, available
}

func hasRuleSetTag(ruleSets []map[string]any, tag string) bool {
	for _, rs := range ruleSets {
		if t, ok := rs["tag"].(string); ok && t == tag {
			return true
		}
	}
	return false
}

func buildRouteRules(
	mode contract.RoutingMode,
	rules []contract.Rule,
	fallbackEnabled bool,
	availableRuleSets map[string]bool,
) ([]map[string]any, string) {
	routeRules := []map[string]any{
		{
			"action": "sniff",
		},
		{
			"protocol": "dns",
			"action":   "hijack-dns",
		},
		{
			"process_name": []string{
				"tailscaled.exe",
				"tailscale-ipn.exe",
				"verge-mihomo.exe",
				"clash-verge.exe",
				"Tailscale",
				"IPNExtension",
				"tailscaled",
				"verge-mihomo",
				"clash-verge",
				"mihomo",
			},
			"outbound": directTag,
		},
		{
			"ip_is_private": true,
			"outbound":      directTag,
		},
		{
			"ip_cidr":  []string{tailscaleCGNATCIDR},
			"outbound": directTag,
		},
	}

	switch mode {
	case contract.ModeDirect:
		return routeRules, directTag
	case contract.ModeGlobal:
		return routeRules, defaultExitTag
	}

	finalOutbound := defaultExitTag
	finalReject := false
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if r.Type == contract.RuleFinal {
			if target, isReject := mapActionTarget(r.Action, fallbackEnabled); isReject {
				finalReject = true
			} else {
				finalReject = false
				finalOutbound = target
			}
			continue
		}

		items := splitCSV(r.Payload)
		if r.Type == contract.RuleSet && availableRuleSets != nil {
			filtered := make([]string, 0, len(items))
			for _, tag := range items {
				if availableRuleSets[tag] {
					filtered = append(filtered, tag)
				}
			}
			items = filtered
		}
		if len(items) == 0 {
			continue
		}

		entry := make(map[string]any, 2)
		switch r.Type {
		case contract.RuleDomainSuffix:
			entry["domain_suffix"] = items
		case contract.RuleDomainKeyword:
			entry["domain_keyword"] = items
		case contract.RuleProcessName:
			entry["process_name"] = items
		case contract.RuleIPCIDR:
			entry["ip_cidr"] = items
		case contract.RuleSet:
			entry["rule_set"] = items
		default:
			continue
		}

		if target, isReject := mapActionTarget(r.Action, fallbackEnabled); isReject {
			entry["action"] = "reject"
		} else {
			entry["outbound"] = target
		}
		routeRules = append(routeRules, entry)
	}

	if finalReject {
		routeRules = append(routeRules, map[string]any{
			"action": "reject",
		})
	}

	return routeRules, finalOutbound
}

func mapActionTarget(action contract.RuleAction, fallbackEnabled bool) (string, bool) {
	switch action {
	case contract.ActionExit:
		return defaultExitTag, false
	case contract.ActionDirect:
		return directTag, false
	case contract.ActionFallbackProxy:
		if fallbackEnabled {
			return fallbackProxyTag, false
		}
		return directTag, false
	case contract.ActionReject:
		return "", true
	default:
		return defaultExitTag, false
	}
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
