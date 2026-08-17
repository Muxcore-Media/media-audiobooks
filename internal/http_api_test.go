package internal_test

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/media-audiobooks/internal"
)

func TestHTTPListAuthorsFixtures(t *testing.T) {
	data := t.TempDir()
	lib := filepath.Join(data, "audiobooks")
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
	if _, err := m.ScanLibrary(); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(t.Context()) })

	hr, err := http.Get("http://" + m.HTTPListenAddr() + "/api/authors")
	if err != nil {
		t.Fatal(err)
	}
	defer hr.Body.Close()
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
}
