package internal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Muxcore-Media/media-audiobooks/internal"
)

func startTestModule(t *testing.T) *internal.Module {
	t.Helper()
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	if err := copyTree(filepath.Join("testdata", "library"), lib); err != nil {
		t.Fatal(err)
	}
	m := internal.NewModule(internal.Config{
		DataDir:    data,
		LibraryDir: lib,
		GRPCAddr:   "127.0.0.1:0",
		HTTPAddr:   "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })
	return m
}

func TestHTTPListAuthorsFixtures(t *testing.T) {
	m := startTestModule(t)

	hr, err := http.Get("http://" + m.HTTPListenAddr() + "/api/authors")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hr.Body.Close() }()
	if hr.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(hr.Body)
		t.Fatalf("status %d: %s", hr.StatusCode, b)
	}
	body, err := io.ReadAll(hr.Body)
	if err != nil {
		t.Fatal(err)
	}
	var authors []map[string]any
	if err := json.Unmarshal(body, &authors); err != nil {
		t.Fatal(err)
	}
	if len(authors) < 1 {
		t.Fatal("expected fixture authors via HTTP API")
	}

	missingResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/missing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = missingResp.Body.Close() }()
	if missingResp.StatusCode != http.StatusOK {
		t.Fatalf("missing status %d", missingResp.StatusCode)
	}
	var missing struct {
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	if err := json.NewDecoder(missingResp.Body).Decode(&missing); err != nil {
		t.Fatal(err)
	}
	if missing.Page != 1 || missing.PageSize != 100 {
		t.Fatalf("unexpected pagination: %+v", missing)
	}
}

func TestHTTPScan(t *testing.T) {
	m := startTestModule(t)
	resp, err := http.Post("http://"+m.HTTPListenAddr()+"/api/scan", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	var res struct {
		FilesFound int `json:"files_found"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.FilesFound < 2 {
		t.Fatalf("expected scanned files: %+v", res)
	}
}

func TestHTTPPatchAudiobookMonitored(t *testing.T) {
	m := startTestModule(t)
	base := "http://" + m.HTTPListenAddr()

	listResp, err := http.Get(base + "/api/audiobooks")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listResp.Body.Close() }()
	var books []struct {
		ID       string `json:"id"`
		AuthorID string `json:"author_id"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&books); err != nil {
		t.Fatal(err)
	}
	if len(books) == 0 {
		t.Fatal("expected audiobooks")
	}

	abReq, err := http.NewRequest(http.MethodPatch, base+"/api/audiobooks/"+books[0].ID, bytes.NewBufferString(`{"monitored":false}`))
	if err != nil {
		t.Fatal(err)
	}
	abResp, err := http.DefaultClient.Do(abReq)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = abResp.Body.Close() }()
	if abResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(abResp.Body)
		t.Fatalf("audiobook patch %d: %s", abResp.StatusCode, b)
	}
	var ab struct {
		Monitored bool `json:"monitored"`
	}
	if err := json.NewDecoder(abResp.Body).Decode(&ab); err != nil {
		t.Fatal(err)
	}
	if ab.Monitored {
		t.Fatal("expected audiobook unmonitored")
	}

	pathReq, err := http.NewRequest(http.MethodPatch, base+"/api/authors/"+books[0].AuthorID, bytes.NewBufferString(`{"path":"/data/audiobooks"}`))
	if err != nil {
		t.Fatal(err)
	}
	pathResp, err := http.DefaultClient.Do(pathReq)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pathResp.Body.Close() }()
	if pathResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(pathResp.Body)
		t.Fatalf("author path patch %d: %s", pathResp.StatusCode, b)
	}
	var author struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(pathResp.Body).Decode(&author); err != nil {
		t.Fatal(err)
	}
	if author.Path != "/data/audiobooks" {
		t.Fatalf("author path %q", author.Path)
	}
}

