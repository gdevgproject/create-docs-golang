//go:build !windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

func openNativeWindow(ctx context.Context, url, title, cacheDir string, onReady func() error) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	_ = cmd.Process.Release()
	if onReady != nil {
		if err := onReady(); err != nil {
			return fmt.Errorf("complete update startup: %w", err)
		}
	}

	// A regular browser does not expose a reliable window-close callback. Keep the
	// local server alive until an OS signal or the shutdown API cancels the app.
	<-ctx.Done()
	return nil
}
