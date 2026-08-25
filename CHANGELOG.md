# Changelog

All notable changes are documented here. CodeDocs follows semantic versioning.

## [Unreleased]

## [1.8.3] - 2026-08-26

### Changed

- Redesigned confirmation and prompt dialogs with compact adaptive sizing, clean bottom action buttons (Cancel / Save / Delete), and removed empty vertical whitespace.

## [1.8.2] - 2026-08-25

### Changed

- Highlighted token count badge in timeline history with distinctive border, contrast colors, and hover styling for quick metric visibility.

## [1.8.1] - 2026-08-13

### Changed

- Increased desktop typography, compact control labels, editor text, and action icon scale for clearer Windows readability.

## [1.8.0] - 2026-08-13

### Added

- Verified, architecture-aware updates with background state, startup handshake, rollback, and legacy API compatibility.
- Responsive desktop workspace with compact drawers, accessible dialogs, persistent UI preferences, and release notes.
- Windows amd64 and arm64 icon, DPI-aware manifest, long-path support, and executable version metadata.
- Release checksums and GitHub build-provenance attestations.

### Changed

- Refactored process ownership around a shared cancellation context and graceful HTTP shutdown.
- Added scoped nested `.gitignore` handling and bounded parallel project statistics.
- Streamed ordered document generation to atomic files with bounded memory.
- Replaced the unbounded token memoization map with a bounded collision-safe LRU.
- Split bookmark persistence into focused storage/history modules with atomic recovery.
- Added ranged content previews, download ranges, request limits, and a consolidated status endpoint.
- Replaced the 3-second UI heartbeat and full-document startup load with event-driven, cancellable requests.

### Fixed

- Exact release asset selection for the running OS and architecture.
- Binary integrity, executable format, and release ownership validation before applying an update.
- Shutdown races, duplicate saved paths, corrupt history handling, large-preview memory pressure, and titlebar icon ID mismatch.

[Unreleased]: https://github.com/gdevgproject/create-docs-golang/compare/v1.8.3...HEAD
[1.8.3]: https://github.com/gdevgproject/create-docs-golang/compare/v1.8.2...v1.8.3
[1.8.2]: https://github.com/gdevgproject/create-docs-golang/compare/v1.8.1...v1.8.2
[1.8.1]: https://github.com/gdevgproject/create-docs-golang/compare/v1.8.0...v1.8.1
[1.8.0]: https://github.com/gdevgproject/create-docs-golang/compare/v1.7.8...v1.8.0
