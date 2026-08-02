package generator

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"codedocs/internal/config"
	"codedocs/internal/scanner"
	"codedocs/internal/tokenizer"
)

var safeFilenameRegex = regexp.MustCompile(`[^A-Za-z0-9_\-]`)

type ProgressEvent struct {
	Type    string         `json:"type"` // "log", "progress", "complete", "error"
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

type GenerateResult struct {
	FileName    string  `json:"file_name"`
	FilePath    string  `json:"file_path"`
	TotalFiles  int     `json:"total"`
	TotalLines  int64   `json:"lines"`
	TotalTokens int64   `json:"tokens"`
	TokenMode   string  `json:"token_mode"`
	Elapsed     float64 `json:"elapsed"`
	SizeBytes   int64   `json:"size"`
	Mode        string  `json:"mode"`
}

type Generator struct {
	cfg       *config.Config
	sc        *scanner.Scanner
	tok       *tokenizer.Tokenizer
	bufferCap int
}

func NewGenerator(cfg *config.Config, sc *scanner.Scanner, tok *tokenizer.Tokenizer) *Generator {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if sc == nil {
		sc = scanner.NewScanner()
	}
	if tok == nil {
		tok = tokenizer.NewTokenizer(cfg.CacheDir)
	}

	return &Generator{
		cfg:       cfg,
		sc:        sc,
		tok:       tok,
		bufferCap: 524288, // 512KB
	}
}

type job struct {
	index int
	path  string
	rel   string
}

type result struct {
	index  int
	rel    string
	chunk  []byte
	lines  int64
	tokens int64
}

// Generate scans projectPath and streams output to target markdown file while sending progress events
func (g *Generator) Generate(ctx context.Context, projectPath string, mode string, events chan<- ProgressEvent) (*GenerateResult, error) {
	startTime := time.Now()

	cleanPath := filepath.Clean(projectPath)
	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("invalid project directory: %s", projectPath)
	}

	if events != nil {
		events <- ProgressEvent{Type: "log", Message: "🚀 Bắt đầu quét dự án..."}
	}

	files, err := g.sc.ScanProjectFiles(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("error scanning project files: %w", err)
	}

	totalFiles := len(files)
	if totalFiles == 0 {
		return nil, fmt.Errorf("no valid files found in project")
	}

	if events != nil {
		events <- ProgressEvent{
			Type:    "log",
			Message: fmt.Sprintf("📦 Tìm thấy %d file. Đang xử lý... (Bỏ qua file > %dMB)", totalFiles, g.cfg.MaxFileSize/(1024*1024)),
		}
	}

	tokenMode := g.tok.Mode()
	if events != nil {
		if tokenMode == "exact" {
			events <- ProgressEvent{Type: "log", Message: "✅ Token counter: CHÍNH XÁC (o200k_base thật — encoding của GPT-4o/GPT-5.x)."}
		} else {
			events <- ProgressEvent{Type: "log", Message: "⚠️ Không tải được vocab thật (server không có internet ra ngoài hoặc bị chặn) — dùng chế độ ƯỚC TÍNH."}
		}
	}

	// Prepare output file name and directory
	if err := os.MkdirAll(g.cfg.TempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	projName := filepath.Base(cleanPath)
	if projName == "" || projName == "." || projName == "/" {
		projName = "project"
	}
	safeName := safeFilenameRegex.ReplaceAllString(projName, "_")
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("docs_%s_%s.md", safeName, timestamp)
	outFilePath := filepath.Join(g.cfg.TempDir, fileName)

	outFile, err := os.Create(outFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	writer := bufio.NewWriterSize(outFile, g.bufferCap)

	// Write Document Header
	header := fmt.Sprintf("# DOCUMENTATION: %s\nGenerated: %s\n\n", projName, time.Now().Format("2006-01-02 15:04:05"))
	header += "## SYSTEM INSTRUCTION (Prompt)\n"
	header += "You are an expert AI assistant. The following text contains the full source code of a project.\n"
	header += "1. **Structure**: Refer to the 'Directory Tree' for file organization.\n"
	header += "2. **Content**: Source code is wrapped in `<file>` tags with `path` attributes.\n"
	header += "3. **Syntax**: Code content is enclosed in `<![CDATA[ ... ]]>` to preserve characters.\n"
	header += "4. **Binary**: Binary/Media files are listed in the tree but their content is excluded to save tokens.\n\n"

	if _, err := writer.WriteString(header); err != nil {
		return nil, err
	}

	// Write Directory Tree
	treeString := scanner.GenerateDirectoryTree(cleanPath, files)
	if _, err := writer.WriteString(fmt.Sprintf("## 1. DIRECTORY TREE\n```text\n%s```\n\n## 2. SOURCE CODE CONTENT\n\n<project_codebase>\n\n", treeString)); err != nil {
		return nil, err
	}

	normRoot := filepath.ToSlash(cleanPath)
	prefixLen := len(normRoot) + 1

	// Setup Worker Pool for File Processing
	jobsChan := make(chan job, totalFiles)
	resultsChan := make(chan result, totalFiles)

	for i, f := range files {
		rel := f
		if len(f) > prefixLen {
			rel = f[prefixLen:]
		}
		jobsChan <- job{index: i, path: f, rel: rel}
	}
	close(jobsChan)

	var wg sync.WaitGroup
	workers := g.cfg.Workers
	if workers < 1 {
		workers = 4
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobsChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				res := g.processFile(j)
				resultsChan <- res
			}
		}()
	}

	wg.Wait()
	close(resultsChan)

	// Collect results ordered by file index
	resMap := make(map[int]result, totalFiles)
	for r := range resultsChan {
		resMap[r.index] = r
	}

	var totalLines int64
	var totalTokens int64

	for i := 0; i < totalFiles; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		r, ok := resMap[i]
		if !ok {
			continue
		}

		totalLines += r.lines
		totalTokens += r.tokens

		if _, err := writer.Write(r.chunk); err != nil {
			return nil, err
		}

		// Progress update
		processed := i + 1
		if events != nil && (processed <= 3 || processed%20 == 0 || processed == totalFiles) {
			elapsedSec := time.Since(startTime).Seconds()
			speed := float64(0)
			if elapsedSec > 0 {
				speed = math.Round(float64(processed) / elapsedSec)
			}
			percent := int((float64(processed) / float64(totalFiles)) * 100)

			events <- ProgressEvent{
				Type:    "progress",
				Message: fmt.Sprintf("Reading: %s", r.rel),
				Data: map[string]any{
					"percent": percent,
					"current": processed,
					"total":   totalFiles,
					"speed":   speed,
				},
			}
		}
	}

	elapsed := math.Round(time.Since(startTime).Seconds()*100) / 100

	footer := "</project_codebase>\n\n"
	tokenLabel := fmt.Sprintf("%d tokens (o200k_base, exact)", totalTokens)
	if tokenMode != "exact" {
		tokenLabel = fmt.Sprintf("~%d tokens (estimate)", totalTokens)
	}
	footer += fmt.Sprintf("<!-- Stats: %d files | %d lines of code | %s | Generated in %.2fs -->", totalFiles, totalLines, tokenLabel, elapsed)
	_, _ = writer.WriteString(footer)

	_ = writer.Flush()
	_ = outFile.Close()

	outInfo, err := os.Stat(outFilePath)
	sizeBytes := int64(0)
	if err == nil {
		sizeBytes = outInfo.Size()
	}

	res := &GenerateResult{
		FileName:    fileName,
		FilePath:    outFilePath,
		TotalFiles:  totalFiles,
		TotalLines:  totalLines,
		TotalTokens: totalTokens,
		TokenMode:   tokenMode,
		Elapsed:     elapsed,
		SizeBytes:   sizeBytes,
		Mode:        mode,
	}

	if events != nil {
		events <- ProgressEvent{
			Type:    "complete",
			Message: fileName,
			Data: map[string]any{
				"total":      totalFiles,
				"lines":      totalLines,
				"tokens":     totalTokens,
				"token_mode": tokenMode,
				"elapsed":    elapsed,
				"size":       sizeBytes,
				"mode":       mode,
			},
		}
	}

	return res, nil
}

