package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitForExitReadyRetriesEgressAndProxyProbe(t *testing.T) {
	var delayAttempts atomic.Int32
	var proxyAttempts atomic.Int32
	groupPath := "/proxies/" + defaultExitTag
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/configs":
			_, _ = w.Write([]byte(`{"mode":"Rule"}`))
		case groupPath:
			_, _ = w.Write([]byte(`{"now":"ep-test-org"}`))
		case groupPath + "/delay":
			attempt := delayAttempts.Add(1)
			if attempt <= int32(len(publicIPProbeURLs)) && r.URL.Query().Get("url") != publicIPProbeURLs[attempt-1] {
				t.Errorf("probe %d sent unexpected target: %s", attempt, r.URL.RawQuery)
			}
			if attempt == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"delay":120}`))
		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	probe := func(_ context.Context, addr string) (string, error) {
		if addr != DefaultMixedProxy {
			t.Fatalf("unexpected mixed proxy addr: %s", addr)
		}
		if proxyAttempts.Add(1) == 1 {
			return "", errors.New("temporary proxy probe failure")
		}
		return "43.153.149.111", nil
	}
	if err := waitForExitReadyWithProbe(ctx, api.URL, "Rule", DefaultMixedProxy, make(chan struct{}), probe); err != nil {
		t.Fatalf("healthy exit failed validation: %v", err)
	}
	if delayAttempts.Load() < 3 {
		t.Fatalf("expected retry after temporary proxy failure, got %d delay attempts", delayAttempts.Load())
	}
	if proxyAttempts.Load() != 2 {
		t.Fatalf("expected 2 proxy probe attempts, got %d", proxyAttempts.Load())
	}
}

func TestWaitForExitReadyRejectsDirectModeAndDirectSelection(t *testing.T) {
	t.Run("rejects stale Direct Clash mode", func(t *testing.T) {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/configs" {
				_, _ = w.Write([]byte(`{"mode":"Direct"}`))
				return
			}
			t.Errorf("must not accept exit readiness while Clash mode is Direct: %s", r.URL.Path)
		}))
		defer api.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		err := waitForExitReadyWithProbe(ctx, api.URL, "Rule", DefaultMixedProxy, make(chan struct{}), func(context.Context, string) (string, error) {
			t.Fatal("proxy probe should not run when Clash mode mismatches")
			return "", nil
		})
		if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "Direct") {
			t.Fatalf("expected Direct mode rejection error, got %v", err)
		}
	})

	t.Run("rejects direct selector outbound", func(t *testing.T) {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/configs":
				_, _ = w.Write([]byte(`{"mode":"Rule"}`))
			case "/proxies/" + defaultExitTag:
				_, _ = w.Write([]byte(`{"now":"direct"}`))
			default:
				t.Errorf("must not probe a direct outbound: %s", r.URL.Path)
			}
		}))
		defer api.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		if err := waitForExitReadyWithProbe(ctx, api.URL, "Rule", "", make(chan struct{}), nil); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected a timeout when no Tailscale exit is selected, got %v", err)
		}
	})
}

func TestWaitForExitReadyStopsOnCoreExit(t *testing.T) {
	done := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/configs":
			_, _ = w.Write([]byte(`{"mode":"Rule"}`))
		case "/proxies/" + defaultExitTag:
			_, _ = w.Write([]byte(`{"now":"ep-test-org"}`))
		default:
			close(done)
			<-r.Context().Done()
		}
	}))
	defer api.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitForExitReadyWithProbe(ctx, api.URL, "Rule", "", done, nil); err == nil || !strings.Contains(err.Error(), "网络内核退出") {
		t.Fatalf("expected core exit error, got %v", err)
	}
}

func TestClashModeAndSelectorSync(t *testing.T) {
	var patchedMode string
	var selectedProxy string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/configs":
			patchedMode = "Rule"
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/proxies/"+defaultExitTag:
			selectedProxy = "ep-org-1"
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	if err := patchClashMode(context.Background(), api.URL, "rule"); err != nil {
		t.Fatalf("patchClashMode failed: %v", err)
	}
	if err := selectClashOutbound(context.Background(), api.URL, "ep-org-1"); err != nil {
		t.Fatalf("selectClashOutbound failed: %v", err)
	}
	if patchedMode != "Rule" || selectedProxy != "ep-org-1" {
		t.Fatalf("unexpected Clash sync state: mode=%q proxy=%q", patchedMode, selectedProxy)
	}
}
