package internal

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type Author struct {
	ID        string
	Name      string
	Monitored bool
	Path      string
}

type Audiobook struct {
	ID              string
	AuthorID        string
	Title           string
	Narrator        string
	ASIN            string
	Year            int32
	DurationSeconds int32
	Monitored       bool
}

type Store struct {
	mu         sync.RWMutex
	authors    map[string]*Author
	audiobooks map[string]*Audiobook
}

func NewStore() *Store {
	return &Store{authors: map[string]*Author{}, audiobooks: map[string]*Audiobook{}}
}

func (s *Store) AddAuthor(a Author) (*Author, error) {
	if strings.TrimSpace(a.Name) == "" {
		return nil, fmt.Errorf("author name required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.ID == "" {
		a.ID = "aa_" + uuid.NewString()[:8]
	}
	cp := a
	s.authors[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) GetAuthor(id string) (*Author, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.authors[id]
	if !ok {
		return nil, fmt.Errorf("author %q not found", id)
	}
	cp := *a
	return &cp, nil
}

func (s *Store) ListAuthors(query string) []*Author {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]*Author, 0, len(s.authors))
	for _, a := range s.authors {
		if q != "" && !strings.Contains(strings.ToLower(a.Name), q) {
			continue
		}
		cp := *a
		out = append(out, &cp)
	}
	return out
}

func (s *Store) RemoveAuthor(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.authors[id]; !ok {
		return fmt.Errorf("author %q not found", id)
	}
	delete(s.authors, id)
	for aid, ab := range s.audiobooks {
		if ab.AuthorID == id {
			delete(s.audiobooks, aid)
		}
	}
	return nil
}

func (s *Store) AddAudiobook(ab Audiobook) (*Audiobook, error) {
	if strings.TrimSpace(ab.Title) == "" {
		return nil, fmt.Errorf("audiobook title required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.authors[ab.AuthorID]; !ok {
		return nil, fmt.Errorf("author %q not found", ab.AuthorID)
	}
	if ab.ID == "" {
		ab.ID = "ab_" + uuid.NewString()[:8]
	}
	cp := ab
	s.audiobooks[cp.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) ListAudiobooks(authorID string) []*Audiobook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Audiobook, 0)
	for _, ab := range s.audiobooks {
		if authorID != "" && ab.AuthorID != authorID {
			continue
		}
		cp := *ab
		out = append(out, &cp)
	}
	return out
}
