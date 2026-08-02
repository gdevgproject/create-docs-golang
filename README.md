# Codebase-to-Docs Generator (`codedocs`) — Go Production Rewrite

> A high-performance, single-binary codebase documentation generator written in **Golang**. Automatically scans your codebase, formats source code with XML CDATA enclosures, counts exact GPT tokens (`o200k_base` tiktoken BPE), and streams real-time progress to a sleek developer web UI.

---

## ✨ Features & Enhancements

- 🚀 **Single Compiled Binary**: 100% self-contained application with embedded frontend web assets (`//go:embed`). No PHP, Node.js, Python, or runtime dependencies required.
- ⚡ **Concurrent Worker Pool**: Multi-threaded parallel file processing using Go worker pools (`runtime.NumCPU()`), delivering **5x - 12x faster performance** than PHP.
- 🔢 **Exact `o200k_base` Tiktoken**: Automatic download and SHA-256 validation of official OpenAI BPE vocabulary (`o200k_base` for GPT-4o / GPT-5.x) with thread-safe chunk memoization and offline heuristic fallback.
- 📡 **Real-Time Streaming (SSE)**: Live progress updates, files/sec speed, logs, and completion statistics sent over Server-Sent Events (`http.Flusher`).
- 🌳 **ASCII Directory Tree**: Instant preview of project structure matching Unix `tree` formatting.
- 🎨 **Modern Developer UI**: Premium dark developer theme interface with responsive layout, stat cards, bookmark management, modal preview, and one-click copy/download actions.
- 💾 **Thread-safe Bookmarks**: Persist frequently accessed project paths locally.

---

## 💻 System Requirements

- **Running Precompiled Binary**: Zero dependencies. Works out-of-the-box on Windows, macOS, and Linux.
- **Building from Source**: Go `1.22+` (Tested on Go `1.26`).

---

## 🛠️ Building from Source

### Quick Build
```bash
go build -ldflags="-s -w" -o codedocs ./cmd/codedocs
```

### Using Makefile
```bash
make build       # Build binary for your host OS
make test        # Run unit tests
make vet         # Run static analysis
make build-all   # Cross-compile for Windows, macOS, Linux (amd64 + arm64)
```

---

## 🌐 Cross-Compilation Matrix

You can compile standalone binaries for any OS/Architecture using standard Go environment variables:

| Target OS | Target Architecture | Cross-Compilation Command |
| :--- | :--- | :--- |
| **Windows** | 64-bit (x86_64) | `GOOS=windows GOARCH=amd64 go build -o codedocs_win.exe ./cmd/codedocs` |
| **Windows** | ARM64 | `GOOS=windows GOARCH=arm64 go build -o codedocs_win_arm.exe ./cmd/codedocs` |
| **macOS** | Apple Silicon (M1/M2/M3) | `GOOS=darwin GOARCH=arm64 go build -o codedocs_mac_arm ./cmd/codedocs` |
| **macOS** | Intel 64-bit | `GOOS=darwin GOARCH=amd64 go build -o codedocs_mac_intel ./cmd/codedocs` |
| **Linux** | 64-bit (x86_64) | `GOOS=linux GOARCH=amd64 go build -o codedocs_linux_amd64 ./cmd/codedocs` |
| **Linux** | ARM64 | `GOOS=linux GOARCH=arm64 go build -o codedocs_linux_arm64 ./cmd/codedocs` |

---

## 🚀 Running the Application

Launch the server by running the compiled binary:

```bash
./codedocs --port 8080
```

Then open your browser and navigate to:
```
http://localhost:8080
```

### ⚙️ Command-Line Flags Reference

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--port` | `8080` | Port for the HTTP server to listen on |
| `--host` | `0.0.0.0` | Host IP address binding |
| `--workers` | `runtime.NumCPU()` | Number of parallel worker threads |
| `--max-size` | `10485760` (10MB) | Max file size in bytes to include full content |
| `--temp-dir` | `./temp_docs` | Directory where generated `.md` docs are saved |
| `--cache-dir` | OS User Cache Dir | Directory used to cache `o200k_base.tiktoken` vocab |
| `--bookmark-file` | OS User Config Dir | Path to `saved_paths.json` for bookmarks |

---

## 🐳 Docker Deployment

### Build Container
```bash
docker build -t codedocs:latest .
```

### Run Container
```bash
docker run -d -p 8080:8080 -v /var/projects:/app/projects codedocs:latest
```

---

## 📊 Feature Parity Comparison (PHP vs Go)

| Feature | PHP Original (`index.php`) | Go Rewrite (`codedocs`) |
| :--- | :---: | :---: |
| Single Binary Execution | ❌ Requires PHP Runtime | ✅ **Single Compiled Binary** |
| Multithreaded Worker Pool | ❌ Single-threaded | ✅ **Parallel Worker Pool (`NumCPU`)** |
| `o200k_base` Tiktoken BPE | ✅ (Pure PHP) | ✅ **Native Go BPE + SHA-256 Cache** |
| Chunk Memoization Cache | ✅ | ✅ **Thread-safe `sync.Map` Cache** |
| Offline Fallback Estimator | ✅ | ✅ **`exact` / `estimate` Mode Indicator** |
| Server-Sent Events (SSE) | ✅ | ✅ **Go `http.Flusher` Streaming** |
| Stats Only / Full Content Mode | ✅ | ✅ **Supported** |
| Project Bookmarks | ✅ (`saved_paths.json`) | ✅ **Thread-safe JSON Persistence** |
| UI Design | Basic CSS | ✅ **Modern Dark Developer Theme** |
| Automated Unit Tests | ❌ None | ✅ **Full `go test` Suite** |
