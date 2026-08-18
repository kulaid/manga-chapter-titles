package sources

import "testing"

func TestRankTable(t *testing.T) {
	// Lower is better. Curated titles are decided by a person and must outrank
	// every scraper; anything unrecognised sorts last so an unknown name can
	// never displace a known one.
	tests := []struct {
		name string
		want int
	}{
		{"curated", 0},
		{"wikipedia", 1},
		{"mangaplus", 2},
		{"comick", 3},
		{"mangadex", 4},
		{"", RankUnknown},
		{"something-new", RankUnknown},
	}

	for _, tt := range tests {
		if got := Rank(tt.name); got != tt.want {
			t.Errorf("Rank(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestRankIsStrictlyOrdered(t *testing.T) {
	order := []string{"curated", "wikipedia", "mangaplus", "comick", "mangadex"}
	for i := 1; i < len(order); i++ {
		if Rank(order[i-1]) >= Rank(order[i]) {
			t.Errorf("Rank(%q)=%d is not better than Rank(%q)=%d",
				order[i-1], Rank(order[i-1]), order[i], Rank(order[i]))
		}
	}
	if Rank("mangadex") >= RankUnknown {
		t.Error("a known source must outrank an unknown one")
	}
}

func TestMergeBetterRankDisplacesStored(t *testing.T) {
	// The whole point of ranking: MangaPlus captured a title weeks ago, and now
	// Wikipedia has the licensed one. Wikipedia is better, so it wins.
	stored := Stored{
		Titles:     Titles{1: "DOG & CHAINSAW"},
		Provenance: map[float64]string{1: "mangaplus"},
	}
	results := []Result{{Found: true, Titles: Titles{1: "Dog & Chainsaw"}}}

	m := Merge(stored, results, []string{"wikipedia"})

	if m.Titles[1] != "Dog & Chainsaw" {
		t.Errorf("chapter 1 = %q, want the better-ranked Wikipedia title", m.Titles[1])
	}
	if m.Provenance[1] != "wikipedia" {
		t.Errorf("chapter 1 provenance = %q, want %q", m.Provenance[1], "wikipedia")
	}
}

func TestMergeWorseRankCannotDisplaceStored(t *testing.T) {
	// Comick and MangaDex carry scanlator titles. Neither may ever overwrite a
	// stored MangaPlus title, which is an official one.
	stored := Stored{
		Titles:     Titles{1: "Dog & Chainsaw"},
		Provenance: map[float64]string{1: "mangaplus"},
	}
	results := []Result{
		{Found: true, Titles: Titles{1: "A Dog and a Chainsaw"}},
		{Found: true, Titles: Titles{1: "Dog n Chainsaw"}},
	}

	m := Merge(stored, results, []string{"comick", "mangadex"})

	if m.Titles[1] != "Dog & Chainsaw" {
		t.Errorf("chapter 1 = %q, want the stored MangaPlus title to survive", m.Titles[1])
	}
	if m.Provenance[1] != "mangaplus" {
		t.Errorf("chapter 1 provenance = %q, want %q", m.Provenance[1], "mangaplus")
	}
}

func TestMergeEqualRankKeepsStored(t *testing.T) {
	// Re-running a source over its own stored titles must not churn the file,
	// so an equal rank is not "strictly better" and loses.
	stored := Stored{
		Titles:     Titles{1: "Stored Wording"},
		Provenance: map[float64]string{1: "comick"},
	}
	results := []Result{{Found: true, Titles: Titles{1: "Reworded Upstream"}}}

	m := Merge(stored, results, []string{"comick"})

	if m.Titles[1] != "Stored Wording" {
		t.Errorf("chapter 1 = %q, want the stored title at equal rank", m.Titles[1])
	}
}

func TestMergeStoredMangaPlusTitleSurvivesAnEmptyRun(t *testing.T) {
	// MangaPlus's free window slides forward, so a title captured today is gone
	// from the API within weeks. Retention is what makes the source worth having:
	// a run where MangaPlus returns nothing must not lose what it gave us before.
	stored := Stored{
		Titles:     Titles{1190: "The One Piece"},
		Provenance: map[float64]string{1190: "mangaplus"},
	}
	results := []Result{
		{Found: false}, // mangaplus no longer serves this chapter
		{Found: true, Titles: Titles{1190: "the one piece!!"}},
	}

	m := Merge(stored, results, []string{"mangaplus", "comick"})

	if m.Titles[1190] != "The One Piece" {
		t.Errorf("chapter 1190 = %q, want the stored MangaPlus title retained", m.Titles[1190])
	}
	if m.Provenance[1190] != "mangaplus" {
		t.Errorf("chapter 1190 provenance = %q, want %q", m.Provenance[1190], "mangaplus")
	}
}

func TestMergeCuratedOutranksEverySource(t *testing.T) {
	// Curated titles are the answer to "the sources are wrong here". Nothing
	// automatic may displace one, not even Wikipedia.
	stored := Stored{
		Titles:     Titles{175.5: "Assassin's Creed: Valhalla x Vinland Saga"},
		Provenance: map[float64]string{175.5: "curated"},
	}
	results := []Result{{Found: true, Titles: Titles{175.5: "Extra Chapter"}}}

	m := Merge(stored, results, []string{"wikipedia"})

	if m.Titles[175.5] != "Assassin's Creed: Valhalla x Vinland Saga" {
		t.Errorf("chapter 175.5 = %q, want the curated title to win", m.Titles[175.5])
	}
	if m.Provenance[175.5] != "curated" {
		t.Errorf("chapter 175.5 provenance = %q, want %q", m.Provenance[175.5], "curated")
	}
}

func TestMergeUnrecordedProvenanceFallsBackToDefault(t *testing.T) {
	// Ten series in the dataset carry no chapter_sources at all: they are
	// Wikipedia-only files that enrichment skipped for want of an AniList ID.
	// Treating their titles as unknown-ranked would let Comick overwrite 617
	// licensed Pokemon Adventures titles the moment one gets an ID pinned.
	stored := Stored{
		Titles:        Titles{1: "A Glimpse of the Glow"},
		DefaultSource: "wikipedia",
	}
	results := []Result{{Found: true, Titles: Titles{1: "Vs. Mew"}}}

	m := Merge(stored, results, []string{"comick"})

	if m.Titles[1] != "A Glimpse of the Glow" {
		t.Errorf("chapter 1 = %q, want the unrecorded title treated as %q",
			m.Titles[1], "wikipedia")
	}
	if m.Provenance[1] != "wikipedia" {
		t.Errorf("chapter 1 provenance = %q, want the default recorded", m.Provenance[1])
	}
}

func TestMergeRecordedProvenanceBeatsDefault(t *testing.T) {
	// The default only covers chapters with nothing recorded; a chapter that
	// names its source uses that, even when the default is better.
	stored := Stored{
		Titles:        Titles{1: "Scanlator Wording", 2: "Licensed Wording"},
		Provenance:    map[float64]string{1: "comick"},
		DefaultSource: "wikipedia",
	}
	results := []Result{{Found: true, Titles: Titles{1: "Official", 2: "Official Two"}}}

	m := Merge(stored, results, []string{"mangaplus"})

	if m.Titles[1] != "Official" {
		t.Errorf("chapter 1 = %q, want mangaplus to displace the comick title", m.Titles[1])
	}
	if m.Titles[2] != "Licensed Wording" {
		t.Errorf("chapter 2 = %q, want the defaulted wikipedia title to hold", m.Titles[2])
	}
}

func TestMergeCountsDisplacementsAsContributions(t *testing.T) {
	// Added drives the per-source counts written into the dataset, so a title
	// that displaced a worse-ranked one has to count as this source's.
	stored := Stored{
		Titles:     Titles{1: "Scanlator One", 2: "Scanlator Two"},
		Provenance: map[float64]string{1: "comick", 2: "comick"},
	}
	results := []Result{{Found: true, Titles: Titles{1: "Official One", 3: "Official Three"}}}

	m := Merge(stored, results, []string{"wikipedia"})

	if len(m.Contributions) != 1 {
		t.Fatalf("got %d contributions, want 1", len(m.Contributions))
	}
	if m.Contributions[0].Added != 2 {
		t.Errorf("Added = %d, want 2 (one displacement, one gap filled)", m.Contributions[0].Added)
	}
	if m.Titles[2] != "Scanlator Two" {
		t.Errorf("chapter 2 = %q, want the untouched comick title", m.Titles[2])
	}
}
