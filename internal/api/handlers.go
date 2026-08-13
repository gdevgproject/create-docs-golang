package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"codedocs/internal/bookmarks"
	"codedocs/internal/config"
	"codedocs/internal/generator"
	"codedocs/internal/scanner"
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

	if !s.decodeJSONBody(w, r, &body, maxJSONBodyBytes) {
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

	if !s.decodeJSONBody(w, r, &body, maxJSONBodyBytes) {
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

	if !s.decodeJSONBody(w, r, &body, maxJSONBodyBytes) {
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

func (s *Server) handleDeleteBookmarkHistory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		GeneratedAt string `json:"generated_at"`
	}

	if !s.decodeJSONBody(w, r, &body, maxJSONBodyBytes) {
		return
	}

	if body.ID == "" {
		s.jsonError(w, "ID is required", http.StatusBadRequest)
		return
	}

	var bms map[string]bookmarks.Bookmark
	var err error

	if strings.TrimSpace(body.GeneratedAt) != "" {
		bms, err = s.bm.DeleteHistoryItem(body.ID, body.GeneratedAt)
	} else {
		bms, err = s.bm.ClearHistory(body.ID)
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

func (s *Server) handleRenameBookmarkHistory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		GeneratedAt string `json:"generated_at"`
		Label       string `json:"label"`
	}

	if !s.decodeJSONBody(w, r, &body, maxJSONBodyBytes) {
		return
	}

	if strings.TrimSpace(body.ID) == "" || strings.TrimSpace(body.GeneratedAt) == "" {
		s.jsonError(w, "Bookmark ID and GeneratedAt timestamp are required", http.StatusBadRequest)
		return
	}

	bms, err := s.bm.RenameHistoryItem(strings.TrimSpace(body.ID), strings.TrimSpace(body.GeneratedAt), body.Label)
	if err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]any{
		"status": "success",
		"data":   bms,
	})
}

