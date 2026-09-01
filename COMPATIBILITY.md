# Compatibility

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.2.0         | 0.5.8+      | Current |
| v0.1.0         | 0.5.2+      | Superseded |

## Capabilities

- `media.audiobooks` / `audiobooks`
- `settings`

## Proto (`muxcore.audiobooks.v1`)

v0.2.0 adds non-breaking RPCs: `ScanLibrary`, `UpdateAuthor`, `UpdateAudiobook`, `GetAudiobook`, `RemoveAudiobook`, `ListAudiobookFiles`, `ListMissing`, `ImportAudiobookFile`. `Audiobook.files` is populated on get/list responses.

## Env

- `AUDIOBOOKS_DATA_DIR` — SQLite + default library root
- `AUDIOBOOKS_LIBRARY_DIR` — optional override for scanned audio tree (defaults to data dir)
