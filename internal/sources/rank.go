package sources

// Source names, as recorded in a series' chapter_sources map.
const (
	// NameCurated marks a title a person wrote down deliberately. It mirrors
	// overrides.SourceCurated, which cannot be referenced here without a cycle.
	NameCurated   = "curated"
	NameWikipedia = "wikipedia"
	NameMangaPlus = "mangaplus"
	NameComick    = "comick"
	NameMangaDex  = "mangadex"
)

// RankUnknown is the rank of any source name not in the table. It sorts last,
// so an unrecognised name can never displace a title from a known source.
const RankUnknown = 99

// ranks orders the sources by how much their titles are trusted. Lower is
// better.
//
//	curated   a person decided this; the dataset is a database of chapter
//	          titles and scraping is only how most of them get in.
//	wikipedia the licensed English titles, bare ("Dog & Chainsaw") and the most
//	          consistently formatted, but it lags recent chapters.
//	mangaplus official Shueisha titles, and current — but only a handful of
//	          chapters per series, and sometimes shouted ("RYOMEN SUKUNA").
//	comick    broad coverage, scanlator titles.
//	mangadex  the same, naming fewer chapters than Comick.
var ranks = map[string]int{
	NameCurated:   0,
	NameWikipedia: 1,
	NameMangaPlus: 2,
	NameComick:    3,
	NameMangaDex:  4,
}

// Rank reports how much a source's titles are trusted, lower being better.
// An unrecognised or empty name gets RankUnknown.
func Rank(name string) int {
	if r, ok := ranks[name]; ok {
		return r
	}
	return RankUnknown
}
