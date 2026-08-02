package generator_test

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codedocs/internal/config"
	"codedocs/internal/generator"
	"codedocs/internal/scanner"
	"codedocs/internal/tokenizer"
)

func TestMicroBenchmarkBreakdown(t *testing.T) {
	testPath := "D:/projects/heygoru"
	if _, err := os.Stat(testPath); err != nil {
		t.Skip("Test directory D:/projects/heygoru not found")
	}

	cfg := config.DefaultConfig()
	sc := scanner.NewScanner()
	tok := tokenizer.NewTokenizer(cfg.CacheDir)

	files, err := sc.ScanProjectFiles(testPath)
	if err != nil || len(files) == 0 {
		t.Fatalf("Scan error: %v", err)
	}

	totalFiles := len(files)

	var ioDurNs int64
	var sanitizeDurNs int64
	var tokenizeDurNs int64

	var wg sync.WaitGroup
	workers := cfg.Workers

	jobsChan := make(chan string, totalFiles)
	for _, f := range files {
		jobsChan <- f
	}
	close(jobsChan)

	t0 := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobsChan {
				// Measure Sub-op A: Disk I/O Read
				tRead := time.Now()
				contentBytes, err := os.ReadFile(path)
				atomic.AddInt64(&ioDurNs, time.Since(tRead).Nanoseconds())
				if err != nil {
					continue
				}

				content := string(contentBytes)

				// Measure Sub-op B: Token Counting
				tTok := time.Now()
				_ = tok.CountTokens(content)
				atomic.AddInt64(&tokenizeDurNs, time.Since(tTok).Nanoseconds())

				// Measure Sub-op C: Sanitization & Escaping
				tSan := time.Now()
				s := generator.SanitizeContent(content)
				_ = generator.EscapeCDATA(s)
				atomic.AddInt64(&sanitizeDurNs, time.Since(tSan).Nanoseconds())
			}
		}()
	}

	wg.Wait()
	totalWallDur := time.Since(t0)

	totalCpuNs := ioDurNs + tokenizeDurNs + sanitizeDurNs

	fmt.Printf("\n==================== MICRO-BENCHMARK BREAKDOWN ====================\n")
	fmt.Printf("📦 Files Processed:  %d files\n", totalFiles)
	fmt.Printf("⏱️  Total Wall Time:  %.3f ms (across %d CPU workers)\n", float64(totalWallDur.Microseconds())/1000.0, workers)
	fmt.Printf("------------------------------------------------------------------\n")
	fmt.Printf("1. 🧠 Tokenization (o200k_base BPE): %.3f ms (%.1f%% of CPU time)\n", float64(tokenizeDurNs)/1e6, (float64(tokenizeDurNs)/float64(totalCpuNs))*100.0)
	fmt.Printf("2. 🧹 Sanitization (LF + CDATA):     %.3f ms (%.1f%% of CPU time)\n", float64(sanitizeDurNs)/1e6, (float64(sanitizeDurNs)/float64(totalCpuNs))*100.0)
	fmt.Printf("3. 💾 Disk File Read (os.ReadFile):  %.3f ms (%.1f%% of CPU time)\n", float64(ioDurNs)/1e6, (float64(ioDurNs)/float64(totalCpuNs))*100.0)
	fmt.Printf("==================================================================\n\n")
}
