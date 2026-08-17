package wikipedia

import "testing"

// The fixtures below are trimmed from real articles; each one covers a list
// format or markup quirk that broke an earlier version of the parser.

func TestParseChapters_numberedList(t *testing.T) {
	// {{Numbered list|start=N}} form, as used by List of Chainsaw Man chapters.
	// The "Power" entry nests {{Ruby-ja}} inside the Japanese parameter, so a
	// naive split on "|" picks up the wrong field.
	wikitext := `
{{Graphic novel list
|VolumeNumber = 1
|ChapterListCol1    =
{{Numbered list|start=1
|{{Nihongo|"Dog & Chainsaw"|犬とチェンソー|Inu to Chensō}}
|{{Nihongo|"The Place Where Pochita Is"|ポチタの行方|Pochita no Yukue}}
}}
|ChapterListCol2    =
{{Numbered list|start=3
|{{Nihongo|"Arrival in Tokyo"|東京到着|Tōkyō Tōchaku}}
|{{Nihongo|"Power"|{{Ruby-ja|力|パワー}}|Pawā}}
}}
|Summary = Denji, an impoverished teenager, works to pay off debts.
}}`

	got, inferred := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		1: "Dog & Chainsaw",
		2: "The Place Where Pochita Is",
		3: "Arrival in Tokyo",
		4: "Power",
	})
	if inferred != 0 {
		t.Errorf("inferred = %d, want 0 (every entry is explicitly numbered)", inferred)
	}
}

func TestParseChapters_numberedBulletsAndRanges(t *testing.T) {
	// "* 012." numbered bullets, including a "* 008–011." range that shares one
	// title across four chapters, as used by List of Berserk chapters.
	wikitext := `
{{Graphic novel list
|ChapterList =
* 007. {{Nihongo|"Master of the Sword (2)"|剣の主(2)|Tsurugi no Aruji (2)}}
* 008–011. {{Nihongo|"Assassin (1–4)"|暗殺者(1〜4)|Ansatsusha (1–4)}}
* 012. {{Nihongo|"Precious Thing"|貴きもの|Tōtokimono}}
|Summary = Guts fights on.
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		7:  "Master of the Sword (2)",
		8:  "Assassin (1–4)",
		9:  "Assassin (1–4)",
		10: "Assassin (1–4)",
		11: "Assassin (1–4)",
		12: "Precious Thing",
	})
}

func TestParseChapters_plainBulletsAreCounted(t *testing.T) {
	// Unnumbered bullets continue from the previous entry's number. The
	// "extra=" named parameter must not be mistaken for the title.
	wikitext := `
{{Graphic novel list
|ChapterList =
* {{Nihongo|"The Black Swordsman"|黒い剣士|Kuroi Kenshi|extra=beginning of the arc}}
* {{Nihongo|"The Brand"|烙印|Rakuin}}
}}
{{Graphic novel list
|ChapterList =
* 004. {{Nihongo|"The Golden Age"|黄金時代|Ōgon Jidai}}
* {{Nihongo|"Sword Wind"|剣風|Kenpū}}
}}`

	got, inferred := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		1: "The Black Swordsman",
		2: "The Brand",
		4: "The Golden Age",
		5: "Sword Wind", // resumes from the explicit 004
	})
	if inferred != 3 {
		t.Errorf("inferred = %d, want 3 (three bullets carry no number)", inferred)
	}
}

func TestParseChapters_skipsBonusEntries(t *testing.T) {
	// A labelled bullet such as "Bonus Material." is not a numbered chapter and
	// must not consume chapter 3's slot.
	wikitext := `
{{Graphic novel list
|ChapterList =
*001. {{Nihongo|"Normanni"|北人|Norumanni}}
*002. {{Nihongo|"Somewhere Not Here"|ここではないどこか|Koko de wa Nai Dokoka}}
*Bonus Material. "Heroic Exploits of Viking Girl Ylva"
*003. {{Nihongo|"Beyond the Edge of the Sea"|海の果ての果て|Umi no Hate no Hate}}
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		1: "Normanni",
		2: "Somewhere Not Here",
		3: "Beyond the Edge of the Sea",
	})
}

func TestParseChapters_firstTitleWinsOnDuplicateNumbers(t *testing.T) {
	// Berserk restarts its numbering per arc, so the same number appears twice
	// on the page. The earlier entry wins.
	wikitext := `
{{Graphic novel list
|ChapterList =
* {{Nihongo|"The Black Swordsman"|黒い剣士|Kuroi Kenshi}}
}}
{{Graphic novel list
|ChapterList =
* 001. {{Nihongo|"A Wind of Swords"|剣風|Kenpū}}
}}`

	got, _ := ParseChapters(wikitext)

	if got[1] != "The Black Swordsman" {
		t.Errorf("chapter 1 = %q, want %q (the first entry in document order)", got[1], "The Black Swordsman")
	}
}

