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

func TestCleanAndValidateText(t *testing.T) {
	// 1. UTF-8 BOM test
	bomUTF8 := append([]byte{0xEF, 0xBB, 0xBF}, []byte("package main\n")...)
	gotStr, isText := CleanAndValidateText(bomUTF8)
	if !isText || gotStr != "package main\n" {
		t.Errorf("UTF-8 BOM decode failed. Got: %q, isText: %v", gotStr, isText)
	}

	// 2. UTF-16 LE BOM test
	utf16LE := []byte{0xFF, 0xFE, 'H', 0x00, 'e', 0x00, 'l', 0x00, 'l', 0x00, 'o', 0x00}
	gotStr16, isText16 := CleanAndValidateText(utf16LE)
	if !isText16 || gotStr16 != "Hello" {
		t.Errorf("UTF-16 LE BOM decode failed. Got: %q, isText: %v", gotStr16, isText16)
	}

	// 3. Null byte binary payload detection test
	nullBinary := []byte("func main() {}\x00\x01\x02\xFF")
	_, isTextNull := CleanAndValidateText(nullBinary)
	if isTextNull {
		t.Errorf("expected null byte binary payload to be flagged as binary (isText=false)")
	}

	// 4. Invalid UTF-8 / Unprintable character stripping test
	invalidBytes := []byte("Hello \xFF\xFE World \x07\x08!\n")
	gotClean, isTextClean := CleanAndValidateText(invalidBytes)
	if !isTextClean || !strings.Contains(gotClean, "Hello") || strings.Contains(gotClean, "\x07") {
		t.Errorf("invalid byte cleaning failed. Got: %q", gotClean)
	}
}

func TestGenerator_TokenStatsExcludesTree(t *testing.T) {
	tempProject := t.TempDir()
	tempOutput := t.TempDir()

	codeContent := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hi\") }\n"
	os.WriteFile(filepath.Join(tempProject, "app.go"), []byte(codeContent), 0644)

	cfg := config.DefaultConfig()
	cfg.TempDir = tempOutput

	sc := scanner.NewScanner()
	tok := tokenizer.NewTokenizer(t.TempDir())
	gen := NewGenerator(cfg, sc, tok)

	res, err := gen.Generate(context.Background(), tempProject, "full", nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	expectedLines := int64(strings.Count(codeContent, "\n")) + 1
	if res.TotalLines != expectedLines {
		t.Errorf("expected res.TotalLines == %d (excluding auto-generated directory tree), got %d", expectedLines, res.TotalLines)
	}

	expectedCodeTokens := tok.CountTokens(codeContent)
	if res.TotalTokens != int64(expectedCodeTokens) {
		t.Errorf("expected res.TotalTokens == %d (pure codebase tokens), got %d", expectedCodeTokens, res.TotalTokens)
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
