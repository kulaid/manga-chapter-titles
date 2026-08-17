package wikipedia

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// Chapters maps a chapter number to its English title.
type Chapters map[float64]string

// wikiChapterListField matches the start of a chapter-list parameter inside a
// {{Graphic novel list}} entry, e.g. "|ChapterList =" or "| ChapterListCol2 =".
var wikiChapterListField = regexp.MustCompile(`(?i)ChapterList(?:Col\d*)?\s*=`)

// wikiNumberedListStart matches the opening of a {{Numbered list|start=N}}
// block, which supplies the chapter number of its first entry.
var wikiNumberedListStart = regexp.MustCompile(`(?is)^\s*\{\{\s*numbered list\s*\|\s*start\s*=\s*(\d+(?:\.\d+)?)`)

// wikiChapterNumPrefix matches an explicit chapter number (or range) that
// prefixes a bullet entry, e.g. "012. " or "008–011. ".
var wikiChapterNumPrefix = regexp.MustCompile(`^\s*(\d+(?:\.\d+)?)\s*(?:[–—-]\s*(\d+(?:\.\d+)?))?\s*\.\s+`)

// wikiSpecialEntryPrefix matches a non-numeric bullet label such as
// "Bonus Material. " or "Extra. ", which denotes an entry that is not a
// numbered chapter and must not consume a chapter slot.
var wikiSpecialEntryPrefix = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 '\-]{0,30}\.\s+`)

// wikiExtraEntry matches a cleaned title that names bonus content rather than a
// numbered chapter, e.g. "Bonus Navigation: Colds and Pudding" or "Afterword".
// These sit inside chapter lists but are not part of the numbering.
var wikiExtraEntry = regexp.MustCompile(`(?i)^(bonus|extra|special|omake|afterword|author'?s note|side ?story|epilogue chapter)\b`)

// wikiLabeledChapterPrefix matches a chapter number written inside the title
// itself rather than as a list marker — "Chapter 12: ", "Navigation 01: ",
// "1 Fasıl: ", "007 ". Many articles number their chapters this way instead of
// using a numbered list, and without this the number would have to be guessed
// from list position and the label would be left stuck to the title.
//
// Group 1 is the label word, 2 the number, 3 the end of a range, 4 the
// separator. A match only counts when a label, a separator, or a zero-padded
// number is present — see stripChapterLabel.
var wikiLabeledChapterPrefix = regexp.MustCompile(
	`^(?i:(chapter|chapters|ch\.|navigation|trick|act|file|episode|case|story|round|stage|step|track|gate|night|fasıl|part|no\.)\s*)?` +
		`(\d+(?:\.\d+)?)(?:\s*[–—-]\s*(\d+(?:\.\d+)?))?` +
		`\s*(?i:fasıl)?\s*([:.\-–—])?(?:\s+|$)`)

// wikiQuoteTrim is the set of quote characters wrapped around list titles.
// Apostrophes are deliberately excluded: they open legitimate titles ("'Tis")
// and italic markup has already been stripped by the time this is applied.
const wikiQuoteTrim = "\"“”"

// wikiRefTag strips <ref>...</ref> and self-closing <ref /> citations.
var wikiRefTag = regexp.MustCompile(`(?is)<ref[^>]*/>|<ref[^>]*>.*?</ref>`)

// wikiComment strips <!-- ... --> comments.
var wikiComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// wikiHTMLTag strips any leftover HTML tags (e.g. <br />, <small>).
var wikiHTMLTag = regexp.MustCompile(`(?s)<[^>]+>`)

