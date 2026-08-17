package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kulaid/manga-chapter-titles/chaptertitles"
	"github.com/kulaid/manga-chapter-titles/internal/overrides"
)

func emptyOverrides(t *testing.T) *overrides.File {
	t.Helper()
	f, err := overrides.Load(filepath.Join(t.TempDir(), "overrides.json"))
	if err != nil {
		t.Fatalf("loading empty overrides: %v", err)
	}
	return f
}

func TestExistingAniListIDs_reusesIDsKeyedByArticle(t *testing.T) {
	dir := t.TempDir()
	err := chaptertitles.WriteIndex(dir, []chaptertitles.IndexEntry{
		{Series: "Chainsaw Man", Slug: "chainsaw-man", Article: "List of Chainsaw Man chapters", AniListID: 105778},
		{Series: "Zetman", Slug: "zetman", Article: "List of Zetman chapters"},
	})
	if err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	ids := existingAniListIDs(dir)

	if got := ids["List of Chainsaw Man chapters"]; got != 105778 {
		t.Errorf("Chainsaw Man = %d, want 105778", got)
	}
	// A series with no ID recorded must not appear, or the build loop would
	// read a zero back and think the lookup had already been done.
	if _, ok := ids["List of Zetman chapters"]; ok {
		t.Errorf("Zetman with no ID should be absent, got entry")
	}
}

func TestExistingAniListIDs_missingDatasetIsNotAnError(t *testing.T) {
	// The first build runs against an empty directory. That is the normal
	// bootstrap path, not a failure.
	ids := existingAniListIDs(filepath.Join(t.TempDir(), "does-not-exist"))
	if len(ids) != 0 {
		t.Errorf("got %d ids from a missing dataset, want 0", len(ids))
	}
}

func TestExistingAniListIDs_malformedIndexIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, chaptertitles.IndexFile), []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("writing index: %v", err)
	}
	// A corrupt index costs a slow rebuild, which is exactly what build does
	// anyway. It must not abort the run.
	if ids := existingAniListIDs(dir); len(ids) != 0 {
		t.Errorf("got %d ids from a malformed index, want 0", len(ids))
	}
}

func TestResolveAniListID_overrideBeatsSeededID(t *testing.T) {
	// Reusing stored IDs must not make a wrong one permanent: overrides.json is
	// the correction path, so it has to win over whatever the dataset carries.
	ovr := emptyOverrides(t)
	ovr.Set("List of Arifureta chapters", overrides.Entry{AniListID: 96834})

	s := &chaptertitles.Series{
		Series:    "Arifureta",
		Article:   "List of Arifureta chapters",
		AniListID: 11111,
	}
	resolveAniListID(nil, ovr, s)
	if s.AniListID != 96834 {
		t.Errorf("AniListID = %d, want the override 96834", s.AniListID)
	}
}

func TestResolveAniListID_seededIDSkipsLookup(t *testing.T) {
	// ani == nil stands in for "the lookup must not happen": if the seeded ID
	// were ignored, the series would come back with 0.
	s := &chaptertitles.Series{Series: "Chainsaw Man", AniListID: 105778}
	resolveAniListID(nil, emptyOverrides(t), s)
	if s.AniListID != 105778 {
		t.Errorf("AniListID = %d, want the seeded 105778", s.AniListID)
	}
}
