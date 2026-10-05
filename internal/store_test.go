package internal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Muxcore-Media/media-audiobooks/internal"
)

func openTempStore(t *testing.T) (*internal.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "audiobooks.db")
	s, err := internal.OpenStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestStoreAuthorAudiobookRoundTrip(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(context.Background(), internal.Author{Name: "Brandon Sanderson", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	ab, err := s.AddAudiobook(context.Background(), internal.Audiobook{
		AuthorID: au.ID, Title: "The Way of Kings", Narrator: "Michael Kramer",
		Year: 2010, DurationSeconds: 45 * 3600, Monitored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ab.Narrator != "Michael Kramer" {
		t.Fatalf("%+v", ab)
	}
	listed, err := s.ListAudiobooks(context.Background(), au.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatal("expected 1 audiobook")
	}
	if err := s.RemoveAuthor(context.Background(), au.ID); err != nil {
		t.Fatal(err)
	}
	books, err := s.ListAudiobooks(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 0 {
		t.Fatal("expected cleared")
	}
}

func TestStoreUpdateAuthor(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(context.Background(), internal.Author{Name: "Original", Monitored: true, Path: "/old"})
	if err != nil {
		t.Fatal(err)
	}
	name := "Updated Author"
	path := "/new/path"
	monitored := false
	updated, err := s.UpdateAuthor(context.Background(), au.ID, &name, &path, &monitored)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || updated.Path != path || updated.Monitored {
		t.Fatalf("%+v", updated)
	}
}

func TestStoreUpdateAudiobook(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(context.Background(), internal.Author{Name: "Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	ab, err := s.AddAudiobook(context.Background(), internal.Audiobook{AuthorID: au.ID, Title: "Old Title", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	title := "New Title"
	narrator := "Narrator X"
	asin := "B012345"
	year := int32(2020)
	monitored := false
	updated, err := s.UpdateAudiobook(context.Background(), ab.ID, &title, &narrator, &asin, &year, &monitored)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != title || updated.Narrator != narrator || updated.ASIN != asin || updated.Year != year || updated.Monitored {
		t.Fatalf("%+v", updated)
	}
}

func TestStoreListMissingAudiobooks(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(context.Background(), internal.Author{Name: "Patrick Rothfuss", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	missing, err := s.AddAudiobook(context.Background(), internal.Audiobook{AuthorID: au.ID, Title: "The Name of the Wind", Year: 2007, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	items, total, err := s.ListMissingAudiobooks(context.Background(), 1, 50)
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

func TestStoreListMissingVanishedFiles(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(context.Background(), internal.Author{Name: "Vanished Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	ab, err := s.AddAudiobook(context.Background(), internal.Audiobook{AuthorID: au.ID, Title: "Ghost Book", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(t.TempDir(), "gone.mp3")
	if err := os.WriteFile(gone, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportAudiobookFile(context.Background(), ab.ID, gone, "chapter"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	items, total, err := s.ListMissingAudiobooks(context.Background(), 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].AudiobookID != ab.ID {
		t.Fatalf("total=%d items=%+v", total, items)
	}
}

func TestStoreRemoveAudiobook(t *testing.T) {
	s, _ := openTempStore(t)
	au, err := s.AddAuthor(context.Background(), internal.Author{Name: "Author", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	ab, err := s.AddAudiobook(context.Background(), internal.Audiobook{AuthorID: au.ID, Title: "Book", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAudiobook(context.Background(), ab.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAudiobook(context.Background(), ab.ID); err == nil {
		t.Fatal("expected audiobook removed")
	}
}

func TestStorePersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audiobooks.db")
	s1, err := internal.OpenStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	au, err := s1.AddAuthor(context.Background(), internal.Author{Name: "N.K. Jemisin", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.AddAudiobook(context.Background(), internal.Audiobook{
		AuthorID: au.ID, Title: "The Fifth Season", Year: 2015, Narrator: "Robin Miles",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := internal.OpenStore(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	authors, err := s2.ListAuthors(context.Background(), "jemisin")
	if err != nil {
		t.Fatal(err)
	}
	if len(authors) != 1 {
		t.Fatalf("authors=%d", len(authors))
	}
	books, err := s2.ListAudiobooks(context.Background(), authors[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "The Fifth Season" {
		t.Fatalf("%+v", books)
	}
}

func TestStorePing(t *testing.T) {
	s, _ := openTempStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
