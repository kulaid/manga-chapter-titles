package overrides

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
func (f *File) ApplyChapters(article string, chapters, sources map[string]string) int {
	if f == nil || chapters == nil {
		return 0
	}
	e, ok := f.Get(article)
	if !ok || len(e.Chapters) == 0 {
		return 0
	}

	changed := 0
	for num, title := range e.Chapters {
		if title == "" {
			if _, exists := chapters[num]; exists {
				delete(chapters, num)
				delete(sources, num)
				changed++
			}
			continue
		}
		chapters[num] = title
		if sources != nil {
			sources[num] = SourceCurated
		}
		changed++
	}
	return changed
}
