package chaptertitles

import (
	"testing"
	"time"
)

// newTestDataset writes a small dataset to a temp dir and returns its path.
func newTestDataset(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	series := []*Series{
		{
			Series: "Chainsaw Man", Slug: "chainsaw-man", MatchKey: "chainsawman",
			ScrapedAt: time.Now().UTC(), ChapterCount: 2,
			Chapters: map[string]string{"1": "Dog & Chainsaw", "4.5": "Interlude"},
		},
		{
			Series: "Hunter × Hunter", Slug: "hunter-hunter", MatchKey: "hunterhunter",
			ScrapedAt: time.Now().UTC(), ChapterCount: 1,
			Chapters: map[string]string{"1": "The Day of Departure"},
		},
	}

	var entries []IndexEntry
	for _, s := range series {
		if err := Write(dir, s); err != nil {
			t.Fatalf("Write %s: %v", s.Slug, err)
		}
		entries = append(entries, IndexEntry{
			Series: s.Series, Slug: s.Slug, MatchKey: s.MatchKey,
			File: s.Slug + ".json", ChapterCount: s.ChapterCount,
		})
	}
	if err := WriteIndex(dir, entries); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	return dir
}

func TestDBTitle(t *testing.T) {
	db, err := Load(newTestDataset(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if db.Len() != 2 {
		t.Errorf("Len = %d, want 2", db.Len())
	}

	tests := []struct {
		name    string
		series  string
		chapter float64
		want    string
		wantOK  bool
	}{
		{name: "exact name", series: "Chainsaw Man", chapter: 1, want: "Dog & Chainsaw", wantOK: true},
		{name: "half chapter", series: "Chainsaw Man", chapter: 4.5, want: "Interlude", wantOK: true},
		{name: "case and punctuation insensitive", series: "chainsaw-man!", chapter: 1, want: "Dog & Chainsaw", wantOK: true},
		{name: "stylised x", series: "Hunter x Hunter", chapter: 1, want: "The Day of Departure", wantOK: true},
		{name: "unknown series", series: "Nonexistent", chapter: 1, wantOK: false},
		{name: "unknown chapter", series: "Chainsaw Man", chapter: 999, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := db.Title(tt.series, tt.chapter)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tt.wantOK, got)
			}
			if ok && got != tt.want {
				t.Errorf("Title() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDBTitles(t *testing.T) {
	db, err := Load(newTestDataset(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	titles, ok := db.Titles("Chainsaw Man")
	if !ok {
		t.Fatal("Titles() reported the series missing")
	}
	if titles[1] != "Dog & Chainsaw" || titles[4.5] != "Interlude" {
		t.Errorf("Titles() = %v", titles)
	}

	// The returned map must be a copy, not the cached one.
	titles[1] = "mutated"
	again, _ := db.Titles("Chainsaw Man")
	if again[1] != "Dog & Chainsaw" {
		t.Error("mutating the returned map corrupted the cached series")
	}
}

func TestLoadMissingDataset(t *testing.T) {
	if _, err := Load(t.TempDir()); err == nil {
		t.Error("expected an error loading a directory with no index.json")
	}
}

func TestExists(t *testing.T) {
	if Exists(t.TempDir()) {
		t.Error("Exists reported true for an empty directory")
	}
	if !Exists(newTestDataset(t)) {
		t.Error("Exists reported false for a real dataset")
	}
}

func TestMatchKey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Hunter × Hunter", "hunterhunter"},
		{"Hunter x Hunter", "hunterhunter"},
		{"Kaguya-sama: Love Is War", "kaguyasamaloveiswar"},
		{"X", "x"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := MatchKey(tt.in); got != tt.want {
			t.Errorf("MatchKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
