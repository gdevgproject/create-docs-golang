# CodeDocs architecture

This document records the contracts that keep the released desktop app safe to evolve.

## Package boundaries

| Area | Responsibility |
| --- | --- |
| `cmd/codedocs` | Process lifecycle, listener selection, native window, graceful shutdown, update-helper bootstrap. |
| `internal/api` | Loopback HTTP boundary, request limits, SSE progress, generated-file access. |
| `internal/scanner` | Cancellable traversal, scoped `.gitignore` rules, bounded project statistics. |
| `internal/generator` | Ordered worker pipeline, sanitization, bounded-memory atomic output. |
| `internal/tokenizer` | Exact `o200k_base` encoding, bounded LRU, verified vocabulary cache, offline fallback. |
| `internal/bookmarks` | Atomic local history, backup recovery, import/merge, path identity. |
| `internal/updater` | Release selection, download verification, helper swap, startup handshake, rollback. |
| `web` | Embedded responsive UI and thin API clients. Business rules stay in Go. |

Dependencies point inward: the entrypoint composes packages, while core packages do not depend on the UI.

## Lifecycle

1. `main` handles private updater-helper mode before parsing public flags.
2. The app creates a signal-aware context and a loopback listener.
3. The HTTP server starts before the WebView2 window navigates.
4. A successful native-window/server startup completes any update handshake.
5. Closing the window, receiving an OS signal, or applying an update cancels the shared context.
6. The server drains with a bounded graceful-shutdown timeout.

Do not reintroduce direct `os.Exit` calls inside handlers or window callbacks. Only `main` owns the final process exit code.

## Update compatibility contract

Released v1.7.8 clients depend on these exact endpoints:

- `GET /api/check-update`
- `POST /api/download-update`
- `GET /api/update-progress`
- `POST /api/apply-update`

They also discover Windows binaries by these exact names:

- `codedocs_windows_amd64.exe`
- `codedocs_windows_arm64.exe`

Never remove or rename those endpoints, legacy JSON fields, or assets in a normal release. Add fields and states compatibly. Windows amd64 must remain the first Windows release asset because the oldest updater did not distinguish CPU architecture; Windows on ARM can execute it through x64 emulation and later follows the matching architecture selected by the new updater.

The current updater accepts only HTTPS assets owned by the configured GitHub repository. A download must match the release size and GitHub SHA-256 digest, then pass PE/ELF/Mach-O OS and architecture validation. The helper swaps adjacent files only after the parent exits, and rolls back unless the new process writes its startup-ready token.

## Performance and state rules

- Every filesystem walk and long operation must accept cancellation.
- Worker counts and queues must be bounded; preserve output ordering separately from work completion.
- Stream large output to a temporary file and atomically rename it only on success.
- Preview endpoints are ranged; the legacy full-content response stays available for compatibility.
- Token cache entries are bounded and keyed without hash-collision ambiguity.
- Persistent JSON writes use a same-directory temporary file, flush, atomic replacement, and backup recovery.
- Never silently replace corrupt user state with an empty state.

## HTTP boundary

Desktop mode is loopback-only. The server validates origin/fetch metadata, emits restrictive browser security headers, caps JSON bodies, and resolves generated-file paths beneath the configured output directory. New endpoints should follow the same method-specific routing and request-limit helpers.

## Windows resources

`winres/winres.json` is the source of truth. Generated `.syso` files live only in `cmd/codedocs`, one for each released Windows architecture. The manifest enables per-monitor DPI v2, long paths, modern common controls, high-resolution scrolling, and segment heap. Icon group `#1` must stay aligned with `appIconResourceID`.

Release builds replace the source version in the resource file and also set `internal/config.Version` through linker flags. Keep those versions aligned.

## Verification

Before a release:

```bash
go test -race -count=1 ./...
go vet ./...
```

Also cross-build Windows amd64 and arm64, inspect Windows FileVersion metadata, exercise updater selection against the real GitHub release API, and run the opt-in Windows helper end-to-end test. UI changes require desktop viewport checks at wide, compact, and drawer breakpoints with no console errors.
