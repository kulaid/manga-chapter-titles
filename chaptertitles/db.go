package chaptertitles

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// DB is a read-only view over a dataset directory, for applications that want
// to look chapter titles up rather than generate them.
//
// Only index.json is read up front; each series file is loaded on first use and
// cached, so an app that touches a handful of series does not pay for the whole
// corpus. A DB is safe for concurrent use.
type DB struct {
	dir     string
	byKey   map[string]IndexEntry // match key -> entry
	entries []IndexEntry

	mu     sync.Mutex
	loaded map[string]*Series // slug -> series
}

// Load opens the dataset in dir by reading its index.
func Load(dir string) (*DB, error) {
	idx, err := ReadIndex(dir)
	if err != nil {
		return nil, fmt.Errorf("loading dataset from %s: %w", dir, err)
	}

	db := &DB{
		dir:     dir,
		byKey:   make(map[string]IndexEntry, len(idx.Series)),
		entries: idx.Series,
		loaded:  make(map[string]*Series),
	}
	for _, e := range idx.Series {
		// The first entry wins, so a later duplicate key cannot displace it.
		if _, exists := db.byKey[e.MatchKey]; !exists {
			db.byKey[e.MatchKey] = e
		}
	}
	return db, nil
}

// Len reports how many series the dataset contains.
func (db *DB) Len() int { return len(db.entries) }

// Entries returns the index rows, for callers that want to enumerate or build
// their own lookup.
func (db *DB) Entries() []IndexEntry { return db.entries }

// Series loads the full record for a series name, matching on the normalised
// key so punctuation and capitalisation differences don't matter.
// It reports false when the dataset has no such series.
func (db *DB) Series(name string) (*Series, bool) {
	entry, ok := db.byKey[MatchKey(name)]
	if !ok {
		return nil, false
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	if s, cached := db.loaded[entry.Slug]; cached {
		return s, true
	}
	s, err := Read(db.dir, entry.Slug)
	if err != nil {
		return nil, false
	}
	db.loaded[entry.Slug] = s
	return s, true
}

// Title returns the title of one chapter of a series, or false when either the
// series or that chapter is absent.
func (db *DB) Title(series string, chapter float64) (string, bool) {
	s, ok := db.Series(series)
	if !ok {
		return "", false
	}
	title, ok := s.Chapters[FormatChapterNumber(chapter)]
	if !ok || title == "" {
		return "", false
	}
	return title, true
}

// Titles returns every chapter title for a series, keyed by chapter number.
// The returned map is a copy, so callers may modify it freely.
func (db *DB) Titles(series string) (map[float64]string, bool) {
	s, ok := db.Series(series)
	if !ok {
		return nil, false
	}
	out := make(map[float64]string, len(s.Chapters))
	for k, v := range s.Chapters {
		n, err := strconv.ParseFloat(k, 64)
		if err != nil {
			continue
		}
		out[n] = v
	}
	return out, true
}

// MatchKey normalises a series name for lookup: lowercase alphanumerics only,
// with a standalone "x" dropped so "Hunter × Hunter" and "Hunter x Hunter"
// agree. Apply it to your own titles before comparing against IndexEntry.
func MatchKey(name string) string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()

	var b strings.Builder
	for _, w := range words {
		if w == "x" && len(words) > 1 {
			continue
		}
		b.WriteString(w)
	}
	return b.String()
}

// Exists reports whether dir looks like a dataset directory.
func Exists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, IndexFile))
	return err == nil
}
