package mangaplus

import "testing"

func TestChapterNumber(t *testing.T) {
	tests := []struct {
		name string
		want float64
		ok   bool
	}{
		{"#001", 1, true},
		{"#232", 232, true},
		{"#1190", 1190, true},
		{"#012", 12, true},
		{"#001.5", 1.5, true},
		{"#ex", 0, false},
		{"One-Shot", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := chapterNumber(tt.name)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("chapterNumber(%q) = %v, %v; want %v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		in   string
		num  float64
		want string
	}{
		{"Chapter 1: Dog & Chainsaw", 1, "Dog & Chainsaw"},
		{"Chapter 3: Enter Zolo: Pirate Hunter", 3, "Enter Zolo: Pirate Hunter"},
		{"Ch. 12: Something Happens", 12, "Something Happens"},
		// Kaiju No. 8 spells the number out.
		{"Episode One: The Beginning", 1, "The Beginning"},
		{"Episode 12: Later On", 12, "Later On"},
		// Sakamoto Days uses no colon at all. The number matching this chapter
		// is what identifies it as a prefix.
		{"Days 269 Inner Beast", 269, "Inner Beast"},
		{"Days 271 Melting Pot of Slaughter", 271, "Melting Pot of Slaughter"},
		// A title that merely opens with a number is not a prefix: 51 is not
		// chapter 7's number and "Area" is not a chapter word.
		{"Area 51 Incident", 7, "Area 51 Incident"},
		// A colon that belongs to the title, with no numbered prefix.
		{"Jujutsu Kaisen: The Movie", 1, "Jujutsu Kaisen: The Movie"},
		{"RYOMEN SUKUNA", 1, "RYOMEN SUKUNA"},
		{"A Thing: Another Thing", 1, "A Thing: Another Thing"},
		{"  Chapter 5:   Spaced Out  ", 5, "Spaced Out"},
		// A one-word title is still a title.
		{"Chapter 5: Solo", 5, "Solo"},
		// Stripping must never consume the whole string and leave nothing.
		{"Chapter 5", 5, "Chapter 5"},
		{"", 1, ""},

		// One-token prefixes. A "#" or a "WORD:" marks a prefix outright;
		// Blue Box writes "#250" and Dragon Ball writes "DBZ:325", whose
		// number is the volume edition's and matches no chapter number.
		{"#250 In This Box", 250, "In This Box"},
		{"DBZ:325 Farewell, Dragon World!", 519, "Farewell, Dragon World!"},
		// A bare number is a prefix only when it is this chapter's own.
		{"255 Bone Crisis, Part 24", 255, "Bone Crisis, Part 24"},
		{"300 Spartans Return Home", 5, "300 Spartans Return Home"},
		// Spy x Family's chapter titles really are "MISSION 137"; stripping
		// would leave nothing, so the whole string stands.
		{"MISSION 137", 137, "MISSION 137"},
		{"#137", 137, "#137"},
		// Not a prefix: a ratio or a time has digits before the colon.
		{"20:30 The Meeting Time", 4, "20:30 The Meeting Time"},
	}
	for _, tt := range tests {
		if got := cleanTitle(tt.in, tt.num); got != tt.want {
			t.Errorf("cleanTitle(%q, %v) = %q, want %q", tt.in, tt.num, got, tt.want)
		}
	}
}
