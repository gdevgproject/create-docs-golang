package generator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"codedocs/internal/scanner"
)

const (
	workerMemoryBudget  = int64(256 << 20)
	maxGeneratorWorkers = 32
)

// Generate scans projectPath and atomically creates an ordered Markdown context
// document while publishing cancellable progress events.
func (g *Generator) Generate(ctx context.Context, projectPath, mode string, events chan<- ProgressEvent) (*GenerateResult, error) {
	startTime := time.Now()
	localNow := startTime.Local()
	generatedAt := localNow.Format("2006-01-02 15:04:05.000 (-07:00)")
	if mode != "stats" {
		mode = "full"
	}

	cleanPath, err := filepath.Abs(filepath.Clean(projectPath))
	if err != nil {
		return nil, fmt.Errorf("invalid project directory: %w", err)
	}
	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("invalid project directory: %s", projectPath)
	}

	if err := emitProgress(ctx, events, ProgressEvent{
		Type:    "progress",
		Message: "Scanning project and ignore rules…",
		Data:    progressData(3, 0, 0, 0),
	}); err != nil {
		return nil, err
	}

	files, err := g.sc.ScanProjectFilesContext(ctx, cleanPath)
	if err != nil {
		return nil, fmt.Errorf("scan project files: %w", err)
	}
	totalFiles := len(files)
	if totalFiles == 0 {
		return nil, fmt.Errorf("no eligible files found in project")
	}

	if err := emitProgress(ctx, events, ProgressEvent{
		Type:    "progress",
		Message: fmt.Sprintf("Found %d files", totalFiles),
		Data:    progressData(8, 0, totalFiles, 0),
	}); err != nil {
		return nil, err
	}

	tokenMode := g.tok.Mode()
	tokenMessage := "Exact o200k_base token counting ready"
	if tokenMode != "exact" {
		tokenMessage = "Offline token estimation active"
	}
	if err := emitProgress(ctx, events, ProgressEvent{Type: "log", Message: tokenMessage}); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(g.cfg.TempDir, 0o700); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	projectName := filepath.Base(cleanPath)
	fileName := outputFileName(projectName, localNow)
	outputPath := filepath.Join(g.cfg.TempDir, fileName)
	partialPath := outputPath + ".part"
	outputFile, err := os.OpenFile(partialPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		// A nanosecond timestamp makes this extremely rare, but retry once with a
		// new clock value rather than overwriting any completed document.
		fileName = outputFileName(projectName, time.Now().Local())
		outputPath = filepath.Join(g.cfg.TempDir, fileName)
		partialPath = outputPath + ".part"
		outputFile, err = os.OpenFile(partialPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create output file: %w", err)
		}
	}

	committed := false
	defer func() {
		_ = outputFile.Close()
		if !committed {
			_ = os.Remove(partialPath)
		}
	}()

	counted := &countingWriter{writer: outputFile}
	writer := bufio.NewWriterSize(counted, g.bufferCap)
	formattedTime := generatedAt
	header := buildDocumentHeader(projectName, formattedTime)
	if _, err := writer.WriteString(header); err != nil {
		return nil, fmt.Errorf("write document header: %w", err)
	}

	tree := scanner.GenerateDirectoryTree(cleanPath, files)
	if _, err := fmt.Fprintf(writer, "## 1. DIRECTORY TREE\n```text\n%s```\n\n## 2. SOURCE CODE CONTENT\n\n<project_codebase>\n\n", tree); err != nil {
		return nil, fmt.Errorf("write directory tree: %w", err)
	}

	workCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()
	workers := g.workerCount(totalFiles)
	jobs := make(chan job, workers)
	results := make(chan result, workers)
	limiter := newMemoryLimiter(workerMemoryBudget)

	go g.produceJobs(workCtx, cleanPath, files, jobs, limiter)
	var workerGroup sync.WaitGroup
	for range workers {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			for currentJob := range jobs {
				processed := g.processFile(workCtx, currentJob, limiter)
				select {
				case results <- processed:
				case <-workCtx.Done():
					limiter.release(processed.memoryBytes)
					return
				}
			}
		}()
	}
	go func() {
		workerGroup.Wait()
		close(results)
	}()

	stats, pipelineErr := g.writeOrderedResults(workCtx, cancelWorkers, writer, results, limiter, totalFiles, startTime, events)
	if pipelineErr != nil {
		return nil, pipelineErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := emitProgress(ctx, events, ProgressEvent{
		Type:    "progress",
		Message: "Finalizing document…",
		Data:    progressData(98, totalFiles, totalFiles, float64(totalFiles)/math.Max(0.1, time.Since(startTime).Seconds())),
	}); err != nil {
		return nil, err
	}

	elapsed := math.Round(time.Since(startTime).Seconds()*100) / 100
	tokenLabel := fmt.Sprintf("%d tokens (o200k_base, exact)", stats.tokens)
	if tokenMode != "exact" {
		tokenLabel = fmt.Sprintf("~%d tokens (estimated)", stats.tokens)
	}
	bytesBeforeFooter := counted.bytes + int64(writer.Buffered())
	footer := fmt.Sprintf(
		"</project_codebase>\n\n<!-- SUMMARY: %d files, %d lines, %s, source bytes: %d, skipped: %d, unreadable: %d, time: %.2fs -->\n",
		totalFiles, stats.lines, tokenLabel, bytesBeforeFooter, stats.skipped, stats.failed, elapsed,
	)
	if _, err := writer.WriteString(footer); err != nil {
		return nil, fmt.Errorf("write document footer: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return nil, fmt.Errorf("flush output: %w", err)
	}
	if err := outputFile.Sync(); err != nil {
		return nil, fmt.Errorf("sync output: %w", err)
	}
	if err := outputFile.Close(); err != nil {
		return nil, fmt.Errorf("close output: %w", err)
	}
	if err := os.Rename(partialPath, outputPath); err != nil {
		return nil, fmt.Errorf("commit output file: %w", err)
	}
	committed = true

	fileInfo, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("inspect completed output: %w", err)
	}
	result := &GenerateResult{
		FileName:        fileName,
		FilePath:        outputPath,
		TotalFiles:      totalFiles,
		TotalLines:      stats.lines,
		TotalTokens:     stats.tokens,
		TokenMode:       tokenMode,
		Elapsed:         elapsed,
		SizeBytes:       fileInfo.Size(),
		Mode:            mode,
		GeneratedAt:     generatedAt,
		BinaryFiles:     stats.binary,
		SkippedFiles:    stats.skipped,
		UnreadableFiles: stats.failed,
	}

	if err := emitProgress(ctx, events, ProgressEvent{
		Type:    "complete",
		Message: fileName,
		Data: map[string]any{
			"total":            result.TotalFiles,
			"lines":            result.TotalLines,
			"tokens":           result.TotalTokens,
			"token_mode":       result.TokenMode,
			"size":             result.SizeBytes,
			"elapsed":          result.Elapsed,
			"file_name":        result.FileName,
			"generated_at":     result.GeneratedAt,
			"mode":             result.Mode,
			"binary_files":     result.BinaryFiles,
			"skipped_files":    result.SkippedFiles,
			"unreadable_files": result.UnreadableFiles,
		},
	}); err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}

	return result, nil
}

