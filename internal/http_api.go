package internal

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (m *Module) registerAudiobooksHTTPAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/authors", m.handleListAuthorsHTTP)
	mux.HandleFunc("POST /api/authors", m.handleAddAuthorHTTP)
	mux.HandleFunc("GET /api/authors/{id}", m.handleGetAuthorHTTP)
	mux.HandleFunc("POST /api/authors/{id}/audiobooks", m.handleAddAudiobookHTTP)
	mux.HandleFunc("PATCH /api/authors/{id}", m.handlePatchAuthorHTTP)
	mux.HandleFunc("GET /api/audiobooks", m.handleListAudiobooksHTTP)
	mux.HandleFunc("GET /api/audiobooks/{id}", m.handleGetAudiobookHTTP)
	mux.HandleFunc("PATCH /api/audiobooks/{id}", m.handlePatchAudiobookHTTP)
	mux.HandleFunc("DELETE /api/audiobooks/{id}", m.handleDeleteAudiobookHTTP)
	mux.HandleFunc("POST /api/audiobooks/{id}/import", m.handleImportAudiobookHTTP)
	mux.HandleFunc("GET /api/missing", m.handleListMissingHTTP)
	mux.HandleFunc("POST /api/scan", m.handleScanHTTP)
	mux.HandleFunc("GET /api/files/{id}/stream", m.handleStreamAudiobookFileHTTP)
}

func (m *Module) handleListAuthorsHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	items, err := m.store.ListAuthors(r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	out := make([]authorJSON, 0, len(items))
	for _, a := range items {
		out = append(out, toAuthorJSON(a))
	}
	writeJSON(w, out)
}

func (m *Module) handleAddAuthorHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Name      string `json:"name"`
		Monitored *bool  `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
		return
	}
	monitored := true
	if body.Monitored != nil {
		monitored = *body.Monitored
	}
	a, err := m.store.AddAuthor(Author{Name: name, Monitored: monitored})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, toAuthorJSON(a))
}

func (m *Module) handleAddAudiobookHTTP(w http.ResponseWriter, r *http.Request) {
	authorID := strings.TrimSpace(r.PathValue("id"))
	if authorID == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	var body struct {
		Title     string `json:"title"`
		Narrator  string `json:"narrator"`
		Year      int32  `json:"year"`
		Monitored *bool  `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		http.Error(w, `{"error":"title required"}`, http.StatusBadRequest)
		return
	}
	monitored := true
	if body.Monitored != nil {
		monitored = *body.Monitored
	}
	ab, err := m.store.AddAudiobook(Audiobook{
		AuthorID: authorID, Title: title, Narrator: strings.TrimSpace(body.Narrator),
		Year: body.Year, Monitored: monitored,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, m.audiobookJSONWithFiles(ab))
}

func (m *Module) handleGetAuthorHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	a, err := m.store.GetAuthor(id)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	books, err := m.store.ListAudiobooks(id)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	detail := authorDetailJSON{Author: toAuthorJSON(a), Audiobooks: make([]audiobookJSON, 0, len(books))}
	for _, b := range books {
		detail.Audiobooks = append(detail.Audiobooks, m.audiobookJSONWithFiles(b))
	}
	writeJSON(w, detail)
}

func (m *Module) handleListAudiobooksHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	items, err := m.store.ListAudiobooks(r.URL.Query().Get("author_id"))
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	out := make([]audiobookJSON, 0, len(items))
	for _, b := range items {
		out = append(out, m.audiobookJSONWithFiles(b))
	}
	writeJSON(w, out)
}

func (m *Module) handleGetAudiobookHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
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
	au, err := m.store.GetAuthor(ab.AuthorID)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, audiobookDetailJSON{
		Author:    toAuthorJSON(au),
		Audiobook: m.audiobookJSONWithFiles(ab),
	})
}

func (m *Module) handlePatchAuthorHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	var body struct {
		Monitored *bool   `json:"monitored"`
		Path      *string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}
	if body.Monitored == nil && body.Path == nil {
		http.Error(w, `{"error":"monitored or path is required"}`, http.StatusBadRequest)
		return
	}
	var path *string
	if body.Path != nil {
		trimmed := strings.TrimSpace(*body.Path)
		path = &trimmed
	}
	a, err := m.store.UpdateAuthor(id, nil, path, body.Monitored)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, toAuthorJSON(a))
}

func (m *Module) handlePatchAudiobookHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	mon, ok := readMonitoredJSON(w, r)
	if !ok {
		return
	}
	ab, err := m.store.UpdateAudiobook(id, nil, nil, nil, nil, &mon)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, m.audiobookJSONWithFiles(ab))
}

func (m *Module) handleDeleteAudiobookHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	deleteFiles := queryDeleteFiles(r)
	if err := m.removeAudiobook(id, deleteFiles); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, map[string]any{"removed": true, "delete_files": deleteFiles})
}

