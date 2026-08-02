# CodePulse AI — Next-Gen AI Code Context Engine

> A high-performance, single-binary codebase documentation & AI context generator written in **Golang**. Automatically scans your codebase, formats source code with XML CDATA enclosures, counts exact GPT tokens (`o200k_base` tiktoken BPE), and streams real-time progress to a sleek developer web UI with native Win32 Immersive Dark Mode.

---

## ✨ Key Features & Architectural Highlights

- ⚡ **Extreme Golang Concurrency**: Multi-threaded parallel file processing using Go worker pools with OS CPU reservation (`runtime.NumCPU() - 1`), delivering **1,500+ files/sec throughput** without freezing OS responsiveness.
- 🎨 **100% Native Win32 Immersive Dark Mode**: Windows desktop window uses DWM dark titlebar API to seamlessly match the sleek developer theme.
- 🧮 **Instant Token Counter Tool**: Built-in interactive modal to paste any text, prompt, or XML snippet for instant exact `o200k_base` BPE token calculation.
- 🌐 **Universal Multi-Ecosystem Exclusion Engine**: Out-of-the-box support for Node.js/Next/React, Java/Spring/Kotlin, Python, Go, C#/.NET, PHP/Laravel, Rust, DevOps, and Game Engines (Unity, Unreal Engine, Steam Modding). Automatically parses project `.gitignore` rules.
- 🔄 **Non-Blocking In-Place Auto-Update System**: One-click GitHub Releases update downloading in the background with progress indicator and atomic 1-click application restart.
- 📄 **High-Performance Non-Blocking Preview**: Instant 300KB fast preview with zero UI lag, controllable async chunk rendering, and instant **"⏸ Stop Rendering"** cancellation.
- 💾 **Thread-Safe Project Bookmarks**: Persist frequently scanned project paths locally.

---

## 💻 System Requirements

- **Precompiled Binary**: 100% self-contained standalone `.exe` / binary. Zero runtime dependencies.
- **Building from Source**: Go `1.22+` (Tested on Go `1.26`).

---

## 🛠️ Building from Source

### Quick Build (Windows Desktop GUI)
```bash
go build -ldflags="-H=windowsgui -s -w -X codedocs/internal/config.Version=v1.4.0" -o CodePulse.exe ./cmd/codedocs
```

### Using Makefile
```bash
make build       # Build binary for host OS
make test        # Run unit test suite
make vet         # Run static analysis
make build-all   # Cross-compile for Windows, macOS, Linux (amd64 + arm64)
```

---

## 🚀 Command-Line Flags Reference

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--port` | `8080` | Port for the HTTP server to listen on |
| `--host` | `0.0.0.0` | Host IP address binding |
| `--workers` | `NumCPU - 1` | Worker pool size (preserves 1 CPU core for OS) |
| `--max-size` | `10485760` (10MB) | Max file size in bytes to include content |
| `--temp-dir` | `./temp_docs` | Directory where generated `.md` docs are saved |
| `--cache-dir` | OS User Cache Dir | Directory used to cache `o200k_base.tiktoken` vocab |
| `--bookmark-file` | OS User Config Dir | Path to `saved_paths.json` for bookmarks |

---

## 📊 Feature Comparison

| Feature | Legacy PHP Scripts | CodePulse AI (Go Rewrite) |
| :--- | :---: | :---: |
| Execution Architecture | ❌ PHP CLI Runtime required | ✅ **Single Compiled Standalone Binary** |
| Multithreaded Worker Pool | ❌ Single-threaded | ✅ **Parallel Workers + CPU Safety** |
| `o200k_base` Tiktoken BPE | ✅ (Pure PHP) | ✅ **Native Go BPE + FNV-1a Hash Cache** |
| Native Win32 Titlebar | ❌ White Browser Frame | ✅ **DWM Immersive Dark Titlebar** |
| In-Place Auto Update | ❌ Manual download | ✅ **Background Download & 1-Click Swap** |
| Universal Exclusion Engine | ❌ Basic array | ✅ **All Stacks + Game Engines + `.gitignore`** |
| Instant Token Counter | ❌ None | ✅ **Interactive Paste Token Calculator** |
| Stop/Cancel Async Rendering | ❌ Infinite Freeze | ✅ **0ms Instant Cancellation Control** |
