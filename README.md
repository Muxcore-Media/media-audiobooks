# Media Audiobooks

Audiobook library manager scaffold for MuxCore.

Exposes `muxcore.audiobooks.v1.AudiobookManagementService` (authors/audiobooks) with an in-memory store in **v0.1.0**, plus SettingsProvider for `library_dir`.

## Ports

| Service | Default |
|---------|---------|
| gRPC | `:9670` |
| Health | `:9671` |

## Build / test

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build -o bin/media-audiobooks ./cmd/module
```

## Status

v0.1.0 scaffold — optional peer. Persistence, Audible/ASIN search, and acquisition wiring are follow-ups.
