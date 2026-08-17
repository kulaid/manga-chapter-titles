package anilist

import "testing"

// media builds a candidate with the given titles.
func media(id int, romaji, english string, synonyms ...string) Media {
	var m Media
	m.ID = id
	m.Title.Romaji = romaji
	m.Title.English = english
	m.Synonyms = synonyms
	return m
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name       string
		series     string
		candidates []Media
		wantID     int
		wantOK     bool
	}{
		{
			name:       "romaji matches",
			series:     "Berserk",
			candidates: []Media{media(30002, "Berserk", "Berserk")},
			wantID:     30002, wantOK: true,
		},
		{
			// Wikipedia uses the licensed English title, AniList's romaji is the
			// Japanese one.
			name:       "english title matches when romaji does not",
			series:     "Attack on Titan",
			candidates: []Media{media(53390, "Shingeki no Kyojin", "Attack on Titan")},
			wantID:     53390, wantOK: true,
		},
		{
			// AniList brackets this title; normalisation drops the punctuation.
			name:       "bracketed styling",
			series:     "Oshi no Ko",
			candidates: []Media{media(117195, "[Oshi no Ko]", "[Oshi no Ko]")},
			wantID:     117195, wantOK: true,
		},
		{
			name:       "stylised multiplication sign",
			series:     "Hunter × Hunter",
			candidates: []Media{media(30026, "HUNTER×HUNTER", "Hunter x Hunter")},
			wantID:     30026, wantOK: true,
		},
		{
			name:       "synonym matches",
			series:     "Bucket List of the Dead",
			candidates: []Media{media(104660, "Zom 100: Zombie ni Naru Made ni Shitai 100 no Koto", "Zom 100: Bucket List of the Dead", "Bucket list of the dead")},
			wantID:     104660, wantOK: true,
		},
		{
			name:       "later candidate wins over an irrelevant first hit",
			series:     "Monster",
			candidates: []Media{media(1, "Monster Musume", "Monster Musume"), media(30001, "MONSTER", "Monster")},
			wantID:     30001, wantOK: true,
		},
		{
			// AniList answers every query with something; an unconfirmed top hit
			// must not become a confident wrong ID.
			name:       "no candidate is the series",
			series:     "Some Series AniList Lacks",
			candidates: []Media{media(1, "Something Else", "Something Else")},
			wantOK:     false,
		},
		{
			name:       "no candidates at all",
			series:     "Berserk",
			candidates: nil,
			wantOK:     false,
		},
		{
			name:       "empty series name",
			series:     "",
			candidates: []Media{media(1, "Berserk", "Berserk")},
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := Match(tt.series, tt.candidates)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (id=%d)", ok, tt.wantOK, id)
			}
			if ok && id != tt.wantID {
				t.Errorf("id = %d, want %d", id, tt.wantID)
			}
		})
	}
}

func TestNames(t *testing.T) {
	m := media(1, "Romaji", "English", "Syn1", "Syn2")
	m.Title.Native = "ネイティブ"

	got := m.Names()
	want := []string{"Romaji", "English", "ネイティブ", "Syn1", "Syn2"}
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNamesSkipsEmptyTitles(t *testing.T) {
	// AniList leaves english empty for many entries; the gap must not become a
	// blank name that matches an empty key.
	m := media(1, "Romaji", "")
	for _, n := range m.Names() {
		if n == "" {
			t.Fatal("Names() returned an empty title")
		}
	}
}

func TestMatch_prefersComicOverNovel(t *testing.T) {
	// AniList's type:MANGA covers light novels too, and it returned the
	// 29-chapter "FAIRY TAIL" novel ahead of the 549-chapter manga. Both
	// normalise to the same title, so search order decided it and the dataset
	// ended up joined to the wrong work.
	candidates := []Media{
		{ID: 92603, Format: "NOVEL"},
		{ID: 30598, Format: "MANGA"},
	}
	candidates[0].Title.Romaji = "FAIRY TAIL"
	candidates[1].Title.Romaji = "FAIRY TAIL"

	got, ok := Match("Fairy Tail", candidates)
	if !ok {
		t.Fatal("Match reported no candidate")
	}
	if got != 30598 {
		t.Errorf("Match = %d, want the manga 30598", got)
	}
}

func TestMatch_acceptsNovelWhenItIsTheOnlyMatch(t *testing.T) {
	// The dataset legitimately carries light novel series, so a novel is a
	// valid answer when nothing else matches the name.
	candidates := []Media{{ID: 12345, Format: "NOVEL"}}
	candidates[0].Title.Romaji = "Monogatari"

	got, ok := Match("Monogatari", candidates)
	if !ok || got != 12345 {
		t.Errorf("Match = %d, %v; want 12345, true", got, ok)
	}
}

func TestMatch_keepsSearchOrderWithinTheSameFormat(t *testing.T) {
	candidates := []Media{{ID: 111, Format: "MANGA"}, {ID: 222, Format: "MANGA"}}
	candidates[0].Title.Romaji = "Berserk"
	candidates[1].Title.Romaji = "Berserk"

	if got, _ := Match("Berserk", candidates); got != 111 {
		t.Errorf("Match = %d, want the first match 111", got)
	}
}

func TestMatch_primaryTitleBeatsSynonymOnAnotherWork(t *testing.T) {
	// "Pantsu Agerune" is a nine-chapter doujin carrying "Fairy Tail" as a
	// synonym. Preferring a comic over a novel is right, but not at the cost of
	// picking an unrelated work over the series itself.
	doujin := Media{ID: 128087, Format: "MANGA", Synonyms: []string{"Fairy Tail"}}
	doujin.Title.Romaji = "Pantsu Agerune"
	novel := Media{ID: 92603, Format: "NOVEL"}
	novel.Title.Romaji = "FAIRY TAIL"
	manga := Media{ID: 30598, Format: "MANGA"}
	manga.Title.Romaji = "FAIRY TAIL"

	if got, _ := Match("Fairy Tail", []Media{novel, doujin, manga}); got != 30598 {
		t.Errorf("Match = %d, want the manga 30598", got)
	}
	// Even with no manga present, the novel that is actually named Fairy Tail
	// beats the doujin that merely lists it.
	if got, _ := Match("Fairy Tail", []Media{doujin, novel}); got != 92603 {
		t.Errorf("Match = %d, want the novel 92603 over the doujin", got)
	}
}

func TestMatch_prefersSerialOverOneShot(t *testing.T) {
	// AniList carries a one-chapter "Medaka Box" one-shot alongside the
	// 194-chapter serial. Both are comics and both match the name, so ranking
	// on format alone left search order to decide and it picked the one-shot.
	// A single-chapter work can never be the right join for a chapter list.
	oneShot := Media{ID: 45143, Format: "ONE_SHOT"}
	oneShot.Title.Romaji = "Medaka Box"
	serial := Media{ID: 43949, Format: "MANGA"}
	serial.Title.Romaji = "Medaka Box"

	if got, _ := Match("Medaka Box", []Media{oneShot, serial}); got != 43949 {
		t.Errorf("Match = %d, want the serial 43949", got)
	}
}

func TestMatch_acceptsOneShotWhenItIsTheOnlyMatch(t *testing.T) {
	// Some entries in the dataset really are one-shots.
	only := Media{ID: 55482, Format: "ONE_SHOT"}
	only.Title.Romaji = "Fairy Tail x Rave"
	if got, ok := Match("Fairy Tail x Rave", []Media{only}); !ok || got != 55482 {
		t.Errorf("Match = %d, %v; want 55482, true", got, ok)
	}
}
