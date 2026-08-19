package main

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestOrderArticlesForMerge_chaptersBeforeVolumes(t *testing.T) {
	// Wikipedia often has both a "volumes" and a "chapters" article for one
	// series. The volumes article numbers entries by position within each
	// volume; the chapters article carries the real chapter numbers. Whichever
	// is merged first keeps the numbering, so the chapters article has to win.
	// Captain Tsubasa's chapters article contributed 0 of its 114 chapters
	// until this ordering existed.
	got := orderArticlesForMerge([]string{
		"List of Captain Tsubasa volumes",
		"List of Captain Tsubasa chapters",
	})
	want := []string{
		"List of Captain Tsubasa chapters",
		"List of Captain Tsubasa volumes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOrderArticlesForMerge_keepsSeriesOrderAndPartOrder(t *testing.T) {
	// Series keep the order they were enumerated in, and the parts of one
	// series keep theirs, so a rebuild stays predictable.
	got := orderArticlesForMerge([]string{
		"List of Zetman chapters",
		"List of Fairy Tail volumes",
		"List of Fairy Tail chapters (volumes 1–15)",
		"List of Fairy Tail chapters (volumes 16–30)",
		"List of Berserk chapters",
	})
	want := []string{
		"List of Zetman chapters",
		"List of Fairy Tail chapters (volumes 1–15)",
		"List of Fairy Tail chapters (volumes 16–30)",
		"List of Fairy Tail volumes",
		"List of Berserk chapters",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestResolveAniListID_negativeOverrideMeansNoID(t *testing.T) {
	// Radiant is a French series AniList does not carry as manga. Its only
	// "matches" are unrelated works that happen to list the name as a synonym,
	// and attaching one pulls another series' chapter titles through Comick.
	// A negative override records "there is deliberately no ID here" so the
	// lookup stops re-attaching junk on every run.
	ovr := emptyOverrides(t)
	ovr.Set("List of Radiant volumes", overrides.Entry{AniListID: -1})

	s := &chaptertitles.Series{
		Series:    "Radiant",
		Article:   "List of Radiant volumes",
		AniListID: 137453,
	}
	resolveAniListID(nil, ovr, s)
	if s.AniListID != 0 {
		t.Errorf("AniListID = %d, want 0 (deliberately unresolved)", s.AniListID)
	}
}

// "fetch" is the only way a series Wikipedia does not file under its chapter-
// list category gets in, and consumers resolve a series through index.json
// alone -- so a series file written without an index row is invisible.

func TestUpsertIndexEntry_registersAFetchedSeries(t *testing.T) {
	dir := t.TempDir()
	if err := chaptertitles.WriteIndex(dir, []chaptertitles.IndexEntry{
		{Series: "Berserk", Slug: "berserk", MatchKey: "berserk", File: "berserk.json", ChapterCount: 381},
	}); err != nil {
		t.Fatal(err)
	}

	s := &chaptertitles.Series{
		Series: "Gachiakuta", Slug: "gachiakuta", MatchKey: "gachiakuta",
		Article: "Gachiakuta", ChapterCount: 174,
	}
	if err := upsertIndexEntry(dir, s); err != nil {
		t.Fatal(err)
	}

	idx, err := chaptertitles.ReadIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Series) != 2 {
		t.Fatalf("index holds %d series, want 2: %+v", len(idx.Series), idx.Series)
	}
	var got *chaptertitles.IndexEntry
	for i, e := range idx.Series {
		if e.Slug == "gachiakuta" {
			got = &idx.Series[i]
		}
	}
	if got == nil {
		t.Fatalf("gachiakuta is not in the index: %+v", idx.Series)
	}
	if got.File != "gachiakuta.json" || got.ChapterCount != 174 {
		t.Errorf("entry = %+v, want file gachiakuta.json with 174 chapters", *got)
	}
}

func TestUpsertIndexEntry_replacesTheRowTheSeriesAlreadyHas(t *testing.T) {
	dir := t.TempDir()
	if err := chaptertitles.WriteIndex(dir, []chaptertitles.IndexEntry{
		{Series: "Gachiakuta", Slug: "gachiakuta", MatchKey: "gachiakuta", File: "gachiakuta.json", ChapterCount: 170},
	}); err != nil {
		t.Fatal(err)
	}

	s := &chaptertitles.Series{
		Series: "Gachiakuta", Slug: "gachiakuta", MatchKey: "gachiakuta",
		Article: "Gachiakuta", ChapterCount: 174,
	}
	if err := upsertIndexEntry(dir, s); err != nil {
		t.Fatal(err)
	}

	idx, err := chaptertitles.ReadIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Series) != 1 {
		t.Fatalf("index holds %d series, want 1 (the row is replaced, not duplicated): %+v", len(idx.Series), idx.Series)
	}
	if idx.Series[0].ChapterCount != 174 {
		t.Errorf("ChapterCount = %d, want the refreshed 174", idx.Series[0].ChapterCount)
	}
}

func TestUpsertIndexEntry_missingIndexIsNotAnError(t *testing.T) {
	dir := t.TempDir()

	s := &chaptertitles.Series{Series: "Gachiakuta", Slug: "gachiakuta", MatchKey: "gachiakuta", ChapterCount: 174}
	if err := upsertIndexEntry(dir, s); err != nil {
		t.Fatalf("upsertIndexEntry on an empty directory: %v", err)
	}

	idx, err := chaptertitles.ReadIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Series) != 1 {
		t.Fatalf("index holds %d series, want 1", len(idx.Series))
	}
}

// A build rewrites the index from the articles it scraped, so a series that is
// in the dataset but not in Wikipedia's category -- everything "fetch" added --
// used to be dropped from the index by the next build, leaving its file
// orphaned in data/.

func TestCarriedIndexEntries_keepsSeriesTheRunDidNotScrape(t *testing.T) {
	dir := t.TempDir()
	if err := chaptertitles.WriteIndex(dir, []chaptertitles.IndexEntry{
		{Series: "Berserk", Slug: "berserk", MatchKey: "berserk", File: "berserk.json", ChapterCount: 381},
		{Series: "Gachiakuta", Slug: "gachiakuta", MatchKey: "gachiakuta", File: "gachiakuta.json", ChapterCount: 174},
	}); err != nil {
		t.Fatal(err)
	}

	scraped := []chaptertitles.IndexEntry{
		{Series: "Berserk", Slug: "berserk", MatchKey: "berserk", File: "berserk.json", ChapterCount: 382},
	}

	carried := carriedIndexEntries(dir, scraped)

	if len(carried) != 1 {
		t.Fatalf("carried %d entries, want 1: %+v", len(carried), carried)
	}
	if carried[0].Slug != "gachiakuta" {
		t.Errorf("carried %q, want gachiakuta (berserk was scraped this run)", carried[0].Slug)
	}
}

func TestCarriedIndexEntries_missingIndexIsNotAnError(t *testing.T) {
	if carried := carriedIndexEntries(t.TempDir(), nil); len(carried) != 0 {
		t.Errorf("carried %+v from an empty directory, want none", carried)
	}
}
