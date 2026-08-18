package main

import (
	"testing"

	"github.com/kulaid/manga-chapter-titles/chaptertitles"
	"github.com/kulaid/manga-chapter-titles/internal/sources"
)

func TestRememberMangaPlusID_recordsTheIDFromTheResultRef(t *testing.T) {
	// The title id costs a MangaDex round trip to derive, so it is persisted and
	// reused. MangaPlus reports it on the result's Ref.
	s := &chaptertitles.Series{Slug: "chainsaw-man"}
	results := []sources.Result{
		{Found: true, Ref: "cm-slug"},
		{Found: true, Ref: "100037"},
	}
	names := []string{sources.NameComick, sources.NameMangaPlus}

	rememberMangaPlusID(s, results, names)

	if s.MangaPlusID != 100037 {
		t.Errorf("MangaPlusID = %d, want 100037", s.MangaPlusID)
	}
}

func TestRememberMangaPlusID_ignoresOtherSourcesRefs(t *testing.T) {
	// Comick's ref is a slug and MangaDex's is a UUID. Neither is a title id,
	// and reading one as a number would attach a nonsense id to the series.
	s := &chaptertitles.Series{Slug: "x"}
	results := []sources.Result{{Found: true, Ref: "12345"}}

	rememberMangaPlusID(s, results, []string{sources.NameComick})

	if s.MangaPlusID != 0 {
		t.Errorf("MangaPlusID = %d, want 0", s.MangaPlusID)
	}
}

func TestRememberMangaPlusID_keepsAKnownIDWhenTheSourceReportsNothing(t *testing.T) {
	// A series MangaPlus has stopped serving still has the same title id.
	s := &chaptertitles.Series{Slug: "x", MangaPlusID: 100037}

	rememberMangaPlusID(s, []sources.Result{{Found: false}}, []string{sources.NameMangaPlus})

	if s.MangaPlusID != 100037 {
		t.Errorf("MangaPlusID = %d, want the stored 100037 kept", s.MangaPlusID)
	}
}

func TestNewMangaPlusClient_seedsTitleIDsFromTheDataset(t *testing.T) {
	// Seeding is what keeps a full run from repeating a MangaDex lookup per
	// series just to re-derive ids the dataset already holds.
	idx := &chaptertitles.Index{Series: []chaptertitles.IndexEntry{
		{Slug: "chainsaw-man", AniListID: 105778, MangaPlusID: 100037},
		{Slug: "berserk", AniListID: 30002},   // no MangaPlus page
		{Slug: "orphan", MangaPlusID: 100999}, // no AniList ID to key on
	}}

	c := newMangaPlusClient(idx)

	if got := c.TitleIDs[105778]; got != 100037 {
		t.Errorf("TitleIDs[105778] = %d, want 100037", got)
	}
	if _, ok := c.TitleIDs[30002]; ok {
		t.Error("a series with no MangaPlus id must not be seeded")
	}
	if len(c.TitleIDs) != 1 {
		t.Errorf("seeded %d ids, want 1: %v", len(c.TitleIDs), c.TitleIDs)
	}
	if c.Resolver == nil {
		t.Error("Resolver = nil; a series not already seeded could never be resolved")
	}
}
