package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"codedocs/internal/api"
	"codedocs/internal/config"
	"codedocs/internal/updater"
	"codedocs/web"
)

const (
	portSearchLimit = 10
	shutdownTimeout = 5 * time.Second
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg := config.ParseFlags()
	updater.CleanupOldFiles()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	appCtx, cancelApp := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelApp()

	listener, err := findAvailableListener(cfg.Host, cfg.Port, portSearchLimit)
	if err != nil {
		slog.Error("unable to start local server", "error", err)
		return 1
	}
	defer listener.Close()

	actualPort := listener.Addr().(*net.TCPAddr).Port
	cfg.Port = actualPort

	serverOptions := []api.Option{api.WithShutdown(cancelApp)}
	if config.IsLoopbackHost(cfg.Host) {
		serverOptions = append(serverOptions, api.WithLocalAccessOnly())
	}
	server := api.NewServer(cfg, web.GetFS(), serverOptions...)

	httpServer := &http.Server{
		Handler:           server,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // SSE streams remain open until their request context is cancelled.
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serveErr := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
		if err != nil {
			slog.Error("local server stopped unexpectedly", "error", err)
			cancelApp()
		}
	}()

	serverURL := localServerURL(cfg)
	slog.Info("CodePulse AI started",
		"version", cfg.Version,
		"url", serverURL,
		"workers", cfg.Workers,
		"max_file_mb", cfg.MaxFileSize/(1024*1024),
	)

	var windowErr error
	if cfg.OpenBrowser {
		windowErr = openNativeWindow(appCtx, serverURL, fmt.Sprintf("CodePulse AI %s", cfg.Version), cfg.CacheDir)
		cancelApp() // Closing the native window owns the desktop application's lifetime.
	} else {
		<-appCtx.Done()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Warn("graceful shutdown timed out; closing active connections", "error", err)
		_ = httpServer.Close()
	}

	select {
	case err := <-serveErr:
		if err != nil {
			return 1
		}
	default:
	}

	if windowErr != nil {
		slog.Error("native window failed", "error", windowErr)
		return 1
	}

	slog.Info("CodePulse AI stopped cleanly")
	return 0
}

func localServerURL(cfg *config.Config) string {
	host := cfg.Host
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + cfg.BasePath
}

func findAvailableListener(host string, startPort, maxTries int) (net.Listener, error) {
	if maxTries < 1 || startPort == 0 {
		maxTries = 1
	}

	var lastErr error
	for i := 0; i < maxTries; i++ {
		port := startPort + i
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			return listener, nil
		}
		lastErr = err
	}

	endPort := startPort + maxTries - 1
	return nil, fmt.Errorf("no available port in range %d-%d on %q: %w", startPort, endPort, host, lastErr)
}
