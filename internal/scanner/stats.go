package scanner

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const maxStatsWorkers = 16

type ProjectStats struct {
	Lines        int64 `json:"lines"`
	Bytes        int64 `json:"bytes"`
	TextFiles    int   `json:"text_files"`
	BinaryFiles  int   `json:"binary_files"`
	SkippedFiles int   `json:"skipped_files"`
	Unreadable   int   `json:"unreadable_files"`
}

type fileStats struct {
	lines      int64
	bytes      int64
	text       bool
	binary     bool
	skipped    bool
	unreadable bool
}

// CountProjectStats calculates preview statistics with bounded parallel I/O.
func (s *Scanner) CountProjectStats(ctx context.Context, files []string, maxFileSize int64, workers int) ProjectStats {
	if len(files) == 0 {
		return ProjectStats{}
	}
	if workers < 1 {
		workers = 1
	}
	if workers > maxStatsWorkers {
		workers = maxStatsWorkers
	}
	if workers > len(files) {
		workers = len(files)
	}

	jobs := make(chan string, workers)
	results := make(chan fileStats, workers)
	var workerGroup sync.WaitGroup
	for range workers {
		workerGroup.Add(1)
		go func() {
			defer workerGroup.Done()
			buffer := make([]byte, 64*1024)
			for path := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				results <- s.countFileStats(path, maxFileSize, buffer)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, path := range files {
			select {
			case jobs <- path:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workerGroup.Wait()
		close(results)
	}()

	var total ProjectStats
	for result := range results {
		total.Lines += result.lines
		total.Bytes += result.bytes
		if result.text {
			total.TextFiles++
		}
		if result.binary {
			total.BinaryFiles++
		}
		if result.skipped {
			total.SkippedFiles++
		}
		if result.unreadable {
			total.Unreadable++
		}
	}
	return total
}

func (s *Scanner) countFileStats(path string, maxFileSize int64, buffer []byte) fileStats {
	if s.IsBinary(filepath.Ext(path)) {
		return fileStats{binary: true}
	}

	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fileStats{unreadable: true}
	}
	if info.Size() == 0 {
		return fileStats{text: true}
	}
	if maxFileSize > 0 && info.Size() > maxFileSize {
		return fileStats{bytes: info.Size(), skipped: true}
	}

	file, err := os.Open(path)
	if err != nil {
		return fileStats{unreadable: true}
	}
	defer file.Close()

	result := fileStats{bytes: info.Size(), text: true, lines: 1}
	for {
		read, readErr := file.Read(buffer)
		if read > 0 {
			result.lines += int64(bytes.Count(buffer[:read], []byte{'\n'}))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			result.text = false
			result.unreadable = true
			result.lines = 0
			break
		}
	}
	return result
}

// CountProjectLinesFast preserves the original API while using the bounded
// parallel statistics implementation.
func (s *Scanner) CountProjectLinesFast(files []string, maxFileSize int64) int64 {
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	return s.CountProjectStats(context.Background(), files, maxFileSize, workers).Lines
}