func (s *Server) handleExportBookmarks(w http.ResponseWriter, r *http.Request) {
	data, err := s.bm.ExportData()
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to export bookmarks: %v", err), http.StatusInternalServerError)
		return
	}

	dateStr := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("codedocs_bookmarks_backup_%s.json", dateStr)

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleImportBookmarks(w http.ResponseWriter, r *http.Request) {
	bodyBytes, ok := s.readBody(w, r, maxImportBytes)
	if !ok {
		return
	}
	if len(bodyBytes) == 0 {
		s.jsonError(w, "Import payload is empty", http.StatusBadRequest)
		return
	}

	bms, err := s.bm.ImportData(bodyBytes)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Import failed: %v", err), http.StatusBadRequest)
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

	files, err := s.sc.ScanProjectFilesContext(r.Context(), path)
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Không thể quét thư mục: %v", err), http.StatusBadRequest)
		return
	}

	totalFiles := len(files)
	stats := s.sc.CountProjectStats(r.Context(), files, s.cfg.MaxFileSize, s.cfg.Workers)
	totalLines := stats.Lines
	localDate := time.Now().Local().Format("2006-01-02 15:04:05 (-07:00)")

	treeString := fmt.Sprintf("<!-- Stats: %d files | %s lines of code | Date: %s -->\n", totalFiles, formatInt(totalLines), localDate)
	treeString += scanner.GenerateDirectoryTree(path, files)

	s.jsonResponse(w, map[string]any{
		"status":           "success",
		"data":             treeString,
		"count":            totalFiles,
		"lines":            totalLines,
		"bytes":            stats.Bytes,
		"text_files":       stats.TextFiles,
		"binary_files":     stats.BinaryFiles,
		"skipped_files":    stats.SkippedFiles,
		"unreadable_files": stats.Unreadable,
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
	file, info, _, err := s.openGeneratedFile(r.URL.Query().Get("file"))
	if err != nil {
		http.Error(w, "Generated file was not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	offset, err := parseOffsetOrLimit(r.URL.Query().Get("offset"), 0)
	if err != nil {
		http.Error(w, "Invalid content offset", http.StatusBadRequest)
		return
	}
	hasLimit := strings.TrimSpace(r.URL.Query().Get("limit")) != ""
	limit, err := parseOffsetOrLimit(r.URL.Query().Get("limit"), info.Size())
	if err != nil {
		http.Error(w, "Invalid content limit", http.StatusBadRequest)
		return
	}
	if hasLimit {
		limit = min(limit, int64(maxPreviewBytes))
	}
	if offset > info.Size() || (offset == info.Size() && info.Size() > 0) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", info.Size()))
		http.Error(w, "Content offset is outside the file", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	length := min(limit, info.Size()-offset)
	end := offset + length

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("X-Content-Size", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("X-Content-Offset", strconv.FormatInt(offset, 10))
	w.Header().Set("X-Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("X-Content-Truncated", strconv.FormatBool(end < info.Size()))
	if offset > 0 || end < info.Size() {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, max(offset, end-1), info.Size()))
		w.WriteHeader(http.StatusPartialContent)
	}
	_, _ = io.CopyN(w, io.NewSectionReader(file, offset, length), length)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	fileParam := r.URL.Query().Get("file")
	if fileParam == "" {
		fileParam = r.URL.Query().Get("download")
	}
	file, info, fileName, err := s.openGeneratedFile(fileParam)
	if err != nil {
		http.Error(w, "Generated file was not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", attachmentHeader(fileName))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, fileName, info.ModTime(), file)
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
		res, err := s.gen.Generate(ctx, projectPath, mode, events)
		if err == nil && res != nil {
			_, _ = s.bm.SaveHistoryResult(projectPath, bookmarks.LastResult{
				FileName:    res.FileName,
				TotalFiles:  res.TotalFiles,
				TotalLines:  res.TotalLines,
				TotalTokens: res.TotalTokens,
				TokenMode:   res.TokenMode,
				SizeBytes:   res.SizeBytes,
				Elapsed:     res.Elapsed,
				GeneratedAt: res.GeneratedAt,
			})
		}
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
	info, err := s.updateManager.Check(r.Context())
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

	if !s.decodeJSONBody(w, r, &body, maxJSONBodyBytes) {
		return
	}

	if body.DownloadURL == "" {
		s.jsonError(w, "Download URL is required", http.StatusBadRequest)
		return
	}

	if err := s.updateManager.StartBackgroundDownload(body.DownloadURL); err != nil {
		s.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]string{
		"status":  "success",
		"message": "Background download started",
	})
}

func (s *Server) handleGetUpdateProgress(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, s.updateManager.GetProgress())
}

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]any{
		"status":        "ready",
		"version":       s.cfg.Version,
		"token_mode":    s.tok.Mode(),
		"workers":       s.cfg.Workers,
		"max_file_size": s.cfg.MaxFileSize,
		"update":        s.updateManager.GetProgress(),
	})
}

func (s *Server) handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	if err := s.updateManager.ApplyPreparedUpdate(); err != nil {
		s.jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	s.jsonResponse(w, map[string]string{
		"status":  "success",
		"message": "Applying update and restarting application...",
	})

	go func() {
		time.Sleep(300 * time.Millisecond)
		s.requestShutdown()
	}()
}

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]string{
		"status":  "success",
		"message": "Shutting down application...",
	})

	go func() {
		time.Sleep(200 * time.Millisecond)
		s.requestShutdown()
	}()
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s.jsonResponse(w, map[string]string{"status": "pong", "version": s.cfg.Version})
}

func (s *Server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}

	if !s.decodeJSONBody(w, r, &body, maxTokenBodyBytes) {
		return
	}

	tokens := s.tok.CountTokens(body.Text)
	mode := s.tok.Mode()
	chars := utf8.RuneCountInString(body.Text)
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
