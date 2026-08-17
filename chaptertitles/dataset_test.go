package chaptertitles

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Chainsaw Man", "chainsaw-man"},
		{"Hunter × Hunter", "hunter-hunter"},
		{"Kaguya-sama: Love Is War", "kaguya-sama-love-is-war"},
		{"7 Seeds", "7-seeds"},
		{"  Spaced  Out  ", "spaced-out"},
		{"???", ""},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatChapterNumber(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{1, "1"},
		{12, "12"},
		{4.5, "4.5"},
		{167.5, "167.5"},
	}
	for _, tt := range tests {
		if got := FormatChapterNumber(tt.in); got != tt.want {
			t.Errorf("FormatChapterNumber(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	want := &Series{
		Series:       "Chainsaw Man",
		Slug:         "chainsaw-man",
		MatchKey:     "chainsawman",
		Article:      "List of Chainsaw Man chapters",
		SourceURL:    "https://en.wikipedia.org/wiki/List_of_Chainsaw_Man_chapters",
		ScrapedAt:    time.Now().UTC().Truncate(time.Second),
		ChapterCount: 2,
		Chapters:     map[string]string{"1": "Dog & Chainsaw", "4.5": "Interlude"},
	}

	if err := Write(dir, want); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(dir, "chainsaw-man")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Series != want.Series || got.ChapterCount != want.ChapterCount {
		t.Errorf("round trip changed metadata: %+v", got)
	}
	if got.Chapters["1"] != "Dog & Chainsaw" || got.Chapters["4.5"] != "Interlude" {
		t.Errorf("round trip changed chapters: %v", got.Chapters)
	}
}

func TestWriteIndexAndRead(t *testing.T) {
	dir := t.TempDir()

	// Deliberately out of order; WriteIndex should sort by slug.
	entries := []IndexEntry{
		{Series: "Zom 100", Slug: "zom-100", MatchKey: "zom100", File: "zom-100.json", ChapterCount: 60},
		{Series: "Berserk", Slug: "berserk", MatchKey: "berserk", File: "berserk.json", ChapterCount: 381},
	}
	if err := WriteIndex(dir, entries); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	idx, err := ReadIndex(dir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if idx.Count != 2 {
		t.Errorf("Count = %d, want 2", idx.Count)
	}
	if idx.Series[0].Slug != "berserk" {
		t.Errorf("index not sorted by slug: %s first", idx.Series[0].Slug)
	}
}

func TestWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := &Series{Slug: "x", Series: "X", Chapters: map[string]string{"1": "One"}}
	if err := Write(dir, s); err != nil {
		t.Fatalf("Write: %v", err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".tmp-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}
