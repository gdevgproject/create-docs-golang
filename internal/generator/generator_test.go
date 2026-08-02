package generator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codedocs/internal/config"
	"codedocs/internal/scanner"
	"codedocs/internal/tokenizer"
)

func TestSanitizeContent(t *testing.T) {
	input := "line1  \r\nline2\t \r\n\n\n\nline3\r\n"
	sanitized := SanitizeContent(input)

	expected := "line1\nline2\n\nline3\n"
	if sanitized != expected {
		t.Errorf("SanitizeContent failed.\nGot: %q\nWant: %q", sanitized, expected)
	}
}

func TestEscapeCDATA(t *testing.T) {
	input := "var x = ']]></script>';"
	escaped := EscapeCDATA(input)

	expected := "var x = ']]]]><![CDATA[></script>';"
	if escaped != expected {
		t.Errorf("EscapeCDATA failed.\nGot: %q\nWant: %q", escaped, expected)
	}
}

func TestGenerator_Generate(t *testing.T) {
	tempProject := t.TempDir()
	tempOutput := t.TempDir()

	os.WriteFile(filepath.Join(tempProject, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(tempProject, "logo.png"), []byte("dummy binary content"), 0644)

	cfg := config.DefaultConfig()
	cfg.TempDir = tempOutput
	cfg.Workers = 2

	sc := scanner.NewScanner()
	tok := tokenizer.NewTokenizer(t.TempDir())
	gen := NewGenerator(cfg, sc, tok)

	events := make(chan ProgressEvent, 100)
	ctx := context.Background()

	res, err := gen.Generate(ctx, tempProject, "full", events)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	close(events)

	if res.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", res.TotalFiles)
	}

	if res.TotalLines <= 0 {
		t.Errorf("expected total lines > 0, got %d", res.TotalLines)
	}

	contentBytes, err := os.ReadFile(res.FilePath)
	if err != nil {
		t.Fatalf("failed to read generated output file: %v", err)
	}

	docContent := string(contentBytes)

	if !strings.Contains(docContent, "# DOCUMENTATION:") {
		t.Errorf("generated file missing header")
	}

	if !strings.Contains(docContent, "package main") {
		t.Errorf("generated file missing source code content")
	}

	if !strings.Contains(docContent, "[BINARY/MEDIA FILE - CONTENT EXCLUDED]") {
		t.Errorf("generated file missing binary file marker")
	}
}
