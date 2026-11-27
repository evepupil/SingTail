package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/singtail/singtail/internal/contract"
)

const (
	defaultExitTag     = "默认出口"
	directTag          = "direct"
	fallbackProxyTag   = "fallback-proxy"
	dnsCNTag           = "dns-cn"
	dnsProxyTag        = "dns-proxy"
	defaultMixedPort   = 7899
	defaultTunMTU      = 1280
	defaultTunCIDR     = "172.19.0.1/30"
	tailscaleCGNATCIDR = "100.64.0.0/10"
)

type singBoxConfig struct {
	Log       singBoxLog       `json:"log"`
	DNS       singBoxDNS       `json:"dns"`
	Inbounds  []map[string]any `json:"inbounds"`
	Endpoints []map[string]any `json:"endpoints"`
	Outbounds []map[string]any `json:"outbounds"`
	Route     singBoxRoute     `json:"route"`
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

// GenerateSingBoxConfig builds an indented sing-box JSON configuration incorporating
// multi-organization Tailscale endpoints, Clash/Tailscale bypass protections,
// anti-pollution DNS, optional fallback proxy, and active routing rules.
func GenerateSingBoxConfig(
	orgs []contract.Organization,
	nodes []contract.ExitNode,
	activeExitID string,
	mode contract.RoutingMode,
	rules []contract.Rule,
	providers []contract.RuleProvider,
	settings contract.SystemSettings,
) ([]byte, error) {
	orgStateDirs := make(map[string]string, len(orgs))
	for _, org := range orgs {
		dir := strings.TrimSpace(org.StateDir)
		if dir == "" && org.ID != "" {
			dir = filepath.ToSlash(filepath.Join("data", "ts-state-"+org.ID))
		}
		orgStateDirs[org.ID] = dir
	}

	endpoints := make([]map[string]any, 0, len(nodes))
	exitTags := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if !node.Online {
			continue
		}
		stateDir := orgStateDirs[node.OrgID]
		if stateDir == "" {
			stateDir = filepath.ToSlash(filepath.Join("data", "ts-state-"+node.OrgID))
		}
		ep := map[string]any{
			"type":                       "tailscale",
			"tag":                        node.ID,
			"state_directory":            stateDir,
			"exit_node":                  node.TailscaleIP,
			"exit_node_allow_lan_access": true,
		}
		endpoints = append(endpoints, ep)
		exitTags = append(exitTags, node.ID)
	}

	selectorOutbounds := make([]string, len(exitTags))
	copy(selectorOutbounds, exitTags)
	if len(selectorOutbounds) == 0 {
		selectorOutbounds = []string{directTag}
	}

	selectedDefault := selectorOutbounds[0]
	for _, tag := range selectorOutbounds {
		if tag == activeExitID {
			selectedDefault = activeExitID
			break
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

	inbounds := []map[string]any{
		{
			"type":                  "tun",
			"tag":                   "tun-in",
			"interface_name":        "singbox",
			"address":               []string{defaultTunCIDR},
			"mtu":                   defaultTunMTU,
			"auto_route":            true,
			"strict_route":          true,
			"route_exclude_address": []string{tailscaleCGNATCIDR},
		},
		{
			"type":        "mixed",
			"tag":         "mixed-in",
			"listen":      "127.0.0.1",
			"listen_port": defaultMixedPort,
		},
	}

	ruleSets, hasGeositeCN := buildRuleSets(providers)

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
	if hasGeositeCN {
		dnsRules = append(dnsRules, map[string]any{
			"rule_set": []string{"geosite-cn"},
			"server":   dnsCNTag,
		})
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

	routeRules, finalOutbound := buildRouteRules(mode, rules, settings.FallbackProxyEnabled)

	cfg := singBoxConfig{
		Log: singBoxLog{
			Level:     "info",
			Timestamp: true,
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

func buildRuleSets(providers []contract.RuleProvider) ([]map[string]any, bool) {
	if len(providers) == 0 {
		return nil, false
	}

	out := make([]map[string]any, 0, len(providers))
	hasGeositeCN := false
	for _, p := range providers {
		tag := strings.TrimSpace(p.Tag)
		url := strings.TrimSpace(p.URL)
		if tag == "" || url == "" {
			continue
		}
		if tag == "geosite-cn" {
			hasGeositeCN = true
		}
		out = append(out, map[string]any{
			"tag":             tag,
			"type":            "remote",
			"format":          "binary",
			"url":             url,
			"download_detour": defaultExitTag,
		})
	}
	return out, hasGeositeCN
}

func buildRouteRules(
	mode contract.RoutingMode,
	rules []contract.Rule,
	fallbackEnabled bool,
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
			"process_name": []string{"tailscaled.exe", "verge-mihomo.exe", "clash-verge.exe"},
			"outbound":     directTag,
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