func (g *Generator) workerCount(totalFiles int) int {
	workers := g.cfg.Workers
	if workers < 1 {
		workers = 1
	}
	if workers > maxGeneratorWorkers {
		workers = maxGeneratorWorkers
	}
	if workers > totalFiles {
		workers = totalFiles
	}
	return workers
}

func (g *Generator) writeOrderedResults(
	ctx context.Context,
	cancel context.CancelFunc,
	writer *bufio.Writer,
	results <-chan result,
	limiter *memoryLimiter,
	totalFiles int,
	startTime time.Time,
	events chan<- ProgressEvent,
) (aggregateStats, error) {
	pending := make(map[int]result)
	nextIndex := 0
	processed := 0
	var stats aggregateStats
	var pipelineErr error

	for current := range results {
		processed++
		if pipelineErr != nil {
			limiter.release(current.memoryBytes)
			continue
		}
		pending[current.index] = current

		for {
			ordered, exists := pending[nextIndex]
			if !exists {
				break
			}
			delete(pending, nextIndex)
			if _, err := writer.Write(ordered.chunk); err != nil {
				pipelineErr = fmt.Errorf("write file %q: %w", ordered.rel, err)
				cancel()
			}
			stats.lines += ordered.lines
			stats.tokens += ordered.tokens
			stats.binary += boolInt(ordered.binary)
			stats.skipped += boolInt(ordered.skipped)
			stats.failed += boolInt(ordered.failed)
			limiter.release(ordered.memoryBytes)
			nextIndex++
			if pipelineErr != nil {
				break
			}
		}

		if pipelineErr == nil && (processed <= 5 || processed%5 == 0 || processed == totalFiles) {
			elapsed := time.Since(startTime).Seconds()
			speed := float64(0)
			if elapsed > 0 {
				speed = math.Round(float64(processed) / elapsed)
			}
			percent := 10 + int((float64(processed)/float64(totalFiles))*80)
			if err := emitProgress(ctx, events, ProgressEvent{
				Type:    "progress",
				Message: fmt.Sprintf("Processing %d/%d · %s", processed, totalFiles, current.rel),
				Data:    progressData(percent, processed, totalFiles, speed),
			}); err != nil {
				pipelineErr = err
				cancel()
			}
		}
	}

	for _, current := range pending {
		limiter.release(current.memoryBytes)
	}
	if pipelineErr != nil {
		return aggregateStats{}, pipelineErr
	}
	if err := ctx.Err(); err != nil {
		return aggregateStats{}, err
	}
	if nextIndex != totalFiles {
		return aggregateStats{}, fmt.Errorf("generation stopped after %d of %d files", nextIndex, totalFiles)
	}
	return stats, nil
}

