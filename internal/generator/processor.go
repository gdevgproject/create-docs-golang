package generator

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const minimumMemoryReservation = int64(64 * 1024)

func (g *Generator) processFile(ctx context.Context, current job, limiter *memoryLimiter) result {
	if ctx.Err() != nil {
		limiter.release(current.memoryBytes)
		return result{index: current.index, rel: current.rel, failed: true}
	}
	if g.sc.IsBinary(filepath.Ext(current.path)) {
		return markerResult(current, "BINARY/MEDIA FILE - CONTENT EXCLUDED", true, false, false)
	}

	info, err := os.Stat(current.path)
	if err != nil || !info.Mode().IsRegular() {
		limiter.release(current.memoryBytes)
		return markerResult(current, "UNREADABLE FILE - CONTENT EXCLUDED", false, false, true)
	}
	if g.cfg.MaxFileSize > 0 && info.Size() > g.cfg.MaxFileSize {
		limiter.release(current.memoryBytes)
		message := fmt.Sprintf("FILE TOO LARGE - CONTENT EXCLUDED (%d bytes; limit %d bytes)", info.Size(), g.cfg.MaxFileSize)
		return markerResult(current, message, false, true, false)
	}

	raw, err := os.ReadFile(current.path)
	if err != nil {
		limiter.release(current.memoryBytes)
		return markerResult(current, "UNREADABLE FILE - CONTENT EXCLUDED", false, false, true)
	}
	if g.cfg.MaxFileSize > 0 && int64(len(raw)) > g.cfg.MaxFileSize {
		limiter.release(current.memoryBytes)
		message := fmt.Sprintf("FILE TOO LARGE - CONTENT EXCLUDED (%d bytes; limit %d bytes)", len(raw), g.cfg.MaxFileSize)
		return markerResult(current, message, false, true, false)
	}

	cleanText, isText := CleanAndValidateText(raw)
	if !isText {
		limiter.release(current.memoryBytes)
		return markerResult(current, "BINARY/MEDIA FILE - CONTENT EXCLUDED", true, false, false)
	}
	cleanText = SanitizeContent(cleanText)

	lines := int64(0)
	if cleanText != "" {
		lines = int64(strings.Count(cleanText, "\n")) + 1
	}
	tokens := int64(g.tok.CountTokensUncached(cleanText))
	escapedContent := EscapeCDATA(cleanText)

	var chunk bytes.Buffer
	chunk.Grow(len(escapedContent) + len(current.rel) + 64)
	fmt.Fprintf(&chunk, "<file path=\"%s\">\n<![CDATA[\n", html.EscapeString(current.rel))
	chunk.WriteString(escapedContent)
	if !strings.HasSuffix(escapedContent, "\n") {
		chunk.WriteByte('\n')
	}
	chunk.WriteString("]]>\n</file>\n\n")

	return result{
		index:       current.index,
		rel:         current.rel,
		chunk:       chunk.Bytes(),
		lines:       lines,
		tokens:      tokens,
		memoryBytes: current.memoryBytes,
	}
}

func markerResult(current job, message string, binary, skipped, failed bool) result {
	path := html.EscapeString(current.rel)
	return result{
		index:   current.index,
		rel:     current.rel,
		chunk:   []byte(fmt.Sprintf("<file path=\"%s\">\n[%s]\n</file>\n\n", path, message)),
		binary:  binary,
		skipped: skipped,
		failed:  failed,
	}
}

type memoryLimiter struct {
	mu      sync.Mutex
	limit   int64
	used    int64
	changed chan struct{}
}

func newMemoryLimiter(limit int64) *memoryLimiter {
	return &memoryLimiter{limit: limit, changed: make(chan struct{})}
}

func (limiter *memoryLimiter) acquire(ctx context.Context, amount int64) error {
	if amount <= 0 {
		return nil
	}
	if amount > limiter.limit {
		amount = limiter.limit
	}
	for {
		limiter.mu.Lock()
		if limiter.used+amount <= limiter.limit {
			limiter.used += amount
			limiter.mu.Unlock()
			return nil
		}
		changed := limiter.changed
		limiter.mu.Unlock()

		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (limiter *memoryLimiter) release(amount int64) {
	if amount <= 0 {
		return
	}
	if amount > limiter.limit {
		amount = limiter.limit
	}
	limiter.mu.Lock()
	limiter.used -= amount
	if limiter.used < 0 {
		limiter.used = 0
	}
	close(limiter.changed)
	limiter.changed = make(chan struct{})
	limiter.mu.Unlock()
}
