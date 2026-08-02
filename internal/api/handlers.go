package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"codedocs/internal/bookmarks"
	"codedocs/internal/config"
	"codedocs/internal/generator"
	"codedocs/internal/scanner"
	"codedocs/internal/updater"
)

func (s *Server) handleGetBookmarks(w http.ResponseWriter, r *http.Request) {
	bms, err := s.bm.GetBookmarks()
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, bms)
}

func (s *Server) handleSaveBookmark(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Note string `json:"note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	cleanPath := strings.TrimSpace(body.Path)
	if cleanPath == "" {
		s.jsonError(w, "Path is required", http.StatusBadRequest)
		return
	}

	bms, err := s.bm.SaveBookmark(cleanPath, body.Note)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]any{
		"status": "success",
		"data":   bms,
	})
}

func (s *Server) handleUpdateBookmark(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action     string   `json:"action"`
		ID         string   `json:"id"`
		Note       string   `json:"note"`
		OrderedIDs []string `json:"ordered_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	var bms map[string]bookmarks.Bookmark
	var err error

	if body.Action == "reorder" || len(body.OrderedIDs) > 0 {
		bms, err = s.bm.ReorderBookmarks(body.OrderedIDs)
	} else if body.ID != "" {
		bms, err = s.bm.UpdateNote(body.ID, body.Note)
	} else {
		s.jsonError(w, "Missing id or ordered_ids", http.StatusBadRequest)
		return
	}

	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]any{
		"status": "success",
		"data":   bms,
	})
}

func (s *Server) handleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if body.ID == "" {
		s.jsonError(w, "ID is required", http.StatusBadRequest)
		return
	}

	bms, err := s.bm.DeleteBookmark(body.ID)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]any{
		"status": "success",
		"data":   bms,
	})
}

func (s *Server) handleGetStructure(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		s.jsonError(w, "Đường dẫn không hợp lệ.", http.StatusBadRequest)
		return
	}

	files, err := s.sc.ScanProjectFiles(path)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Không thể quét thư mục: %v", err), http.StatusBadRequest)
		return
	}

	totalFiles := len(files)
	totalLines := s.sc.CountProjectLinesFast(files, s.cfg.MaxFileSize)
	localDate := time.Now().Local().Format("2006-01-02 15:04:05 (-07:00)")

	treeString := fmt.Sprintf("<!-- Stats: %d files | %s lines of code | Date: %s -->\n", totalFiles, formatInt(totalLines), localDate)
	treeString += scanner.GenerateDirectoryTree(path, files)

	s.jsonResponse(w, map[string]any{
		"status": "success",
		"data":   treeString,
		"count":  totalFiles,
		"lines":  totalLines,
	})
}

func (s *Server) handleGetExclusions(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]any{
		"dirs":       config.ExcludedDirs,
		"files":      config.ExcludedFiles,
		"extensions": config.BinaryExtensions,
	})
}

func (s *Server) handleGetContent(w http.ResponseWriter, r *http.Request) {
	fileParam := filepath.Base(r.URL.Query().Get("file"))
	if fileParam == "" || fileParam == "." || fileParam == "/" {
		http.Error(w, "Invalid file parameter", http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join(s.cfg.TempDir, fileParam)
	data, err := os.ReadFile(targetPath)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("File không tồn tại hoặc đã bị xóa."))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	fileParam := filepath.Base(r.URL.Query().Get("file"))
	if fileParam == "" {
		fileParam = filepath.Base(r.URL.Query().Get("download"))
	}
	if fileParam == "" || fileParam == "." || fileParam == "/" {
		http.Error(w, "File không tồn tại hoặc đã hết hạn.", http.StatusBadRequest)
		return
	}

	targetPath := filepath.Join(s.cfg.TempDir, fileParam)
	info, err := os.Stat(targetPath)
	if err != nil {
		http.Error(w, "File không tồn tại hoặc đã hết hạn.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Description", "File Transfer")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileParam))
	w.Header().Set("Expires", "0")
	w.Header().Set("Cache-Control", "must-revalidate")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))

	f, err := os.Open(targetPath)
	if err != nil {
		http.Error(w, "Error reading file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	io.Copy(w, f)
}

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	projectPath := strings.TrimSpace(r.URL.Query().Get("path"))
	mode := r.URL.Query().Get("mode")
	if mode != "stats" {
		mode = "full"
	}

	events := make(chan generator.ProgressEvent, 200)

	ctx := r.Context()

	go func() {
		defer close(events)
		_, err := s.gen.Generate(ctx, projectPath, mode, events)
		if err != nil {
			events <- generator.ProgressEvent{
				Type:    "error",
				Message: err.Error(),
			}
		}
	}()

	for event := range events {
		dataMap := event.Data
		if dataMap == nil {
			dataMap = make(map[string]any)
		}
		dataMap["message"] = event.Message

		payloadBytes, err := json.Marshal(dataMap)
		if err != nil {
			continue
		}

		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, payloadBytes)
		flusher.Flush()
	}
}

func (s *Server) jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "error",
		"message": msg,
	})
}

func formatInt(n int64) string {
	in := strconv.FormatInt(n, 10)
	out := make([]byte, 0, len(in)+(len(in)-1)/3)
	for i, c := range in {
		if i > 0 && (len(in)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func (s *Server) handleGetVersion(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]string{
		"version":     s.cfg.Version,
		"github_repo": config.GitHubRepo,
	})
}

func (s *Server) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	info, err := updater.CheckUpdate(s.cfg.Version)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, info)
}

func (s *Server) handleDownloadUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DownloadURL string `json:"download_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if body.DownloadURL == "" {
		s.jsonError(w, "Download URL is required", http.StatusBadRequest)
		return
	}

	if err := updater.StartBackgroundDownload(body.DownloadURL); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]string{
		"status":  "success",
		"message": "Background download started",
	})
}

func (s *Server) handleGetUpdateProgress(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, updater.GetProgress())
}

func (s *Server) handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]string{
		"status":  "success",
		"message": "Applying update and restarting application...",
	})

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = updater.ApplyPreparedUpdate()
	}()
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]string{
		"status":  "success",
		"message": "Shutting down application...",
	})

	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(0)
	}()
}

var (
	hbMu               sync.RWMutex
	lastHeartbeatTime  = time.Now()
	hasClientConnected = false
)

func GetLastHeartbeat() time.Time {
	hbMu.RLock()
	defer hbMu.RUnlock()
	return lastHeartbeatTime
}

func HasConnected() bool {
	hbMu.RLock()
	defer hbMu.RUnlock()
	return hasClientConnected
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	hbMu.Lock()
	lastHeartbeatTime = time.Now()
	hasClientConnected = true
	hbMu.Unlock()

	s.jsonResponse(w, map[string]string{"status": "pong"})
}

func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.jsonError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	tokens := s.tok.CountTokens(body.Text)
	mode := s.tok.Mode()
	chars := len(body.Text)
	lines := strings.Count(body.Text, "\n") + 1
	if body.Text == "" {
		lines = 0
	}

	s.jsonResponse(w, map[string]any{
		"tokens":     tokens,
		"characters": chars,
		"lines":      lines,
		"token_mode": mode,
	})
}
