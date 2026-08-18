package mangaplus

import (
	"strconv"
	"strings"
)

// chapterNumber reads the chapter number out of a MangaPlus chapter name, which
// is written "#001" and occasionally "#001.5". Names that are not numbered at
// all — "#ex", "One-Shot" — yield false, since there is no chapter to attach a
// title to.
func chapterNumber(name string) (float64, bool) {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "#"))
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// numberWords are the spelled-out chapter numbers MangaPlus uses. Kaiju No. 8
// numbers its chapters "Episode One:", so a digits-only test would leave the
// prefix on every one of its titles.
var numberWords = map[string]float64{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
	"six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14,
	"fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18,
	"nineteen": 19, "twenty": 20,
}

// chapterWords are the words MangaPlus opens a numbered prefix with.
var chapterWords = map[string]bool{
	"chapter": true, "episode": true, "ch": true, "act": true, "chap": true,
}

// cleanTitle removes a leading "Chapter 12:" style prefix from a MangaPlus
// sub-title, leaving the chapter's actual name. num is the chapter's own
// number, used to tell a prefix from a title that merely opens with a number.
//
// Series do not agree on the format. Most write "Chapter 1: Dog & Chainsaw",
// Kaiju No. 8 spells the number out as "Episode One:", and Sakamoto Days uses
// no colon at all — "Days 269 Inner Beast". A prefix is therefore recognised as
// a word followed by a number, with or without a trailing colon, and is removed
// only when the word is a known chapter word or the number is this chapter's
// own. That second condition is what keeps a real title like "Area 51 Incident"
// intact on chapter 7 while stripping "Days 269" from chapter 269.
//
// Only the prefix is touched, so "Chapter 3: Enter Zolo: Pirate Hunter" keeps
// the colon that belongs to the title.
func cleanTitle(sub string, num float64) string {
	s := strings.TrimSpace(sub)
	fields := strings.Fields(s)
	if len(fields) < 3 {
		// Too short to be a prefix plus a title; anything left would be the
		// whole name, and removing it would lose the title entirely.
		return s
	}

	word := strings.ToLower(strings.Trim(fields[0], ".:"))
	got, ok := parseNumberToken(fields[1])
	if !ok {
		return s
	}
	if !chapterWords[word] && got != num {
		return s
	}

	// Rejoin from the third token so the original spacing of the title is not
	// preserved but its content is; MangaPlus pads some prefixes oddly.
	rest := strings.TrimSpace(strings.Join(fields[2:], " "))
	if rest == "" {
		return s
	}
	return rest
}

// parseNumberToken reads a prefix's number, which may be spelled out and may
// carry the trailing colon that separates it from the title.
func parseNumberToken(tok string) (float64, bool) {
	t := strings.TrimSuffix(strings.TrimSpace(tok), ":")
	if t == "" {
		return 0, false
	}
	if n, err := strconv.ParseFloat(t, 64); err == nil {
		return n, true
	}
	if n, ok := numberWords[strings.ToLower(t)]; ok {
		return n, true
	}
	return 0, false
}
