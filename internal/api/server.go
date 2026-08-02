package api

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"runtime/debug"

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
}

func NewServer(cfg *config.Config, webFS fs.FS) *Server {
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

	s.routes()
	return s
}

func (s *Server) routes() {
	// API Endpoints
	s.mux.HandleFunc("GET /api/bookmarks", s.handleGetBookmarks)
	s.mux.HandleFunc("POST /api/bookmarks", s.handleSaveBookmark)
	s.mux.HandleFunc("DELETE /api/bookmarks", s.handleDeleteBookmark)
	s.mux.HandleFunc("GET /api/structure", s.handleGetStructure)
	s.mux.HandleFunc("GET /api/exclusions", s.handleGetExclusions)
	s.mux.HandleFunc("GET /api/content", s.handleGetContent)
	s.mux.HandleFunc("GET /api/download", s.handleDownload)
	s.mux.HandleFunc("GET /api/generate", s.handleGenerate)

	// Legacy PHP route compatibility support
	s.mux.HandleFunc("GET /index.php", s.handleLegacyQueryRoute)
	s.mux.HandleFunc("POST /index.php", s.handleLegacyQueryRoute)

	// Static Web Assets & UI
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		s.mux.Handle("GET /", fileServer)
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
			http.FileServer(http.FS(s.webFS)).ServeHTTP(w, r)
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

	s.mux.ServeHTTP(w, r)
}
