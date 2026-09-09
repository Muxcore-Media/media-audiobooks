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
	PosterURL       string
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
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
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
		CREATE TABLE IF NOT EXISTS history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			item_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			source_title TEXT DEFAULT '',
			quality TEXT DEFAULT '',
			data TEXT DEFAULT '{}',
			date TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_history_item ON history(item_id);
	`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE audiobooks ADD COLUMN poster_url TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return fmt.Errorf("migrate poster_url: %w", err)
	}
	return nil
}

// Ping verifies the database connection.
func (s *Store) Ping() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store not open")
	}
	return s.db.QueryRow(`SELECT 1`).Scan(new(int))
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
	defer func() { _ = rows.Close() }()
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

func (s *Store) UpdateAuthor(id string, name, path *string, monitored *bool) (*Author, error) {
	a, err := s.GetAuthor(id)
	if err != nil {
		return nil, err
	}
	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return nil, fmt.Errorf("author name required")
		}
		a.Name = *name
	}
	if path != nil {
		a.Path = *path
	}
	if monitored != nil {
		a.Monitored = *monitored
	}
	m := 0
	if a.Monitored {
		m = 1
	}
	_, err = s.db.Exec(`
		UPDATE authors SET name = ?, monitored = ?, path = ? WHERE id = ?
	`, a.Name, m, a.Path, id)
	if err != nil {
		return nil, fmt.Errorf("update author: %w", err)
	}
	return a, nil
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

func (s *Store) GetAudiobook(id string) (*Audiobook, error) {
	row := s.db.QueryRow(`
		SELECT id, author_id, title, narrator, asin, year, duration_seconds, monitored, poster_url
		FROM audiobooks WHERE id = ?
	`, id)
	return scanAudiobook(row)
}

func (s *Store) ListAudiobooks(authorID string) ([]*Audiobook, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if authorID != "" {
		rows, err = s.db.Query(`
			SELECT id, author_id, title, narrator, asin, year, duration_seconds, monitored, poster_url
			FROM audiobooks WHERE author_id = ? ORDER BY year, title
		`, authorID)
	} else {
		rows, err = s.db.Query(`
			SELECT id, author_id, title, narrator, asin, year, duration_seconds, monitored, poster_url
			FROM audiobooks ORDER BY title
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("list audiobooks: %w", err)
	}
	defer func() { _ = rows.Close() }()
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

func (s *Store) UpdateAudiobook(id string, title, narrator, asin *string, year *int32, monitored *bool) (*Audiobook, error) {
	ab, err := s.GetAudiobook(id)
	if err != nil {
		return nil, err
	}
	if title != nil {
		if strings.TrimSpace(*title) == "" {
			return nil, fmt.Errorf("audiobook title required")
		}
		ab.Title = *title
	}
	if narrator != nil {
		ab.Narrator = *narrator
	}
	if asin != nil {
		ab.ASIN = *asin
	}
	if year != nil {
		ab.Year = *year
	}
	if monitored != nil {
		ab.Monitored = *monitored
	}
	m := 0
	if ab.Monitored {
		m = 1
	}
	_, err = s.db.Exec(`
		UPDATE audiobooks SET title = ?, narrator = ?, asin = ?, year = ?, monitored = ?, poster_url = ?
		WHERE id = ?
	`, ab.Title, ab.Narrator, ab.ASIN, ab.Year, m, ab.PosterURL, id)
	if err != nil {
		return nil, fmt.Errorf("update audiobook: %w", err)
	}
	return ab, nil
}

func (s *Store) SetAudiobookPosterURL(id, posterURL string) (*Audiobook, error) {
	if _, err := s.GetAudiobook(id); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`UPDATE audiobooks SET poster_url = ? WHERE id = ?`, posterURL, id); err != nil {
		return nil, fmt.Errorf("update audiobook poster: %w", err)
	}
	return s.GetAudiobook(id)
}

