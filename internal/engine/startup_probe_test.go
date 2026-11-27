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

func TestWaitForExitReadyRetriesEgress(t *testing.T) {
	var attempts atomic.Int32
	groupPath := "/proxies/" + defaultExitTag
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case groupPath:
			_, _ = w.Write([]byte(`{"now":"ep-test-org"}`))
		case groupPath + "/delay":
			attempt := attempts.Add(1)
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitForExitReady(ctx, api.URL, make(chan struct{})); err != nil {
		t.Fatalf("healthy exit failed validation: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected successful retry, got %d probes", attempts.Load())
	}
}

func TestWaitForExitReadyRejectsDirectSelection(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/proxies/"+defaultExitTag {
			t.Errorf("must not probe a direct outbound: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"now":"direct"}`))
	}))
	defer api.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := waitForExitReady(ctx, api.URL, make(chan struct{})); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a timeout when no Tailscale exit is selected, got %v", err)
	}
}

func TestWaitForExitReadyStopsOnCoreExit(t *testing.T) {
	done := make(chan struct{})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/proxies/"+defaultExitTag {
			_, _ = w.Write([]byte(`{"now":"ep-test-org"}`))
			return
		}
		close(done)
		<-r.Context().Done()
	}))
	defer api.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := waitForExitReady(ctx, api.URL, done); err == nil || !strings.Contains(err.Error(), "网络内核退出") {
		t.Fatalf("expected core exit error, got %v", err)
	}
}
