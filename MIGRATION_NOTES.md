# Migration Notes: PHP `index.php` to Golang `codedocs`

This document summarizes the technical architecture, performance improvements, and feature verification results of porting the Codebase-to-Docs Generator from single-file PHP to Golang.

---

## 🏛️ Architectural Changes

```
PHP Original (index.php)               Golang Rewrite (codedocs)
┌──────────────────────────┐           ┌────────────────────────────────────────┐
│  Single PHP File         │           │  cmd/codedocs/main.go (CLI Entrypoint) │
│  - Web UI (HTML/JS)      │           │  internal/config (Exclusions & Flags) │
│  - SSE Handlers          │  ──────>  │  internal/scanner (File Walk & Tree)   │
│  - Tokenizer             │           │  internal/tokenizer (o200k_base BPE)   │
│  - File Scanner          │           │  internal/generator (Worker Pool)      │
└──────────────────────────┘           │  internal/bookmarks (JSON Storage)     │
                                       │  internal/api (HTTP & SSE Handlers)    │
                                       │  web/ (Embedded Dark Theme HTML/CSS/JS)│
                                       └────────────────────────────────────────┘
```

---

## ⚡ Performance Improvements

1. **Concurrency**: The PHP version processed files sequentially in a single process loop. The Go version utilizes a **Goroutine Worker Pool** (`runtime.NumCPU()`), processing multiple files in parallel across CPU cores.
2. **Memory Overhead**: Go uses `bufio.Writer` with a fixed 512KB buffer, streaming generated output directly to disk rather than accumulating massive strings in RAM.
3. **Single-Pass I/O**: Files are read once to simultaneously compute line count, exact token count, CDATA formatting, and size metrics.
4. **Binary Compilation**: Native machine binary execution eliminates interpreter startup overhead.

---

## ✅ Feature Parity Audit Matrix

| Checklist Item | PHP Original | Go Implementation | Audit Status |
| :--- | :---: | :---: | :---: |
| Recursive Directory Scanning | Yes | `internal/scanner/scanner.go` | PASS |
| Exact Exclusions (`dirs`, `files`, `extensions`) | Yes | `internal/config/config.go` | PASS |
| ASCII Directory Tree (`tree` format) | Yes | `internal/scanner/tree.go` | PASS |
| System Instructions Header | Yes | `internal/generator/generator.go` | PASS |
| XML `<file>` and `<![CDATA[ ... ]]>` formatting | Yes | `internal/generator/generator.go` | PASS |
| Sanitize Content (LF normalization, line trimming) | Yes | `internal/generator/sanitizer.go` | PASS |
| CDATA escaping (`]]]]><![CDATA[>`) | Yes | `internal/generator/sanitizer.go` | PASS |
| Single-Pass Lines & Token Counting | Yes | `internal/generator/generator.go` | PASS |
| `o200k_base` Tiktoken BPE | Yes | `internal/tokenizer/tokenizer.go` | PASS |
| SHA-256 Checksum Validation | Yes | `internal/tokenizer/tokenizer.go` | PASS |
| Vocab Local Cache in OS User Dir | Yes | `internal/tokenizer/tokenizer.go` | PASS |
| Memoization Cache for BPE Chunks | Yes | `internal/tokenizer/tokenizer.go` (`sync.Map`) | PASS |
| Offline Heuristic Fallback | Yes | `internal/tokenizer/tokenizer.go` | PASS |
| Token Mode Indicator (`exact` / `estimate`) | Yes | `internal/api/handlers.go` | PASS |
| Full Content vs Stats Only Mode | Yes | `internal/generator/generator.go` | PASS |
| Real-time SSE Progress Streaming | Yes | `internal/api/handlers.go` (`http.Flusher`) | PASS |
| Preview Structure Endpoint (`/api/structure`) | Yes | `internal/api/handlers.go` | PASS |
| Exclusions List Endpoint (`/api/exclusions`) | Yes | `internal/api/handlers.go` | PASS |
| Download Endpoint (`/api/download`) | Yes | `internal/api/handlers.go` | PASS |
| Load Content Endpoint (`/api/content`) | Yes | `internal/api/handlers.go` | PASS |
| Bookmarks Manager (GET, POST, DELETE) | Yes | `internal/bookmarks/bookmarks.go` | PASS |
| Max File Size Limit (10MB) | Yes | `internal/config/config.go` | PASS |
| Embedded Web UI | Basic | `web/` (Dark Developer Theme) | PASS |
