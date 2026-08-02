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
	"sync/atomic"
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
	GeneratedAt string  `json:"generated_at"`
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
	localNow := startTime.Local()
	formattedLocalTime := localNow.Format("2006-01-02 15:04:05 (-07:00)")

	cleanPath := filepath.Clean(projectPath)
	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("invalid project directory: %s", projectPath)
	}

	if events != nil {
		events <- ProgressEvent{
			Type:    "progress",
			Message: "🔍 Scanning project directory & ignore rules...",
			Data: map[string]any{
				"percent": 3,
				"current": 0,
				"total":   0,
				"speed":   0,
			},
		}
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
			Type:    "progress",
			Message: fmt.Sprintf("📦 Found %d files. Preparing worker pool...", totalFiles),
			Data: map[string]any{
				"percent": 8,
				"current": 0,
				"total":   totalFiles,
				"speed":   0,
			},
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
	timestamp := localNow.Format("20060102_150405")
	fileName := fmt.Sprintf("docs_%s_%s.md", safeName, timestamp)
	outFilePath := filepath.Join(g.cfg.TempDir, fileName)

	outFile, err := os.Create(outFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	writer := bufio.NewWriterSize(outFile, g.bufferCap)

	// Write Document Header with exact local datetime
	header := fmt.Sprintf("# DOCUMENTATION: %s\nGenerated: %s\n\n", projName, formattedLocalTime)
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

	var processedCount int64
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

				// Real-time smooth atomic progress streaming!
				count := atomic.AddInt64(&processedCount, 1)
				if events != nil && (count <= 5 || count%5 == 0 || count == int64(totalFiles)) {
					elapsedSec := time.Since(startTime).Seconds()
					speed := float64(0)
					if elapsedSec > 0 {
						speed = math.Round(float64(count) / elapsedSec)
					}
					// Scale worker progress smoothly between 10% and 90%
					pct := 10 + int((float64(count)/float64(totalFiles))*80)

					events <- ProgressEvent{
						Type:    "progress",
						Message: fmt.Sprintf("Processing (%d/%d): %s", count, totalFiles, res.rel),
						Data: map[string]any{
							"percent": pct,
							"current": count,
							"total":   totalFiles,
							"speed":   speed,
						},
					}
				}
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
	}

	if events != nil {
		events <- ProgressEvent{
			Type:    "progress",
			Message: "Writing final output file...",
			Data: map[string]any{
				"percent": 98,
				"current": totalFiles,
				"total":   totalFiles,
				"speed":   float64(totalFiles) / math.Max(0.1, time.Since(startTime).Seconds()),
			},
		}
	}

	elapsed := math.Round(time.Since(startTime).Seconds()*100) / 100

	footer := "</project_codebase>\n\n"
	tokenLabel := fmt.Sprintf("%d tokens (o200k_base, exact)", totalTokens)
	if tokenMode != "exact" {
		tokenLabel = fmt.Sprintf("~%d tokens (estimated)", totalTokens)
	}

	footer += fmt.Sprintf("<!-- SUMMARY: %d files, %d lines, %s, size: %d bytes, time: %.2fs -->\n",
		totalFiles, totalLines, tokenLabel, writer.Buffered(), elapsed)

	if _, err := writer.WriteString(footer); err != nil {
		return nil, err
	}

	if err := writer.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush writer: %w", err)
	}

	fileInfo, err := outFile.Stat()
	sizeBytes := int64(0)
	if err == nil {
		sizeBytes = fileInfo.Size()
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
		GeneratedAt: formattedLocalTime,
	}

	if events != nil {
		events <- ProgressEvent{
			Type:    "complete",
			Message: fileName,
			Data: map[string]any{
				"total":        res.TotalFiles,
				"lines":        res.TotalLines,
				"tokens":       res.TotalTokens,
				"token_mode":   res.TokenMode,
				"size":         res.SizeBytes,
				"elapsed":      res.Elapsed,
				"file_name":    res.FileName,
				"generated_at": res.GeneratedAt,
			},
		}
	}

	return res, nil
}

func (g *Generator) processFile(j job) result {
	if g.sc.IsBinary(filepath.Ext(j.path)) {
		var buf bytes.Buffer
		buf.WriteString(fmt.Sprintf("<file path=\"%s\">\n[BINARY/MEDIA FILE - CONTENT EXCLUDED]\n</file>\n\n", j.rel))
		return result{
			index:  j.index,
			rel:    j.rel,
			chunk:  buf.Bytes(),
			lines:  1,
			tokens: 0,
		}
	}

	data, err := os.ReadFile(j.path)
	if err != nil || len(data) == 0 {
		return result{index: j.index, rel: j.rel}
	}

	lines := int64(bytes.Count(data, []byte("\n"))) + 1
	tokens := int64(g.tok.CountTokens(string(data)))

	var buf bytes.Buffer
	buf.Grow(len(data) + 128)

	buf.WriteString(fmt.Sprintf("<file path=\"%s\">\n<![CDATA[\n", j.rel))
	buf.Write(data)
	if !bytes.HasSuffix(data, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString("]]>\n</file>\n\n")

	return result{
		index:  j.index,
		rel:    j.rel,
		chunk:  buf.Bytes(),
		lines:  lines,
		tokens: tokens,
	}
}
