# CodePulse AI — Developer Guidelines & Agent Skill Manual

This document defines the core architectural rules, safety contracts, performance requirements, and testing standards for **CodePulse AI (Next-Gen AI Code Context Engine)**. All AI agents and developers modifying this codebase MUST follow these strict rules.

---

## 1. Real-Time SSE Progress Streaming Contract (`internal/generator`)

- **Worker Synchronization Rule**:
  NEVER place `wg.Wait()` before real-time progress events are emitted!
  Progress events MUST be emitted **inside** the worker goroutines as files are processed using atomic counters (`atomic.AddInt64(&processedCount, 1)`).
- **Multi-Phase Scaling**:
  - `0% - 8%`: Initial directory scanning & `.gitignore` parsing.
  - `10% - 90%`: Real-time atomic file reading & tokenization.
  - `95% - 100%`: Writing ordered output file to disk & final completion payload.
- **Tokenizer Background Warm-Up**:
  `go tok.Mode()` MUST be called in background during `NewServer` initialization so `o200k_base.tiktoken` ranks are pre-warmed in RAM before user triggers generation.

---

## 2. Windows GUI & Security Contract (`cmd/codedocs`)

- **Antivirus False Positive Safeguard**:
  DO NOT introduce raw, dynamic `user32.dll` or `dwmapi.dll` syscalls via `unsafe.Pointer` function calls.
  Windows Defender heuristic scanners flag Go executables using low-level dynamic window hijacking as false positives (`Trojan:Win32/Worefl`).
- **Native Webview2 GUI**:
  Windows GUI windows MUST use standard `webview2.New(false)` bindings.
  DWM Immersive Dark Titlebar MUST use official `DwmSetWindowAttribute` (`DWMWA_USE_IMMERSIVE_DARK_MODE` = 20).

---

## 3. Auto-Updater Atomic Swap & Rollback Safeguard (`internal/updater`)

- **Network Resilience**:
  HTTP Client for downloads MUST use explicit timeouts (`5 * time.Minute`) and set `User-Agent: CodePulse-Updater/vX.Y.Z`.
- **Payload Validation**:
  Before marking download state as `ready`, verify file size is > 1MB.
- **Atomic Rollback**:
  Update application by renaming `codedocs.exe` ➔ `codedocs.exe.old` and `codedocs.exe.new` ➔ `codedocs.exe`.
  If rename fails, immediately execute ROLLBACK: restore `codedocs.exe.old` ➔ `codedocs.exe`.
- **User-Controlled Check**:
  DO NOT add automatic continuous `setInterval` polling loops!
  Update check MUST be user-controlled (e.g. `🔄 Check Update` button) with `Cache-Control: no-cache` headers.

---

## 4. Multi-Ecosystem Exclusion Engine (`internal/scanner`, `internal/config`)

- **Automatic `.gitignore` Support**:
  `scanner.go` MUST parse local `.gitignore` rules in the project root directory.
- **Multi-Ecosystem Default Exclusions**:
  Maintain comprehensive exclusions across:
  - **JavaScript / Node / Next.js / Nest.js**: `node_modules`, `.next`, `.nuxt`, `.turbo`, `dist`, `build`.
  - **Java / Kotlin / Spring / Gradle**: `target`, `.gradle`, `build`, `.m2`, `.mvn`.
  - **Python**: `__pycache__`, `.venv`, `venv`, `env`, `.pytest_cache`, `.egg-info`.
  - **Go**: `vendor`, `bin`, `pkg`.
  - **C# / .NET**: `bin`, `obj`, `.vs`, `.nuget`.
  - **Game Engines / Steam / Minecraft**: `Library`, `Temp`, `Logs`, `Build`, `Builds`, `Intermediate`.

---

## 5. Verification & Testing Protocol

Before committing any code or releasing a version:
1. Run `go test ./...` and ensure ALL package tests pass.
2. Run `go vet ./...` and ensure ZERO static analysis warnings.
3. Build production binary: `go build -ldflags="-H=windowsgui -s -w -X codedocs/internal/config.Version=vX.Y.Z" -o codedocs.exe ./cmd/codedocs`.
