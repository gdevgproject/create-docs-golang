package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

func TestSanitizeContentSinglePass(t *testing.T) {
	input := "line  \r\n\r\n\r\nnext\t"
	if got, want := SanitizeContent(input), "line\n\nnext"; got != want {
		t.Fatalf("SanitizeContent() = %q, want %q", got, want)
	}
}

func TestGenerator_SanitizesCDATAAndEscapesPath(t *testing.T) {
	projectDir := t.TempDir()
	outputDir := t.TempDir()
	content := "first  \r\n\r\n\r\nvalue ]]> tail\r\n"
	if err := os.WriteFile(filepath.Join(projectDir, "a&b.go"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.TempDir = outputDir
	tok := tokenizer.NewTokenizer(t.TempDir())
	gen := NewGenerator(cfg, scanner.NewScanner(), tok)
	result, err := gen.Generate(context.Background(), projectDir, "full", nil)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	document, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(document)
	for _, expected := range []string{`<file path="a&amp;b.go">`, "first\n\nvalue ]]]]><![CDATA[> tail\n"} {
		if !strings.Contains(text, expected) {
			t.Errorf("output is missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "first  \r") || strings.Contains(text, "\n\n\n") {
		t.Errorf("output was not normalized: %q", text)
	}

	expectedTokens := int64(tok.CountTokensUncached(SanitizeContent(content)))
	if result.TotalTokens != expectedTokens {
		t.Errorf("tokens = %d, want %d", result.TotalTokens, expectedTokens)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3} \([+-]\d{2}:\d{2}\)$`).MatchString(result.GeneratedAt) {
		t.Errorf("generated_at is not a stable millisecond timestamp: %q", result.GeneratedAt)
	}
}

func TestGenerator_ExcludesOversizedFilesFromTokens(t *testing.T) {
	projectDir := t.TempDir()
	outputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "small.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "large.txt"), []byte(strings.Repeat("large", 100)), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig()
	cfg.TempDir = outputDir
	cfg.MaxFileSize = 32
	tok := tokenizer.NewTokenizer(t.TempDir())
	result, err := NewGenerator(cfg, scanner.NewScanner(), tok).Generate(context.Background(), projectDir, "stats", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.SkippedFiles != 1 || result.TotalFiles != 2 || result.Mode != "stats" {
		t.Errorf("unexpected result state: %+v", result)
	}
	if want := int64(tok.CountTokensUncached("ok\n")); result.TotalTokens != want {
		t.Errorf("tokens = %d, want only small-file tokens %d", result.TotalTokens, want)
	}
	document, _ := os.ReadFile(result.FilePath)
	if !strings.Contains(string(document), "FILE TOO LARGE - CONTENT EXCLUDED") {
		t.Errorf("large-file marker missing from output")
	}
	if matches, _ := filepath.Glob(filepath.Join(outputDir, "*.part")); len(matches) != 0 {
		t.Errorf("successful generation left partial files: %v", matches)
	}
}

func TestGenerator_CancellationRemovesPartialOutput(t *testing.T) {
	projectDir := t.TempDir()
	outputDir := t.TempDir()
	for index := range 100 {
		name := filepath.Join(projectDir, "file_"+formatTestIndex(index)+".go")
		if err := os.WriteFile(name, []byte(strings.Repeat("package sample\n", 200)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := config.DefaultConfig()
	cfg.TempDir = outputDir
	cfg.Workers = 4
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan ProgressEvent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for event := range events {
			if event.Type == "progress" {
				if percent, ok := event.Data["percent"].(int); ok && percent >= 10 {
					cancel()
				}
			}
		}
	}()

	_, err := NewGenerator(cfg, scanner.NewScanner(), tokenizer.NewTokenizer(t.TempDir())).Generate(ctx, projectDir, "full", events)
	close(events)
	<-done
	if err == nil {
		t.Fatal("expected cancelled generation to return an error")
	}
	files, globErr := filepath.Glob(filepath.Join(outputDir, "*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(files) != 0 {
		t.Errorf("cancelled generation left output artifacts: %v", files)
	}
}

func formatTestIndex(index int) string {
	return fmt.Sprintf("%03d", index)
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
