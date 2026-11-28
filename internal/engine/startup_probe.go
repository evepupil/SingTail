package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const startupProbeTimeout = 6 * time.Second

type egressProbeFn func(ctx context.Context, proxyAddr string) (string, error)

func waitForExitReady(ctx context.Context, clashURL, expectedMode, mixedProxyAddr string, done <-chan struct{}) error {
	return waitForExitReadyWithProbe(ctx, clashURL, expectedMode, mixedProxyAddr, done, probeViaHTTP)
}

func waitForExitReadyWithProbe(
	ctx context.Context,
	clashURL string,
	expectedMode string,
	mixedProxyAddr string,
	done <-chan struct{},
	proxyProbe egressProbeFn,
) error {
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-done:
			cancel()
		case <-probeCtx.Done():
		}
	}()

	client := &http.Client{Timeout: startupProbeTimeout + time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastReason string
	for targetIndex := 0; probeCtx.Err() == nil; targetIndex++ {
		ok, reason := checkExitEgress(probeCtx, client, clashURL, expectedMode, mixedProxyAddr, publicIPProbeURLs[targetIndex%len(publicIPProbeURLs)], proxyProbe)
		if ok && probeCtx.Err() == nil {
			select {
			case <-done:
				return errors.New("核验出口时网络内核退出，未开启虚拟网卡")
			default:
				return nil
			}
		}
		if reason != "" {
			lastReason = reason
		}
		select {
		case <-probeCtx.Done():
		case <-ticker.C:
		}
	}

	select {
	case <-done:
		return errors.New("核验出口时网络内核退出，未开启虚拟网卡")
	default:
		if lastReason != "" {
			return fmt.Errorf("出口未能连通公网（%s），未开启虚拟网卡: %w", lastReason, ctx.Err())
		}
		return fmt.Errorf("出口未能连通公网，未开启虚拟网卡: %w", ctx.Err())
	}
}

func checkExitEgress(
	ctx context.Context,
	client *http.Client,
	clashURL string,
	expectedMode string,
	mixedProxyAddr string,
	targetURL string,
	proxyProbe egressProbeFn,
) (bool, string) {
	if strings.TrimSpace(expectedMode) != "" {
		var cfg struct {
			Mode string `json:"mode"`
		}
		if !readClashProbeJSON(ctx, client, clashURL+"/configs", &cfg) {
			return false, "内核分流模式接口未响应"
		}
		if !strings.EqualFold(strings.TrimSpace(cfg.Mode), strings.TrimSpace(expectedMode)) {
			return false, fmt.Sprintf("内核仍处于 %s 模式", cfg.Mode)
		}
	}

	groupURL := clashURL + "/proxies/" + url.PathEscape(defaultExitTag)
	var selector struct {
		Now string `json:"now"`
	}
	if !readClashProbeJSON(ctx, client, groupURL, &selector) {
		return false, "未能读取默认出口状态"
	}
	if !strings.HasPrefix(selector.Now, "ep-") {
		return false, fmt.Sprintf("默认出口当前指向 %s", selector.Now)
	}

	query := url.Values{
		"timeout": {strconv.FormatInt(startupProbeTimeout.Milliseconds(), 10)},
		"url":     {targetURL},
	}
	var result struct {
		Delay int `json:"delay"`
	}
	if !readClashProbeJSON(ctx, client, groupURL+"/delay?"+query.Encode(), &result) || result.Delay <= 0 {
		return false, "Tailscale 出口暂未连通"
	}

	if strings.TrimSpace(mixedProxyAddr) != "" && proxyProbe != nil {
		attemptCtx, cancel := context.WithTimeout(ctx, startupProbeTimeout)
		_, err := proxyProbe(attemptCtx, mixedProxyAddr)
		cancel()
		if err != nil {
			return false, fmt.Sprintf("本地分流代理出网验证失败: %v", err)
		}
	}
	return true, ""
}

func readClashProbeJSON(ctx context.Context, client *http.Client, endpoint string, target any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(target) == nil
}
