# Changelog

## [Unreleased]

### Security
- Library file paths are confined with pathguard, including symlink targets (NFR-SEC-008).

## [0.2.3] - 2026-10-05

### Changed
- Built on core v0.6.14 / sdk/go/module v0.6.4: unregisters on shutdown and re-registers after core restarts (ADR-0022).

## [0.2.2] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.2.0] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [v0.2.0] — 2026-08-31

### Added
- `ScanLibrary` gRPC + `POST /api/scan`; startup library scan on `Module.Start`
- `UpdateAuthor` / `UpdateAudiobook`, `GetAudiobook` / `RemoveAudiobook`
- `ListAudiobookFiles`, `ListMissing`, `ImportAudiobookFile` gRPC + HTTP import
- `GET /api/audiobooks/{id}`, `GET /api/files/{id}/stream` with `files` + `stream_url` on list
- Missing-on-disk detection (purge vanished files on scan; list treats ghost files as missing)
- `delete_files` honored on `RemoveAuthor` / `RemoveAudiobook` (library-root only)
- gRPC integration tests; fixture stubs under `internal/testdata/library`

### Changed
- Default `AUDIOBOOKS_LIBRARY_DIR` to `AUDIOBOOKS_DATA_DIR` (no nested `audiobooks/` subdir)
- `Module.Info().HTTPAddr` reports HTTP listen address
- `Health()` pings SQLite; `PRAGMA foreign_keys=ON` on store open
- `muxcore.json` version 0.2.0

## [v0.1.0] — 2026-08-10

### Added
- `AudiobookManagementService` (authors/audiobooks CRUD)
- SQLite library store
- SettingsProvider (`library_dir`)
- Health `:9671` with `GET /api/authors`, `GET /api/authors/{id}`, `GET /api/audiobooks`, `GET /api/missing`
