//go:build windows || darwin

package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/singtail/singtail/internal/syswin"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func runDesktopWindow(staticFS fs.FS, apiHandler http.Handler, apiBaseURL string) error {
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	closeStop := func() {
		stopOnce.Do(func() { close(stopCh) })
	}
	defer closeStop()

	var ctxMu sync.RWMutex
	var appCtx context.Context

	showMainWindow := func() {
		ctxMu.RLock()
		ctx := appCtx
		ctxMu.RUnlock()
		if ctx == nil {
			return
		}
		wailsRuntime.WindowUnminimise(ctx)
		wailsRuntime.WindowShow(ctx)
		wailsRuntime.Show(ctx)
	}

	quitApp := func() {
		closeStop()
		ctxMu.RLock()
		ctx := appCtx
		ctxMu.RUnlock()
		if ctx != nil {
			wailsRuntime.Quit(ctx)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			quitApp()
		case <-stopCh:
		}
		signal.Stop(sigCh)
	}()

	if runtime.GOOS == "windows" {
		trayCfg := syswin.TrayConfig{
			Tooltip: "SingTail — 多组织出口网关",
			OnOpen:  showMainWindow,
			OnToggle: func() {
				if err := toggleTunViaAPI(apiBaseURL); err != nil {
					log.Printf("toggle routing failed: %v", err)
				}
			},
			OnQuit: quitApp,
			BuildActions: func() []syswin.TrayAction {
				return buildLiveTrayActions(apiBaseURL)
			},
		}
		go func() {
			if trayErr := syswin.RunTray(trayCfg, stopCh); trayErr != nil {
				log.Printf("system tray exited: %v", trayErr)
			}
		}()
	}

	return wails.Run(&options.App{
		Title:             "SingTail",
		Width:             1360,
		Height:            860,
		MinWidth:          960,
		MinHeight:         620,
		Frameless:         true,
		HideWindowOnClose: true,
		BackgroundColour:  &options.RGBA{R: 22, G: 22, B: 24, A: 255},
		AssetServer: &assetserver.Options{
			Assets:  staticFS,
			Handler: apiHandler,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				HideTitleBar:               true,
				FullSizeContent:            true,
			},
			About: &mac.AboutInfo{
				Title:   "SingTail",
				Message: "SingTail — 多组织 Tailscale 出口分流网关",
			},
		},
		OnStartup: func(ctx context.Context) {
			ctxMu.Lock()
			appCtx = ctx
			ctxMu.Unlock()
		},
		OnShutdown: func(_ context.Context) {
			closeStop()
		},
	})
}
