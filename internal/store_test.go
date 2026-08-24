package internal_test

import (
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/media-audiobooks/internal"
)

func openTempStore(t *testing.T) (*internal.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "audiobooks.db")
	s, err := internal.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestStoreAuthorAudiobookRoundTrip(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(internal.Author{Name: "Brandon Sanderson", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	ab, err := s.AddAudiobook(internal.Audiobook{
		AuthorID: au.ID, Title: "The Way of Kings", Narrator: "Michael Kramer",
		Year: 2010, DurationSeconds: 45 * 3600, Monitored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ab.Narrator != "Michael Kramer" {
		t.Fatalf("%+v", ab)
	}
	listed, err := s.ListAudiobooks(au.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatal("expected 1 audiobook")
	}
	if err := s.RemoveAuthor(au.ID); err != nil {
		t.Fatal(err)
	}
	books, err := s.ListAudiobooks("")
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 0 {
		t.Fatal("expected cleared")
	}
}

func TestStoreListMissingAudiobooks(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(internal.Author{Name: "Patrick Rothfuss", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	missing, err := s.AddAudiobook(internal.Audiobook{AuthorID: au.ID, Title: "The Name of the Wind", Year: 2007, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	items, total, err := s.ListMissingAudiobooks(1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d items=%d", total, len(items))
	}
	if items[0].AudiobookID != missing.ID || items[0].AuthorName != "Patrick Rothfuss" {
		t.Fatalf("%+v", items[0])
	}
}

func TestStorePersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audiobooks.db")
	s1, err := internal.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	au, err := s1.AddAuthor(internal.Author{Name: "N.K. Jemisin", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.AddAudiobook(internal.Audiobook{
		AuthorID: au.ID, Title: "The Fifth Season", Year: 2015, Narrator: "Robin Miles",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := internal.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	authors, err := s2.ListAuthors("jemisin")
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("authors=%d", len(authors))
	}
	books, err := s2.ListAudiobooks(authors[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "The Fifth Season" {
		t.Fatalf("%+v", books)
	}
}
