package wikipedia

import (
	"strings"
	"testing"
)

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
	// "* 012." numbered bullets, including a "* 008–011." range that collapses
	// four chapters onto one line, as used by List of Berserk chapters. The
	// "(1–4)" label enumerates the span, so each chapter takes its own part
	// number -- which is also how the releases themselves are named.
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
		8:  "Assassin (1)",
		9:  "Assassin (2)",
		10: "Assassin (3)",
		11: "Assassin (4)",
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
	// on the page. Between two entries that both state their number, the
	// earlier one wins.
	//
	// This test used to assert the opposite of the case below it: that an
	// unnumbered bullet which happened to land on 1 outranked an explicit
	// "* 001.". That is the actual shape of List of Berserk chapters, and it
	// cost the dataset the real titles for chapters 1-11, which were replaced
	// by the prologue arc's. The assertion described the bug rather than the
	// intent, so it now covers stated-versus-stated only.
	wikitext := `
{{Graphic novel list
|ChapterList =
* 001. {{Nihongo|"The Black Swordsman"|黒い剣士|Kuroi Kenshi}}
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

// A {{efn}} footnote annotates a title; its body is prose about the title, not
// part of it. Unwrapping it to the first positional parameter — correct for
// {{Nihongo}} — spliced a paragraph of commentary into Vinland Saga chapter
// 101, producing a "title" far too long to be a filename.
func TestCleanTitle_dropsExplanatoryFootnotes(t *testing.T) {
	entry := `{{Nihongo|"The Fettered Tern (1)"{{efn|In the Japanese release, the ` +
		`titles of the "Fettered Tern" chapters feature the word クリーア in ` +
		`parentheses. This is a rendering of kría, the Icelandic word for the ` +
		`Arctic tern.}}|繋がれた鴎（クリーア）|Tsunagareta Kurīa}}`

	got := CleanTitle(entry)

	if want := "The Fettered Tern (1)"; got != want {
		t.Errorf("CleanTitle() = %q, want %q", got, want)
	}
}

func TestCleanTitle_dropsCitationsAndRefTemplates(t *testing.T) {
	cases := map[string]string{
		`"Ragnarok"{{refn|group=note|Serialised out of order.}}`: "Ragnarok",
		`"Sword"{{sfn|Yukimura|2007|p=12}}`:                      "Sword",
		`"Cage"{{cite web|url=http://x|title=Not The Title}}`:    "Cage",
		`"Troll"{{r|source1}}`:                                   "Troll",
	}

	for entry, want := range cases {
		if got := CleanTitle(entry); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", entry, got, want)
		}
	}
}

// The drop list must not swallow templates that legitimately supply title text.
func TestCleanTitle_keepsTitleBearingTemplates(t *testing.T) {
	cases := map[string]string{
		`{{Nihongo|"Normanni"|ノルマンニ|Norumanni}}`: "Normanni",
		`{{W|Shueisha}}`: "Shueisha",
	}

	for entry, want := range cases {
		if got := CleanTitle(entry); got != want {
			t.Errorf("CleanTitle(%q) = %q, want %q", entry, got, want)
		}
	}
}

// resolveTemplates removes one template per pass, so a fixed pass cap silently
// left the surplus in place. Entries that bundle a chapter with its extras —
// each a {{Nihongo}} wrapping a {{Ruby-ja}} — cleared ten easily, leaking raw
// "}}" and the next entry's text into the title (Arifureta chapter 20 and 65
// others in the built dataset).
func TestCleanTitle_resolvesMoreThanTenTemplates(t *testing.T) {
	entry := `{{Nihongo|"The Reisen Labyrinth"|樹海の迷宮|Jukai no Meikyū}}`
	for i := 0; i < 15; i++ {
		entry += ` {{W|extra}}`
	}

	got := CleanTitle(entry)

	if strings.Contains(got, "{{") || strings.Contains(got, "}}") {
		t.Errorf("unresolved template markup survived: %q", got)
	}
	if !strings.HasPrefix(got, "The Reisen Labyrinth") {
		t.Errorf("CleanTitle() = %q, want it to start with the real title", got)
	}
}

// Articles routinely follow a {{Numbered list}} with ":*" bullets for epilogues
// and extra chapters. When one of those bullets contained its own {{Nihongo}},
// taking the last "}}" in the field as the list's close swallowed the trailing
// bullets into the final chapter title (Arifureta chapters 20, 26, 32 and 63
// others in the built dataset).
func TestParseChapters_trailingBulletsAfterNumberedList(t *testing.T) {
	wikitext := `
{{Graphic novel list
| ChapterList     =
{{Numbered list|start=19
 | "Rabbit Reformation"
 | "The Reisen Labyrinth"
}}
:* "Epilogue"
:* Extra Chapter: {{Nihongo|"Yeah, I'm a Monster."|魔物ですが何か|Mamono desu ga Nanika}}
| LineColor       = CC0000
}}`

	got, _ := ParseChapters(wikitext)

	if title := got[20]; title != "The Reisen Labyrinth" {
		t.Errorf("chapter 20 = %q, want %q — trailing bullets leaked into the title", title, "The Reisen Labyrinth")
	}
	for num, title := range got {
		if strings.Contains(title, "{{") || strings.Contains(title, "}}") {
			t.Errorf("chapter %v kept raw template markup: %q", num, title)
		}
	}
}