func (s *Store) RemoveAudiobook(id string) error {
	title := id
	if ab, err := s.GetAudiobook(id); err == nil && ab != nil {
		title = ab.Title
	}
	s.appendHistory(id, historyDeleteItem, title, "")
	res, err := s.db.Exec(`DELETE FROM audiobooks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete audiobook: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("audiobook %q not found", id)
	}
	return nil
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
	defer func() { _ = rows.Close() }()
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

func (s *Store) GetAudiobookFile(id string) (*AudiobookFile, error) {
	row := s.db.QueryRow(`
		SELECT id, audiobook_id, author_id, title, path FROM audiobook_files WHERE id = ?
	`, id)
	return scanAudiobookFile(row)
}

func (s *Store) ListAudiobookFilesByAuthor(authorID string) ([]*AudiobookFile, error) {
	rows, err := s.db.Query(`
		SELECT id, audiobook_id, author_id, title, path FROM audiobook_files
		WHERE author_id = ? ORDER BY title
	`, authorID)
	if err != nil {
		return nil, fmt.Errorf("list author files: %w", err)
	}
	defer func() { _ = rows.Close() }()
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

func (s *Store) ListAudiobookFilesByAudiobook(audiobookID string) ([]*AudiobookFile, error) {
	return s.ListAudiobookFiles(audiobookID)
}

// MissingAudiobook is a monitored audiobook with no on-disk files.
type MissingAudiobook struct {
	AudiobookID string
	AuthorID    string
	Title       string
	AuthorName  string
	Year        int32
}

// ListMissingAudiobooks returns monitored audiobooks with no present files on disk.
func (s *Store) ListMissingAudiobooks(page, pageSize int) ([]MissingAudiobook, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 100
	}
	if _, err := s.purgeVanishedFiles(); err != nil {
		return nil, 0, err
	}
	all, err := s.listMissingCandidates()
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	offset := (page - 1) * pageSize
	if offset >= total {
		return []MissingAudiobook{}, total, nil
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (s *Store) listMissingCandidates() ([]MissingAudiobook, error) {
	rows, err := s.db.Query(`
		SELECT ab.id, ab.author_id, ab.title, ab.year, a.name
		FROM audiobooks ab
		JOIN authors a ON a.id = ab.author_id
		WHERE ab.monitored = 1 AND a.monitored = 1
		ORDER BY a.name, ab.title
	`)
	if err != nil {
		return nil, fmt.Errorf("list missing candidates: %w", err)
	}
	candidates := make([]MissingAudiobook, 0)
	for rows.Next() {
		var item MissingAudiobook
		if err := rows.Scan(&item.AudiobookID, &item.AuthorID, &item.Title, &item.Year, &item.AuthorName); err != nil {
			_ = rows.Close()
			return nil, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]MissingAudiobook, 0, len(candidates))
	for _, item := range candidates {
		files, err := s.ListAudiobookFiles(item.AudiobookID)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			out = append(out, item)
			continue
		}
		present := 0
		for _, f := range files {
			if _, err := os.Stat(f.Path); err == nil {
				present++
			}
		}
		if present == 0 {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Store) purgeVanishedFiles() (int, error) {
	files, err := s.ListAudiobookFiles("")
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, f := range files {
		if _, err := os.Stat(f.Path); err != nil {
			if _, err := s.db.Exec(`DELETE FROM audiobook_files WHERE id = ?`, f.ID); err != nil {
				return removed, fmt.Errorf("delete vanished file row: %w", err)
			}
			removed++
		}
	}
	return removed, nil
}

// ImportAudiobookFile attaches an existing on-disk file to an audiobook.
func (s *Store) ImportAudiobookFile(audiobookID, absPath, fileTitle string) (*AudiobookFile, error) {
	ab, err := s.GetAudiobook(audiobookID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if fileTitle == "" {
		fileTitle = strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	}
	f, err := s.upsertAudiobookFile(AudiobookFile{
		AudiobookID: ab.ID,
		AuthorID:    ab.AuthorID,
		Title:       fileTitle,
		Path:        absPath,
	})
	if err != nil {
		return nil, err
	}
	s.appendHistory(ab.ID, historyImport, fileTitle, "")
	return f, nil
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
		SELECT id, author_id, title, narrator, asin, year, duration_seconds, monitored, poster_url FROM audiobooks
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
	if err := row.Scan(&ab.ID, &ab.AuthorID, &ab.Title, &ab.Narrator, &ab.ASIN, &ab.Year, &ab.DurationSeconds, &monitored, &ab.PosterURL); err != nil {
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
