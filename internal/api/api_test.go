package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codedocs/internal/config"
)

func TestAPI_GetExclusions(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg, nil)

	req := httptest.NewRequest("GET", "/api/exclusions", nil)
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res map[string][]string
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if len(res["dirs"]) == 0 || len(res["files"]) == 0 || len(res["extensions"]) == 0 {
		t.Errorf("exclusions response missing expected arrays: %v", res)
	}
}

func TestAPI_LocalAccessGuard(t *testing.T) {
	server := NewServer(config.DefaultConfig(), nil, WithLocalAccessOnly())

	allowed := httptest.NewRequest("GET", "http://127.0.0.1/api/ping", nil)
	allowed.RemoteAddr = "127.0.0.1:54321"
	allowed.Host = "127.0.0.1:8080"
	allowedW := httptest.NewRecorder()
	server.ServeHTTP(allowedW, allowed)
	if allowedW.Code != http.StatusOK {
		t.Fatalf("expected loopback request to pass, got %d", allowedW.Code)
	}

	for name, mutate := range map[string]func(*http.Request){
		"remote client":  func(r *http.Request) { r.RemoteAddr = "192.0.2.1:1234" },
		"host rebinding": func(r *http.Request) { r.Host = "attacker.example:8080" },
		"cross-site":     func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://127.0.0.1/api/ping", nil)
			req.RemoteAddr = "127.0.0.1:54321"
			req.Host = "127.0.0.1:8080"
			mutate(req)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected request to be rejected, got %d", w.Code)
			}
		})
	}
}

