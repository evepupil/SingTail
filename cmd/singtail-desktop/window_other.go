//go:build !windows && !darwin

package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/singtail/singtail/internal/syswin"
)

func runDesktopWindow(_ fs.FS, _ http.Handler, apiBaseURL string) error {
	if err := syswin.OpenBrowser(apiBaseURL); err != nil {
		log.Printf("open browser failed: %v", err)
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)
	return nil
}