func (m *Module) handleImportAudiobookHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	root := m.libraryRoot()
	abs, err := pathUnderRoot(root, body.Path)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusBadRequest)
		return
	}
	f, err := m.store.ImportAudiobookFile(id, abs, "")
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, fmtJSONError(err), status)
		return
	}
	writeJSON(w, toAudiobookFileJSON(f))
}

func (m *Module) handleScanHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	res, err := m.ScanLibrary()
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, scanResultJSON{
		FilesFound: res.FilesFound, FilesImported: res.FilesImported,
		FilesSkipped: res.FilesSkipped, FilesRemoved: res.FilesRemoved,
	})
}

func (m *Module) handleListMissingHTTP(w http.ResponseWriter, r *http.Request) {
	if m.store == nil {
		http.Error(w, `{"error":"store not open"}`, http.StatusServiceUnavailable)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	items, total, err := m.store.ListMissingAudiobooks(page, pageSize)
	if err != nil {
		http.Error(w, fmtJSONError(err), http.StatusInternalServerError)
		return
	}
	out := make([]missingAudiobookJSON, 0, len(items))
	for _, it := range items {
		out = append(out, missingAudiobookJSON(it))
	}
	writeJSON(w, missingAudiobooksResponse{
		Items: out, Total: total, Page: page, PageSize: pageSize,
	})
}

func (m *Module) handleStreamAudiobookFileHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || m.store == nil {
		http.NotFound(w, r)
		return
	}
	f, err := m.store.GetAudiobookFile(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	root := m.libraryRoot()
	abs, err := pathUnderRoot(root, f.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, abs)
}

func (m *Module) audiobookJSONWithFiles(b *Audiobook) audiobookJSON {
	out := toAudiobookJSON(b)
	if m.store == nil {
		return out
	}
	files, err := m.store.ListAudiobookFiles(b.ID)
	if err != nil {
		return out
	}
	out.Files = make([]audiobookFileJSON, 0, len(files))
	for _, f := range files {
		out.Files = append(out.Files, toAudiobookFileJSON(f))
	}
	return out
}

type authorJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Monitored bool   `json:"monitored"`
	Path      string `json:"path"`
}

type audiobookFileJSON struct {
	ID          string `json:"id"`
	AudiobookID string `json:"audiobook_id"`
	Title       string `json:"title"`
	Path        string `json:"path,omitempty"`
	StreamURL   string `json:"stream_url,omitempty"`
}

type audiobookJSON struct {
	ID              string              `json:"id"`
	AuthorID        string              `json:"author_id"`
	Title           string              `json:"title"`
	Narrator        string              `json:"narrator"`
	ASIN            string              `json:"asin"`
	Year            int32               `json:"year"`
	DurationSeconds int32               `json:"duration_seconds"`
	Monitored       bool                `json:"monitored"`
	Files           []audiobookFileJSON `json:"files,omitempty"`
}

type authorDetailJSON struct {
	Author     authorJSON      `json:"author"`
	Audiobooks []audiobookJSON `json:"audiobooks"`
}

type audiobookDetailJSON struct {
	Author    authorJSON    `json:"author"`
	Audiobook audiobookJSON `json:"audiobook"`
}

type missingAudiobookJSON struct {
	AudiobookID string `json:"audiobook_id"`
	AuthorID    string `json:"author_id"`
	Title       string `json:"title"`
	AuthorName  string `json:"author_name"`
	Year        int32  `json:"year"`
}

type missingAudiobooksResponse struct {
	Items    []missingAudiobookJSON `json:"items"`
	Total    int                    `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

type scanResultJSON struct {
	FilesFound    int `json:"files_found"`
	FilesImported int `json:"files_imported"`
	FilesSkipped  int `json:"files_skipped"`
	FilesRemoved  int `json:"files_removed"`
}

func toAuthorJSON(a *Author) authorJSON {
	return authorJSON{ID: a.ID, Name: a.Name, Monitored: a.Monitored, Path: a.Path}
}

func toAudiobookJSON(b *Audiobook) audiobookJSON {
	return audiobookJSON{
		ID: b.ID, AuthorID: b.AuthorID, Title: b.Title, Narrator: b.Narrator,
		ASIN: b.ASIN, Year: b.Year, DurationSeconds: b.DurationSeconds, Monitored: b.Monitored,
	}
}

func toAudiobookFileJSON(f *AudiobookFile) audiobookFileJSON {
	return audiobookFileJSON{
		ID: f.ID, AudiobookID: f.AudiobookID, Title: f.Title,
		StreamURL: "/api/files/" + f.ID + "/stream",
	}
}

func queryDeleteFiles(r *http.Request) bool {
	raw := strings.TrimSpace(r.URL.Query().Get("delete_files"))
	return raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes")
}

func readMonitoredJSON(w http.ResponseWriter, r *http.Request) (bool, bool) {
	var body struct {
		Monitored *bool `json:"monitored"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Monitored == nil {
		http.Error(w, `{"error":"monitored is required"}`, http.StatusBadRequest)
		return false, false
	}
	return *body.Monitored, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func fmtJSONError(err error) string {
	b, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(b)
}
