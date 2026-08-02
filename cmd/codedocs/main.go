package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

	webFS := web.GetFS()
	server := api.NewServer(cfg, webFS)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      server,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 0 for unlimited SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	shutdownComplete := make(chan struct{})

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		slog.Info("Shutting down server gracefully...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
		close(shutdownComplete)
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
			openBrowser(serverURL)
		}()
	}

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Server failed to start", "error", err)
		os.Exit(1)
	}

	<-shutdownComplete
	slog.Info("Server stopped clean.")
}

func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}

	_ = cmd.Start()
}
