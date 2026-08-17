package chaptertitles

import "testing"

func TestFoldAccents(t *testing.T) {
	tests := []struct{ in, want string }{
		// Romanisation macrons are the common case in this dataset.
		{"Boku wa Imōto ni Koi o Suru", "Boku wa Imoto ni Koi o Suru"},
		{"Rainbow: Nisha Rokubō no Shichinin", "Rainbow: Nisha Rokubo no Shichinin"},
		{"Jūjutsu", "Jujutsu"},
		{"Übel Blatt", "Ubel Blatt"},
		{"Pokémon", "Pokemon"},
		{"Ōoku", "Ooku"},

		// Multi-letter expansions.
		{"Æon", "AEon"},
		{"Straße", "Strasse"},

		// Untouched: pure ASCII, and non-letters with no ASCII equivalent.
		{"Chainsaw Man", "Chainsaw Man"},
		{"Hunter × Hunter", "Hunter × Hunter"},
		{"Ranma ½", "Ranma ½"},
		{"進撃の巨人", "進撃の巨人"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := FoldAccents(tt.in); got != tt.want {
			t.Errorf("FoldAccents(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSlugifyFoldsAccents(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Boku wa Imōto ni Koi o Suru", "boku-wa-imoto-ni-koi-o-suru"},
		{"Rainbow: Nisha Rokubō no Shichinin", "rainbow-nisha-rokubo-no-shichinin"},
		{"Übel Blatt", "ubel-blatt"},
		// A non-letter still acts as a separator.
		{"Hunter × Hunter", "hunter-hunter"},
		{"Ranma ½", "ranma"},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMatchKeyFoldsAccents(t *testing.T) {
	// The whole point: the accented and plain-ASCII spellings must agree, since
	// nobody types the macron.
	pairs := [][2]string{
		{"Boku wa Imōto ni Koi o Suru", "Boku wa Imoto ni Koi o Suru"},
		{"Rainbow: Nisha Rokubō no Shichinin", "rainbow nisha rokubo no shichinin"},
		{"Übel Blatt", "Ubel Blatt"},
		{"Pokémon", "pokemon"},
	}
	for _, p := range pairs {
		a, b := MatchKey(p[0]), MatchKey(p[1])
		if a != b {
			t.Errorf("MatchKey(%q) = %q but MatchKey(%q) = %q; want them equal", p[0], a, p[1], b)
		}
		if a == "" {
			t.Errorf("MatchKey(%q) came out empty", p[0])
		}
	}

	// Previously-correct behaviour must be preserved.
	if MatchKey("Hunter × Hunter") != MatchKey("Hunter x Hunter") {
		t.Error("the stylised multiplication sign no longer folds")
	}
}