func TestParseChapters_splitsTrailingBullet(t *testing.T) {
	// List of Jujutsu Kaisen chapters glues an unnumbered "* Epilogue" bullet
	// onto the last pipe entry; it must become its own chapter, not be
	// concatenated onto chapter 271's title.
	wikitext := `
{{Graphic novel list
| ChapterListCol2 =
{{Numbered list|start=270
|{{Nihongo|"The End of the Dream"|夢の終わリ|Yume no Owari}}
|{{Nihongo|"From Now On"|これから|Kore Kara}}
* {{Nihongo|"Epilogue"|エピローグ|Epirōgu}}
}}
| LicensedTitle   = From Now On
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		270: "The End of the Dream",
		271: "From Now On",
		272: "Epilogue",
	})
}

func TestParseChapters_stripsMarkup(t *testing.T) {
	wikitext := `
{{Graphic novel list
|ChapterList =
* 001. {{Nihongo|"The [[Hunter × Hunter|Hunter]] Exam"|試験|Shiken}}<ref name="vol1">{{cite web|url=http://example.com}}</ref>
* 002. ''"A &amp; B"''<!-- placeholder -->
* 003. "Line<br />Break"
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		1: "The Hunter Exam",
		2: "A & B",
		3: "Line Break",
	})
}

func TestParseChapters_ignoresNonChapterFields(t *testing.T) {
	// Summary prose and volume metadata must not leak in as chapter titles.
	wikitext := `
{{Graphic novel list
|VolumeNumber = 1
|OriginalTitle = {{Nihongo2|犬とチェンソー}}
|ChapterList =
* 001. {{Nihongo|"Real Title"|本当|Hontō}}
|Summary = Some prose about the volume that mentions chapters.
}}`

	got, _ := ParseChapters(wikitext)

	if len(got) != 1 {
		t.Fatalf("got %d chapters, want 1: %v", len(got), got)
	}
	if got[1] != "Real Title" {
		t.Errorf("chapter 1 = %q, want %q", got[1], "Real Title")
	}
}

func TestParseChapters_halfChapters(t *testing.T) {
	// Decimal chapter numbers must survive as decimals.
	wikitext := `
{{Graphic novel list
|ChapterList =
* 4. {{Nihongo|"Four"|四|Yon}}
* 4.5. {{Nihongo|"Interlude"|幕間|Makuai}}
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		4:   "Four",
		4.5: "Interlude",
	})
}

func TestParseChapters_numberInsideTitle(t *testing.T) {
	// Many articles number chapters inside the title rather than as a list
	// marker. The number is authoritative and the label must not survive into
	// the title. Trimming the prefix also re-exposes the opening quote, which
	// was balanced before the prefix was removed.
	wikitext := `
{{Graphic novel list
|ChapterList =
* {{Nihongo|Chapter 1: "Maomao"|猫猫|Maomao}}
* {{Nihongo|Navigation 02: The Water Planet|水の惑星|Mizu no Wakusei}}
* {{Nihongo|Chapter 46|第46話}}
* {{Nihongo|Chapter 47: "Return"|帰還|Kikan}}
}}`

	got, inferred := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		1:  "Maomao",
		2:  "The Water Planet",
		47: "Return", // chapter 46 has no title, so it is dropped
	})
	if inferred != 0 {
		t.Errorf("inferred = %d, want 0 (the numbers are stated in the titles)", inferred)
	}
}

func TestParseChapters_skipsBonusContentInsideLists(t *testing.T) {
	// Bonus entries sit inside chapter lists but are outside the numbering, so
	// they must not consume the next chapter's slot.
	wikitext := `
{{Graphic novel list
|ChapterList =
* {{Nihongo|Navigation 01: The Water Planet|水の惑星|Mizu no Wakusei}}
* {{Nihongo|Bonus Navigation: Colds and Pudding|風邪|Kaze}}
* {{Nihongo|Navigation 02: Fireworks|花火|Hanabi}}
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		1: "The Water Planet",
		2: "Fireworks",
	})
}

func TestParseChapters_bareNumbersOnlyWhenArticleNumbersTitles(t *testing.T) {
	// "Arisa" zero-pads 01–09 and then writes a plain "10". Once an article is
	// seen numbering titles this way, a bare leading number is trusted too.
	numbered := `
{{Graphic novel list
|ChapterList =
* {{Nihongo|09 Ninth|九|Kyū}}
* {{Nihongo|10 Mariko Takagi|髙木マリコ|Takagi Mariko}}
}}`

	got, _ := ParseChapters(numbered)
	assertChapters(t, got, map[float64]string{
		9:  "Ninth",
		10: "Mariko Takagi",
	})

	// An article that never numbers its titles must keep a leading number that
	// is part of the title itself.
	plain := `
{{Graphic novel list
|ChapterList =
* {{Nihongo|20th Century Boys|20世紀少年|Nijusseiki Shōnen}}
* {{Nihongo|7 Seeds|セブンシーズ|Sebun Shīzu}}
}}`

	got, inferred := ParseChapters(plain)
	assertChapters(t, got, map[float64]string{
		1: "20th Century Boys",
		2: "7 Seeds",
	})
	if inferred != 2 {
		t.Errorf("inferred = %d, want 2", inferred)
	}
}

func TestStripChapterLabel(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantStart float64
		wantEnd   float64
		wantRest  string
		wantOK    bool
	}{
		{name: "label and colon", in: `Chapter 12: Foo`, wantStart: 12, wantEnd: 12, wantRest: "Foo", wantOK: true},
		{name: "label and space", in: `Chapter 12 Foo`, wantStart: 12, wantEnd: 12, wantRest: "Foo", wantOK: true},
		{name: "unbalanced quote after prefix", in: `Chapter 1: "Maomao`, wantStart: 1, wantEnd: 1, wantRest: "Maomao", wantOK: true},
		{name: "zero padded number", in: `012 Tsubasa and Arisa`, wantStart: 12, wantEnd: 12, wantRest: "Tsubasa and Arisa", wantOK: true},
		{name: "separator only", in: `12: Foo`, wantStart: 12, wantEnd: 12, wantRest: "Foo", wantOK: true},
		{name: "range", in: `Trick 1–5`, wantStart: 1, wantEnd: 5, wantRest: "", wantOK: true},
		{name: "label after number", in: `1 Fasıl: "The Golden Eagle General`, wantStart: 1, wantEnd: 1, wantRest: "The Golden Eagle General", wantOK: true},

		// A bare number followed by a space is ambiguous, so it is left alone.
		{name: "title starting with a number", in: `20th Century Boys`, wantOK: false},
		{name: "number that is the title", in: `7 Seeds`, wantOK: false},
		{name: "ordinary title", in: `Dog & Chainsaw`, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, rest, ok := stripChapterLabel(tt.in, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (rest=%q)", ok, tt.wantOK, rest)
			}
			if !ok {
				return
			}
			if start != tt.wantStart || end != tt.wantEnd {
				t.Errorf("range = %v..%v, want %v..%v", start, end, tt.wantStart, tt.wantEnd)
			}
			if rest != tt.wantRest {
				t.Errorf("rest = %q, want %q", rest, tt.wantRest)
			}
		})
	}
}

