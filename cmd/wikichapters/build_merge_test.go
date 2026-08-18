package main

import (
	"path/filepath"
	"testing"

	"github.com/kulaid/manga-chapter-titles/chaptertitles"
)

// A rebuild used to replace data/<slug>.json wholesale, which threw away every
// aggregator and curated title and is why "build" could never be run without
// "enrich" behind it. These cover the merge that replaced that.

func TestCarryForward_aggregatorTitlesSurviveARescrape(t *testing.T) {
	prior := &chaptertitles.Series{
		Series:   "Chainsaw Man",
		Slug:     "chainsaw-man",
		MatchKey: "chainsaw man",
		Chapters: map[string]string{
			"1": "Dog & Chainsaw",
			"2": "The Place Where Pochita Is",
		},
		ChapterSources: map[string]string{"1": "wikipedia", "2": "comick"},
	}
	scraped := &chaptertitles.Series{
		Series:   "Chainsaw Man",
		Slug:     "chainsaw-man",
		MatchKey: "chainsaw man",
		Article:  "List of Chainsaw Man chapters",
		Chapters: map[string]string{"1": "Dog & Chainsaw"},
	}

	got := carryForward(prior, scraped)

	if got.Chapters["2"] != "The Place Where Pochita Is" {
		t.Errorf("chapter 2 = %q, want the Comick title to have survived", got.Chapters["2"])
	}
	if got.ChapterSources["2"] != "comick" {
		t.Errorf("chapter 2 provenance = %q, want %q", got.ChapterSources["2"], "comick")
	}
	if got.ChapterCount != 2 {
		t.Errorf("ChapterCount = %d, want 2", got.ChapterCount)
	}
}

func TestCarryForward_wikipediaDisplacesAStoredAggregatorTitle(t *testing.T) {
	// The point of a rescrape: Wikipedia now names a chapter that only Comick
	// named before, and the licensed title is the better one.
	prior := &chaptertitles.Series{
		Slug:           "chainsaw-man",
		MatchKey:       "chainsaw man",
		Chapters:       map[string]string{"5": "A Way to Touch Some Boobs"},
		ChapterSources: map[string]string{"5": "comick"},
	}
	scraped := &chaptertitles.Series{
		Slug:     "chainsaw-man",
		MatchKey: "chainsaw man",
		Article:  "List of Chainsaw Man chapters",
		Chapters: map[string]string{"5": "Grab the Eyes"},
	}

	got := carryForward(prior, scraped)

	if got.Chapters["5"] != "Grab the Eyes" {
		t.Errorf("chapter 5 = %q, want the Wikipedia title", got.Chapters["5"])
	}
	if got.ChapterSources["5"] != "wikipedia" {
		t.Errorf("chapter 5 provenance = %q, want %q", got.ChapterSources["5"], "wikipedia")
	}
}

func TestCarryForward_curatedTitleBeatsAFreshScrape(t *testing.T) {
	// Curated titles are the answer to "Wikipedia is wrong here", so a rescrape
	// must not undo one.
	prior := &chaptertitles.Series{
		Slug:           "vinland-saga",
		MatchKey:       "vinland saga",
		Chapters:       map[string]string{"175.5": "Assassin's Creed: Valhalla x Vinland Saga"},
		ChapterSources: map[string]string{"175.5": "curated"},
	}
	scraped := &chaptertitles.Series{
		Slug:     "vinland-saga",
		MatchKey: "vinland saga",
		Article:  "List of Vinland Saga chapters",
		Chapters: map[string]string{"175.5": "Extra"},
	}

	got := carryForward(prior, scraped)

	if got.Chapters["175.5"] != "Assassin's Creed: Valhalla x Vinland Saga" {
		t.Errorf("chapter 175.5 = %q, want the curated title", got.Chapters["175.5"])
	}
}

func TestCarryForward_keepsAniListIDAndTakesFreshArticleMetadata(t *testing.T) {
	// The ID is expensive and unstable to re-derive, so it is carried over; the
	// article metadata is what the rescrape is for, so that is refreshed.
	prior := &chaptertitles.Series{
		Series:    "Vinland Saga",
		Slug:      "vinland-saga",
		MatchKey:  "vinland saga",
		AniListID: 30642,
		Article:   "List of Vinland Saga chapters (old title)",
		SourceURL: "https://en.wikipedia.org/wiki/Old",
		Chapters:  map[string]string{"1": "Normanni"},
	}
	scraped := &chaptertitles.Series{
		Series:          "Vinland Saga",
		Slug:            "vinland-saga",
		MatchKey:        "vinland saga",
		Article:         "List of Vinland Saga chapters",
		SourceURL:       "https://en.wikipedia.org/wiki/New",
		InferredNumbers: 3,
		Chapters:        map[string]string{"1": "Normanni"},
	}

	got := carryForward(prior, scraped)

	if got.AniListID != 30642 {
		t.Errorf("AniListID = %d, want 30642 carried over", got.AniListID)
	}
	if got.Article != "List of Vinland Saga chapters" {
		t.Errorf("Article = %q, want the freshly scraped one", got.Article)
	}
	if got.SourceURL != "https://en.wikipedia.org/wiki/New" {
		t.Errorf("SourceURL = %q, want the freshly scraped one", got.SourceURL)
	}
	if got.InferredNumbers != 3 {
		t.Errorf("InferredNumbers = %d, want 3 from the fresh scrape", got.InferredNumbers)
	}
}

