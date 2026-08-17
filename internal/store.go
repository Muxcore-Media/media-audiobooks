package internal

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
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

// AudiobookFile is a single audio file in the library.
type AudiobookFile struct {
	ID          string
	AudiobookID string
	AuthorID    string
	Title       string
	Path        string
}

// Store persists the audiobook library in SQLite.
type Store struct {
	db *sql.DB
}

// OpenStore opens or creates the SQLite database at path (WAL mode).
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS authors (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			monitored   INTEGER NOT NULL DEFAULT 1,
			path        TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE IF NOT EXISTS audiobooks (
			id                TEXT PRIMARY KEY,
			author_id         TEXT NOT NULL,
			title             TEXT NOT NULL,
			narrator          TEXT NOT NULL DEFAULT '',
			asin              TEXT NOT NULL DEFAULT '',
			year              INTEGER NOT NULL DEFAULT 0,
			duration_seconds  INTEGER NOT NULL DEFAULT 0,
			monitored         INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY (author_id) REFERENCES authors(id) ON DELETE CASCADE
		);
		CREATE TABLE IF NOT EXISTS audiobook_files (
			id            TEXT PRIMARY KEY,
			audiobook_id  TEXT NOT NULL,
			author_id     TEXT NOT NULL,
			title         TEXT NOT NULL,
			path          TEXT NOT NULL UNIQUE,
			FOREIGN KEY (audiobook_id) REFERENCES audiobooks(id) ON DELETE CASCADE,
			FOREIGN KEY (author_id) REFERENCES authors(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_authors_name ON authors(name);
		CREATE INDEX IF NOT EXISTS idx_audiobooks_author ON audiobooks(author_id);
		CREATE INDEX IF NOT EXISTS idx_audiobook_files_book ON audiobook_files(audiobook_id);
		CREATE INDEX IF NOT EXISTS idx_audiobook_files_path ON audiobook_files(path);
	`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) AddAuthor(a Author) (*Author, error) {
	if strings.TrimSpace(a.Name) == "" {
		return nil, fmt.Errorf("author name required")
	}
	if a.ID == "" {
		a.ID = "aa_" + uuid.NewString()[:8]
	}
	monitored := 0
	if a.Monitored {
		monitored = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO authors (id, name, monitored, path)
		VALUES (?, ?, ?, ?)
	`, a.ID, a.Name, monitored, a.Path)
	if err != nil {
		return nil, fmt.Errorf("insert author: %w", err)
	}
	out := a
	return &out, nil
}

func (s *Store) GetAuthor(id string) (*Author, error) {
	row := s.db.QueryRow(`
		SELECT id, name, monitored, path FROM authors WHERE id = ?
	`, id)
	return scanAuthor(row)
}

func (s *Store) ListAuthors(query string) ([]*Author, error) {
	rows, err := s.db.Query(`
		SELECT id, name, monitored, path FROM authors ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list authors: %w", err)
	}
	defer rows.Close()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]*Author, 0)
	for rows.Next() {
		a, err := scanAuthor(rows)
		if err != nil {
			return nil, err
		}
		if q != "" && !strings.Contains(strings.ToLower(a.Name), q) {
			continue
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) RemoveAuthor(id string) error {
	res, err := s.db.Exec(`DELETE FROM authors WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete author: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("author %q not found", id)
	}
	// Cascades may be off without PRAGMA foreign_keys; clean children explicitly.
	_, _ = s.db.Exec(`DELETE FROM audiobook_files WHERE author_id = ?`, id)
	_, _ = s.db.Exec(`DELETE FROM audiobooks WHERE author_id = ?`, id)
	return nil
}

func (s *Store) AddAudiobook(ab Audiobook) (*Audiobook, error) {
	if strings.TrimSpace(ab.Title) == "" {
		return nil, fmt.Errorf("audiobook title required")
	}
	var exists string
	err := s.db.QueryRow(`SELECT id FROM authors WHERE id = ?`, ab.AuthorID).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("author %q not found", ab.AuthorID)
	}
	if err != nil {
		return nil, fmt.Errorf("lookup author: %w", err)
	}
	if ab.ID == "" {
		ab.ID = "ab_" + uuid.NewString()[:8]
	}
	monitored := 0
	if ab.Monitored {
		monitored = 1
	}
	_, err = s.db.Exec(`
		INSERT INTO audiobooks (id, author_id, title, narrator, asin, year, duration_seconds, monitored)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, ab.ID, ab.AuthorID, ab.Title, ab.Narrator, ab.ASIN, ab.Year, ab.DurationSeconds, monitored)
	if err != nil {
		return nil, fmt.Errorf("insert audiobook: %w", err)
	}
	out := ab
	return &out, nil
}

func (s *Store) ListAudiobooks(authorID string) ([]*Audiobook, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if authorID != "" {
		rows, err = s.db.Query(`
			SELECT id, author_id, title, narrator, asin, year, duration_seconds, monitored
			FROM audiobooks WHERE author_id = ? ORDER BY year, title
		`, authorID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, author_id, title, narrator, asin, year, duration_seconds, monitored
			FROM audiobooks ORDER BY title
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list audiobooks: %w", err)
	}
	defer rows.Close()
	out := make([]*Audiobook, 0)
	for rows.Next() {
		ab, err := scanAudiobook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ab)
	}
	return out, rows.Err()
}

// ListAudiobookFiles returns files, optionally filtered by audiobook ID.
func (s *Store) ListAudiobookFiles(audiobookID string) ([]*AudiobookFile, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if audiobookID != "" {
		rows, err = s.db.Query(`
			SELECT id, audiobook_id, author_id, title, path FROM audiobook_files
			WHERE audiobook_id = ? ORDER BY title
		`, audiobookID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, audiobook_id, author_id, title, path FROM audiobook_files ORDER BY title
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list audiobook files: %w", err)
	}
	defer rows.Close()
	out := make([]*AudiobookFile, 0)
	for rows.Next() {
		f, err := scanAudiobookFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) findAuthorByName(name string) (*Author, error) {
	row := s.db.QueryRow(`
		SELECT id, name, monitored, path FROM authors
		WHERE lower(name) = lower(?) LIMIT 1
	`, name)
	a, err := scanAuthor(row)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

func (s *Store) findAudiobook(authorID, title string) (*Audiobook, error) {
	row := s.db.QueryRow(`
		SELECT id, author_id, title, narrator, asin, year, duration_seconds, monitored FROM audiobooks
		WHERE author_id = ? AND lower(title) = lower(?) LIMIT 1
	`, authorID, title)
	ab, err := scanAudiobook(row)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return ab, nil
}

func (s *Store) findAudiobookFileByPath(path string) (*AudiobookFile, error) {
	row := s.db.QueryRow(`
		SELECT id, audiobook_id, author_id, title, path FROM audiobook_files WHERE path = ?
	`, path)
	f, err := scanAudiobookFile(row)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	return f, nil
}

func (s *Store) upsertAudiobookFile(f AudiobookFile) (*AudiobookFile, error) {
	existing, err := s.findAudiobookFileByPath(f.Path)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		_, err := s.db.Exec(`
			UPDATE audiobook_files SET audiobook_id = ?, author_id = ?, title = ? WHERE id = ?
		`, f.AudiobookID, f.AuthorID, f.Title, existing.ID)
		if err != nil {
			return nil, fmt.Errorf("update audiobook file: %w", err)
		}
		existing.AudiobookID = f.AudiobookID
		existing.AuthorID = f.AuthorID
		existing.Title = f.Title
		return existing, nil
	}
	if f.ID == "" {
		f.ID = "af_" + uuid.NewString()[:8]
	}
	_, err = s.db.Exec(`
		INSERT INTO audiobook_files (id, audiobook_id, author_id, title, path)
		VALUES (?, ?, ?, ?, ?)
	`, f.ID, f.AudiobookID, f.AuthorID, f.Title, f.Path)
	if err != nil {
		return nil, fmt.Errorf("insert audiobook file: %w", err)
	}
	out := f
	return &out, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAuthor(row rowScanner) (*Author, error) {
	var a Author
	var monitored int
	if err := row.Scan(&a.ID, &a.Name, &monitored, &a.Path); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("author not found")
		}
		return nil, err
	}
	a.Monitored = monitored != 0
	return &a, nil
}

func scanAudiobook(row rowScanner) (*Audiobook, error) {
	var ab Audiobook
	var monitored int
	if err := row.Scan(&ab.ID, &ab.AuthorID, &ab.Title, &ab.Narrator, &ab.ASIN, &ab.Year, &ab.DurationSeconds, &monitored); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("audiobook not found")
		}
		return nil, err
	}
	ab.Monitored = monitored != 0
	return &ab, nil
}

func scanAudiobookFile(row rowScanner) (*AudiobookFile, error) {
	var f AudiobookFile
	if err := row.Scan(&f.ID, &f.AudiobookID, &f.AuthorID, &f.Title, &f.Path); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("audiobook file not found")
		}
		return nil, err
	}
	return &f, nil
}