func TestParseChapters_liValueSetsChapterNumber(t *testing.T) {
	// List of Naruto chapters (Part II, volumes 28–48) continues a series that
	// began in another article. The ordered list would restart at 1, so the
	// real number is carried as an HTML <li value="..."> attribute and every
	// following entry counts on from it. Ignoring it numbered 450+ licensed
	// titles from 1, where they collided with Part I and were discarded.
	wikitext := `
{{Graphic novel list
|VolumeNumber=28
|ChapterList=
#<li value="245">{{nihongo|"Homecoming!!"|ナルトの帰郷!!|"Naruto no kikyō!!"}}</li>
#{{nihongo-s|"My, How They've Grown!!"|二人の成長!!|"Futari no seichō!!"}}
#{{nihongo-s|"Intruders in the Sand"|砂への侵入者たち|"Suna e no shinnyūsha-tachi"}}
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		245: "Homecoming!!",
		246: "My, How They've Grown!!",
		247: "Intruders in the Sand",
	})
}

func TestParseChapters_liValueAppliesPerVolume(t *testing.T) {
	// Each volume block carries its own <li value=...>, and the count has to
	// pick up from each one rather than run on from the previous list.
	wikitext := `
{{Graphic novel list
|VolumeNumber=28
|ChapterList=
#<li value="245">{{nihongo|"Homecoming!!"|ナルトの帰郷!!}}</li>
#{{nihongo-s|"My, How They've Grown!!"|二人の成長!!}}
}}
{{Graphic novel list
|VolumeNumber=29
|ChapterList=
#<li value="254">{{nihongo|"Naruto's Growth!!"|ナルトの成長!!}}</li>
#{{nihongo-s|"The Next Step"|次の一歩}}
}}`

	got, _ := ParseChapters(wikitext)

	assertChapters(t, got, map[float64]string{
		245: "Homecoming!!",
		246: "My, How They've Grown!!",
		254: "Naruto's Growth!!",
		255: "The Next Step",
	})
}

func TestParseChapters_explicitNumberBeatsInferredOne(t *testing.T) {
	// List of Berserk chapters opens with unnumbered bullets for the prologue
	// arc and then switches to explicit "* 001." numbering. Numbering the
	// bullets positionally claimed 1-8, and because the first title seen for a
	// number used to win, every explicitly numbered chapter that followed was
	// discarded -- the dataset held prologue titles for chapters 1-11 and lost
	// the real ones entirely.
	wikitext := `
{{Graphic novel list
|VolumeNumber=1
|ChapterList=
* {{Nihongo|"The Black Swordsman"|黒い剣士}}
* {{Nihongo|"The Brand"|烙印}}
}}
{{Graphic novel list
|VolumeNumber=5
|ChapterList=
* 001. {{Nihongo|"A Wind of Swords"|剣風}}
* 002. {{Nihongo|"Nosferatu Zodd"|ゾッド}}
}}`

	got, _ := ParseChapters(wikitext)

	if got[1] != "A Wind of Swords" {
		t.Errorf("chapter 1 = %q, want %q (the explicitly numbered entry)", got[1], "A Wind of Swords")
	}
	if got[2] != "Nosferatu Zodd" {
		t.Errorf("chapter 2 = %q, want %q", got[2], "Nosferatu Zodd")
	}
}

func TestParseChapters_explicitNumberDoesNotOverwriteAnotherExplicitOne(t *testing.T) {
	// Berserk really does reuse numbers across arcs. Between two entries that
	// both state their number, the first still wins -- only inferred ones are
	// replaceable.
	wikitext := `
{{Graphic novel list
|ChapterList=
* 001. {{Nihongo|"First Arc Opening"|A}}
* 001. {{Nihongo|"Second Arc Opening"|B}}
}}`

	got, _ := ParseChapters(wikitext)

	if got[1] != "First Arc Opening" {
		t.Errorf("chapter 1 = %q, want the first explicit entry", got[1])
	}
}

func TestParseChapters_expandsRangeLabelPerChapter(t *testing.T) {
	// Wikipedia collapses consecutive same-titled chapters onto one line:
	// "* 002–005. Nosferatu Zodd (1–4)" is four chapters whose titles are
	// "(1)" through "(4)". Repeating the range label on each one loses which
	// part is which, and disagrees with how the releases are actually named.
	wikitext := `
{{Graphic novel list
|ChapterList=
* 001. {{Nihongo|"A Wind of Swords"|剣風}}
* 002–005. {{Nihongo|"Nosferatu Zodd (1–4)"|ゾッド}}
* 006. {{Nihongo|"Master of the Sword (1)"|剣の主}}
}}`

	got, _ := ParseChapters(wikitext)

	for num, want := range map[float64]string{
		1: "A Wind of Swords",
		2: "Nosferatu Zodd (1)",
		3: "Nosferatu Zodd (2)",
		4: "Nosferatu Zodd (3)",
		5: "Nosferatu Zodd (4)",
		6: "Master of the Sword (1)",
	} {
		if got[num] != want {
			t.Errorf("chapter %v = %q, want %q", num, got[num], want)
		}
	}
}

func TestParseChapters_rangeLabelLeftAloneWhenCountsDisagree(t *testing.T) {
	// Only expand when the label enumerates exactly as many parts as the range
	// covers. Anything else is a guess, and a wrong guess relabels every
	// chapter it touches.
	wikitext := `
{{Graphic novel list
|ChapterList=
* 002–005. {{Nihongo|"Something (1–2)"|X}}
}}`

	got, _ := ParseChapters(wikitext)

	for _, n := range []float64{2, 3, 4, 5} {
		if got[n] != "Something (1–2)" {
			t.Errorf("chapter %v = %q, want the label left intact", n, got[n])
		}
	}
}
