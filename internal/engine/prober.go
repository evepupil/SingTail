// Package engine generates sing-box configurations, probes exit egress IPs,
// manages the sing-box subprocess and Clash API, and orchestrates exit switching.
package engine

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/singtail/singtail/internal/contract"
)

// IPProber verifies the effective public egress IP for a target exit node.
type IPProber interface {
	ProbePublicIP(ctx context.Context, proxyAddr string, target contract.ExitNode) (string, error)
}

type defaultProber struct {
	realHTTP bool
}

var publicIPProbeURLs = [...]string{"https://api.ipify.org", "https://ifconfig.me/ip"}

// NewDefaultProber constructs an IPProber.
// When realHTTP is true, it queries public IP echo services through proxyAddr;
// when false, it simulates verification using the node health metadata.
func NewDefaultProber(realHTTP bool) IPProber {
	return &defaultProber{realHTTP: realHTTP}
}

func (p *defaultProber) ProbePublicIP(ctx context.Context, proxyAddr string, target contract.ExitNode) (string, error) {
	if ctx != nil && ctx.Err() != nil {
		return "", ctx.Err()
	}

	if !p.realHTTP {
		if !target.Online || !target.Healthy {
			return "", fmt.Errorf("exit node %s (%s) health check failed", target.Label, target.ID)
		}
		if strings.TrimSpace(target.PublicIP) == "" {
			return "198.51.100.1", nil
		}
		return target.PublicIP, nil
	}

	return probeViaHTTP(ctx, proxyAddr)
}

func probeViaHTTP(ctx context.Context, proxyAddr string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
	}
	if addr := strings.TrimSpace(proxyAddr); addr != "" {
		rawURL := addr
		if !strings.Contains(rawURL, "://") {
			rawURL = "http://" + rawURL
		}
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return "", fmt.Errorf("invalid proxy address %q: %w", proxyAddr, err)
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{Transport: transport}

	var lastErr error
	for _, endpoint := range publicIPProbeURLs {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		ip, err := fetchIP(ctx, client, endpoint)
		if err == nil {
			return ip, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func fetchIP(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ip probe %s returned HTTP %d", endpoint, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}

	candidate := strings.TrimSpace(string(body))
	parsed := net.ParseIP(candidate)
	if parsed == nil || parsed.IsUnspecified() {
		return "", fmt.Errorf("invalid IP response %q from %s", candidate, endpoint)
	}
	return parsed.String(), nil
}
