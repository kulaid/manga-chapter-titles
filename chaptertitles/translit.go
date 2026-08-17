package chaptertitles

import "strings"

// foldRune maps an accented Latin letter to its unaccented ASCII equivalent.
//
// Series names arrive with romanisation macrons ("Boku wa Imōto ni Koi o Suru")
// and European accents ("Übel Blatt"). Discarding those characters outright
// mangles both the slug and the match key — "Imōto" collapses to "im-to", and a
// user typing the plain-ASCII spelling then fails to find the series. Folding
// them to the base letter keeps names readable and makes the two spellings
// agree.
//
// The table covers Latin-1 Supplement and Latin Extended-A, which is every
// accented letter these sources produce. Anything outside it — CJK, the
// multiplication sign in "Hunter × Hunter", "½" — is not a letter with an ASCII
// equivalent and is left for the caller to treat as a separator.
//
// This is deliberately hand-rolled rather than pulled from golang.org/x/text:
// the mapping is small and fixed, and the repo has no third-party dependencies.
var foldRune = map[rune]string{
	'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Ä': "A", 'Å': "A", 'Ā': "A", 'Ă': "A", 'Ą': "A",
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'Æ': "AE", 'æ': "ae",
	'Ç': "C", 'Ć': "C", 'Ĉ': "C", 'Ċ': "C", 'Č': "C",
	'ç': "c", 'ć': "c", 'ĉ': "c", 'ċ': "c", 'č': "c",
	'Ď': "D", 'Đ': "D", 'ď': "d", 'đ': "d",
	'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E", 'Ē': "E", 'Ĕ': "E", 'Ė': "E", 'Ę': "E", 'Ě': "E",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ĕ': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'Ĝ': "G", 'Ğ': "G", 'Ġ': "G", 'Ģ': "G",
	'ĝ': "g", 'ğ': "g", 'ġ': "g", 'ģ': "g",
	'Ĥ': "H", 'Ħ': "H", 'ĥ': "h", 'ħ': "h",
	'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I", 'Ĩ': "I", 'Ī': "I", 'Ĭ': "I", 'Į': "I", 'İ': "I",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ĩ': "i", 'ī': "i", 'ĭ': "i", 'į': "i", 'ı': "i",
	'Ĵ': "J", 'ĵ': "j",
	'Ķ': "K", 'ķ': "k",
	'Ĺ': "L", 'Ļ': "L", 'Ľ': "L", 'Ł': "L",
	'ĺ': "l", 'ļ': "l", 'ľ': "l", 'ł': "l",
	'Ñ': "N", 'Ń': "N", 'Ņ': "N", 'Ň': "N",
	'ñ': "n", 'ń': "n", 'ņ': "n", 'ň': "n",
	'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ö': "O", 'Ø': "O", 'Ō': "O", 'Ŏ': "O", 'Ő': "O",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ō': "o", 'ŏ': "o", 'ő': "o",
	'Œ': "OE", 'œ': "oe",
	'Ŕ': "R", 'Ŗ': "R", 'Ř': "R", 'ŕ': "r", 'ŗ': "r", 'ř': "r",
	'Ś': "S", 'Ŝ': "S", 'Ş': "S", 'Š': "S",
	'ś': "s", 'ŝ': "s", 'ş': "s", 'š': "s",
	'ß': "ss",
	'Ţ': "T", 'Ť': "T", 'Ŧ': "T", 'ţ': "t", 'ť': "t", 'ŧ': "t",
	'Ù': "U", 'Ú': "U", 'Û': "U", 'Ü': "U", 'Ũ': "U", 'Ū': "U", 'Ŭ': "U", 'Ů': "U", 'Ű': "U", 'Ų': "U",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ũ': "u", 'ū': "u", 'ŭ': "u", 'ů': "u", 'ű': "u", 'ų': "u",
	'Ŵ': "W", 'ŵ': "w",
	'Ý': "Y", 'Ŷ': "Y", 'Ÿ': "Y", 'ý': "y", 'ŷ': "y", 'ÿ': "y",
	'Ź': "Z", 'Ż': "Z", 'Ž': "Z", 'ź': "z", 'ż': "z", 'ž': "z",
	'Þ': "TH", 'þ': "th", 'Ð': "D", 'ð': "d",
}

// FoldAccents replaces accented Latin letters with their ASCII equivalents,
// leaving every other character untouched.
func FoldAccents(s string) string {
	// Almost every title is pure ASCII; skip the rebuild when there is nothing
	// to fold.
	hasAccent := false
	for _, r := range s {
		if r > 127 {
			if _, ok := foldRune[r]; ok {
				hasAccent = true
				break
			}
		}
	}
	if !hasAccent {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if folded, ok := foldRune[r]; ok {
			b.WriteString(folded)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
