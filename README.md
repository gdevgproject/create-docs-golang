# CodeDocs

CodeDocs is a local-first desktop app that turns a source tree into compact AI-ready context. It scans with `.gitignore` support, counts `o200k_base` tokens, generates ordered XML or text output, and keeps project history on the local machine.

## Highlights

- Native Windows window powered by WebView2; no installer or background service.
- Bounded concurrent scanning, token counting, and output generation.
- Streaming generation and range-based previews for large repositories.
- Local, atomic, recoverable bookmark history.
- Verified in-app updates with architecture checks and automatic rollback.
- Responsive three-pane workspace that collapses into drawers on small windows.

## Install on Windows

Download the matching binary from the [latest release](https://github.com/gdevgproject/create-docs-golang/releases/latest):

- `codedocs_windows_amd64.exe` for most Windows PCs.
- `codedocs_windows_arm64.exe` for native Windows on ARM.

Run the executable directly. Microsoft Edge WebView2 Runtime is included with current Windows 10/11 installations; the app shows a clear error if it must be installed or repaired.

Existing v1.7.8 installations can update in place. Release asset names and the legacy update API remain stable so older builds can move forward normally.

## Use

1. Choose or paste a project directory.
2. Select **Context** to include content or **Stats** for metadata only.
3. Generate, then copy, save, or download the result.

Bookmarks, preferences, tokenizer data, and generated output stay local. By default, the HTTP server binds only to `127.0.0.1` and rejects non-loopback browser requests.

## Command line

```text
codedocs.exe --port 8080 --host 127.0.0.1
```

Useful options:

| Option | Default | Purpose |
| --- | --- | --- |
| `--port` | `8080` | Preferred local port; the app can select another free port. |
| `--host` | `127.0.0.1` | HTTP bind address. Desktop mode requires loopback. |
| `--max-size` | `10485760` | Maximum source file size included in output. |
| `--workers` | CPU-aware | Bounded scan/generation concurrency. |
| `--temp-dir` | `./temp_docs` | Generated document directory. |
| `--cache-dir` | OS user cache | Tokenizer and WebView2 cache. |
| `--bookmark-file` | OS user config | Local history JSON file. |
| `--open-browser` | `true` | Open a browser on platforms without the native window. |

## Develop

The module declares the required Go toolchain in `go.mod`.

```bash
go test ./...
go vet ./...
go run ./cmd/codedocs
```

On Windows, build a versioned desktop executable with:

```powershell
./scripts/build-windows.ps1 -Version v1.8.0 -Architecture amd64
```

The script regenerates the architecture-specific icon, manifest, and version resource using pinned `go-winres` v0.3.3.

## Releases and updates

Tags matching `v*` run the release pipeline. Every release keeps the four historical asset names, publishes `SHA256SUMS.txt`, and creates GitHub build-provenance attestations. The updater additionally verifies GitHub's SHA-256 digest, expected size, executable format, OS, and CPU architecture before restart.

See [ARCHITECTURE.md](ARCHITECTURE.md), [UPGRADING.md](UPGRADING.md), and [CHANGELOG.md](CHANGELOG.md) before changing lifecycle, persistence, or update code.

## Docker

The container is an optional headless mode and intentionally listens on all container interfaces:

```bash
docker build --build-arg VERSION=dev -t codedocs .
docker run --rm -p 8080:8080 -v codedocs-data:/data codedocs
```

Open `http://127.0.0.1:8080`. Do not expose this local file-reading tool directly to an untrusted network.
