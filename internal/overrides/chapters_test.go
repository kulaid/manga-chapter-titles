package overrides

import "testing"

func TestApplyChapters_curatedTitlesWin(t *testing.T) {
	// The dataset is a database of chapter titles; scraping is one way data
	// gets into it, not the authority. A curated title beats whatever the
	// scrapers produced.
	f := &File{Overrides: map[string]Entry{
		"List of Berserk chapters": {
			Series:   "Berserk",
			Chapters: map[string]string{"0.01": "The Black Swordsman", "1": "A Wind of Swords"},
		},
	}}

	chapters := map[string]string{"0.01": "Prologue 1", "1": "A Wind of Swords", "2": "Nosferatu Zodd (1)"}
	sources := map[string]string{"0.01": "comick", "1": "wikipedia", "2": "wikipedia"}

	changed := f.ApplyChapters("List of Berserk chapters", chapters, sources)

	if chapters["0.01"] != "The Black Swordsman" {
		t.Errorf("0.01 = %q, want the curated title", chapters["0.01"])
	}
	if sources["0.01"] != SourceCurated {
		t.Errorf("0.01 source = %q, want %q", sources["0.01"], SourceCurated)
	}
	// Untouched chapters keep their scraped provenance.
	if sources["2"] != "wikipedia" {
		t.Errorf("2 source = %q, want wikipedia", sources["2"])
	}
	// A curated title identical to the scraped one still counts as curated,
	// so a later scrape cannot quietly move it.
	if sources["1"] != SourceCurated {
		t.Errorf("1 source = %q, want %q", sources["1"], SourceCurated)
	}
	if changed != 2 {
		t.Errorf("changed = %d, want 2", changed)
	}
}

func TestApplyChapters_addsChaptersNoSourceHas(t *testing.T) {
	// Vinland Saga's volume-end extras are numbered one lower in the releases
	// than Comick numbers them, so the title exists but lands on a chapter the
	// library does not have. Writing it at the right number is a data fix.
	f := &File{Overrides: map[string]Entry{
		"List of Vinland Saga chapters": {
			Chapters: map[string]string{"175.5": "Assassin's Creed: Valhalla x Vinland Saga"},
		},
	}}
	chapters := map[string]string{"175": "Voyage to the West (9)"}
	sources := map[string]string{"175": "wikipedia"}

	f.ApplyChapters("List of Vinland Saga chapters", chapters, sources)

	if chapters["175.5"] != "Assassin's Creed: Valhalla x Vinland Saga" {
		t.Errorf("175.5 = %q, want the curated title", chapters["175.5"])
	}
}

func TestApplyChapters_emptyValueDeletes(t *testing.T) {
	// An empty curated title removes a wrong one outright, which is the only
	// way to say "no source has a title for this and the one on file is junk".
	f := &File{Overrides: map[string]Entry{
		"A": {Chapters: map[string]string{"176.5": ""}},
	}}
	chapters := map[string]string{"176.5": "Assassin's Creed: Valhalla x Vinland Saga"}
	sources := map[string]string{"176.5": "comick"}

	f.ApplyChapters("A", chapters, sources)

	if _, exists := chapters["176.5"]; exists {
		t.Errorf("176.5 = %q, want it removed", chapters["176.5"])
	}
	if _, exists := sources["176.5"]; exists {
		t.Error("176.5 provenance should be removed too")
	}
}

func TestApplyChapters_unknownArticleIsANoop(t *testing.T) {
	f := &File{Overrides: map[string]Entry{}}
	chapters := map[string]string{"1": "Original"}
	if changed := f.ApplyChapters("Nothing", chapters, map[string]string{}); changed != 0 {
		t.Errorf("changed = %d, want 0", changed)
	}
	if chapters["1"] != "Original" {
		t.Errorf("chapter 1 = %q, want it untouched", chapters["1"])
	}
}

func TestApplyChapters_nilReceiverIsSafe(t *testing.T) {
	var f *File
	if changed := f.ApplyChapters("A", map[string]string{}, map[string]string{}); changed != 0 {
		t.Errorf("changed = %d, want 0", changed)
	}
}