// wikiNamedParam detects a "name=value" template parameter.
var wikiNamedParam = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_ \-]*=`)

// wikiWhitespace collapses runs of whitespace.
var wikiWhitespace = regexp.MustCompile(`\s+`)

// ParseChapters walks every chapter-list parameter in the wikitext, in document
// order, and maps chapter numbers to titles. It also returns a count of entries
// whose number had to be inferred from list position because the article does
// not number them, which callers can surface as a confidence signal.
//
// Three list formats appear in the wild and are all handled:
//   - {{Numbered list|start=N}} with one entry per pipe (e.g. Chainsaw Man)
//   - "* 012. {{Nihongo|...}}" numbered bullets, including "* 008–011." ranges
//     that share a single title (e.g. Vinland Saga, Berserk)
//   - "* {{Nihongo|...}}" plain bullets with no numbers at all (e.g. Berserk's
//     first volume), which continue from the previous entry's number
//   - "# {{Nihongo|...}}" ordered-list markup, treated exactly like a plain
//     bullet (e.g. Attack on Titan)
func ParseChapters(wikitext string) (Chapters, int) {
	// Articles that number chapters inside the title are inconsistent about
	// zero-padding: "Arisa" writes "01 Tsubasa" through "09 ..." and then plain
	// "10 Mariko Takagi". A bare number is too ambiguous to strip on its own
	// (it would eat the title of "7 Seeds"), so the first pass establishes
	// whether this article labels chapters that way at all, and only then does
	// the second pass trust a bare leading number.
	_, _, labeled := parseChapters(wikitext, false)
	chapters, inferred, _ := parseChapters(wikitext, labeled > 0)
	return chapters, inferred
}

// parseChapters does the work of ParseChapters. allowBareNumbers relaxes the
// in-title number rule; labeled reports how many entries carried one.
func parseChapters(wikitext string, allowBareNumbers bool) (Chapters, int, int) {
	chapters := make(Chapters)
	inferred := 0
	labeled := 0

	// next is the chapter number to assign when an entry carries no explicit
	// number. It runs across every list on the page, in document order.
	next := 1.0

	// A few series (Berserk being the notable one) restart their chapter
	// numbering per arc, so the same number appears more than once on the page.
	// Keep the first title seen for a number.
	assign := func(num float64, title string) {
		if title == "" {
			return
		}
		if _, exists := chapters[num]; !exists {
			chapters[num] = title
		}
	}

	for _, field := range extractChapterListFields(wikitext) {
		if m := wikiNumberedListStart.FindStringSubmatch(field); m != nil {
			// {{Numbered list|start=N|entry|entry|...}}
			start, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			num := start
			for _, entry := range numberedListEntries(field) {
				assign(num, CleanTitle(entry))
				num++
			}
			next = num
			continue
		}

		for _, line := range strings.Split(field, "\n") {
			line = strings.TrimSpace(line)
			// "*" is an unordered list, "#" an ordered one. Articles use both
			// for chapter lists — Attack on Titan numbers its first volume with
			// "#" — and neither marker carries a chapter number of its own.
			if !strings.HasPrefix(line, "*") && !strings.HasPrefix(line, "#") {
				continue
			}
			entry := strings.TrimSpace(strings.TrimLeft(line, "*#"))
			if entry == "" {
				continue
			}

			if m := wikiChapterNumPrefix.FindStringSubmatch(entry); m != nil {
				// Explicit number or range, e.g. "012." or "008–011.".
				start, err := strconv.ParseFloat(m[1], 64)
				if err != nil {
					continue
				}
				end := start
				if m[2] != "" {
					if e, eerr := strconv.ParseFloat(m[2], 64); eerr == nil && e >= start {
						end = e
					}
				}
				title := CleanTitle(entry[len(m[0]):])
				if title == "" {
					continue
				}
				// A range shares one title across every chapter it covers.
				for n := start; n <= end; n++ {
					assign(n, title)
				}
				next = end + 1
				continue
			}

			// A lettered label such as "Bonus Material." marks an extra that is
			// not part of the chapter numbering; skip it without advancing.
			if wikiSpecialEntryPrefix.MatchString(entry) {
				continue
			}

			title := CleanTitle(entry)
			if title == "" {
				continue
			}

			// The number may live inside the title ("Chapter 12: Foo") rather
			// than as a list marker, in which case it is authoritative and the
			// label should not survive into the title.
			if start, end, rest, ok := stripChapterLabel(title, allowBareNumbers); ok {
				labeled++
				if rest == "" {
					// A bare "Chapter 46" with no title carries no information.
					next = end + 1
					continue
				}
				for n := start; n <= end; n++ {
					assign(n, rest)
				}
				next = end + 1
				continue
			}

			// Bonus content sits in these lists but is outside the numbering.
			if wikiExtraEntry.MatchString(title) {
				continue
			}

			assign(next, title)
			inferred++
			next++
		}
	}

	return chapters, inferred, labeled
}

// stripChapterLabel pulls a chapter number out of the front of an already
// cleaned title, returning the number range it covers and the remaining title.
//
// It reports false unless the prefix is unambiguously a chapter marker: a label
// word ("Chapter 12 Foo"), a separator ("12: Foo"), or a zero-padded number
// ("012 Foo"). A bare number followed by a space is left alone, so titles that
// legitimately open with one — "20th Century Boys", "7 Seeds" — survive intact.
func stripChapterLabel(title string, allowBareNumbers bool) (start, end float64, rest string, ok bool) {
	m := wikiLabeledChapterPrefix.FindStringSubmatch(title)
	if m == nil {
		return 0, 0, "", false
	}
	label, number, rangeEnd, separator := m[1], m[2], m[3], m[4]

	zeroPadded := len(number) > 1 && number[0] == '0'
	if label == "" && separator == "" && !zeroPadded && !allowBareNumbers {
		return 0, 0, "", false
	}

	start, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, 0, "", false
	}
	end = start
	if rangeEnd != "" {
		if e, eerr := strconv.ParseFloat(rangeEnd, 64); eerr == nil && e >= start {
			end = e
		}
	}

	// Removing the prefix can expose a quote that was balanced in the original
	// ("Chapter 1: \"Maomao\"" leaves "\"Maomao"), so trim quotes again.
	rest = strings.TrimSpace(title[len(m[0]):])
	rest = strings.Trim(rest, wikiQuoteTrim)
	return start, end, strings.TrimSpace(rest), true
}

// HasChapterList reports whether the wikitext contains at least one
// chapter-list parameter.
func HasChapterList(wikitext string) bool {
	return wikiChapterListField.FindStringIndex(wikitext) != nil
}

// extractChapterListFields returns the raw value of every ChapterList /
// ChapterListColN template parameter, in document order. Values end at the next
// pipe or closing brace that sits at the depth of the enclosing template, so
// nested templates and wikilinks containing pipes are kept intact.
func extractChapterListFields(wikitext string) []string {
	var fields []string

	for offset := 0; offset < len(wikitext); {
		loc := wikiChapterListField.FindStringIndex(wikitext[offset:])
		if loc == nil {
			break
		}
		valueStart := offset + loc[1]
		value, end := readFieldValue(wikitext, valueStart)
		if strings.TrimSpace(value) != "" {
			fields = append(fields, value)
		}
		if end <= valueStart {
			offset = valueStart + 1
		} else {
			offset = end
		}
	}

	return fields
}

// readFieldValue reads a template parameter value starting at from, ending at
// the first "|" or "}}" that is not nested inside a template or wikilink.
// It returns the value and the index just past it.
func readFieldValue(s string, from int) (string, int) {
	depth := 0
	for i := from; i < len(s); i++ {
		switch {
		case strings.HasPrefix(s[i:], "{{"), strings.HasPrefix(s[i:], "[["):
			depth++
			i++
		case strings.HasPrefix(s[i:], "}}"), strings.HasPrefix(s[i:], "]]"):
			if depth == 0 {
				return s[from:i], i
			}
			depth--
			i++
		case s[i] == '|' && depth == 0:
			return s[from:i], i
		}
	}
	return s[from:], len(s)
}

// numberedListEntries splits a {{Numbered list|start=N|a|b|c}} block into its
// positional entries, dropping the named start= parameter.
func numberedListEntries(field string) []string {
	start := strings.Index(field, "{{")
	if start < 0 {
		return nil
	}
	// Trim to the template body, excluding the outer "{{" and "}}".
	body := field[start+2:]
	if end := strings.LastIndex(body, "}}"); end >= 0 {
		body = body[:end]
	}

	var entries []string
	for i, p := range splitParams(body) {
		if i == 0 {
			// The template name plus its start= parameter.
			continue
		}
		if wikiNamedParam.MatchString(strings.TrimSpace(p)) {
			continue
		}
		if strings.TrimSpace(p) == "" {
			continue
		}
		// Some articles tack an unnumbered "* Epilogue" bullet onto the last
		// pipe entry instead of giving it its own pipe. Split those out so they
		// become entries of their own rather than being glued to the previous
		// title.
		for _, sub := range strings.Split(p, "\n*") {
			if strings.TrimSpace(sub) != "" {
				entries = append(entries, sub)
			}
		}
	}
	return entries
}

// splitParams splits template parameters on "|" at the top nesting level,
// leaving pipes inside nested templates and wikilinks untouched.
func splitParams(s string) []string {
	var params []string
	depth := 0
	last := 0
	for i := 0; i < len(s); i++ {
		switch {
		case strings.HasPrefix(s[i:], "{{"), strings.HasPrefix(s[i:], "[["):
			depth++
			i++
		case strings.HasPrefix(s[i:], "}}"), strings.HasPrefix(s[i:], "]]"):
			if depth > 0 {
				depth--
			}
			i++
		case s[i] == '|' && depth == 0:
			params = append(params, s[last:i])
			last = i + 1
		}
	}
	params = append(params, s[last:])
	return params
}

// CleanTitle turns a raw wikitext chapter entry into a plain-text English
// title: it unwraps {{Nihongo}} and similar templates to their first parameter,
// resolves wikilinks, and strips refs, comments, markup and surrounding quotes.
func CleanTitle(entry string) string {
	s := wikiComment.ReplaceAllString(entry, "")
	s = wikiRefTag.ReplaceAllString(s, "")
	s = resolveTemplates(s)
	s = resolveLinks(s)
	// Replace with a space, not "", so <br /> keeps the words either side apart;
	// the whitespace collapse below tidies up the result.
	s = wikiHTMLTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)

	// Strip bold/italic markup.
	s = strings.ReplaceAll(s, "'''", "")
	s = strings.ReplaceAll(s, "''", "")

	s = wikiWhitespace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	// Titles are conventionally quoted in these lists; drop the quotes.
	s = strings.Trim(s, wikiQuoteTrim)
	return strings.TrimSpace(s)
}

// resolveTemplates replaces every {{...}} template with its first positional
// parameter, working innermost-first. For {{Nihongo|"X"|漢字|romaji}} this
// yields the English title; for {{W|Shueisha}} it yields the plain word.
func resolveTemplates(s string) string {
	// Bounded loop: each pass removes at least one template, and deeply nested
	// titles are rare, so this converges well before the limit.
	for pass := 0; pass < 10; pass++ {
		open := -1
		for i := 0; i+1 < len(s); i++ {
			if s[i] == '{' && s[i+1] == '{' {
				open = i
			}
			if s[i] == '}' && s[i+1] == '}' && open >= 0 {
				body := s[open+2 : i]
				s = s[:open] + firstPositionalParam(body) + s[i+2:]
				open = -1
				break
			}
		}
		if open == -1 && !strings.Contains(s, "{{") {
			break
		}
		if !strings.Contains(s, "}}") {
			break
		}
	}
	return s
}

// firstPositionalParam returns the first positional parameter of a template
// body (i.e. the one after the template name), ignoring named parameters such
// as "extra=". It falls back to the next non-empty positional parameter, which
// keeps templates whose first argument is a script-only field usable.
func firstPositionalParam(body string) string {
	params := splitParams(body)
	if len(params) < 2 {
		return ""
	}
	for _, p := range params[1:] {
		p = strings.TrimSpace(p)
		if p == "" || wikiNamedParam.MatchString(p) {
			continue
		}
		return p
	}
	return ""
}

// resolveLinks converts [[target|label]] to label and [[target]] to target.
func resolveLinks(s string) string {
	for pass := 0; pass < 10; pass++ {
		open := strings.Index(s, "[[")
		if open < 0 {
			break
		}
		end := strings.Index(s[open:], "]]")
		if end < 0 {
			break
		}
		end += open
		inner := s[open+2 : end]
		if pipe := strings.LastIndex(inner, "|"); pipe >= 0 {
			inner = inner[pipe+1:]
		}
		s = s[:open] + inner + s[end+2:]
	}
	return s
}
