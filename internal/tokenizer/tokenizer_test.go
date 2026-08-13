package tokenizer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenizer_EstimateTokensHeuristic(t *testing.T) {
	tok := NewTokenizer(t.TempDir())

	tests := []struct {
		input    string
		minCount int
	}{
		{"", 0},
		{"Hello, world!", 3},
		{"func main() {\n\tfmt.Println(\"Hello\")\n}", 8},
	}

	for _, tt := range tests {
		got := tok.EstimateTokensHeuristic(tt.input)
		if tt.input == "" && got != 0 {
			t.Errorf("EstimateTokensHeuristic(%q) = %d; want 0", tt.input, got)
		}
		if tt.input != "" && got < tt.minCount {
			t.Errorf("EstimateTokensHeuristic(%q) = %d; want >= %d", tt.input, got, tt.minCount)
		}
	}
}

func TestTokenizer_CountTokensExact(t *testing.T) {
	tok := NewTokenizer(t.TempDir())
	mode := tok.Mode()

	if mode != "exact" {
		t.Logf("tokenizer operating in mode: %s", mode)
	}

	count := tok.CountTokens("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello world\")\n}")
	if count <= 0 {
		t.Errorf("expected count > 0, got %d", count)
	}
}

func TestTokenizer_HeuristicFallbackForced(t *testing.T) {
	tok := NewEstimator()

	count := tok.CountTokens("hello world")
	if count <= 0 {
		t.Errorf("expected count > 0, got %d", count)
	}
	if tok.Mode() != "estimate" {
		t.Errorf("NewEstimator unexpectedly switched to %q mode", tok.Mode())
	}
}

func TestCountCacheIsBoundedAndCollisionFree(t *testing.T) {
	cache := newCountCache(8, 2)
	cache.add("alpha", 1)
	cache.add("be", 2)
	if got, ok := cache.get("alpha"); !ok || got != 1 {
		t.Fatalf("cache miss for exact text key: %d, %v", got, ok)
	}
	cache.add("charlie", 3)
	if cache.bytes > 8 || cache.recency.Len() > 2 {
		t.Fatalf("cache exceeded bounds: bytes=%d entries=%d", cache.bytes, cache.recency.Len())
	}
	if _, ok := cache.get("be"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if got, ok := cache.get("charlie"); !ok || got != 3 {
		t.Fatalf("new cache entry missing: %d, %v", got, ok)
	}
	cache.clear()
	if cache.bytes != 0 || cache.recency.Len() != 0 {
		t.Fatalf("ClearCache did not release state")
	}
}

func TestEnsureVocabFileAtomicVerifiedCache(t *testing.T) {
	payload := []byte("YQ== 0\nYg== 1\n")
	hash := sha256.Sum256(payload)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))

	tok := newTestDownloadTokenizer(t, server, hex.EncodeToString(hash[:]), 1, 1024)
	path, err := tok.EnsureVocabFile()
	if err != nil {
		t.Fatalf("EnsureVocabFile failed: %v", err)
	}
	server.Close()
	cached, err := tok.EnsureVocabFile()
	if err != nil {
		t.Fatalf("verified cache should work offline: %v", err)
	}
	if cached != path || requests != 1 {
		t.Fatalf("unexpected cache behavior: path=%q cached=%q requests=%d", path, cached, requests)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("cached vocabulary mismatch: %v", err)
	}
	temporary, _ := filepath.Glob(filepath.Join(tok.cacheDir, "*.tmp"))
	if len(temporary) != 0 {
		t.Fatalf("temporary vocabulary files leaked: %v", temporary)
	}
}

func TestEnsureVocabFileRejectsHTTPAndChecksumFailures(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer server.Close()
		tok := newTestDownloadTokenizer(t, server, strings.Repeat("0", 64), 1, 1024)
		_, err := tok.EnsureVocabFile()
		if err == nil || !strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "%!") {
			t.Fatalf("expected useful HTTP status error, got %v", err)
		}
	})

	t.Run("checksum", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-the-expected-vocabulary"))
		}))
		defer server.Close()
		tok := newTestDownloadTokenizer(t, server, strings.Repeat("0", 64), 1, 1024)
		_, err := tok.EnsureVocabFile()
		if err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatalf("expected checksum error, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(tok.cacheDir, "o200k_base.tiktoken")); !os.IsNotExist(statErr) {
			t.Fatalf("invalid vocabulary was committed")
		}
	})
}

func TestEnsureVocabFileHonorsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	tok := newTestDownloadTokenizer(t, server, strings.Repeat("0", 64), 1, 1024)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tok.EnsureVocabFileContext(ctx)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func newTestDownloadTokenizer(t *testing.T, server *httptest.Server, hash string, minSize, maxSize int64) *Tokenizer {
	t.Helper()
	tok := NewTokenizer(t.TempDir())
	tok.vocabURL = server.URL
	tok.expectedSHA = hash
	tok.minVocabSize = minSize
	tok.maxVocabSize = maxSize
	tok.httpClient = server.Client()
	return tok
}