func (g *Generator) produceJobs(ctx context.Context, root string, files []string, jobs chan<- job, limiter *memoryLimiter) {
	defer close(jobs)
	for index, path := range files {
		nativePath := filepath.Clean(filepath.FromSlash(path))
		rel, err := filepath.Rel(root, nativePath)
		if err != nil {
			rel = filepath.Base(path)
		}
		reservation := g.memoryReservation(nativePath)
		if err := limiter.acquire(ctx, reservation); err != nil {
			return
		}
		current := job{index: index, path: nativePath, rel: filepath.ToSlash(rel), memoryBytes: reservation}
		select {
		case jobs <- current:
		case <-ctx.Done():
			limiter.release(reservation)
			return
		}
	}
}

func (g *Generator) memoryReservation(path string) int64 {
	if g.sc.IsBinary(filepath.Ext(path)) {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || (g.cfg.MaxFileSize > 0 && info.Size() > g.cfg.MaxFileSize) {
		return 0
	}
	reservation := info.Size() * 3
	if reservation < minimumMemoryReservation {
		reservation = minimumMemoryReservation
	}
	return reservation
}

func emitProgress(ctx context.Context, events chan<- ProgressEvent, event ProgressEvent) error {
	if events == nil {
		return ctx.Err()
	}
	select {
	case events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func progressData(percent, current, total int, speed float64) map[string]any {
	return map[string]any{
		"percent": percent,
		"current": current,
		"total":   total,
		"speed":   speed,
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func buildDocumentHeader(projectName, generatedAt string) string {
	return fmt.Sprintf(`# DOCUMENTATION: %s
Generated: %s

## SYSTEM INSTRUCTION (Prompt)
You are an expert AI assistant. The following text contains the source context of a project.
1. **Structure**: Refer to the Directory Tree for file organization.
2. **Content**: Source code is wrapped in <file> elements with path attributes.
3. **Syntax**: Text is enclosed in CDATA sections to preserve source characters.
4. **Binary/Large files**: These remain visible in the tree but their content is excluded.

`, projectName, generatedAt)
}

type countingWriter struct {
	writer *os.File
	bytes  int64
}

func (writer *countingWriter) Write(data []byte) (int, error) {
	written, err := writer.writer.Write(data)
	writer.bytes += int64(written)
	return written, err
}