func TestAPI_ShutdownUsesApplicationHookOnce(t *testing.T) {
	called := make(chan struct{}, 2)
	server := NewServer(config.DefaultConfig(), nil, WithShutdown(func() { called <- struct{}{} }))

	for range 2 {
		req := httptest.NewRequest("POST", "/api/shutdown", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("shutdown failed with status %d", w.Code)
		}
	}

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("shutdown hook was not called")
	}
	select {
	case <-called:
		t.Fatal("shutdown hook was called more than once")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestAPI_CountTokens(t *testing.T) {
	cfg := config.DefaultConfig()
	server := NewServer(cfg, nil)

	body, _ := json.Marshal(map[string]string{
		"text": "Hello world! This is a test snippet for o200k_base tiktoken.",
	})

	req := httptest.NewRequest("POST", "/api/count-tokens", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res struct {
		Tokens    int    `json:"tokens"`
		TokenMode string `json:"token_mode"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if res.Tokens <= 0 {
		t.Errorf("expected positive token count, got %d", res.Tokens)
	}
}

func TestAPI_CountTokensReportsUnicodeCharacters(t *testing.T) {
	server := NewServer(config.DefaultConfig(), nil)
	body, _ := json.Marshal(map[string]string{"text": "é😊"})
	request := httptest.NewRequest(http.MethodPost, "/api/count-tokens", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("count tokens returned %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Characters int `json:"characters"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Characters != 2 {
		t.Fatalf("characters = %d, want 2 Unicode code points", result.Characters)
	}
}

func TestAPI_RejectsOversizedJSONBody(t *testing.T) {
	server := NewServer(config.DefaultConfig(), nil)
	body := `{"path":"` + strings.Repeat("x", maxJSONBodyBytes) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/bookmarks", strings.NewReader(body))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body returned %d, want 413: %s", response.Code, response.Body.String())
	}
}

func TestAPI_ContentPreviewAndLegacyFullResponse(t *testing.T) {
	tempDirectory := t.TempDir()
	content := []byte("0123456789")
	if err := os.WriteFile(filepath.Join(tempDirectory, "document.md"), content, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.TempDir = tempDirectory
	server := NewServer(cfg, nil)

	previewRequest := httptest.NewRequest(http.MethodGet, "/api/content?file=document.md&offset=2&limit=4", nil)
	previewResponse := httptest.NewRecorder()
	server.ServeHTTP(previewResponse, previewRequest)
	if previewResponse.Code != http.StatusPartialContent || previewResponse.Body.String() != "2345" {
		t.Fatalf("preview = status %d body %q", previewResponse.Code, previewResponse.Body.String())
	}
	if previewResponse.Header().Get("X-Content-Size") != "10" ||
		previewResponse.Header().Get("X-Content-Truncated") != "true" {
		t.Fatalf("preview metadata missing: %v", previewResponse.Header())
	}

	fullRequest := httptest.NewRequest(http.MethodGet, "/api/content?file=document.md", nil)
	fullResponse := httptest.NewRecorder()
	server.ServeHTTP(fullResponse, fullRequest)
	if fullResponse.Code != http.StatusOK || !bytes.Equal(fullResponse.Body.Bytes(), content) {
		t.Fatalf("legacy full content changed: status %d body %q", fullResponse.Code, fullResponse.Body.String())
	}

	rangeRequest := httptest.NewRequest(http.MethodGet, "/api/download?file=document.md", nil)
	rangeRequest.Header.Set("Range", "bytes=3-6")
	rangeResponse := httptest.NewRecorder()
	server.ServeHTTP(rangeResponse, rangeRequest)
	if rangeResponse.Code != http.StatusPartialContent || rangeResponse.Body.String() != "3456" {
		t.Fatalf("download range = status %d body %q", rangeResponse.Code, rangeResponse.Body.String())
	}
}

func TestAPI_StatusSnapshot(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Version = "v-test"
	server := NewServer(cfg, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status returned %d", response.Code)
	}
	var result struct {
		Status    string `json:"status"`
		Version   string `json:"version"`
		TokenMode string `json:"token_mode"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "ready" || result.Version != "v-test" || result.TokenMode == "" {
		t.Fatalf("unexpected status snapshot: %+v", result)
	}
}

func TestAPI_BookmarksFlow(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BookmarkFile = filepath.Join(t.TempDir(), "bookmarks.json")

	server := NewServer(cfg, nil)

	// Save Bookmark
	body, _ := json.Marshal(map[string]string{
		"path": "/test/path",
		"note": "Test Project",
	})
	req := httptest.NewRequest("POST", "/api/bookmarks", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("save bookmark failed: status %d", w.Code)
	}

	var saveRes struct {
		Status string `json:"status"`
		Data   map[string]struct {
			ID   string `json:"id"`
			Path string `json:"path"`
			Note string `json:"note"`
		} `json:"data"`
	}
	_ = json.NewDecoder(w.Body).Decode(&saveRes)

	if saveRes.Status != "success" || len(saveRes.Data) != 1 {
		t.Fatalf("expected 1 bookmark saved, got: %v", saveRes)
	}

	var savedID string
	for id := range saveRes.Data {
		savedID = id
	}

	// Delete Bookmark
	delBody, _ := json.Marshal(map[string]string{"id": savedID})
	delReq := httptest.NewRequest("DELETE", "/api/bookmarks", bytes.NewBuffer(delBody))
	delW := httptest.NewRecorder()
	server.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("delete bookmark failed: status %d", delW.Code)
	}
}

func TestAPI_ImportExportAndHistory(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BookmarkFile = filepath.Join(t.TempDir(), "bookmarks.json")
	server := NewServer(cfg, nil)

	// 1. Export empty bookmarks
	reqExp := httptest.NewRequest("GET", "/api/bookmarks/export", nil)
	wExp := httptest.NewRecorder()
	server.ServeHTTP(wExp, reqExp)
	if wExp.Code != http.StatusOK {
		t.Fatalf("export bookmarks failed: status %d", wExp.Code)
	}

	// 2. Import bookmark payload
	importJSON := `[{"id":"1","path":"/test/import-path","note":"Imported App","history":[{"file_name":"f1.md","total":5,"lines":100,"tokens":500,"generated_at":"2026-08-03 05:00:00"}]}]`
	reqImp := httptest.NewRequest("POST", "/api/bookmarks/import", bytes.NewBufferString(importJSON))
	wImp := httptest.NewRecorder()
	server.ServeHTTP(wImp, reqImp)
	if wImp.Code != http.StatusOK {
		t.Fatalf("import bookmarks failed: status %d", wImp.Code)
	}

	var impRes struct {
		Status string `json:"status"`
		Data   map[string]struct {
			ID      string `json:"id"`
			Path    string `json:"path"`
			Note    string `json:"note"`
			History []any  `json:"history"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wImp.Body).Decode(&impRes)
	if impRes.Status != "success" || len(impRes.Data) != 1 {
		t.Fatalf("expected 1 imported bookmark, got: %v", impRes)
	}

	var importedID string
	for id := range impRes.Data {
		importedID = id
	}

	// 3. Delete history item
	delHistBody, _ := json.Marshal(map[string]string{
		"id":           importedID,
		"generated_at": "2026-08-03 05:00:00",
	})
	reqDelHist := httptest.NewRequest("DELETE", "/api/bookmarks/history", bytes.NewBuffer(delHistBody))
	wDelHist := httptest.NewRecorder()
	server.ServeHTTP(wDelHist, reqDelHist)

	if wDelHist.Code != http.StatusOK {
		t.Fatalf("delete bookmark history failed: status %d", wDelHist.Code)
	}

	// 4. Test Rename history item
	importJSON2 := `[{"id":"2","path":"/test/rename-path","note":"Rename App","history":[{"file_name":"f2.md","total":8,"lines":200,"tokens":800,"generated_at":"2026-08-12 01:00:00"}]}]`
	reqImp2 := httptest.NewRequest("POST", "/api/bookmarks/import", bytes.NewBufferString(importJSON2))
	wImp2 := httptest.NewRecorder()
	server.ServeHTTP(wImp2, reqImp2)

	var impRes2 struct {
		Status string `json:"status"`
		Data   map[string]struct {
			ID      string `json:"id"`
			Path    string `json:"path"`
			History []struct {
				GeneratedAt string `json:"generated_at"`
				Label       string `json:"label"`
			} `json:"history"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wImp2.Body).Decode(&impRes2)
	var renameBmID string
	for id, bm := range impRes2.Data {
		if bm.Path == "/test/rename-path" {
			renameBmID = id
		}
	}

	renameBody, _ := json.Marshal(map[string]string{
		"id":           renameBmID,
		"generated_at": "2026-08-12 01:00:00",
		"label":        "Release Candidate 1",
	})
	reqRename := httptest.NewRequest("PUT", "/api/bookmarks/history", bytes.NewBuffer(renameBody))
	wRename := httptest.NewRecorder()
	server.ServeHTTP(wRename, reqRename)

	if wRename.Code != http.StatusOK {
		t.Fatalf("rename bookmark history failed: status %d, body: %s", wRename.Code, wRename.Body.String())
	}

	var renameRes struct {
		Status string `json:"status"`
		Data   map[string]struct {
			History []struct {
				Label string `json:"label"`
			} `json:"history"`
		} `json:"data"`
	}
	_ = json.NewDecoder(wRename.Body).Decode(&renameRes)
	if renameRes.Data[renameBmID].History[0].Label != "Release Candidate 1" {
		t.Errorf("expected renamed history label 'Release Candidate 1', got: %v", renameRes)
	}
}

func TestAPI_Structure(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n"), 0644)

	cfg := config.DefaultConfig()
	server := NewServer(cfg, nil)

	req := httptest.NewRequest("GET", "/api/structure?path="+tempDir, nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("structure query failed: status %d", w.Code)
	}

	var res struct {
		Status string `json:"status"`
		Data   string `json:"data"`
		Count  int    `json:"count"`
	}
	_ = json.NewDecoder(w.Body).Decode(&res)

	if res.Status != "success" || res.Count != 1 || !strings.Contains(res.Data, "main.go") {
		t.Errorf("unexpected structure response: %v", res)
	}
}