func TestHTTPDeleteAudiobook(t *testing.T) {
	m := startTestModule(t)
	base := "http://" + m.HTTPListenAddr()

	listResp, err := http.Get(base + "/api/audiobooks")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listResp.Body.Close() }()
	var books []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&books); err != nil {
		t.Fatal(err)
	}
	if len(books) == 0 {
		t.Fatal("expected audiobooks")
	}

	req, err := http.NewRequest(http.MethodDelete, base+"/api/audiobooks/"+books[0].ID+"?delete_files=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("audiobook delete %d: %s", resp.StatusCode, b)
	}
	var body struct {
		Removed     bool `json:"removed"`
		DeleteFiles bool `json:"delete_files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Removed || !body.DeleteFiles {
		t.Fatalf("unexpected audiobook delete: %+v", body)
	}

	missing, err := http.Get(base + "/api/audiobooks/" + books[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = missing.Body.Close() }()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected audiobook 404, got %d", missing.StatusCode)
	}
}

func TestHTTPAudiobookDetailAndStream(t *testing.T) {
	m := startTestModule(t)

	listResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/audiobooks")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listResp.Body.Close() }()
	var books []struct {
		ID    string `json:"id"`
		Files []struct {
			ID        string `json:"id"`
			StreamURL string `json:"stream_url"`
		} `json:"files"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&books); err != nil {
		t.Fatal(err)
	}
	if len(books) == 0 || len(books[0].Files) == 0 {
		t.Fatal("expected audiobooks with files from startup scan")
	}
	if books[0].Files[0].StreamURL == "" {
		t.Fatal("expected stream_url on list")
	}

	detailResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/audiobooks/" + books[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = detailResp.Body.Close() }()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("detail status %d", detailResp.StatusCode)
	}

	streamResp, err := http.Get("http://" + m.HTTPListenAddr() + books[0].Files[0].StreamURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = streamResp.Body.Close() }()
	if streamResp.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", streamResp.StatusCode)
	}
}

func TestHTTPStreamPathEscape404(t *testing.T) {
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(data, "outside.mp3")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := internal.NewModule(internal.Config{
		DataDir: data, LibraryDir: lib,
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	s, err := internal.OpenStore(context.Background(), filepath.Join(data, "audiobooks.db"))
	if err != nil {
		t.Fatal(err)
	}
	au, err := s.AddAuthor(context.Background(), internal.Author{Name: "Esc", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	ab, err := s.AddAudiobook(context.Background(), internal.Audiobook{AuthorID: au.ID, Title: "Book", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.ImportAudiobookFile(context.Background(), ab.ID, outside, "outside")
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })

	resp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/files/" + f.ID + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for path escape, got %d", resp.StatusCode)
	}
}

func TestHTTPImportAudiobook(t *testing.T) {
	data := t.TempDir()
	lib := filepath.Join(data, "library")
	if err := copyTree(filepath.Join("testdata", "library"), lib); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(lib, "Fixture Author", "Fixture Book", "03-import.m4b")
	if err := os.WriteFile(stub, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	m := internal.NewModule(internal.Config{
		DataDir: data, LibraryDir: lib,
		GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0",
	})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })

	listResp, err := http.Get("http://" + m.HTTPListenAddr() + "/api/audiobooks")
	if err != nil {
		t.Fatal(err)
	}
	var books []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&books); err != nil {
		t.Fatal(err)
	}
	_ = listResp.Body.Close()
	if len(books) == 0 {
		t.Fatal("expected audiobook")
	}

	body, _ := json.Marshal(map[string]string{"path": stub})
	importResp, err := http.Post(
		"http://"+m.HTTPListenAddr()+"/api/audiobooks/"+books[0].ID+"/import",
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = importResp.Body.Close() }()
	if importResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(importResp.Body)
		t.Fatalf("import status %d: %s", importResp.StatusCode, b)
	}

	outsideBody, _ := json.Marshal(map[string]string{"path": "/etc/passwd"})
	badResp, err := http.Post(
		"http://"+m.HTTPListenAddr()+"/api/audiobooks/"+books[0].ID+"/import",
		"application/json", bytes.NewReader(outsideBody),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = badResp.Body.Close() }()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for outside path, got %d", badResp.StatusCode)
	}
}

func TestNewModuleDefaultLibraryDir(t *testing.T) {
	data := t.TempDir()
	m := internal.NewModule(internal.Config{DataDir: data})
	if err := m.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })
	root := filepath.Join(data, "Author", "Title", "file.mp3")
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := m.ScanLibrary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesImported != 1 {
		t.Fatalf("expected scan at data root, got %+v", res)
	}
}

func TestModuleInfoHTTPAddrAfterStart(t *testing.T) {
	m := startTestModule(t)
	info := m.Info()
	if !strings.Contains(info.HTTPAddr, ":") {
		t.Fatalf("HTTPAddr should be bound address, got %q", info.HTTPAddr)
	}
	if info.HTTPAddr != m.HTTPListenAddr() {
		t.Fatalf("Info HTTPAddr=%q listen=%q", info.HTTPAddr, m.HTTPListenAddr())
	}
}

