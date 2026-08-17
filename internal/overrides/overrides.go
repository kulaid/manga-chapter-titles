// Package overrides stores hand-curated corrections that automatic resolution
// cannot make safely.
//
// Some series genuinely cannot be resolved automatically. Wikipedia titles
// "KonoSuba" what AniList lists as "Kono Subarashii Sekai ni Shukufuku wo!",
// and AniList's search answers that query with spin-offs — so accepting a fuzzy
// match would attach the wrong work's ID. The scraper therefore refuses, and
// the correct value is recorded here by hand instead.
//
// The file lives outside the generated dataset on purpose: `build` rewrites
// data/ from scratch, so a correction written into a series file would be lost
// on the next run. Overrides are applied after automatic resolution on every
// build, which makes them permanent.
//
// Entries are keyed by Wikipedia article title, the one identifier that does
// not change when a series name, slug or match key is adjusted.
package overrides

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultFile is the conventional location of the overrides file.
const DefaultFile = "overrides.json"

// Entry is a manual correction for one article.
type Entry struct {
	// Series is the article's series name, stored purely so the file reads
	// sensibly to a human; the key is the article title.
	Series string `json:"series,omitempty"`
	// AniListID is the hand-verified AniList manga ID.
	AniListID int `json:"anilist_id,omitempty"`
	// AniListTitle is the title that ID resolves to, recorded at the time the
	// override was made so a wrong number is obvious on review rather than
	// indistinguishable from a right one.
	AniListTitle string `json:"anilist_title,omitempty"`
	// Note records why the automatic lookup could not be trusted, so a future
	// reader can tell a deliberate correction from a stale guess.
	Note string `json:"note,omitempty"`
	// Chapters are hand-written chapter titles, keyed the same way the dataset
	// keys them. They beat every scraped source and survive a rebuild; an empty
	// value deletes a wrong title rather than setting one. See ApplyChapters.
	Chapters map[string]string `json:"chapters,omitempty"`
}

// File is the whole overrides file: article title -> correction.
type File struct {
	Overrides map[string]Entry `json:"overrides"`
}

// Load reads the overrides file. A missing file is not an error — it just means
// nothing has been corrected yet.
func Load(path string) (*File, error) {
	f := &File{Overrides: map[string]Entry{}}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if f.Overrides == nil {
		f.Overrides = map[string]Entry{}
	}
	return f, nil
}

// Get returns the correction recorded for an article.
func (f *File) Get(article string) (Entry, bool) {
	if f == nil {
		return Entry{}, false
	}
	e, ok := f.Overrides[strings.TrimSpace(article)]
	return e, ok
}

// Set records a correction, replacing any previous one for that article.
func (f *File) Set(article string, e Entry) {
	if f.Overrides == nil {
		f.Overrides = map[string]Entry{}
	}
	f.Overrides[strings.TrimSpace(article)] = e
}

// Len reports how many corrections are recorded.
func (f *File) Len() int {
	if f == nil {
		return 0
	}
	return len(f.Overrides)
}

// Articles returns the recorded article titles in sorted order.
func (f *File) Articles() []string {
	if f == nil {
		return nil
	}
	out := make([]string, 0, len(f.Overrides))
	for a := range f.Overrides {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Save writes the overrides file atomically, with stable key ordering so the
// committed file produces a readable diff when a correction is added.
func (f *File) Save(path string) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding overrides: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".overrides-*.json")
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
