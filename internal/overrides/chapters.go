package overrides

import (
	"strconv"

	"github.com/kulaid/manga-chapter-titles/chaptertitles"
)

// SourceCurated is the provenance recorded for a hand-written chapter title.
// It is deliberately distinct from the scraper names so a reader can tell which
// titles were decided by a person.
const SourceCurated = "curated"

// ApplyChapters overwrites a series' chapter titles with the curated ones
// recorded for its article, returning how many it changed.
//
// This dataset is a database of chapter titles. Scraping is how most of the
// data gets in, not what makes it true: where Wikipedia and the aggregators are
// wrong, incomplete, or number a chapter differently from the release, the
// right answer is to write the correct title down. Curated entries therefore
// beat every scraped source and survive a rebuild.
//
// An empty value deletes the chapter instead of setting it, which is how to say
// that a title on file is wrong and no source has a right one.
//
// Keys are canonicalised before use. The dataset writes them with
// FormatChapterNumber, so chapter 0.10 is stored as "0.1"; a curated key of
// "0.10" names that same chapter and must overwrite it rather than sit beside
// it. Taking the string literally added a second key, and since consumers parse
// both back to the same float, which title won came down to map iteration
// order. Any other spelling of a canonicalised key is removed for the same
// reason.
func (f *File) ApplyChapters(article string, chapters, sources map[string]string) int {
	if f == nil || chapters == nil {
		return 0
	}
	e, ok := f.Get(article)
	if !ok || len(e.Chapters) == 0 {
		return 0
	}

	// Group the keys already on file by the canonical form they reduce to, so
	// a curated entry can clear every spelling of its chapter.
	spellings := make(map[string][]string, len(chapters))
	for k := range chapters {
		spellings[canonicalChapterKey(k)] = append(spellings[canonicalChapterKey(k)], k)
	}

	changed := 0
	for num, title := range e.Chapters {
		key := canonicalChapterKey(num)

		existed := false
		for _, k := range spellings[key] {
			if k == key {
				existed = true
				continue
			}
			delete(chapters, k)
			delete(sources, k)
			existed = true
		}

		if title == "" {
			if _, still := chapters[key]; still {
				delete(chapters, key)
				delete(sources, key)
			}
			if existed {
				changed++
			}
			continue
		}

		chapters[key] = title
		if sources != nil {
			sources[key] = SourceCurated
		}
		changed++
	}
	return changed
}

// canonicalChapterKey renders a chapter key the way the dataset stores it, so
// "0.10" and "0.1" name the same chapter. A key that is not a number is left
// alone rather than mangled.
func canonicalChapterKey(key string) string {
	n, err := strconv.ParseFloat(key, 64)
	if err != nil {
		return key
	}
	return chaptertitles.FormatChapterNumber(n)
}
