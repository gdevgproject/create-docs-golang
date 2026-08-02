package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"codedocs/internal/api"
	"codedocs/internal/config"
	"codedocs/web"
)

func main() {
	cfg := config.ParseFlags()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Auto find available port if default port is occupied
	listener, err := findAvailableListener(cfg.Host, cfg.Port, 10)
	if err != nil {
		slog.Error("Failed to bind network port", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	actualPort := listener.Addr().(*net.TCPAddr).Port
	cfg.Port = actualPort

	webFS := web.GetFS()
	server := api.NewServer(cfg, webFS)

	httpServer := &http.Server{
		Handler:      server,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 0 for unlimited SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	shutdownComplete := make(chan struct{})

	// Signal & App Cancellation handler
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		select {
		case <-sigChan:
			slog.Info("Received OS termination signal...")
		case <-appCtx.Done():
			slog.Info("App window closed. Shutting down application...")
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()

		_ = httpServer.Shutdown(shutdownCtx)
		close(shutdownComplete)
	}()

	// Heartbeat monitor: After client connects, if no ping received for > 8s (meaning app window was closed), shut down!
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if api.HasConnected() && time.Since(api.GetLastHeartbeat()) > 8*time.Second {
					slog.Info("No web UI connected for > 8s. Auto closing server...")
					cancelApp()
					return
				}
			case <-appCtx.Done():
				return
			}
		}
	}()

	serverURL := fmt.Sprintf("http://localhost:%d%s", cfg.Port, cfg.BasePath)

	fmt.Println("================================================================")
	fmt.Printf("🚀 Codebase-to-Docs Generator (Go Edition %s)\n", cfg.Version)
	fmt.Printf("🌐 Server running at: %s\n", serverURL)
	fmt.Printf("⚡ Worker Pool: %d goroutines | Max File Size: %dMB\n", cfg.Workers, cfg.MaxFileSize/(1024*1024))
	fmt.Println("================================================================")

	if cfg.OpenBrowser {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openAppWindow(serverURL)
		}()
	}

	if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Server error", "error", err)
		cancelApp()
	}

	<-shutdownComplete
	slog.Info("Application stopped cleanly. Zero background processes left.")
	os.Exit(0)
}

func findAvailableListener(host string, startPort int, maxTries int) (net.Listener, error) {
	for i := 0; i < maxTries; i++ {
		port := startPort + i
		addr := fmt.Sprintf("%s:%d", host, port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			return l, nil
		}
	}
	return nil, fmt.Errorf("no free ports found between %d and %d", startPort, startPort+maxTries-1)
}

func openAppWindow(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		edgePaths := []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			os.Getenv("LOCALAPPDATA") + `\Microsoft\Edge\Application\msedge.exe`,
		}

		var foundEdge string
		for _, p := range edgePaths {
			if _, err := os.Stat(p); err == nil {
				foundEdge = p
				break
			}
		}

		if foundEdge != "" {
			cmd = exec.Command(foundEdge, "--app="+url, "--window-size=1280,850")
		} else {
			cmd = exec.Command("cmd", "/c", "start", url)
		}
	case "darwin":
		cmd = exec.Command("open", "-a", "Google Chrome", "--args", "--app="+url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	if cmd != nil {
		_ = cmd.Start()
	}
}
