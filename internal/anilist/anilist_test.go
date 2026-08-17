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
