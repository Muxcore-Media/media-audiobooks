package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// audio extensions recognized during library scans (local files only; no metadata APIs).
var audioExts = map[string]struct{}{
	".mp3":  {},
	".m4b":  {},
	".m4a":  {},
	".flac": {},
	".aac":  {},
	".ogg":  {},
	".opus": {},
	".wav":  {},
	".wma":  {},
}

// ScanResult summarizes a library root scan.
type ScanResult struct {
	FilesFound    int
	FilesImported int
	FilesSkipped  int
	FilesRemoved  int
}

// ScanLibraryRoot walks root for audio files and upserts authors/audiobooks/files.
// Layout expected: Author/Title/file.ext (Author/file.ext → title from filename).
// Metadata is derived only from path/filename — no network lookups.
func (s *Store) ScanLibraryRoot(root string) (*ScanResult, error) {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("library root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("library root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("library root is not a directory: %s", root)
	}

	res := &ScanResult{}
	removed, err := s.purgeVanishedFiles()
	if err != nil {
		return nil, err
	}
	res.FilesRemoved = removed

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if _, ok := audioExts[ext]; !ok {
			return nil
		}
		res.FilesFound++
		abs, err := filepath.Abs(path)
		if err != nil {
			res.FilesSkipped++
			return nil
		}
		authorName, bookTitle, fileTitle := inferFromPath(root, abs)
		imported, err := s.importAudioFile(authorName, bookTitle, fileTitle, abs)
		if err != nil {
			return err
		}
		if imported {
			res.FilesImported++
		} else {
			res.FilesSkipped++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *Store) importAudioFile(authorName, bookTitle, fileTitle, absPath string) (imported bool, err error) {
	existing, err := s.findAudiobookFileByPath(absPath)
	if err != nil {
		return false, err
	}

	au, err := s.findAuthorByName(authorName)
	if err != nil {
		return false, err
	}
	if au == nil {
		au, err = s.AddAuthor(Author{
			Name:      authorName,
			Monitored: true,
			Path:      filepath.Dir(filepath.Dir(absPath)),
		})
		if err != nil {
			return false, err
		}
	}

	ab, err := s.findAudiobook(au.ID, bookTitle)
	if err != nil {
		return false, err
	}
	if ab == nil {
		ab, err = s.AddAudiobook(Audiobook{
			AuthorID:  au.ID,
			Title:     bookTitle,
			Monitored: true,
		})
		if err != nil {
			return false, err
		}
	}

	_, err = s.upsertAudiobookFile(AudiobookFile{
		AudiobookID: ab.ID,
		AuthorID:    au.ID,
		Title:       fileTitle,
		Path:        absPath,
	})
	if err != nil {
		return false, err
	}
	return existing == nil, nil
}

// inferFromPath derives author/title/file from Author/Title/file.ext under root.
func inferFromPath(root, absPath string) (author, title, fileTitle string) {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		rel = filepath.Base(absPath)
	}
	parts := strings.Split(rel, string(filepath.Separator))
	base := parts[len(parts)-1]
	fileTitle = stripTrackNumber(strings.TrimSuffix(base, filepath.Ext(base)))
	switch len(parts) {
	case 1:
		return "Unknown Author", fileTitle, fileTitle
	case 2:
		return parts[0], fileTitle, fileTitle
	default:
		return parts[0], parts[1], fileTitle
	}
}

func stripTrackNumber(name string) string {
	name = strings.TrimSpace(name)
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 {
		return name
	}
	rest := strings.TrimSpace(name[i:])
	if strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, "_") {
		rest = strings.TrimSpace(rest[1:])
		if rest != "" {
			return rest
		}
	}
	return name
}
