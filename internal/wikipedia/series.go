package wikipedia

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/kulaid/manga-chapter-titles/chaptertitles"
)

// ChapterListCategory holds every "List of <series> chapters/volumes" article
// on the English Wikipedia. It is the source of truth for a full corpus build.
const ChapterListCategory = "Category:Lists of manga volumes and chapters"

// articlePathRegex pulls the article title out of a /wiki/<title> path.
var articlePathRegex = regexp.MustCompile(`/wiki/([^?#]+)`)

// chapterListTitleRegex matches an article title of the form
// "List of <series> chapters", the conventional name for a chapter list, and
// captures the series name.
var chapterListTitleRegex = regexp.MustCompile(`(?i)^List of (.+) chapters$`)

// chapterListLinkRegex finds a reference to a dedicated chapter-list article,
// used to hop from a series article to its "List of X chapters" page. Series
// articles name that page in several ways — a [[wikilink]], a {{Main}} or
// {{See also}} hatnote (sometimes with a {{!}} pipe escape), or an infobox
// "volume_list =" parameter — so this matches the article name wherever it
// appears rather than trying to enumerate the wrappers.
var chapterListLinkRegex = regexp.MustCompile(`List of [^\n\[\]{}|=]{1,60}? chapters`)

// Result is everything scraped for one series.
type Result struct {
	Article  string   // resolved article title, e.g. "List of Berserk chapters"
	URL      string   // canonical article URL
	Chapters Chapters // chapter number -> English title
	Inferred int      // chapters whose number came from list position, not the article
	Followed bool     // true when a series article delegated to a chapter-list article
}

// Fetch scrapes chapter titles for one article reference, which may be a full
// Wikipedia URL, a bare article title, or a series name.
//
// When the named article carries no chapter list of its own — common for series
// articles, which delegate to a separate page — the linked "List of X chapters"
// article is followed once.
func (c *Client) Fetch(ref string) (*Result, error) {
	article, err := ArticleFromRef(ref)
	if err != nil {
		return nil, err
	}

	title, wikitext, err := c.Wikitext(article)
	if err != nil {
		return nil, fmt.Errorf("fetching %q: %w", article, err)
	}

	followed := false
	if !HasChapterList(wikitext) {
		if linked := FindChapterListLink(wikitext); linked != "" && linked != title {
			linkedTitle, linkedText, lerr := c.Wikitext(linked)
			if lerr == nil {
				title, wikitext, followed = linkedTitle, linkedText, true
			}
		}
	}

	chapters, inferred := ParseChapters(wikitext)

	return &Result{
		Article:  title,
		URL:      c.ArticleURL(title),
		Chapters: chapters,
		Inferred: inferred,
		Followed: followed,
	}, nil
}

// FindSeries locates the article most likely to hold a series' chapter list and
// returns its title, or "" when there is no confident match.
func (c *Client) FindSeries(seriesTitle string) (string, error) {
	seriesTitle = strings.TrimSpace(seriesTitle)
	if seriesTitle == "" {
		return "", nil
	}
	hits, err := c.Search(fmt.Sprintf("List of %s chapters", seriesTitle), 5)
	if err != nil {
		return "", err
	}
	return PickChapterListArticle(seriesTitle, hits), nil
}

// PickChapterListArticle chooses the search hit that actually belongs to
// seriesTitle, or "" when none does.
//
// Wikipedia's search is fuzzy, so a series with no chapter-list article still
// gets offered other series' lists — searching "Solo Leveling" returns
// "List of Hunter × Hunter chapters". Matching a hit therefore requires the
// series name to match exactly once normalised; a loose contains-check would
// pair "Monster" with "List of Monster Musume chapters". Returning nothing is
// the safe outcome, since a wrong article yields another manga's titles.
func PickChapterListArticle(seriesTitle string, hits []string) string {
	want := NormalizeName(seriesTitle)
	if want == "" {
		return ""
	}

	for _, hit := range hits {
		if m := chapterListTitleRegex.FindStringSubmatch(hit); m != nil && NormalizeName(m[1]) == want {
			return hit
		}
	}
	// No dedicated list article: fall back to the series article itself, which
	// may carry the chapter list inline or link out to it. Only an exact title
	// match is accepted.
	for _, hit := range hits {
		if NormalizeName(hit) == want {
			return hit
		}
	}
	return ""
}

// articleDescriptorWords are the trailing nouns Wikipedia appends when naming a
// list article — "List of Girls und Panzer books", "List of Lupin the Third
// manga", "List of Angels of Death manga chapters". They describe the article,
// not the series, so they must come off before the name is used to search other
// sites.
var articleDescriptorWords = []string{"chapters", "volumes", "books", "manga", "light novels", "novels"}

// SeriesNameFromArticle strips the "List of ..." wrapper and any trailing
// descriptor words from an article title, returning the series name. Titles
// that don't follow the convention are returned unchanged.
func SeriesNameFromArticle(article string) string {
	name := strings.TrimSpace(article)
	name = strings.TrimPrefix(name, "List of ")

	// Peel descriptors off one at a time: "Angels of Death manga chapters"
	// needs both "chapters" and "manga" removed. A descriptor is only dropped
	// when something is left behind, so a series actually called "Manga" or
	// "Books" survives.
	for changed := true; changed; {
		changed = false
		for _, w := range articleDescriptorWords {
			suffix := " " + w
			if !strings.HasSuffix(strings.ToLower(name), suffix) {
				continue
			}
			trimmed := strings.TrimSpace(name[:len(name)-len(suffix)])
			if trimmed == "" {
				continue
			}
			name, changed = trimmed, true
			break
		}
	}

	return strings.TrimSpace(name)
}

// FindChapterListLink returns the title of a linked "List of <series> chapters"
// article, or "" when the wikitext references none.
func FindChapterListLink(wikitext string) string {
	m := chapterListLinkRegex.FindString(wikitext)
	if m == "" {
		return ""
	}
	// Hatnotes often italicise the series name in the display text; the article
	// title itself never contains the markup.
	m = strings.ReplaceAll(m, "'''", "")
	m = strings.ReplaceAll(m, "''", "")
	return strings.TrimSpace(wikiWhitespace.ReplaceAllString(m, " "))
}

// ArticleFromRef turns a Wikipedia URL or a bare article title into an article
// title suitable for the API.
func ArticleFromRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty article reference")
	}
	if !strings.Contains(ref, "://") && !strings.Contains(ref, "/wiki/") {
		return ref, nil
	}

	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("parsing URL: %w", err)
	}
	if t := u.Query().Get("title"); t != "" {
		return t, nil
	}
	m := articlePathRegex.FindStringSubmatch(u.Path)
	if len(m) < 2 || m[1] == "" {
		return "", fmt.Errorf("could not extract article title from %q", ref)
	}
	decoded, derr := url.PathUnescape(m[1])
	if derr != nil {
		decoded = m[1]
	}
	return strings.ReplaceAll(decoded, "_", " "), nil
}

// NormalizeName normalises a series name for comparison. It is the same rule
// consumers apply via chaptertitles.MatchKey, delegated so the scraper and the
// dataset can never disagree about what counts as the same series.
func NormalizeName(s string) string {
	return chaptertitles.MatchKey(s)
}
