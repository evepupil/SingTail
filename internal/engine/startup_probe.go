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

const startupProbeTimeout = 3 * time.Second

func waitForExitReady(ctx context.Context, clashURL string, done <-chan struct{}) error {
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
	for targetIndex := 0; probeCtx.Err() == nil; targetIndex++ {
		if checkExitEgress(probeCtx, client, clashURL, publicIPProbeURLs[targetIndex%len(publicIPProbeURLs)]) && probeCtx.Err() == nil {
			select {
			case <-done:
				return errors.New("核验出口时网络内核退出，未开启虚拟网卡")
			default:
				return nil
			}
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
		return fmt.Errorf("出口未能连通公网，未开启虚拟网卡: %w", ctx.Err())
	}
}

func checkExitEgress(ctx context.Context, client *http.Client, clashURL, targetURL string) bool {
	groupURL := clashURL + "/proxies/" + url.PathEscape(defaultExitTag)
	var selector struct {
		Now string `json:"now"`
	}
	if !readClashProbeJSON(ctx, client, groupURL, &selector) || !strings.HasPrefix(selector.Now, "ep-") {
		return false
	}

	query := url.Values{
		"timeout": {strconv.FormatInt(startupProbeTimeout.Milliseconds(), 10)},
		"url":     {targetURL},
	}
	var result struct {
		Delay int `json:"delay"`
	}
	return readClashProbeJSON(ctx, client, groupURL+"/delay?"+query.Encode(), &result) && result.Delay > 0
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
