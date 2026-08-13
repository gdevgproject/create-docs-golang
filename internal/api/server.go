package api

import (
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"

	"codedocs/internal/bookmarks"
	"codedocs/internal/config"
	"codedocs/internal/generator"
	"codedocs/internal/scanner"
	"codedocs/internal/tokenizer"
)

type Server struct {
	cfg   *config.Config
	sc    *scanner.Scanner
	tok   *tokenizer.Tokenizer
	gen   *generator.Generator
	bm    *bookmarks.Manager
	webFS fs.FS
	mux   *http.ServeMux

	shutdownFn   func()
	shutdownOnce sync.Once
	localOnly    bool
}

// Option configures optional desktop-only server behavior while preserving the
// original NewServer(cfg, fs) construction contract for embedders and tests.
type Option func(*Server)

func WithShutdown(fn func()) Option {
	return func(s *Server) {
		s.shutdownFn = fn
	}
}

func WithLocalAccessOnly() Option {
	return func(s *Server) {
		s.localOnly = true
	}
}

func NewServer(cfg *config.Config, webFS fs.FS, options ...Option) *Server {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	sc := scanner.NewScanner()
	tok := tokenizer.NewTokenizer(cfg.CacheDir)
	gen := generator.NewGenerator(cfg, sc, tok)
	bm := bookmarks.NewManager(cfg.BookmarkFile)

	s := &Server{
		cfg:   cfg,
		sc:    sc,
		tok:   tok,
		gen:   gen,
		bm:    bm,
		webFS: webFS,
		mux:   http.NewServeMux(),
	}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}

	// Warm up tiktoken o200k_base dictionary in background on startup for 0ms cold start
	go tok.Mode()

	s.routes()
	return s
}

func (s *Server) routes() {
	bp := s.cfg.BasePath

	// API Endpoints
	s.mux.HandleFunc("GET "+bp+"/api/bookmarks", s.handleGetBookmarks)
	s.mux.HandleFunc("POST "+bp+"/api/bookmarks", s.handleSaveBookmark)
	s.mux.HandleFunc("PUT "+bp+"/api/bookmarks", s.handleUpdateBookmark)
	s.mux.HandleFunc("PUT "+bp+"/api/bookmarks/history", s.handleRenameBookmarkHistory)
	s.mux.HandleFunc("DELETE "+bp+"/api/bookmarks", s.handleDeleteBookmark)
	s.mux.HandleFunc("DELETE "+bp+"/api/bookmarks/history", s.handleDeleteBookmarkHistory)
	s.mux.HandleFunc("GET "+bp+"/api/bookmarks/export", s.handleExportBookmarks)
	s.mux.HandleFunc("POST "+bp+"/api/bookmarks/import", s.handleImportBookmarks)
	s.mux.HandleFunc("GET "+bp+"/api/structure", s.handleGetStructure)
	s.mux.HandleFunc("GET "+bp+"/api/exclusions", s.handleGetExclusions)
	s.mux.HandleFunc("GET "+bp+"/api/content", s.handleGetContent)
	s.mux.HandleFunc("GET "+bp+"/api/download", s.handleDownload)
	s.mux.HandleFunc("GET "+bp+"/api/generate", s.handleGenerate)
	s.mux.HandleFunc("GET "+bp+"/api/version", s.handleGetVersion)
	s.mux.HandleFunc("GET "+bp+"/api/check-update", s.handleCheckUpdate)
	s.mux.HandleFunc("POST "+bp+"/api/download-update", s.handleDownloadUpdate)
	s.mux.HandleFunc("GET "+bp+"/api/update-progress", s.handleGetUpdateProgress)
	s.mux.HandleFunc("POST "+bp+"/api/apply-update", s.handleApplyUpdate)
	s.mux.HandleFunc("POST "+bp+"/api/shutdown", s.handleShutdown)
	s.mux.HandleFunc("GET "+bp+"/api/ping", s.handlePing)
	s.mux.HandleFunc("POST "+bp+"/api/count-tokens", s.handleCountTokens)

	// Legacy PHP route compatibility support
	s.mux.HandleFunc("GET "+bp+"/index.php", s.handleLegacyQueryRoute)
	s.mux.HandleFunc("POST "+bp+"/index.php", s.handleLegacyQueryRoute)

	// Root redirect if BasePath is configured
	if bp != "" {
		s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, bp+"/", http.StatusFound)
		})
	}

	// Static Web Assets & UI
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		if bp != "" {
			s.mux.Handle("GET "+bp+"/", http.StripPrefix(bp, fileServer))
		} else {
			s.mux.Handle("GET /", fileServer)
		}
	}
}

// handleLegacyQueryRoute routes calls using `?action=...` parameter for 100% PHP URL compatibility
func (s *Server) handleLegacyQueryRoute(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	if r.URL.Query().Has("download") {
		s.handleDownload(w, r)
		return
	}

	switch action {
	case "get_bookmarks":
		s.handleGetBookmarks(w, r)
	case "save_bookmark":
		s.handleSaveBookmark(w, r)
	case "delete_bookmark":
		s.handleDeleteBookmark(w, r)
	case "rename_history":
		s.handleRenameBookmarkHistory(w, r)
	case "get_structure":
		s.handleGetStructure(w, r)
	case "get_content":
		s.handleGetContent(w, r)
	case "get_exclusions":
		s.handleGetExclusions(w, r)
	case "generate":
		s.handleGenerate(w, r)
	default:
		if s.webFS != nil {
			fs := http.FileServer(http.FS(s.webFS))
			if s.cfg.BasePath != "" {
				http.StripPrefix(s.cfg.BasePath, fs).ServeHTTP(w, r)
			} else {
				fs.ServeHTTP(w, r)
			}
			return
		}
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("[PANIC RECOVER] %v\nstack: %s", err, string(debug.Stack()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"status":"error","message":"Internal Server Error"}`)
		}
	}()

	if s.localOnly && !isLocalRequest(r) {
		http.Error(w, "local access only", http.StatusForbidden)
		return
	}

	setSecurityHeaders(w)
	if !strings.Contains(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) requestShutdown() {
	if s.shutdownFn == nil {
		return
	}
	s.shutdownOnce.Do(s.shutdownFn)
}

func isLocalRequest(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}

	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	remoteIP := net.ParseIP(strings.Trim(remoteHost, "[]"))
	if remoteIP == nil || !remoteIP.IsLoopback() {
		return false
	}

	host := r.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	hostIP := net.ParseIP(host)
	return hostIP != nil && hostIP.IsLoopback()
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'none'; img-src 'self' data:; connect-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")
}
