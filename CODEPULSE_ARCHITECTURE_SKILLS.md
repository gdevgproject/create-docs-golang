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

## 2. Text Extraction & Encoding Safety Contract (`internal/generator`)

- **Automatic BOM & Multi-Encoding Decoding**:
  `CleanAndValidateText` MUST detect and decode:
  - UTF-8 BOM (`\xEF\xBB\xBF`) ➔ Stripped cleanly.
  - UTF-16 LE BOM (`\xFF\xFE`) ➔ Decoded to valid UTF-8 string text.
  - UTF-16 BE BOM (`\xFE\xFF`) ➔ Decoded to valid UTF-8 string text.
- **Embedded Null-Byte Binary Safeguard (`\x00`)**:
  If a file contains embedded null bytes `\x00` in the first 1024 bytes (even if named `.go`, `.ts`, `.txt`, `.dat`), it MUST be treated as a binary file (`isText = false`) and safely excluded (`[BINARY/MEDIA FILE - CONTENT EXCLUDED]`).
- **Gibberish Control Code Sanitization**:
  Use `strings.ToValidUTF8` and filter non-printable control characters (`\x00` - `\x1F` except `\n`, `\r`, `\t`) to prevent unprintable gibberish (`#fx#'`) from polluting Tiktoken BPE calculations or XML prompts.

---

## 3. Windows GUI & Security Contract (`cmd/codedocs`)

- **Antivirus False Positive Safeguard**:
  DO NOT introduce raw, dynamic `user32.dll` or `dwmapi.dll` syscalls via `unsafe.Pointer` function calls.
  Windows Defender heuristic scanners flag Go executables using low-level dynamic window hijacking as false positives (`Trojan:Win32/Worefl`).
- **Native Webview2 GUI**:
  Windows GUI windows MUST use standard `webview2.New(false)` bindings.
  DWM Immersive Dark Titlebar MUST use official `DwmSetWindowAttribute` (`DWMWA_USE_IMMERSIVE_DARK_MODE` = 20).

---

## 4. Auto-Updater Atomic Swap & Rollback Safeguard (`internal/updater`)

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

## 5. Multi-Ecosystem Exclusion Engine (`internal/scanner`, `internal/config`)

- **Automatic `.gitignore` Support**:
  `scanner.go` MUST parse local `.gitignore` rules in the project root directory.
- **Multi-Ecosystem Default Exclusions**:
  Maintain comprehensive exclusions across:
  - **JavaScript / Node / Next.js / Bun / React**: `node_modules`, `.next`, `.nuxt`, `.turbo`, `.bun`, `.vercel`, `.swc`, `dist`, `build`, `bun.lockb`, `bun.lock`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`.
  - **Java / Kotlin / Spring / Gradle**: `target`, `.gradle`, `build`, `.m2`, `.mvn`.
  - **Python**: `__pycache__`, `.venv`, `venv`, `env`, `.pytest_cache`, `.egg-info`, `poetry.lock`.
  - **Go**: `vendor`, `bin`, `pkg`.
  - **C# / .NET**: `bin`, `obj`, `.vs`, `.nuget`.
  - **Game Clients & Game Engines (Genshin Impact, GTA V, Steam, Unity, Unreal)**: `Library`, `Temp`, `Logs`, `Builds`, `Intermediate`, `GenshinImpact_Data`, `Genshin Impact Game`, `GTA V`, `SteamLibrary`, `steamapps`, `ClientData`, `.rpf`, `.pck`, `.bik`, `.bik2`, `.bk2`, `.uasset`, `.unity3d`, `.bundle`.

---

## 6. Bookmark Management & Last Result Persistence (`internal/bookmarks`)

- **In-Place Rename & Reorder**:
  Bookmarks support `PUT /api/bookmarks` for both label renaming (`{"id": "...", "note": "..."}`) and reordering (`{"action": "reorder", "ordered_ids": [...]}`).
- **Order Attribute Persistence**:
  Bookmarks MUST be sorted by `Order` attribute (`(a.order ?? 0) - (b.order ?? 0)`) so custom user reordering is strictly preserved upon application restart.
- **Automatic Last Generation Stats Caching**:
  When generation finishes, if project path matches a saved bookmark, `SaveLastResult` MUST automatically attach and persist `LastResult` (`total`, `lines`, `tokens`, `size`, `elapsed`, `generated_at`, `file_name`) into `saved_paths.json`. Clicking a bookmark restores stats cards and action buttons instantly.

---

## 7. Verification & Testing Protocol

Before committing any code or releasing a version:
1. Run `go test -count=1 ./...` and ensure ALL package tests pass.
2. Run `go vet ./...` and ensure ZERO static analysis warnings.
3. Build production binary: `go build -ldflags="-H=windowsgui -s -w -X codedocs/internal/config.Version=vX.Y.Z" -o codedocs.exe ./cmd/codedocs`.
