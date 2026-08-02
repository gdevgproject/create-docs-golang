//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

func openNativeWindow(url string, title string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-a", "Google Chrome", "--args", "--app="+url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if cmd != nil {
		_ = cmd.Start()
	}
}
