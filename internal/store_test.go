package internal_test

import (
	"testing"

	"github.com/Muxcore-Media/media-audiobooks/internal"
)

func TestStoreAuthorAudiobookRoundTrip(t *testing.T) {
	s := internal.NewStore()
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
	if len(s.ListAudiobooks(au.ID)) != 1 {
		t.Fatal("expected 1 audiobook")
	}
	if err := s.RemoveAuthor(au.ID); err != nil {
		t.Fatal(err)
	}
	if len(s.ListAudiobooks("")) != 0 {
		t.Fatal("expected cleared")
	}
}