func TestHasChapterList(t *testing.T) {
	if HasChapterList("{{Graphic novel list|VolumeNumber = 1|Summary = x}}") {
		t.Error("reported a chapter list on a volume entry that has none")
	}
	if !HasChapterList("{{Graphic novel list|ChapterListCol1 = \n* 001. Foo\n}}") {
		t.Error("failed to detect a ChapterListCol1 field")
	}
}

// assertChapters compares a parsed chapter map against the expected map.
func assertChapters(t *testing.T, got Chapters, want map[float64]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("got %d chapters, want %d: %v", len(got), len(want), got)
	}
	for num, title := range want {
		if got[num] != title {
			t.Errorf("chapter %v = %q, want %q", num, got[num], title)
		}
	}
}

func TestParseChapters_orderedListMarkup(t *testing.T) {
	// "#" is MediaWiki's ordered-list marker and appears in chapter lists just
	// as often as "*" — Attack on Titan numbers its first volume this way.
	// Skipping those lines silently truncated the series.
	wikitext := `
{{Graphic novel list
| ChapterList     =
# {{nihongo|"To You, 2,000 Years from Now"|二千年後の君へ|Ni Sen Nen Go no Kimi e}}
# {{nihongo|"That Day"|その日|Sono Hi}}
# {{nihongo|"Night of the Disbanding Ceremony"|解散式の夜|Kaisanshiki no Yoru}}
| Summary = With the advent of giant humanoid beings known as "Titans"...
}}`

	got, inferred := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		1: "To You, 2,000 Years from Now",
		2: "That Day",
		3: "Night of the Disbanding Ceremony",
	})
	if inferred != 3 {
		t.Errorf("inferred = %d, want 3 (ordered-list markup carries no explicit number)", inferred)
	}
}

func TestParseChapters_mixedBulletAndOrderedMarkers(t *testing.T) {
	// An article may switch markers between volumes; numbering must run
	// continuously across both.
	wikitext := `
{{Graphic novel list
|ChapterList =
# {{Nihongo|"One"|一|Ichi}}
# {{Nihongo|"Two"|二|Ni}}
}}
{{Graphic novel list
|ChapterList =
* {{Nihongo|"Three"|三|San}}
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{1: "One", 2: "Two", 3: "Three"})
}