func (g *Generator) processFile(j job) result {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("<file path=\"%s\">\n", j.rel))

	ext := filepath.Ext(j.path)
	if g.sc.IsBinary(ext) {
		buf.WriteString("    <!-- [BINARY/MEDIA FILE - CONTENT EXCLUDED] -->\n</file>\n\n")
		return result{index: j.index, rel: j.rel, chunk: buf.Bytes(), lines: 0, tokens: 0}
	}

	info, err := os.Stat(j.path)
	if err != nil || info.Size() < 0 {
		buf.WriteString("    <!-- [ERROR READING FILE] -->\n</file>\n\n")
		return result{index: j.index, rel: j.rel, chunk: buf.Bytes(), lines: 0, tokens: 0}
	}

	if info.Size() == 0 {
		buf.WriteString("    <!-- [EMPTY FILE] -->\n</file>\n\n")
		return result{index: j.index, rel: j.rel, chunk: buf.Bytes(), lines: 0, tokens: 0}
	}

	if g.cfg.MaxFileSize > 0 && info.Size() > g.cfg.MaxFileSize {
		mb := info.Size() / (1024 * 1024)
		buf.WriteString(fmt.Sprintf("    <!-- [FILE TOO LARGE (%dMB) - CONTENT EXCLUDED] -->\n</file>\n\n", mb))
		return result{index: j.index, rel: j.rel, chunk: buf.Bytes(), lines: 0, tokens: 0}
	}

	contentBytes, err := os.ReadFile(j.path)
	if err != nil {
		buf.WriteString("    <!-- [ERROR READING FILE] -->\n</file>\n\n")
		return result{index: j.index, rel: j.rel, chunk: buf.Bytes(), lines: 0, tokens: 0}
	}

	content := string(contentBytes)
	lines := int64(bytes.Count(contentBytes, []byte("\n")) + 1)
	tokens := int64(g.tok.CountTokens(content))

	sanitized := SanitizeContent(content)
	escaped := EscapeCDATA(sanitized)

	buf.WriteString("<![CDATA[\n")
	buf.WriteString(escaped)
	buf.WriteString("\n]]>\n</file>\n\n")

	return result{
		index:  j.index,
		rel:    j.rel,
		chunk:  buf.Bytes(),
		lines:  lines,
		tokens: tokens,
	}
}
