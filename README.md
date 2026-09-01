# Media Audiobooks

Audiobook library manager for MuxCore.

Exposes `muxcore.audiobooks.v1.AudiobookManagementService` with SQLite persistence and offline library-root scanning (path-derived metadata only; no paid APIs).

## Ports

| Service | Default |
|---------|---------|
| gRPC | `:9670` |
| HTTP | `:9671` (`/healthz` plus JSON API below) |

## Data layout

| Path | Purpose |
|------|---------|
| `$AUDIOBOOKS_DATA_DIR/audiobooks.db` | SQLite catalog |
| `$AUDIOBOOKS_LIBRARY_DIR` | Scanned audio tree (`Author/Title/file.ext`) |

When `AUDIOBOOKS_LIBRARY_DIR` is unset it defaults to `AUDIOBOOKS_DATA_DIR` (not a nested `audiobooks/` subfolder). On vault, set `AUDIOBOOKS_DATA_DIR=$DATA/audiobooks` and either point `AUDIOBOOKS_LIBRARY_DIR` at the same path or at your NAS audiobook root.

## Env

| Variable | Default |
|----------|---------|
| `AUDIOBOOKS_DATA_DIR` | `./data` (SQLite at `audiobooks.db`) |
| `AUDIOBOOKS_LIBRARY_DIR` | same as `AUDIOBOOKS_DATA_DIR` |

## HTTP API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/authors` | List authors (`?q=` filter) |
| GET | `/api/authors/{id}` | Author detail + audiobooks |
| GET | `/api/audiobooks` | List audiobooks (`?author_id=`) with `files[]` and `stream_url` |
| GET | `/api/audiobooks/{id}` | Audiobook detail + author + `files[]` |
| POST | `/api/audiobooks/{id}/import` | Attach on-disk file under library root (`{"path":"..."}`) |
| GET | `/api/missing` | Monitored titles with no on-disk files (`?page=&page_size=`) |
| POST | `/api/scan` | Rescan library root into SQLite |
| GET | `/api/files/{id}/stream` | Stream audio (Range supported; path must stay under library root) |

## gRPC

`ScanLibrary`, `ListMissing`, `ListAudiobookFiles`, `ImportAudiobookFile`, `UpdateAuthor`, `UpdateAudiobook`, `GetAudiobook`, and `RemoveAudiobook` are available on `AudiobookManagementService`. Proto: `proto/muxcore/audiobooks/v1/audiobooks.proto`.

## Build / test

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build -o bin/media-audiobooks ./cmd/module
```

## Status

v0.2.0 — SQLite library, startup/rescan import, missing list, fixture import, and HTTP streaming for consumer UIs.
