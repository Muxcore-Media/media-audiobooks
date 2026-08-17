# Media Audiobooks

Audiobook library manager for MuxCore.

Exposes `muxcore.audiobooks.v1.AudiobookManagementService` (authors/audiobooks) with SQLite persistence and offline library-root scanning (path-derived metadata only; no paid APIs).

## Ports

| Service | Default |
|---------|---------|
| gRPC | `:9670` |
| Health | `:9671` (`/healthz`, JSON stubs `GET /api/authors`, `GET /api/authors/{id}`, `GET /api/audiobooks`) |

## Env

| Variable | Default |
|----------|---------|
| `AUDIOBOOKS_DATA_DIR` | `./data` (SQLite at `audiobooks.db`) |
| `AUDIOBOOKS_LIBRARY_DIR` | `$DATA_DIR/audiobooks` |

## Build / test

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build -o bin/media-audiobooks ./cmd/module
```

## Status

v0.2.0 — SQLite library + scan fixtures. Audible/ASIN search and acquisition wiring are follow-ups.