func TestHTTPAddAuthorAndAudiobook(t *testing.T) {
	m := startTestModule(t)
	base := "http://" + m.HTTPListenAddr()

	authorBody, _ := json.Marshal(map[string]any{"name": "Patrick Rothfuss"})
	authorResp, err := http.Post(base+"/api/authors", "application/json", bytes.NewReader(authorBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = authorResp.Body.Close() }()
	if authorResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(authorResp.Body)
		t.Fatalf("add author %d: %s", authorResp.StatusCode, b)
	}
	var author struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(authorResp.Body).Decode(&author); err != nil {
		t.Fatal(err)
	}
	if author.ID == "" || author.Name != "Patrick Rothfuss" {
		t.Fatalf("author: %+v", author)
	}

	bookBody, _ := json.Marshal(map[string]any{"title": "The Name of the Wind", "year": 2007, "narrator": "Nick Podehl"})
	bookResp, err := http.Post(base+"/api/authors/"+author.ID+"/audiobooks", "application/json", bytes.NewReader(bookBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bookResp.Body.Close() }()
	if bookResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(bookResp.Body)
		t.Fatalf("add audiobook %d: %s", bookResp.StatusCode, b)
	}
	var added struct {
		ID       string `json:"id"`
		AuthorID string `json:"author_id"`
		Title    string `json:"title"`
		Year     int32  `json:"year"`
		Narrator string `json:"narrator"`
	}
	if err := json.NewDecoder(bookResp.Body).Decode(&added); err != nil {
		t.Fatal(err)
	}
	if added.ID == "" || added.AuthorID != author.ID || added.Title != "The Name of the Wind" || added.Year != 2007 || added.Narrator != "Nick Podehl" {
		t.Fatalf("added: %+v", added)
	}
}

func TestHTTPAudiobookArtwork(t *testing.T) {
	m := startTestModule(t)
	base := "http://" + m.HTTPListenAddr()

	authorBody, _ := json.Marshal(map[string]any{"name": "Artwork Author"})
	authorResp, err := http.Post(base+"/api/authors", "application/json", bytes.NewReader(authorBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = authorResp.Body.Close() }()
	var author struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(authorResp.Body).Decode(&author); err != nil {
		t.Fatal(err)
	}
	bookBody, _ := json.Marshal(map[string]any{"title": "Cover Book", "year": 2020})
	bookResp, err := http.Post(base+"/api/authors/"+author.ID+"/audiobooks", "application/json", bytes.NewReader(bookBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bookResp.Body.Close() }()
	var added struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(bookResp.Body).Decode(&added); err != nil {
		t.Fatal(err)
	}

	emptyResp, err := http.Get(base + "/api/audiobooks/" + added.ID + "/artwork")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = emptyResp.Body.Close() }()
	if emptyResp.StatusCode != http.StatusOK {
		t.Fatalf("list empty status %d", emptyResp.StatusCode)
	}
	var listed struct {
		Available bool `json:"available"`
		Items     []struct {
			URL string `json:"url"`
		} `json:"items"`
	}
	if err := json.NewDecoder(emptyResp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if !listed.Available || len(listed.Items) != 0 {
		t.Fatalf("empty artwork: %+v", listed)
	}

	replaceBody, _ := json.Marshal(map[string]any{
		"type": "poster", "filename": "cover.jpg", "data": "iVBORw0KGgo=",
	})
	replaceResp, err := http.Post(base+"/api/audiobooks/"+added.ID+"/artwork", "application/json", bytes.NewReader(replaceBody))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = replaceResp.Body.Close() }()
	if replaceResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(replaceResp.Body)
		t.Fatalf("replace %d: %s", replaceResp.StatusCode, b)
	}
	var replaced struct {
		OK      bool `json:"ok"`
		Artwork struct {
			URL string `json:"url"`
		} `json:"artwork"`
	}
	if err := json.NewDecoder(replaceResp.Body).Decode(&replaced); err != nil {
		t.Fatal(err)
	}
	if !replaced.OK || !strings.Contains(replaced.Artwork.URL, "/images/"+added.ID+"/poster.jpg") {
		t.Fatalf("replaced: %+v", replaced)
	}

	listedResp, err := http.Get(base + "/api/audiobooks/" + added.ID + "/artwork")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listedResp.Body.Close() }()
	if err := json.NewDecoder(listedResp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if !listed.Available || len(listed.Items) != 1 || !strings.Contains(listed.Items[0].URL, "/images/"+added.ID+"/poster.jpg") {
		t.Fatalf("listed: %+v", listed)
	}
}
