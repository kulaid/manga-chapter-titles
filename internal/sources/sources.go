// Package sources defines the common shape of a chapter-title source and the
// rules for merging several of them into one set of titles.
//
// Wikipedia is the best source where it exists — it carries the licensed
// English titles — but it only covers licensed/notable series and lags recent
// chapters. The aggregators cover far more series and stay current, at the cost
// of carrying scanlator titles. Merging them means a series gets the official
// title where one is known and a scanlator title everywhere else, rather than
// nothing.
//
// Every source is joined on the AniList ID, never on a name. Each one is asked
// to confirm that the entry it found carries that exact ID before its titles
// are accepted, so a series the source does not have contributes nothing rather
// than something plausible-looking from a different manga.
package sources

import "sort"

// Titles maps a chapter number to its title.
type Titles map[float64]string

// Result is what one source knows about one series.
type Result struct {
	// Found is false when the source has no entry for this AniList ID.
	Found bool
	// Ref identifies the entry within the source (a MangaDex UUID, a Comick
	// slug, a Wikipedia article), recorded so a curator can check the match.
	Ref string
	// URL is a human-followable link to that entry, when the source has one.
	URL string
	// Titles are the chapter titles found.
	Titles Titles
}

// Source fetches chapter titles for a series.
type Source interface {
	// Name is the short identifier recorded as a title's provenance.
	Name() string
	// Fetch looks the series up by AniList ID. seriesName is a search hint
	// only — the match is confirmed against anilistID, never against the name.
	Fetch(anilistID int, seriesName string) (Result, error)
}

// Contribution records what one source added to a merged set.
type Contribution struct {
	Name  string
	Ref   string
	URL   string
	Added int // titles this source supplied that no earlier source had
	Total int // titles this source knew about, merged or not
}

// Merged is the outcome of combining several sources.
type Merged struct {
	Titles Titles
	// Provenance maps a chapter number to the name of the source its title
	// came from.
	Provenance map[float64]string
	// Contributions are per-source, in the priority order they were merged.
	Contributions []Contribution
}

// Stored is what a dataset file already holds for a series, as the input to a
// merge.
type Stored struct {
	// Titles are the chapter titles on file.
	Titles Titles
	// Provenance maps a chapter number to the source its stored title came
	// from, as persisted in chapter_sources.
	Provenance map[float64]string
	// DefaultSource ranks chapters that Provenance does not name. Files written
	// before provenance was recorded — and Wikipedia-only files that enrichment
	// skipped for want of an AniList ID — carry titles with no recorded source,
	// and ranking those as unknown would invite an aggregator to overwrite
	// licensed titles. Callers pass the source such a file's titles must have
	// come from; empty means genuinely unknown.
	DefaultSource string
}

// sourceOf reports the source that owns a stored chapter's title.
func (s Stored) sourceOf(num float64) string {
	if name := s.Provenance[num]; name != "" {
		return name
	}
	return s.DefaultSource
}

// Merge combines a series' stored titles with freshly fetched ones, keeping for
// each chapter the title whose source ranks best. See Rank for the order.
//
// A fetched title replaces a stored one only on a strictly better rank. Equal
// ranks keep the stored value, so re-running a source over its own titles never
// churns the file, and a stored title whose source has stopped serving that
// chapter is retained because nothing arrives to displace it. That retention is
// what makes MangaPlus worth consulting at all: its free window slides forward,
// so a title captured today is gone from the API within weeks.
//
// results and names are parallel; names identify each result's source for both
// ranking and provenance.
func Merge(stored Stored, results []Result, names []string) Merged {
	out := Merged{
		Titles:     make(Titles, len(stored.Titles)),
		Provenance: make(map[float64]string, len(stored.Titles)),
	}

	for num, title := range stored.Titles {
		if title == "" {
			continue
		}
		out.Titles[num] = title
		if name := stored.sourceOf(num); name != "" {
			out.Provenance[num] = name
		}
	}

	for i, r := range results {
		if !r.Found {
			continue
		}
		name := ""
		if i < len(names) {
			name = names[i]
		}
		rank := Rank(name)

		c := Contribution{Name: name, Ref: r.Ref, URL: r.URL, Total: len(r.Titles)}
		for num, title := range r.Titles {
			if title == "" {
				continue
			}
			if _, taken := out.Titles[num]; taken && rank >= Rank(out.Provenance[num]) {
				continue
			}
			out.Titles[num] = title
			out.Provenance[num] = name
			c.Added++
		}
		out.Contributions = append(out.Contributions, c)
	}

	return out
}

// SortedNumbers returns the chapter numbers of a title set in ascending order.
func SortedNumbers(t Titles) []float64 {
	nums := make([]float64, 0, len(t))
	for n := range t {
		nums = append(nums, n)
	}
	sort.Float64s(nums)
	return nums
}
