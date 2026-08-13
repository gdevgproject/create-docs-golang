# Upgrading CodeDocs

## From v1.7.8

Use the in-app update action as usual. v1.8.0 deliberately retains the legacy update endpoints, response fields, and Windows asset names. The old updater downloads the new executable; after that first transition, the hardened updater verifies release digests and supports rollback.

If automatic update is unavailable, download the matching executable from the latest GitHub release, close CodeDocs, and replace the old file. Local bookmarks and tokenizer caches live in the OS user config/cache directories and are not removed with the executable.

## Local data

The v1.8 storage layer reads the existing `saved_paths.json` shape. Its first successful write adds safer atomic replacement and a `.bak` recovery copy. No manual migration is required.

If the primary file is corrupt but the backup is valid, CodeDocs restores it automatically. If both are corrupt, the app reports the error instead of silently erasing history.

## Generated documents

Output remains compatible with previous XML/text consumers. Generation now writes a `.part` file and publishes the final document only after all ordered work succeeds. Temporary interrupted files can be removed safely while CodeDocs is closed.

## Rollback behavior

During an in-app update the adjacent files can briefly include:

- `<app>.codedocs-new`
- `<app>.codedocs-old`
- `<app>.codedocs-update.json`
- a uniquely named updater helper executable

The helper restores the old executable when the replacement does not start and confirm readiness within the deadline. A normal later launch removes exact stale updater artifacts; it does not glob or delete unrelated files.

## Release maintainers

Do not publish a release unless both historical Windows asset names are present and each binary is larger than 1 MB. This preserves discovery by v1.7.8. Publish amd64 before arm64 in the release asset list, retain all four legacy platform filenames, and verify `SHA256SUMS.txt` after upload.