func TestCarryForward_freshScrapeSeedsAniListIDWhenPriorHasNone(t *testing.T) {
	prior := &chaptertitles.Series{Slug: "x", MatchKey: "x"}
	scraped := &chaptertitles.Series{Slug: "x", MatchKey: "x", AniListID: 123, Chapters: map[string]string{"1": "One"}}

	if got := carryForward(prior, scraped); got.AniListID != 123 {
		t.Errorf("AniListID = %d, want 123", got.AniListID)
	}
}

func TestPriorSeriesFor_findsARenamedSeriesByMatchKey(t *testing.T) {
	// The parser fixes rename articles, so the slug can move. Looking the prior
	// record up by match key finds it anyway and keeps its enrichment.
	dir := t.TempDir()
	prior := &chaptertitles.Series{
		Series: "One Piece", Slug: "one-piece-2", MatchKey: "one piece",
		Chapters:       map[string]string{"1": "Romance Dawn"},
		ChapterSources: map[string]string{"1": "comick"},
	}
	if err := chaptertitles.Write(dir, prior); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := chaptertitles.WriteIndex(dir, []chaptertitles.IndexEntry{
		{Series: "One Piece", Slug: "one-piece-2", MatchKey: "one piece", File: "one-piece-2.json"},
	}); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	byKey := priorSlugsByMatchKey(dir)
	got := priorSeriesFor(dir, &chaptertitles.Series{Slug: "one-piece", MatchKey: "one piece"}, byKey)

	if got == nil {
		t.Fatal("priorSeriesFor returned nil, want the renamed record")
	}
	if got.Chapters["1"] != "Romance Dawn" {
		t.Errorf("chapter 1 = %q", got.Chapters["1"])
	}
}

func TestPriorSeriesFor_refusesADifferentSeriesOnTheSameSlug(t *testing.T) {
	// usedSlugs disambiguation means two genuinely distinct series can sit on
	// neighbouring slugs. Merging one into the other would graft another manga's
	// chapter titles onto this one, which is the failure this dataset exists to
	// avoid.
	dir := t.TempDir()
	other := &chaptertitles.Series{
		Series: "Gantz", Slug: "gantz", MatchKey: "gantz",
		Chapters: map[string]string{"1": "Not This Series"},
	}
	if err := chaptertitles.Write(dir, other); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := priorSeriesFor(dir, &chaptertitles.Series{Slug: "gantz", MatchKey: "gantz g"}, nil)

	if got != nil {
		t.Errorf("priorSeriesFor = %+v, want nil for a mismatched match key", got)
	}
}

func TestPriorSeriesFor_missingDatasetIsNotAnError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope")
	if got := priorSeriesFor(dir, &chaptertitles.Series{Slug: "x", MatchKey: "x"}, nil); got != nil {
		t.Errorf("got %+v from a missing dataset, want nil", got)
	}
}

func TestMergeSeriesChapters_licensedPartDisplacesAStoredAggregatorTitle(t *testing.T) {
	// A split series' second article carries licensed titles for chapters the
	// aggregators already named. Blocking it on "already present" would keep the
	// scanlator title forever.
	dst := &chaptertitles.Series{
		Chapters:       map[string]string{"200": "Scanlator Title"},
		ChapterSources: map[string]string{"200": "comick"},
	}
	src := &chaptertitles.Series{Chapters: map[string]string{"200": "Licensed Title"}}

	if added := mergeSeriesChapters(dst, src); added != 1 {
		t.Errorf("added = %d, want 1", added)
	}
	if dst.Chapters["200"] != "Licensed Title" {
		t.Errorf("chapter 200 = %q, want the licensed title", dst.Chapters["200"])
	}
	if dst.ChapterSources["200"] != "wikipedia" {
		t.Errorf("provenance = %q, want %q", dst.ChapterSources["200"], "wikipedia")
	}
}

func TestMergeSeriesChapters_recordsProvenanceForAddedChapters(t *testing.T) {
	dst := &chaptertitles.Series{Chapters: map[string]string{}}
	src := &chaptertitles.Series{Chapters: map[string]string{"187": "Ambition"}}

	mergeSeriesChapters(dst, src)

	if dst.ChapterSources["187"] != "wikipedia" {
		t.Errorf("provenance = %q, want %q", dst.ChapterSources["187"], "wikipedia")
	}
}

