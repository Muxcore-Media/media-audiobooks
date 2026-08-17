package internal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/media-audiobooks/internal"
)

// TestScanLibraryRootFixtures walks local stub audio files under testdata/library.
// Files are empty stubs (no paid metadata APIs).
func TestScanLibraryRootFixtures(t *testing.T) {
	root := filepath.Join("testdata", "library")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("fixture library missing: %v", err)
	}

	s, _ := openTempStore(t)
	res, err := s.ScanLibraryRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesFound < 2 {
		t.Fatalf("expected >=2 audio stubs, found=%d imported=%d skipped=%d",
			res.FilesFound, res.FilesImported, res.FilesSkipped)
	}
	if res.FilesImported < 2 {
		t.Fatalf("expected imports, got %+v", res)
	}

	authors, err := s.ListAuthors("")
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) < 1 {
		t.Fatal("expected at least one author from fixtures")
	}
	var fixtureAuthor *internal.Author
	for _, a := range authors {
		if a.Name == "Fixture Author" {
			fixtureAuthor = a
			break
		}
	}
	if fixtureAuthor == nil {
		t.Fatalf("Fixture Author not found: %+v", authors)
	}
	books, err := s.ListAudiobooks(fixtureAuthor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) < 1 {
		t.Fatal("expected audiobook from fixtures")
	}
	files, err := s.ListAudiobookFiles(books[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 2 {
		t.Fatalf("expected mp3+m4b stubs, files=%d", len(files))
	}

	// Idempotent rescan should skip already-imported paths.
	res2, err := s.ScanLibraryRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if res2.FilesImported != 0 {
		t.Fatalf("expected no new imports on rescan, got %+v", res2)
	}
	if res2.FilesSkipped < 2 {
		t.Fatalf("expected skips on rescan, got %+v", res2)
	}
}

func TestScanLibraryRootMissing(t *testing.T) {
	s, _ := openTempStore(t)
	_, err := s.ScanLibraryRoot(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestModuleInitScan(t *testing.T) {
	data := t.TempDir()
	lib := filepath.Join(data, "audiobooks")
	src := filepath.Join("testdata", "library")
	if err := copyTree(src, lib); err != nil {
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
	t.Cleanup(func() { _ = m.Stop(t.Context()) })

	res, err := m.ScanLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesImported < 2 {
		t.Fatalf("%+v", res)
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
