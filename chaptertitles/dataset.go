// Package dataset defines the on-disk JSON format produced by this tool and
// the helpers for reading and writing it.
//
// The layout is one file per series plus a single index:
//
//	data/index.json
//	data/chainsaw-man.json
//	data/berserk.json
//
// Chapter numbers are object keys rather than array positions because they are
// sparse and not always integers ("4.5" half-chapters are common).
package chaptertitles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// IndexFile is the name of the index within the data directory.
const IndexFile = "index.json"

// Series is one scraped series, written to data/<slug>.json.
type Series struct {
	// Series is the human-readable series name, e.g. "Chainsaw Man".
	Series string `json:"series"`
	// Slug is the filename stem and stable identifier.
	Slug string `json:"slug"`
	// MatchKey is Series normalised for lookup; see wikipedia.NormalizeName.
	MatchKey string `json:"match_key"`
	// AniListID is the series' AniList manga ID, or 0 when it could not be
	// confirmed. Consumers that already hold an AniList ID should match on it
	// rather than on MatchKey, since it is exact.
	AniListID int `json:"anilist_id,omitempty"`
	// Article is the Wikipedia article the titles came from.
	Article string `json:"article"`
	// SourceURL is the canonical URL of that article.
	SourceURL string `json:"source_url"`
	// ScrapedAt is when this file was last written (UTC, RFC 3339).
	ScrapedAt time.Time `json:"scraped_at"`
	// ChapterCount is len(Chapters), duplicated for convenience.
	ChapterCount int `json:"chapter_count"`
	// InferredNumbers counts chapters whose number came from list position
	// rather than the article. A non-zero value means the numbering for those
	// entries is a best effort — see the README.
	InferredNumbers int `json:"inferred_numbers"`
	// Chapters maps a chapter number, as a decimal string, to its title.
	Chapters map[string]string `json:"chapters"`
	// ChapterSources records which source each title came from, keyed the same
	// way as Chapters. It exists so a curator can see why a title looks the way
	// it does — a licensed Wikipedia title reads differently from a scanlator
	// one — and can be ignored by consumers that only want the titles.
	ChapterSources map[string]string `json:"chapter_sources,omitempty"`
	// Sources lists every source consulted for this series, in the priority
	// order they were merged.
	Sources []SourceRef `json:"sources,omitempty"`
}

// SourceRef records one source's contribution to a series.
type SourceRef struct {
	// Name is the source identifier, e.g. "wikipedia", "comick", "mangadex".
	Name string `json:"name"`
	// Ref identifies the series within that source (an article title, a
	// MangaDex UUID, a Comick slug).
	Ref string `json:"ref,omitempty"`
	// URL links to that source's page for the series.
	URL string `json:"url,omitempty"`
	// Count is how many titles this source supplied that no higher-priority
	// source already had.
	Count int `json:"count"`
}

// IndexEntry is one row of index.json.
type IndexEntry struct {
	Series       string `json:"series"`
	Slug         string `json:"slug"`
	MatchKey     string `json:"match_key"`
	AniListID    int    `json:"anilist_id,omitempty"`
	File         string `json:"file"`
	Article      string `json:"article"`
	ChapterCount int    `json:"chapter_count"`
	// SourceNames lists the sources that contributed titles, so the index alone
	// shows the coverage without opening every series file.
	SourceNames []string `json:"sources,omitempty"`
}

// Index is the contents of index.json.
type Index struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Count       int          `json:"count"`
	Series      []IndexEntry `json:"series"`
}

// FormatChapterNumber renders a chapter number as a compact decimal string,
// so 12 becomes "12" and 12.5 becomes "12.5".
func FormatChapterNumber(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// Slugify converts a series name into a filesystem- and URL-safe stem.
// Accented letters are folded to ASCII first, so "Boku wa Imōto ni Koi o Suru"
// becomes "boku-wa-imoto-..." rather than losing the ō to a separator.
func Slugify(name string) string {
	var b strings.Builder
	lastDash := true // leading dashes are suppressed
	for _, r := range strings.ToLower(FoldAccents(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// Write saves a series to <dir>/<slug>.json, creating dir if needed.
func Write(dir string, s *Series) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, s.Slug+".json")
	return writeJSON(path, s)
}

// WriteIndex saves the index of every series file in dir.
func WriteIndex(dir string, entries []IndexEntry) error {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	idx := Index{
		GeneratedAt: time.Now().UTC(),
		Count:       len(entries),
		Series:      entries,
	}
	return writeJSON(filepath.Join(dir, IndexFile), idx)
}

// ReadIndex loads index.json from dir.
func ReadIndex(dir string) (*Index, error) {
	data, err := os.ReadFile(filepath.Join(dir, IndexFile))
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}
	return &idx, nil
}

// Read loads a single series file by slug.
func Read(dir, slug string) (*Series, error) {
	data, err := os.ReadFile(filepath.Join(dir, slug+".json"))
	if err != nil {
		return nil, err
	}
	var s Series
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", slug, err)
	}
	return &s, nil
}

// writeJSON marshals v with indentation and writes it atomically, so a crashed
// or cancelled build never leaves a half-written file behind.
func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	return os.Rename(tmpName, path)
}
