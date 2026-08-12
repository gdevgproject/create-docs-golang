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
