package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxArtworkBytes = 20 << 20

func (m *Module) getImageDir() string {
	if m.imageDir != "" {
		return m.imageDir
	}
	return filepath.Join(m.dataDir, "images")
}

func artworkURL(httpAddr, relPath string) string {
	relPath = strings.TrimPrefix(filepath.ToSlash(relPath), "/")
	return fmt.Sprintf("http://%s/images/%s", httpAddr, relPath)
}

func extFromFilenameOrMIME(filename, contentType string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		if ext == ".jpeg" {
			return ".jpg"
		}
		return ext
	}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	case strings.Contains(ct, "gif"):
		return ".gif"
	default:
		return ".jpg"
	}
}

func (m *Module) writeArtworkBytes(itemID, filename, contentType string, data []byte) (relPath, mime string, err error) {
	if itemID == "" {
		return "", "", fmt.Errorf("item id required")
	}
	if len(data) == 0 {
		return "", "", fmt.Errorf("empty artwork data")
	}
	if len(data) > maxArtworkBytes {
		return "", "", fmt.Errorf("artwork exceeds %d bytes", maxArtworkBytes)
	}
	ext := extFromFilenameOrMIME(filename, contentType)
	dir := filepath.Join(m.getImageDir(), itemID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("create artwork dir: %w", err)
	}
	relPath = filepath.ToSlash(filepath.Join(itemID, "poster"+ext))
	abs := filepath.Join(m.getImageDir(), filepath.FromSlash(relPath))
	if err := os.WriteFile(abs, data, 0o600); err != nil {
		return "", "", fmt.Errorf("write artwork: %w", err)
	}
	mime = contentType
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	return relPath, mime, nil
}

func (m *Module) servableArtworkURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "/images/") {
		return raw
	}
	return artworkURL(m.httpAddr, raw)
}

func decodeArtworkPayload(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, ","); i >= 0 && strings.Contains(raw[:i], "base64") {
		raw = raw[i+1:]
	}
	return base64.StdEncoding.DecodeString(raw)
}

func (m *Module) handleListAudiobookArtworkHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	ab, err := m.store.GetAudiobook(id)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	items := make([]map[string]any, 0, 1)
	if url := m.servableArtworkURL(ab.PosterURL); url != "" {
		items = append(items, map[string]any{
			"id": ab.ID + "_poster", "item_id": ab.ID, "type": "poster", "url": url,
		})
	}
	writeJSON(w, map[string]any{"available": true, "items": items})
}

func (m *Module) handleReplaceAudiobookArtworkHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	if _, err := m.store.GetAudiobook(id); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	var body struct {
		Type     string `json:"type"`
		Filename string `json:"filename"`
		Data     string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	data, err := decodeArtworkPayload(body.Data)
	if err != nil || len(data) == 0 {
		http.Error(w, `{"error":"artwork data required"}`, http.StatusBadRequest)
		return
	}
	filename := strings.TrimSpace(body.Filename)
	if filename == "" {
		filename = "artwork.jpg"
	}
	rel, mime, err := m.writeArtworkBytes(id, filename, "", data)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusBadRequest)
		return
	}
	if _, err := m.store.SetAudiobookPosterURL(id, rel); err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"ok": true,
		"artwork": map[string]any{
			"id": id + "_poster", "item_id": id, "type": "poster",
			"url": artworkURL(m.httpAddr, rel), "mime_type": mime,
		},
	})
}