func TestMergeSeriesChapters_curatedTitleSurvivesAPart(t *testing.T) {
	dst := &chaptertitles.Series{
		Chapters:       map[string]string{"175.5": "Curated"},
		ChapterSources: map[string]string{"175.5": "curated"},
	}
	src := &chaptertitles.Series{Chapters: map[string]string{"175.5": "Scraped"}}

	if added := mergeSeriesChapters(dst, src); added != 0 {
		t.Errorf("added = %d, want 0", added)
	}
	if dst.Chapters["175.5"] != "Curated" {
		t.Errorf("chapter 175.5 = %q, want the curated title", dst.Chapters["175.5"])
	}
}

func TestCarryForward_freshScrapeCorrectsAStoredWikipediaTitle(t *testing.T) {
	// A rebuild exists to pick up parser fixes. The old parser let a section
	// heading leak in as Monster's chapter 87 title; the fixed one reads the
	// real title. Wikipedia re-read is the same source, not a competing one, so
	// the fresh value has to replace the stored one rather than tie with it.
	prior := &chaptertitles.Series{
		Slug: "monster", MatchKey: "monster",
		Chapters:       map[string]string{"87": "Monster Chronicle"},
		ChapterSources: map[string]string{"87": "wikipedia"},
	}
	scraped := &chaptertitles.Series{
		Slug: "monster", MatchKey: "monster",
		Article:  "List of Monster chapters",
		Chapters: map[string]string{"87": "Double Darkness"},
	}

	got := carryForward(prior, scraped)

	if got.Chapters["87"] != "Double Darkness" {
		t.Errorf("chapter 87 = %q, want the corrected title %q", got.Chapters["87"], "Double Darkness")
	}
}

func TestCarryForward_dropsWikipediaChaptersTheFreshScrapeNoLongerHas(t *testing.T) {
	// The numbering fixes renumber chapters. Keeping the old key alongside the
	// new one would leave the same chapter in the file twice under two numbers,
	// so Wikipedia's share is replaced wholesale rather than added to.
	prior := &chaptertitles.Series{
		Slug: "x", MatchKey: "x",
		Chapters:       map[string]string{"1": "First", "244": "Misnumbered"},
		ChapterSources: map[string]string{"1": "wikipedia", "244": "wikipedia"},
	}
	scraped := &chaptertitles.Series{
		Slug: "x", MatchKey: "x", Article: "List of X chapters",
		Chapters: map[string]string{"1": "First", "245": "Correctly Numbered"},
	}

	got := carryForward(prior, scraped)

	if _, still := got.Chapters["244"]; still {
		t.Errorf("chapter 244 survived; want the stale Wikipedia numbering dropped")
	}
	if got.Chapters["245"] != "Correctly Numbered" {
		t.Errorf("chapter 245 = %q", got.Chapters["245"])
	}
}

func TestCarryForward_replacingWikipediaKeepsOtherSources(t *testing.T) {
	// Replacing Wikipedia's share must not touch anyone else's: the aggregator
	// and curated titles are the whole reason build stopped overwriting.
	prior := &chaptertitles.Series{
		Slug: "x", MatchKey: "x",
		Chapters: map[string]string{
			"1": "Stale Wiki", "2": "From Comick", "3": "Hand Written", "4": "From MangaDex",
		},
		ChapterSources: map[string]string{
			"1": "wikipedia", "2": "comick", "3": "curated", "4": "mangadex",
		},
	}
	scraped := &chaptertitles.Series{
		Slug: "x", MatchKey: "x", Article: "List of X chapters",
		Chapters: map[string]string{"1": "Fresh Wiki", "3": "Scraped Over Curated"},
	}

	got := carryForward(prior, scraped)

	want := map[string]string{
		"1": "Fresh Wiki", "2": "From Comick", "3": "Hand Written", "4": "From MangaDex",
	}
	for k, v := range want {
		if got.Chapters[k] != v {
			t.Errorf("chapter %s = %q, want %q", k, got.Chapters[k], v)
		}
	}
	if got.ChapterCount != 4 {
		t.Errorf("ChapterCount = %d, want 4", got.ChapterCount)
	}
}

func TestCarryForward_unattributedPriorTitlesCountAsWikipedia(t *testing.T) {
	// The ten files with no chapter_sources are Wikipedia-only. Their titles are
	// Wikipedia's share and must be refreshed like any other.
	prior := &chaptertitles.Series{
		Slug: "pokemon-adventures", MatchKey: "pokemon adventures",
		Article:  "List of Pokemon Adventures chapters",
		Chapters: map[string]string{"1": "Stale"},
	}
	scraped := &chaptertitles.Series{
		Slug: "pokemon-adventures", MatchKey: "pokemon adventures",
		Article:  "List of Pokemon Adventures chapters",
		Chapters: map[string]string{"1": "A Glimpse of the Glow"},
	}

	got := carryForward(prior, scraped)

	if got.Chapters["1"] != "A Glimpse of the Glow" {
		t.Errorf("chapter 1 = %q, want the fresh title", got.Chapters["1"])
	}
}
