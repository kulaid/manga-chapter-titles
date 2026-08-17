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

func TestMergeSeriesChapters_unionsAcrossArticles(t *testing.T) {
	// One Piece is split over six articles. Each covers a disjoint range, and
	// all of them are the same series.
	dst := &chaptertitles.Series{
		Series:   "One Piece",
		Article:  "List of One Piece chapters (1–186)",
		Chapters: map[string]string{"1": "Romance Dawn", "2": "They Call Him Straw Hat Luffy"},
	}
	src := &chaptertitles.Series{
		Series:          "One Piece",
		Article:         "List of One Piece chapters (187–388)",
		Chapters:        map[string]string{"187": "Ambition", "188": "Sky Island"},
		InferredNumbers: 2,
	}

	if added := mergeSeriesChapters(dst, src); added != 2 {
		t.Errorf("added = %d, want 2", added)
	}

	if got := len(dst.Chapters); got != 4 {
		t.Errorf("merged chapter count = %d, want 4", got)
	}
	if dst.ChapterCount != 4 {
		t.Errorf("ChapterCount = %d, want 4", dst.ChapterCount)
	}
	if dst.Chapters["187"] != "Ambition" {
		t.Errorf("chapter 187 = %q, want %q", dst.Chapters["187"], "Ambition")
	}
	if dst.InferredNumbers != 2 {
		t.Errorf("InferredNumbers = %d, want 2 (summed)", dst.InferredNumbers)
	}
	// The first article stays as the record's provenance.
	if dst.Article != "List of One Piece chapters (1–186)" {
		t.Errorf("Article = %q, want the first article", dst.Article)
	}
}

func TestMergeSeriesChapters_firstArticleWinsOnOverlap(t *testing.T) {
	// Ranges occasionally overlap by a chapter. Whichever article was scraped
	// first keeps the title, so a rebuild is deterministic.
	dst := &chaptertitles.Series{Chapters: map[string]string{"186": "Original"}}
	src := &chaptertitles.Series{Chapters: map[string]string{"186": "Duplicate", "187": "New"}}

	if added := mergeSeriesChapters(dst, src); added != 1 {
		t.Errorf("added = %d, want 1 (186 collided)", added)
	}

	if dst.Chapters["186"] != "Original" {
		t.Errorf("chapter 186 = %q, want %q", dst.Chapters["186"], "Original")
	}
	if dst.Chapters["187"] != "New" {
		t.Errorf("chapter 187 = %q, want %q", dst.Chapters["187"], "New")
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

func TestMergeCollisionWarning(t *testing.T) {
	// Naruto's Part II articles restart numbering at 1, so nearly everything
	// they hold collides with Part I and is dropped. That has to be visible.
	if w := mergeCollisionWarning("List of Naruto chapters (Part II)", 4, 208); w == "" {
		t.Error("expected a warning when a part contributes 4 of 208 chapters")
	}
	// A healthy part of a continuously numbered series says nothing.
	if w := mergeCollisionWarning("List of One Piece chapters (187–388)", 202, 202); w != "" {
		t.Errorf("expected no warning for a clean merge, got %q", w)
	}
	// An article that legitimately holds nothing must not warn either.
	if w := mergeCollisionWarning("Empty", 0, 0); w != "" {
		t.Errorf("expected no warning for an empty part, got %q", w)
	}
}
