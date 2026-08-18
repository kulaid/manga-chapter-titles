package sources

import "testing"

func TestMergeExistingTitlesWin(t *testing.T) {
	// Titles already in the file are the licensed Wikipedia ones. Re-running
	// enrichment must never overwrite them with a scanlator title.
	existing := Titles{1: "Dog & Chainsaw"}
	results := []Result{
		{Found: true, Ref: "cm", Titles: Titles{1: "A Dog and a Chainsaw", 2: "Pochita's Whereabouts"}},
	}

	m := Merge(Stored{Titles: existing, DefaultSource: "wikipedia"}, results, []string{"comick"})

	if m.Titles[1] != "Dog & Chainsaw" {
		t.Errorf("chapter 1 = %q, want the existing title to survive", m.Titles[1])
	}
	if m.Provenance[1] != "wikipedia" {
		t.Errorf("chapter 1 provenance = %q, want %q", m.Provenance[1], "wikipedia")
	}
	if m.Titles[2] != "Pochita's Whereabouts" {
		t.Errorf("chapter 2 = %q, want the gap to be filled", m.Titles[2])
	}
	if m.Provenance[2] != "comick" {
		t.Errorf("chapter 2 provenance = %q, want %q", m.Provenance[2], "comick")
	}
}

func TestMergeRespectsSourcePriority(t *testing.T) {
	// Sources are passed highest-priority first; the earlier one keeps the
	// chapter and the later only fills what is still missing.
	results := []Result{
		{Found: true, Titles: Titles{1: "From Comick"}},
		{Found: true, Titles: Titles{1: "From MangaDex", 2: "Only MangaDex"}},
	}

	m := Merge(Stored{}, results, []string{"comick", "mangadex"})

	if m.Titles[1] != "From Comick" {
		t.Errorf("chapter 1 = %q, want the higher-priority source", m.Titles[1])
	}
	if m.Titles[2] != "Only MangaDex" {
		t.Errorf("chapter 2 = %q, want the lower-priority source to fill the gap", m.Titles[2])
	}

	if len(m.Contributions) != 2 {
		t.Fatalf("got %d contributions, want 2", len(m.Contributions))
	}
	if m.Contributions[0].Added != 1 || m.Contributions[1].Added != 1 {
		t.Errorf("contributions = %+v, want one added each", m.Contributions)
	}
	// Total counts what the source knew, not what it contributed.
	if m.Contributions[1].Total != 2 {
		t.Errorf("mangadex Total = %d, want 2", m.Contributions[1].Total)
	}
}

func TestMergeSkipsNotFoundSources(t *testing.T) {
	results := []Result{
		{Found: false},
		{Found: true, Titles: Titles{1: "Real"}},
	}

	m := Merge(Stored{}, results, []string{"comick", "mangadex"})

	if len(m.Contributions) != 1 || m.Contributions[0].Name != "mangadex" {
		t.Errorf("contributions = %+v, want only the source that found the series", m.Contributions)
	}
	if m.Titles[1] != "Real" {
		t.Errorf("chapter 1 = %q", m.Titles[1])
	}
}

func TestMergeIgnoresEmptyTitles(t *testing.T) {
	// An empty title is an absent one: it must not occupy the chapter and block
	// a later source from naming it.
	results := []Result{
		{Found: true, Titles: Titles{1: "", 2: "Named"}},
		{Found: true, Titles: Titles{1: "Filled Later"}},
	}

	m := Merge(Stored{Titles: Titles{3: ""}, DefaultSource: "wikipedia"}, results, []string{"comick", "mangadex"})

	if m.Titles[1] != "Filled Later" {
		t.Errorf("chapter 1 = %q, want the empty title to have been skipped", m.Titles[1])
	}
	if _, present := m.Titles[3]; present {
		t.Error("an empty existing title was carried into the merge")
	}
	if len(m.Titles) != 2 {
		t.Errorf("got %d titles, want 2: %v", len(m.Titles), m.Titles)
	}
}

func TestMergeHandlesDecimalChapters(t *testing.T) {
	m := Merge(Stored{}, []Result{
		{Found: true, Titles: Titles{4: "Four", 4.5: "Interlude"}},
	}, []string{"comick"})

	if m.Titles[4.5] != "Interlude" {
		t.Errorf("chapter 4.5 = %q, want %q", m.Titles[4.5], "Interlude")
	}
}

func TestSortedNumbers(t *testing.T) {
	got := SortedNumbers(Titles{10: "b", 2: "a", 4.5: "c"})
	want := []float64{2, 4.5, 10}
	if len(got) != len(want) {
		t.Fatalf("SortedNumbers() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SortedNumbers()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
