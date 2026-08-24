package internal

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (m *Module) registerAudiobooksHTTPAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/authors", m.handleListAuthorsHTTP)
	mux.HandleFunc("GET /api/authors/{id}", m.handleGetAuthorHTTP)
	mux.HandleFunc("GET /api/audiobooks", m.handleListAudiobooksHTTP)
	mux.HandleFunc("GET /api/missing", m.handleListMissingHTTP)
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
		detail.Audiobooks = append(detail.Audiobooks, toAudiobookJSON(b))
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
		out = append(out, toAudiobookJSON(b))
	}
	writeJSON(w, out)
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

type authorJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Monitored bool   `json:"monitored"`
	Path      string `json:"path"`
}

type audiobookJSON struct {
	ID              string `json:"id"`
	AuthorID        string `json:"author_id"`
	Title           string `json:"title"`
	Narrator        string `json:"narrator"`
	ASIN            string `json:"asin"`
	Year            int32  `json:"year"`
	DurationSeconds int32  `json:"duration_seconds"`
	Monitored       bool   `json:"monitored"`
}

type authorDetailJSON struct {
	Author     authorJSON      `json:"author"`
	Audiobooks []audiobookJSON `json:"audiobooks"`
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

func toAuthorJSON(a *Author) authorJSON {
	return authorJSON{ID: a.ID, Name: a.Name, Monitored: a.Monitored, Path: a.Path}
}

func toAudiobookJSON(b *Audiobook) audiobookJSON {
	return audiobookJSON{
		ID: b.ID, AuthorID: b.AuthorID, Title: b.Title, Narrator: b.Narrator,
		ASIN: b.ASIN, Year: b.Year, DurationSeconds: b.DurationSeconds, Monitored: b.Monitored,
	}
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
