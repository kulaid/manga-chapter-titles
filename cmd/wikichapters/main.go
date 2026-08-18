// Command wikichapters scrapes manga chapter titles from Wikipedia into JSON.
//
//	wikichapters build                 # scrape every series Wikipedia lists
//	wikichapters fetch "Chainsaw Man"  # scrape one series
//	wikichapters list                  # print the articles build would scrape
//	wikichapters lookup "Berserk"      # read a title back out of the dataset
//
// Run "wikichapters <command> -h" for the flags of each command.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kulaid/manga-chapter-titles/chaptertitles"
	"github.com/kulaid/manga-chapter-titles/internal/anilist"
	"github.com/kulaid/manga-chapter-titles/internal/comick"
	"github.com/kulaid/manga-chapter-titles/internal/mangadex"
	"github.com/kulaid/manga-chapter-titles/internal/overrides"
	"github.com/kulaid/manga-chapter-titles/internal/sources"
	"github.com/kulaid/manga-chapter-titles/internal/wikipedia"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "build":
		err = runBuild(os.Args[2:])
	case "add":
		err = runAdd(os.Args[2:])
	case "fetch":
		err = runFetch(os.Args[2:])
	case "list":
		err = runList(os.Args[2:])
	case "lookup":
		err = runLookup(os.Args[2:])
	case "anilist":
		err = runAniList(os.Args[2:])
	case "override":
		err = runOverride(os.Args[2:])
	case "missing":
		err = runMissing(os.Args[2:])
	case "enrich":
		err = runEnrich(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `wikichapters — scrape manga chapter titles from Wikipedia into JSON

Commands:
  build     Scrape every series in Wikipedia's chapter-list category
  add       Scrape and enrich only series missing from an existing dataset
  fetch     Scrape one series by name, article title, or URL
  list      Print the articles that build would scrape
  lookup    Read a series back out of an existing dataset
  anilist   Backfill missing AniList IDs into an existing dataset
  enrich    Fill chapter-title gaps from Comick and MangaDex
  missing   List series whose AniList ID could not be resolved automatically
  override  Record a hand-verified AniList ID that automatic lookup cannot find

Run "wikichapters <command> -h" for flags.
`)
}

// parseArgs parses fs from args, tolerating flags that appear after positional
// arguments. Go's flag package stops at the first non-flag word, which would
// silently swallow `fetch "Chainsaw Man" -stdout` into the series name, so
// flags are hoisted to the front before parsing.
func parseArgs(fs *flag.FlagSet, args []string) error {
	var flags, positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			positional = append(positional, a)
			continue
		}

		name := strings.TrimLeft(a, "-")
		value := ""
		hasInlineValue := false
		if eq := strings.Index(name, "="); eq >= 0 {
			name, value, hasInlineValue = name[:eq], name[eq+1:], true
		}

		f := fs.Lookup(name)
		if f == nil {
			// Unknown flag: hand it to the flag package so it reports the error.
			flags = append(flags, a)
			continue
		}
		if hasInlineValue {
			flags = append(flags, "-"+name+"="+value)
			continue
		}
		// Bool flags stand alone; every other flag consumes the next word.
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			flags = append(flags, "-"+name)
			continue
		}
		if i+1 < len(args) {
			flags = append(flags, "-"+name, args[i+1])
			i++
		} else {
			flags = append(flags, "-"+name)
		}
	}

	return fs.Parse(append(flags, positional...))
}

// newClient builds an API client from the shared rate-limit flags.
func newClient(delay time.Duration) *wikipedia.Client {
	c := wikipedia.NewClient()
	c.Delay = delay
	return c
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	out := fs.String("out", "data", "directory to write JSON files into")
	delay := fs.Duration("delay", wikipedia.DefaultDelay, "pause between API requests")
	limit := fs.Int("limit", 0, "stop after N series (0 = no limit); useful for a smoke test")
	minChapters := fs.Int("min-chapters", 1, "skip series with fewer than N chapter titles")
	withAniList := fs.Bool("anilist", true, "resolve each series' AniList ID (adds ~1.5s per series)")
	ovrPath := fs.String("overrides", overrides.DefaultFile, "hand-curated corrections applied after automatic resolution")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	client := newClient(*delay)

	var ani *anilist.Client
	if *withAniList {
		ani = anilist.NewClient()
	}

	ovr, err := overrides.Load(*ovrPath)
	if err != nil {
		return err
	}
	if ovr.Len() > 0 {
		fmt.Fprintf(os.Stderr, "Applying %d manual override(s) from %s\n", ovr.Len(), *ovrPath)
	}

	fmt.Fprintf(os.Stderr, "Enumerating %s...\n", wikipedia.ChapterListCategory)
	articles, err := client.CategoryMembers(wikipedia.ChapterListCategory)
	if err != nil {
		return fmt.Errorf("listing category: %w", err)
	}
	if *limit > 0 && len(articles) > *limit {
		articles = articles[:*limit]
	}
	articles = orderArticlesForMerge(articles)
	fmt.Fprintf(os.Stderr, "Scraping %d articles into %s/ (delay %s)\n\n", len(articles), *out, *delay)

	// Reuse the IDs an earlier run already resolved. Each lookup costs ~1.5s of
	// rate-limited AniList traffic, so re-deriving IDs that are already on disk
	// roughly doubles the runtime of an incremental rebuild for no new data.
	// Use "wikichapters anilist -force" to re-resolve them deliberately.
	knownIDs := existingAniListIDs(*out)
	if len(knownIDs) > 0 {
		fmt.Fprintf(os.Stderr, "Reusing %d AniList ID(s) already in %s/\n\n", len(knownIDs), *out)
	}

	// A rebuild merges into what is already on disk rather than replacing it,
	// so every aggregator and curated title survives the scraper. This maps a
	// series to the slug its file currently sits under, which is how a series
	// whose name the parser has since changed is still recognised.
	priorSlugByKey := priorSlugsByMatchKey(*out)

	var entries []chaptertitles.IndexEntry
	usedSlugs := map[string]bool{}
	// Articles of a split series resolve to the same name, so they are folded
	// into the record already written for it instead of taking a second slug.
	// Keyed by MatchKey, which is the same normalisation consumers look up by.
	seriesByKey := map[string]*chaptertitles.Series{}
	entryByKey := map[string]int{}
	// Chapter numbers this run's Wikipedia articles have supplied, per series.
	// The collision warning below has to be judged against the articles scraped
	// this run, not against the record on disk: that record already holds every
	// part's chapters from last time, so measuring against it would report a
	// numbering restart for every healthy split series on every rebuild.
	wikiSeenByKey := map[string]map[string]bool{}
	var skipped, failed, merged int

	for i, article := range articles {
		progress := fmt.Sprintf("[%d/%d]", i+1, len(articles))

		result, err := client.Fetch(article)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %-60s FAILED: %v\n", progress, truncate(article, 60), err)
			failed++
			continue
		}
		if len(result.Chapters) < *minChapters {
			// Many entries in this category are volume lists with no chapter
			// titles at all; that is expected, not an error.
			fmt.Fprintf(os.Stderr, "%s %-60s skipped (%d titles)\n", progress, truncate(article, 60), len(result.Chapters))
			skipped++
			continue
		}

		// Resolve the name first so a further part of an already-scraped
		// series can be recognised before a slug is claimed for it.
		if key := wikipedia.NormalizeName(wikipedia.SeriesNameFromArticle(result.Article)); key != "" {
			if existing, ok := seriesByKey[key]; ok {
				part := toSeries(article, result, nil)
				novel := countUnseen(wikiSeenByKey[key], part.Chapters)
				gained := mergeSeriesChapters(existing, part)
				rebuildSourceRefs(existing, nil)
				applyCuratedChapters(existing, ovr)
				markSeen(wikiSeenByKey, key, part.Chapters)
				if err := chaptertitles.Write(*out, existing); err != nil {
					return fmt.Errorf("writing %s: %w", existing.Slug, err)
				}
				entries[entryByKey[key]] = indexEntryFor(existing)
				merged++
				fmt.Fprintf(os.Stderr, "%s %-60s +%d titles, merged into %s (%d)\n",
					progress, truncate(article, 60), gained, existing.Slug, existing.ChapterCount)
				if w := mergeCollisionWarning(article, novel, len(part.Chapters)); w != "" {
					fmt.Fprint(os.Stderr, w)
				}
				continue
			}
		}

		s := toSeries(article, result, usedSlugs)
		scrapedChapters := s.Chapters
		s.AniListID = knownIDs[article]

		// Fold the scrape onto the record a previous run left, so the
		// aggregator and curated titles attached to it are not thrown away.
		// A series with no prior record is written exactly as scraped.
		kept := 0
		if prior := priorSeriesFor(*out, s, priorSlugByKey); prior != nil {
			s = carryForward(prior, s)
			kept = len(s.Chapters) - len(scrapedChapters)
		}

		resolveAniListID(ani, ovr, s)
		applyCuratedChapters(s, ovr)
		rebuildSourceRefs(s, nil)
		if err := chaptertitles.Write(*out, s); err != nil {
			return fmt.Errorf("writing %s: %w", s.Slug, err)
		}
		usedSlugs[s.Slug] = true
		if s.MatchKey != "" {
			seriesByKey[s.MatchKey] = s
			entryByKey[s.MatchKey] = len(entries)
			markSeen(wikiSeenByKey, s.MatchKey, scrapedChapters)
		}

		entries = append(entries, indexEntryFor(s))

		// Write the index as we go, not just at the end: the index is how
		// consumers find these files, so an interrupted run would otherwise
		// leave a directory of series nothing can look up.
		const flushEvery = 20
		if len(entries)%flushEvery == 0 {
			if ferr := writeIndexCopy(*out, entries); ferr != nil {
				return fmt.Errorf("writing index: %w", ferr)
			}
		}

		note := ""
		if result.Inferred > 0 {
			note = fmt.Sprintf(" (%d inferred)", result.Inferred)
		}
		if kept > 0 {
			note += fmt.Sprintf(" (+%d kept)", kept)
		}
		if s.AniListID != 0 {
			note += fmt.Sprintf(" [al:%d]", s.AniListID)
		}
		fmt.Fprintf(os.Stderr, "%s %-60s %d titles%s\n", progress, truncate(article, 60), s.ChapterCount, note)
	}

	if err := writeIndexCopy(*out, entries); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}

	total, withIDs := 0, 0
	for _, e := range entries {
		total += e.ChapterCount
		if e.AniListID != 0 {
			withIDs++
		}
	}
	fmt.Fprintf(os.Stderr, "\nDone: %d series, %d chapter titles, %d with AniList IDs, %d merged parts, %d skipped, %d failed\n",
		len(entries), total, withIDs, merged, skipped, failed)
	return nil
}

// runAdd scrapes and enriches only the series a dataset does not already have.
//
// A full "build" plus "enrich" costs the better part of an hour, nearly all of
// it re-deriving series that have not changed. Picking up newly reachable
// articles — a dozen after the subcategory fix — should cost a minute, so this
// filters the category listing against the existing index and touches nothing
// else.
func runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	dir := fs.String("data", "data", "dataset directory to add to")
	series := fs.String("series", "", "add only this series (by name); default is every missing one")
	delay := fs.Duration("delay", wikipedia.DefaultDelay, "pause between API requests")
	minChapters := fs.Int("min-chapters", 1, "skip series with fewer than N chapter titles")
	ovrPath := fs.String("overrides", overrides.DefaultFile, "hand-curated corrections applied after automatic resolution")
	useComick := fs.Bool("comick", true, "consult Comick")
	useMangaDex := fs.Bool("mangadex", true, "consult MangaDex")
	dryRun := fs.Bool("dry-run", false, "list what would be added without scraping")
	refresh := fs.Bool("refresh", false, "re-scrape -series even though it is already in the dataset")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	idx, err := chaptertitles.ReadIndex(*dir)
	if err != nil {
		return fmt.Errorf("reading dataset: %w", err)
	}

	have := make(map[string]bool, len(idx.Series))
	usedSlugs := make(map[string]bool, len(idx.Series))
	for _, e := range idx.Series {
		have[e.MatchKey] = true
		usedSlugs[e.Slug] = true
	}

	// AniList IDs already resolved cost ~1.5s each to re-derive, and a refresh
	// would otherwise drop the one it has.
	knownIDs := existingAniListIDs(*dir)

	ovr, err := overrides.Load(*ovrPath)
	if err != nil {
		return err
	}

	client := newClient(*delay)
	fmt.Fprintf(os.Stderr, "Enumerating %s...\n", wikipedia.ChapterListCategory)
	articles, err := client.CategoryMembers(wikipedia.ChapterListCategory)
	if err != nil {
		return fmt.Errorf("listing category: %w", err)
	}

	// Filter on the article title alone, before spending a fetch on it.
	wantKey := ""
	if *series != "" {
		wantKey = chaptertitles.MatchKey(*series)
	}

	// A refresh rebuilds one series from its articles. Forget what is held for
	// it — the match key, its slug and its index row — so the normal scrape
	// path treats it as new and every part is merged in from scratch.
	if *refresh {
		if wantKey == "" {
			return fmt.Errorf("-refresh needs -series to say which series to re-scrape")
		}
		if !have[wantKey] {
			return fmt.Errorf("-refresh: %q is not in the dataset", *series)
		}
		kept := idx.Series[:0]
		for _, e := range idx.Series {
			if e.MatchKey == wantKey {
				delete(usedSlugs, e.Slug)
				continue
			}
			kept = append(kept, e)
		}
		idx.Series = kept
		delete(have, wantKey)
		// Re-resolve the AniList ID too. A refresh is how a wrong one gets
		// corrected, so seeding the stored value back would defeat it.
		for article := range knownIDs {
			if wikipedia.NormalizeName(wikipedia.SeriesNameFromArticle(article)) == wantKey {
				delete(knownIDs, article)
			}
		}
	}
	var todo []string
	for _, a := range articles {
		key := wikipedia.NormalizeName(wikipedia.SeriesNameFromArticle(a))
		if key == "" || have[key] {
			continue
		}
		if wantKey != "" && key != wantKey {
			continue
		}
		todo = append(todo, a)
	}

	todo = orderArticlesForMerge(todo)

	if len(todo) == 0 {
		fmt.Fprintln(os.Stderr, "Nothing to add: every series in the category is already in the dataset.")
		return nil
	}
	fmt.Fprintf(os.Stderr, "%d article(s) not in the dataset:\n", len(todo))
	for _, a := range todo {
		fmt.Fprintf(os.Stderr, "  %s\n", a)
	}
	if *dryRun {
		return nil
	}
	fmt.Fprintln(os.Stderr)

	ani := anilist.NewClient()
	seriesByKey := map[string]*chaptertitles.Series{}
	entryByKey := map[string]int{}
	// See runBuild: a split series' collision warning is judged against the
	// articles scraped this run, not against the record on disk.
	wikiSeenByKey := map[string]map[string]bool{}
	var added, mergedParts, skipped, failed int

	for i, article := range todo {
		progress := fmt.Sprintf("[%d/%d]", i+1, len(todo))

		result, err := client.Fetch(article)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %-60s FAILED: %v\n", progress, truncate(article, 60), err)
			failed++
			continue
		}
		if len(result.Chapters) < *minChapters {
			fmt.Fprintf(os.Stderr, "%s %-60s skipped (%d titles)\n", progress, truncate(article, 60), len(result.Chapters))
			skipped++
			continue
		}

		// The fetch may have followed a redirect to an article whose name
		// resolves to a series already held, so re-check after fetching.
		key := wikipedia.NormalizeName(wikipedia.SeriesNameFromArticle(result.Article))
		if key != "" {
			if existing, ok := seriesByKey[key]; ok {
				part := toSeries(article, result, nil)
				novel := countUnseen(wikiSeenByKey[key], part.Chapters)
				gained := mergeSeriesChapters(existing, part)
				rebuildSourceRefs(existing, nil)
				applyCuratedChapters(existing, ovr)
				markSeen(wikiSeenByKey, key, part.Chapters)
				if err := chaptertitles.Write(*dir, existing); err != nil {
					return fmt.Errorf("writing %s: %w", existing.Slug, err)
				}
				idx.Series[entryByKey[key]] = indexEntryFor(existing)
				mergedParts++
				fmt.Fprintf(os.Stderr, "%s %-60s +%d titles, merged into %s (%d)\n",
					progress, truncate(article, 60), gained, existing.Slug, existing.ChapterCount)
				if w := mergeCollisionWarning(article, novel, len(part.Chapters)); w != "" {
					fmt.Fprint(os.Stderr, w)
				}
				continue
			}
			if have[key] {
				fmt.Fprintf(os.Stderr, "%s %-60s already in the dataset\n", progress, truncate(article, 60))
				skipped++
				continue
			}
		}

		s := toSeries(article, result, usedSlugs)
		s.AniListID = knownIDs[article]
		resolveAniListID(ani, ovr, s)
		applyCuratedChapters(s, ovr)
		if err := chaptertitles.Write(*dir, s); err != nil {
			return fmt.Errorf("writing %s: %w", s.Slug, err)
		}
		usedSlugs[s.Slug] = true
		have[s.MatchKey] = true
		if s.MatchKey != "" {
			seriesByKey[s.MatchKey] = s
			entryByKey[s.MatchKey] = len(idx.Series)
			markSeen(wikiSeenByKey, s.MatchKey, s.Chapters)
		}
		idx.Series = append(idx.Series, indexEntryFor(s))
		added++

		note := ""
		if s.AniListID != 0 {
			note = fmt.Sprintf(" [al:%d]", s.AniListID)
		}
		fmt.Fprintf(os.Stderr, "%s %-60s %d titles%s\n", progress, truncate(article, 60), len(result.Chapters), note)
	}

	if err := writeIndexCopy(*dir, idx.Series); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\nScraped: %d added, %d merged parts, %d skipped, %d failed\n",
		added, mergedParts, skipped, failed)

	if added == 0 {
		return nil
	}

	var srcs []sources.Source
	if *useComick {
		srcs = append(srcs, comick.New(wikipedia.DefaultUserAgent))
	}
	if *useMangaDex {
		srcs = append(srcs, mangadex.New(wikipedia.DefaultUserAgent))
	}
	if len(srcs) == 0 {
		return nil
	}

	// Enrich exactly what was just scraped, leaving the rest of the dataset
	// untouched.
	fresh := make(map[string]bool, len(seriesByKey))
	for key := range seriesByKey {
		fresh[key] = true
	}
	fmt.Fprintf(os.Stderr, "\nEnriching %d new series...\n", len(fresh))
	return enrichDataset(*dir, idx, srcs, func(e *chaptertitles.IndexEntry) bool {
		return fresh[e.MatchKey]
	}, 0, ovr)
}

func runFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	out := fs.String("out", "data", "directory to write the JSON file into")
	delay := fs.Duration("delay", wikipedia.DefaultDelay, "pause between API requests")
	stdout := fs.Bool("stdout", false, "print the JSON instead of writing a file")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: wikichapters fetch <series name | article title | URL>")
	}
	ref := strings.Join(fs.Args(), " ")

	client := newClient(*delay)

	// A bare series name is resolved through search first; an article title or
	// URL is used as given.
	article := ref
	if !strings.Contains(ref, "://") && !strings.HasPrefix(strings.ToLower(ref), "list of ") {
		found, err := client.FindSeries(ref)
		if err != nil {
			return fmt.Errorf("searching for %q: %w", ref, err)
		}
		if found == "" {
			return fmt.Errorf("no Wikipedia chapter list found for %q", ref)
		}
		fmt.Fprintf(os.Stderr, "Resolved %q -> %q\n", ref, found)
		article = found
	}

	result, err := client.Fetch(article)
	if err != nil {
		return err
	}
	if result.Followed {
		fmt.Fprintf(os.Stderr, "Followed link to %q\n", result.Article)
	}
	if len(result.Chapters) == 0 {
		return fmt.Errorf("no chapter titles found in %q", result.Article)
	}

	s := toSeries(article, result, nil)
	ovr, oerr := overrides.Load(overrides.DefaultFile)
	if oerr != nil {
		return oerr
	}
	resolveAniListID(anilist.NewClient(), ovr, s)

	if *stdout {
		return printJSON(s)
	}
	if err := chaptertitles.Write(*out, s); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Wrote %s/%s.json (%d titles", *out, s.Slug, s.ChapterCount)
	if result.Inferred > 0 {
		fmt.Fprintf(os.Stderr, ", %d inferred", result.Inferred)
	}
	fmt.Fprintln(os.Stderr, ")")
	return nil
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	delay := fs.Duration("delay", wikipedia.DefaultDelay, "pause between API requests")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	articles, err := newClient(*delay).CategoryMembers(wikipedia.ChapterListCategory)
	if err != nil {
		return err
	}
	for _, a := range articles {
		fmt.Println(a)
	}
	fmt.Fprintf(os.Stderr, "\n%d articles\n", len(articles))
	return nil
}

func runLookup(args []string) error {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	dir := fs.String("data", "data", "dataset directory to read")
	chapter := fs.String("chapter", "", "print only this chapter's title")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: wikichapters lookup <series name> [-chapter N]")
	}
	query := wikipedia.NormalizeName(strings.Join(fs.Args(), " "))

	idx, err := chaptertitles.ReadIndex(*dir)
	if err != nil {
		return fmt.Errorf("reading dataset: %w", err)
	}

	var match *chaptertitles.IndexEntry
	for i := range idx.Series {
		if idx.Series[i].MatchKey == query {
			match = &idx.Series[i]
			break
		}
	}
	if match == nil {
		return fmt.Errorf("no series in %s matches %q", *dir, strings.Join(fs.Args(), " "))
	}

	s, err := chaptertitles.Read(*dir, match.Slug)
	if err != nil {
		return err
	}

	if *chapter != "" {
		title, ok := s.Chapters[*chapter]
		if !ok {
			return fmt.Errorf("%s has no chapter %s", s.Series, *chapter)
		}
		fmt.Println(title)
		return nil
	}

	fmt.Printf("%s — %d chapters (%s)\n", s.Series, s.ChapterCount, s.SourceURL)
	nums := make([]float64, 0, len(s.Chapters))
	for k := range s.Chapters {
		if f, err := strconv.ParseFloat(k, 64); err == nil {
			nums = append(nums, f)
		}
	}
	sort.Float64s(nums)
	for _, n := range nums {
		key := chaptertitles.FormatChapterNumber(n)
		fmt.Printf("%8s  %s\n", key, s.Chapters[key])
	}
	return nil
}

// toSeries converts a scrape result into the on-disk record, assigning a slug
// that does not collide with one already used in this run.
func toSeries(requestedArticle string, r *wikipedia.Result, usedSlugs map[string]bool) *chaptertitles.Series {
	name := wikipedia.SeriesNameFromArticle(r.Article)
	if name == "" {
		name = wikipedia.SeriesNameFromArticle(requestedArticle)
	}

	slug := chaptertitles.Slugify(name)
	if slug == "" {
		slug = chaptertitles.Slugify(r.Article)
	}
	if usedSlugs != nil {
		base := slug
		for i := 2; usedSlugs[slug]; i++ {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
	}

	chapters := make(map[string]string, len(r.Chapters))
	for num, title := range r.Chapters {
		chapters[chaptertitles.FormatChapterNumber(num)] = title
	}

	return &chaptertitles.Series{
		Series:          name,
		Slug:            slug,
		MatchKey:        wikipedia.NormalizeName(name),
		Article:         r.Article,
		SourceURL:       r.URL,
		ScrapedAt:       time.Now().UTC(),
		ChapterCount:    len(chapters),
		InferredNumbers: r.Inferred,
		Chapters:        chapters,
	}
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// resolveAniListID fills in s.AniListID, leaving it zero when AniList has no
// entry that can be confirmed as this series. A lookup failure is reported but
// never fatal: the chapter titles are the point, the ID is a convenience.
func resolveAniListID(ani *anilist.Client, ovr *overrides.File, s *chaptertitles.Series) {
	// A hand-verified ID always wins, and short-circuits the lookup: it was
	// recorded precisely because automatic resolution gets this series wrong.
	if e, ok := ovr.Get(s.Article); ok && e.AniListID != 0 {
		// A negative ID records that the series deliberately has none: AniList
		// carries no entry for it, and the only candidates that match its name
		// are unrelated works listing it as a synonym. Attaching one of those
		// pulls another series' chapter titles through the aggregators, so no
		// ID is the correct answer and it has to stick across runs.
		if e.AniListID < 0 {
			s.AniListID = 0
			return
		}
		s.AniListID = e.AniListID
		return
	}
	if ani == nil || s.AniListID != 0 {
		return
	}
	id, ok, err := ani.FindID(s.Series)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  AniList lookup for %q failed: %v\n", s.Series, err)
		return
	}
	if ok {
		s.AniListID = id
	}
}

// orderArticlesForMerge groups a category listing by series and puts each
// series' chapter-list articles ahead of its volume-list articles.
//
// Wikipedia frequently carries both for one series. A volume list numbers its
// entries by position within each volume, while the chapter list carries the
// real chapter numbers, so whichever is merged first decides the numbering for
// the whole series. Captain Tsubasa's chapters article contributed 0 of its 114
// chapters — every one collided with the volume list scraped before it.
//
// Series keep the order they were enumerated in, and the parts within a series
// keep theirs, so the result is stable across runs.
func orderArticlesForMerge(articles []string) []string {
	type keyed struct {
		article  string
		seriesAt int
		prec     int
		at       int
	}

	seriesAt := map[string]int{}
	items := make([]keyed, 0, len(articles))
	for i, a := range articles {
		key := wikipedia.NormalizeName(wikipedia.SeriesNameFromArticle(a))
		if _, seen := seriesAt[key]; !seen {
			seriesAt[key] = len(seriesAt)
		}
		items = append(items, keyed{article: a, seriesAt: seriesAt[key], prec: articleMergePrecedence(a), at: i})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].seriesAt != items[j].seriesAt {
			return items[i].seriesAt < items[j].seriesAt
		}
		if items[i].prec != items[j].prec {
			return items[i].prec < items[j].prec
		}
		return items[i].at < items[j].at
	})

	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.article
	}
	return out
}

// articleMergePrecedence ranks an article title by how trustworthy its chapter
// numbering is: a "List of X chapters" article beats a "List of X volumes" one.
// The descriptor is the word before any trailing parenthetical, so
// "List of Fairy Tail chapters (volumes 1–15)" counts as a chapter list.
func articleMergePrecedence(article string) int {
	name := strings.ToLower(stripTrailingParenthetical(article))
	if strings.HasSuffix(name, "chapters") {
		return 0
	}
	return 1
}

// trailingParenthetical matches a parenthetical at the end of an article title.
var trailingParenthetical = regexp.MustCompile(`\s*\([^()]*\)\s*$`)

// stripTrailingParenthetical exposes the descriptor word an article title ends
// with, so "List of Fairy Tail chapters (volumes 1–15)" reads as a chapter list
// rather than a volume one.
func stripTrailingParenthetical(article string) string {
	return strings.TrimSpace(trailingParenthetical.ReplaceAllString(article, ""))
}

// applyCuratedChapters writes the hand-curated titles for a series over
// whatever was scraped, and keeps the count in step. Every command that writes
// a series calls this last, so curated data survives a rebuild.
func applyCuratedChapters(s *chaptertitles.Series, ovr *overrides.File) int {
	if s.Chapters == nil {
		s.Chapters = map[string]string{}
	}
	if s.ChapterSources == nil {
		s.ChapterSources = map[string]string{}
	}
	n := ovr.ApplyChapters(s.Article, s.Chapters, s.ChapterSources)
	if n > 0 {
		s.ChapterCount = len(s.Chapters)
	}
	return n
}

// mergeSeriesChapters folds src's freshly scraped Wikipedia chapters into dst.
//
// Wikipedia splits a long series across several articles — One Piece has six,
// Naruto three — and each is scraped separately. They are one series, so their
// chapters are unioned rather than written to competing slugs. dst keeps its own
// article as the record's provenance.
//
// The union is rank-aware rather than first-wins, because dst now arrives
// carrying whatever a previous run enriched it with. A licensed title from a
// second article has to be able to displace the scanlator title an aggregator
// supplied for that chapter, while a curated title, or an equally-ranked
// Wikipedia title from the article scraped before this one, holds.
//
// It returns how many chapters src won. A part that wins far fewer than it holds
// is the signature of an article that restarts its numbering at 1 — Naruto's
// "Part II" articles do — so its chapters collide with the first part's and are
// dropped. The count lets the caller say so rather than silently losing several
// hundred licensed titles. Guessing an offset to realign them is deliberately
// not attempted: a wrong guess attaches the wrong title to every chapter it
// touches.
func mergeSeriesChapters(dst, src *chaptertitles.Series) int {
	added := applyWikipediaTitles(dst, src.Chapters)
	dst.InferredNumbers += src.InferredNumbers
	return added
}

// applyWikipediaTitles merges a scrape's titles into a series record at
// Wikipedia's rank, returning how many chapters they won.
func applyWikipediaTitles(dst *chaptertitles.Series, scraped map[string]string) int {
	stored := storedFor(dst)
	// Anything already in this record with no source recorded came from
	// Wikipedia: the record is itself Wikipedia-derived, and this merge only
	// ever folds in another Wikipedia article. Saying so explicitly, rather than
	// leaning on whether Article happens to be populated, is what keeps two
	// parts of a split series at equal rank so the first one scraped wins.
	if stored.DefaultSource == "" {
		stored.DefaultSource = sources.NameWikipedia
	}
	fetched := make(sources.Titles, len(scraped))
	for num, title := range scraped {
		if n, err := strconv.ParseFloat(num, 64); err == nil {
			fetched[n] = title
		}
	}

	merged := sources.Merge(stored, []sources.Result{{
		Found: true, Ref: dst.Article, URL: dst.SourceURL, Titles: fetched,
	}}, []string{sources.NameWikipedia})

	writeTitles(dst, merged)
	if len(merged.Contributions) == 0 {
		return 0
	}
	return merged.Contributions[0].Added
}

// storedFor describes what a series record already holds, for a merge.
//
// Chapters with nothing in chapter_sources are attributed to Wikipedia when the
// record names an article. Ten series in the dataset are in exactly that state —
// Wikipedia-only files that enrichment skipped for want of an AniList ID — and
// ranking their titles as unknown would let an aggregator overwrite 617 licensed
// Pokémon Adventures titles the moment one gets an ID pinned.
func storedFor(s *chaptertitles.Series) sources.Stored {
	stored := sources.Stored{
		Titles:        make(sources.Titles, len(s.Chapters)),
		Provenance:    make(map[float64]string, len(s.ChapterSources)),
		DefaultSource: sourceNameForExisting(s),
	}
	for num, title := range s.Chapters {
		n, err := strconv.ParseFloat(num, 64)
		if err != nil {
			continue
		}
		stored.Titles[n] = title
		if src := s.ChapterSources[num]; src != "" {
			stored.Provenance[n] = src
		}
	}
	return stored
}

// writeTitles writes a merge's titles and provenance back onto a series record.
func writeTitles(s *chaptertitles.Series, merged sources.Merged) {
	chapters := make(map[string]string, len(merged.Titles))
	provenance := make(map[string]string, len(merged.Provenance))
	for num, title := range merged.Titles {
		key := chaptertitles.FormatChapterNumber(num)
		chapters[key] = title
		if src, ok := merged.Provenance[num]; ok && src != "" {
			provenance[key] = src
		}
	}
	s.Chapters = chapters
	s.ChapterSources = provenance
	s.ChapterCount = len(chapters)
}

// priorSlugsByMatchKey maps each series in a dataset to the slug its file is
// under, so a series whose slug has changed can still be found.
//
// The parser fixes rename articles — stripping "(1–186)" turns six One Piece
// slugs into one — and a rename must not read as "no previous record" and
// discard the enrichment attached to it.
func priorSlugsByMatchKey(dir string) map[string]string {
	byKey := map[string]string{}
	idx, err := chaptertitles.ReadIndex(dir)
	if err != nil {
		return byKey
	}
	for _, e := range idx.Series {
		if e.MatchKey != "" && e.Slug != "" {
			byKey[e.MatchKey] = e.Slug
		}
	}
	return byKey
}

// priorSeriesFor loads the record a previous run wrote for this series, or nil
// when the dataset has none.
//
// The match key must agree. usedSlugs disambiguation puts genuinely distinct
// series on neighbouring slugs, and merging one into the other would graft
// another manga's chapter titles onto this one — the exact failure this dataset
// joins on IDs to avoid. Every failure yields nil, which just means the series
// is rebuilt from Wikipedia alone, as it was before.
func priorSeriesFor(dir string, s *chaptertitles.Series, slugByKey map[string]string) *chaptertitles.Series {
	slugs := []string{s.Slug}
	if prior, ok := slugByKey[s.MatchKey]; ok && prior != s.Slug {
		slugs = append(slugs, prior)
	}
	for _, slug := range slugs {
		prior, err := chaptertitles.Read(dir, slug)
		if err != nil || prior.MatchKey != s.MatchKey {
			continue
		}
		return prior
	}
	return nil
}

// carryForward folds a fresh scrape onto the record already on disk, so a
// rebuild refreshes Wikipedia without discarding what enrichment added.
//
// build used to write the scrape straight over the file, which is why it could
// never be run without enrich behind it: every aggregator and curated title in
// the dataset disappeared the moment the scraper ran. The scrape now merges in
// at Wikipedia's rank instead. Article metadata is taken from the scrape, since
// refreshing it is the point; the AniList ID is kept, since re-deriving it is
// slow and AniList's search is not stable enough to do it for free.
func carryForward(prior, scraped *chaptertitles.Series) *chaptertitles.Series {
	out := *prior

	out.Series = scraped.Series
	out.Slug = scraped.Slug
	out.MatchKey = scraped.MatchKey
	out.Article = scraped.Article
	out.SourceURL = scraped.SourceURL
	out.ScrapedAt = scraped.ScrapedAt
	out.InferredNumbers = scraped.InferredNumbers
	if scraped.AniListID != 0 {
		out.AniListID = scraped.AniListID
	}

	// Wikipedia's share of the record is replaced, not added to. A rebuild
	// exists to pick up parser fixes, and those fixes both correct titles in
	// place — the old parser read a section heading as Monster's chapter 87 —
	// and renumber chapters, so merging additively would keep every stale title
	// and leave renumbered chapters in the file twice under two numbers. Titles
	// owned by anyone else are untouched, which is the whole point of build no
	// longer overwriting.
	stripSource(&out, sources.NameWikipedia)
	applyWikipediaTitles(&out, scraped.Chapters)
	rebuildSourceRefs(&out, nil)
	return &out
}

// stripSource removes the chapters a given source owns from a series record,
// so a fresh read of that source can replace them. Chapters with no source
// recorded count as the record's default owner; see storedFor.
func stripSource(s *chaptertitles.Series, name string) {
	def := sourceNameForExisting(s)
	chapters := make(map[string]string, len(s.Chapters))
	provenance := make(map[string]string, len(s.ChapterSources))
	for num, title := range s.Chapters {
		owner := s.ChapterSources[num]
		if owner == "" {
			owner = def
		}
		if owner == name {
			continue
		}
		chapters[num] = title
		if owner != "" {
			provenance[num] = owner
		}
	}
	s.Chapters = chapters
	s.ChapterSources = provenance
	s.ChapterCount = len(chapters)
}

// rebuildSourceRefs recomputes a series' per-source counts from the provenance
// map, which is the only thing that still says who owns each title once merges
// are ranked: a source that supplied nothing this run can still own titles it
// supplied in an earlier one. contribs, when given, refreshes the ref and URL
// for the sources consulted this run.
func rebuildSourceRefs(s *chaptertitles.Series, contribs []sources.Contribution) {
	counts := map[string]int{}
	for num := range s.Chapters {
		if src := s.ChapterSources[num]; src != "" {
			counts[src]++
		}
	}

	type meta struct{ ref, url string }
	metas := map[string]meta{}
	for _, r := range s.Sources {
		metas[r.Name] = meta{r.Ref, r.URL}
	}
	if s.Article != "" {
		metas[sources.NameWikipedia] = meta{s.Article, s.SourceURL}
	}
	for _, c := range contribs {
		if c.Ref != "" || c.URL != "" {
			metas[c.Name] = meta{c.Ref, c.URL}
		}
		// A source consulted this run stays listed even when it owns nothing,
		// so the record shows it was asked.
		if _, ok := counts[c.Name]; !ok {
			counts[c.Name] = 0
		}
	}

	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if ri, rj := sources.Rank(names[i]), sources.Rank(names[j]); ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})

	refs := make([]chaptertitles.SourceRef, 0, len(names))
	for _, name := range names {
		refs = append(refs, chaptertitles.SourceRef{
			Name: name, Ref: metas[name].ref, URL: metas[name].url, Count: counts[name],
		})
	}
	s.Sources = refs
}

// mergeCollisionWarning describes a part whose chapters mostly collided with
// what the series already had, or "" when the merge looks healthy.
func mergeCollisionWarning(article string, added, held int) string {
	if held == 0 || added*2 >= held {
		return ""
	}
	return fmt.Sprintf("      warning: %s contributed %d of its %d chapters; "+
		"the rest collided, so this article probably restarts numbering at 1\n",
		article, added, held)
}

// countUnseen reports how many of a part's chapter numbers no earlier article
// of the same series has supplied this run.
func countUnseen(seen map[string]bool, chapters map[string]string) int {
	n := 0
	for num := range chapters {
		if !seen[num] {
			n++
		}
	}
	return n
}

// markSeen records the chapter numbers an article supplied for a series.
func markSeen(seenByKey map[string]map[string]bool, key string, chapters map[string]string) {
	if key == "" {
		return
	}
	seen, ok := seenByKey[key]
	if !ok {
		seen = make(map[string]bool, len(chapters))
		seenByKey[key] = seen
	}
	for num := range chapters {
		seen[num] = true
	}
}

// existingAniListIDs reads the AniList IDs a previous run already resolved,
// keyed by Wikipedia article title. Article is the key rather than slug because
// a slug can change when a series is renamed, while the article it was scraped
// from is what build iterates over.
//
// Every failure here is silent and yields an empty map: no dataset is the
// normal first-run case, and a corrupt index only costs the AniList lookups
// this is trying to avoid. Neither is a reason to abort a scrape.
func existingAniListIDs(dir string) map[string]int {
	ids := map[string]int{}
	idx, err := chaptertitles.ReadIndex(dir)
	if err != nil {
		return ids
	}
	for _, e := range idx.Series {
		if e.Article != "" && e.AniListID != 0 {
			ids[e.Article] = e.AniListID
		}
	}
	return ids
}

// indexEntryFor builds the index row for a series record.
func indexEntryFor(s *chaptertitles.Series) chaptertitles.IndexEntry {
	return chaptertitles.IndexEntry{
		Series:       s.Series,
		Slug:         s.Slug,
		MatchKey:     s.MatchKey,
		AniListID:    s.AniListID,
		File:         s.Slug + ".json",
		Article:      s.Article,
		ChapterCount: s.ChapterCount,
	}
}

// runAniList backfills AniList IDs into an existing dataset, so adding the IDs
// does not require re-scraping every Wikipedia article.
func runAniList(args []string) error {
	fs := flag.NewFlagSet("anilist", flag.ExitOnError)
	dir := fs.String("data", "data", "dataset directory to update in place")
	delay := fs.Duration("delay", anilist.DefaultDelay, "pause between AniList requests")
	force := fs.Bool("force", false, "re-resolve series that already have an ID")
	ovrPath := fs.String("overrides", overrides.DefaultFile, "hand-curated corrections applied after automatic resolution")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	idx, err := chaptertitles.ReadIndex(*dir)
	if err != nil {
		return fmt.Errorf("reading dataset: %w", err)
	}

	ani := anilist.NewClient()
	ani.Delay = *delay

	ovr, oerr := overrides.Load(*ovrPath)
	if oerr != nil {
		return oerr
	}
	if ovr.Len() > 0 {
		fmt.Fprintf(os.Stderr, "Applying %d manual override(s) from %s\n", ovr.Len(), *ovrPath)
	}

	var entries []chaptertitles.IndexEntry
	var resolved, already, missed int

	// The index is what consumers look series up through, so it has to be
	// rewritten as we go rather than only at the end: an interrupted run would
	// otherwise leave IDs in the series files that nothing can find. Rows not
	// yet reached are carried over unchanged, so every flush is a complete index.
	flushIndex := func(processed int) error {
		complete := append(append([]chaptertitles.IndexEntry{}, entries...), idx.Series[processed:]...)
		return chaptertitles.WriteIndex(*dir, complete)
	}

	for i, e := range idx.Series {
		progress := fmt.Sprintf("[%d/%d]", i+1, len(idx.Series))

		s, rerr := chaptertitles.Read(*dir, e.Slug)
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "%s %-44s FAILED: %v\n", progress, truncate(e.Series, 44), rerr)
			entries = append(entries, e)
			continue
		}

		// An override is a deliberate correction, so it applies even to a
		// series that already carries an ID. Without this a wrong ID, or one
		// recorded as deliberately absent, could only be fixed by re-resolving
		// the whole dataset -- and AniList's search is not stable enough run to
		// run for that to be a safe thing to reach for.
		if e, ok := ovr.Get(s.Article); ok && e.AniListID != 0 {
			want := e.AniListID
			if want < 0 {
				want = 0
			}
			if s.AniListID != want {
				s.AniListID = want
				if werr := chaptertitles.Write(*dir, s); werr != nil {
					return fmt.Errorf("writing %s: %w", s.Slug, werr)
				}
				fmt.Fprintf(os.Stderr, "[%d/%d] %-38s override -> %d\n", i+1, len(idx.Series), truncate(s.Series, 38), want)
				resolved++
			}
			// This loop rebuilds the index from entries, so every path has to
			// contribute its row or the series drops out of the dataset.
			entries = append(entries, indexEntryFor(s))
			continue
		}

		if s.AniListID != 0 && !*force {
			already++
			entries = append(entries, indexEntryFor(s))
			continue
		}
		if *force {
			s.AniListID = 0
		}

		resolveAniListID(ani, ovr, s)
		if s.AniListID != 0 {
			resolved++
			fmt.Fprintf(os.Stderr, "%s %-44s al:%d\n", progress, truncate(e.Series, 44), s.AniListID)
		} else {
			missed++
			fmt.Fprintf(os.Stderr, "%s %-44s no confident match\n", progress, truncate(e.Series, 44))
		}

		if werr := chaptertitles.Write(*dir, s); werr != nil {
			return fmt.Errorf("writing %s: %w", s.Slug, werr)
		}
		entries = append(entries, indexEntryFor(s))

		const flushEvery = 20
		if len(entries)%flushEvery == 0 {
			if ferr := flushIndex(i + 1); ferr != nil {
				return fmt.Errorf("writing index: %w", ferr)
			}
		}
	}

	if err := flushIndex(len(idx.Series)); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nDone: %d resolved, %d already had an ID, %d without a confident match\n",
		resolved, already, missed)
	return nil
}

// writeIndexCopy writes the index from a copy of entries. WriteIndex sorts what
// it is given, and the build loop keeps appending to its accumulator afterwards,
// so handing over the live slice would shuffle it mid-run.
func writeIndexCopy(dir string, entries []chaptertitles.IndexEntry) error {
	return chaptertitles.WriteIndex(dir, append([]chaptertitles.IndexEntry{}, entries...))
}

// runMissing lists the series whose AniList ID could not be resolved. It is the
// entry point to the manual pass: whatever it prints is what needs a hand-
// verified override.
func runMissing(args []string) error {
	fs := flag.NewFlagSet("missing", flag.ExitOnError)
	dir := fs.String("data", "data", "dataset directory to inspect")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	idx, err := chaptertitles.ReadIndex(*dir)
	if err != nil {
		return fmt.Errorf("reading dataset: %w", err)
	}

	var missing []chaptertitles.IndexEntry
	for _, e := range idx.Series {
		if e.AniListID == 0 {
			missing = append(missing, e)
		}
	}

	for _, e := range missing {
		// Print the article title, since that is the key `override` takes.
		fmt.Printf("%-44s %s\n", truncate(e.Series, 44), e.Article)
	}
	fmt.Fprintf(os.Stderr, "\n%d of %d series have no AniList ID.\n", len(missing), len(idx.Series))
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Record one with:\n  wikichapters override \"<series>\" -anilist <id>\n")
	}
	return nil
}

// runOverride records a hand-verified AniList ID for a series, and applies it to
// the dataset immediately so the correction takes effect without a full rebuild.
func runOverride(args []string) error {
	fs := flag.NewFlagSet("override", flag.ExitOnError)
	dir := fs.String("data", "data", "dataset directory to update")
	ovrPath := fs.String("overrides", overrides.DefaultFile, "overrides file to write")
	anilistID := fs.Int("anilist", 0, "hand-verified AniList manga ID")
	note := fs.String("note", "", "why automatic lookup could not find it")
	article := fs.String("article", "", "target the Wikipedia article directly instead of by series name")
	force := fs.Bool("force", false, "record the ID without contacting AniList at all")
	yes := fs.Bool("yes", false, "record the ID even though its AniList title does not match the series name")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	name := strings.Join(fs.Args(), " ")
	if name == "" && *article == "" {
		return fmt.Errorf("usage: wikichapters override \"<series>\" -anilist <id> [-note \"...\"]")
	}
	if *anilistID <= 0 {
		return fmt.Errorf("-anilist must be a positive AniList manga ID")
	}

	idx, err := chaptertitles.ReadIndex(*dir)
	if err != nil {
		return fmt.Errorf("reading dataset: %w", err)
	}

	// Resolve the series to its article, which is what overrides are keyed on:
	// article titles survive the series name, slug and match key being adjusted.
	var target *chaptertitles.IndexEntry
	for i := range idx.Series {
		e := &idx.Series[i]
		if *article != "" {
			if e.Article == *article {
				target = e
				break
			}
			continue
		}
		if e.MatchKey == chaptertitles.MatchKey(name) || strings.EqualFold(e.Series, name) {
			target = e
			break
		}
	}
	if target == nil {
		return fmt.Errorf("no series in %s matches %q — run \"wikichapters missing\" to see the exact names", *dir, name+*article)
	}

	// Confirm the ID is the series the curator meant. An override skips the
	// title verification that automatic resolution applies, and a wrong number
	// is usually still a valid AniList ID belonging to some other manga, so
	// without this a typo records silently and then looks authoritative.
	anilistTitle := ""
	if !*force {
		m, found, lerr := anilist.NewClient().ByID(*anilistID)
		if lerr != nil {
			return fmt.Errorf("confirming AniList ID %d: %w (use -force to skip the check)", *anilistID, lerr)
		}
		if !found {
			return fmt.Errorf("AniList has no manga with ID %d", *anilistID)
		}
		anilistTitle = m.Title.Romaji
		if anilistTitle == "" {
			anilistTitle = m.Title.English
		}

		// Overrides exist precisely because the names disagree, so a mismatch
		// cannot simply be rejected. It can be made impossible to miss: the
		// command stops and shows what the ID actually is, and recording it
		// takes a second, deliberate run.
		if _, ok := anilist.Match(target.Series, []anilist.Media{m}); !ok && !*yes {
			fmt.Fprintf(os.Stderr, "AniList %d is %q.\n", *anilistID, anilistTitle)
			for _, n := range m.Names() {
				fmt.Fprintf(os.Stderr, "    also known as: %s\n", n)
			}
			return fmt.Errorf("that does not look like %q — if it is correct, re-run with -yes", target.Series)
		}
		fmt.Fprintf(os.Stderr, "AniList %d is %q\n", *anilistID, anilistTitle)
	}

	ovr, err := overrides.Load(*ovrPath)
	if err != nil {
		return err
	}
	ovr.Set(target.Article, overrides.Entry{
		Series:       target.Series,
		AniListID:    *anilistID,
		AniListTitle: anilistTitle,
		Note:         *note,
	})
	if err := ovr.Save(*ovrPath); err != nil {
		return err
	}

	// Apply it now, so the dataset is correct without waiting for a rebuild.
	s, err := chaptertitles.Read(*dir, target.Slug)
	if err != nil {
		return fmt.Errorf("reading %s: %w", target.Slug, err)
	}
	s.AniListID = *anilistID
	if err := chaptertitles.Write(*dir, s); err != nil {
		return fmt.Errorf("writing %s: %w", target.Slug, err)
	}
	target.AniListID = *anilistID
	if err := chaptertitles.WriteIndex(*dir, idx.Series); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s (%s) -> AniList %d\nRecorded in %s; it will survive future builds.\n",
		target.Series, target.Article, *anilistID, *ovrPath)
	return nil
}

// runEnrich fills chapter-title gaps from the aggregator sources.
//
// It works on an existing dataset rather than rebuilding it: Wikipedia supplies
// the licensed titles, and this adds titles for the chapters — and the series —
// Wikipedia does not cover, without re-scraping anything already collected.
//
// Every source is joined on the AniList ID, so a series without one is skipped
// rather than matched by name.
func runEnrich(args []string) error {
	fs := flag.NewFlagSet("enrich", flag.ExitOnError)
	dir := fs.String("data", "data", "dataset directory to update in place")
	only := fs.String("only", "", "restrict to one series (by name), for spot checks")
	limit := fs.Int("limit", 0, "stop after N series (0 = no limit)")
	useComick := fs.Bool("comick", true, "consult Comick")
	useMangaDex := fs.Bool("mangadex", true, "consult MangaDex")
	ovrPath := fs.String("overrides", overrides.DefaultFile, "hand-curated corrections applied after the sources")
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	ovr, err := overrides.Load(*ovrPath)
	if err != nil {
		return err
	}

	idx, err := chaptertitles.ReadIndex(*dir)
	if err != nil {
		return fmt.Errorf("reading dataset: %w", err)
	}

	// Priority order: the licensed Wikipedia titles already in the file win,
	// then Comick (which names more chapters than MangaDex), then MangaDex.
	var srcs []sources.Source
	if *useComick {
		srcs = append(srcs, comick.New(wikipedia.DefaultUserAgent))
	}
	if *useMangaDex {
		srcs = append(srcs, mangadex.New(wikipedia.DefaultUserAgent))
	}
	if len(srcs) == 0 {
		return fmt.Errorf("no sources enabled")
	}

	want := func(e *chaptertitles.IndexEntry) bool {
		return *only == "" || chaptertitles.MatchKey(*only) == e.MatchKey
	}
	return enrichDataset(*dir, idx, srcs, want, *limit, ovr)
}

// enrichDataset merges the aggregator sources into every series of idx that
// want selects, writing each file and the index back as it goes. It is shared
// by "enrich", which selects everything or one named series, and by "add",
// which selects only the series it has just scraped.
func enrichDataset(dir string, idx *chaptertitles.Index, srcs []sources.Source, want func(*chaptertitles.IndexEntry) bool, limit int, ovr *overrides.File) error {
	var processed, enriched, addedTotal, skipped int

	for i := range idx.Series {
		e := &idx.Series[i]
		if limit > 0 && processed >= limit {
			break
		}
		if want != nil && !want(e) {
			continue
		}
		if e.AniListID == 0 {
			// No exact key, so there is nothing safe to join on.
			skipped++
			continue
		}
		processed++

		s, rerr := chaptertitles.Read(dir, e.Slug)
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "[%d] %-38s FAILED: %v\n", processed, truncate(e.Series, 38), rerr)
			continue
		}

		before := len(s.Chapters)

		var results []sources.Result
		var names []string
		for _, src := range srcs {
			r, ferr := src.Fetch(e.AniListID, s.Series)
			if ferr != nil {
				fmt.Fprintf(os.Stderr, "      %s: %v\n", src.Name(), ferr)
			}
			results = append(results, r)
			names = append(names, src.Name())
		}

		merged := sources.Merge(storedFor(s), results, names)
		applyMerge(s, merged)
		applyCuratedChapters(s, ovr)

		if werr := chaptertitles.Write(dir, s); werr != nil {
			return fmt.Errorf("writing %s: %w", s.Slug, werr)
		}
		e.ChapterCount = s.ChapterCount
		e.SourceNames = sourceNames(s)

		added := len(s.Chapters) - before
		addedTotal += added
		if added > 0 {
			enriched++
		}
		fmt.Fprintf(os.Stderr, "[%d] %-38s %d -> %d (+%d) %v\n",
			processed, truncate(e.Series, 38), before, len(s.Chapters), added, e.SourceNames)

		const flushEvery = 20
		if processed%flushEvery == 0 {
			if ferr := writeIndexCopy(dir, idx.Series); ferr != nil {
				return fmt.Errorf("writing index: %w", ferr)
			}
		}
	}

	if err := writeIndexCopy(dir, idx.Series); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nDone: %d series checked, %d gained titles, %d titles added, %d skipped (no AniList ID)\n",
		processed, enriched, addedTotal, skipped)
	return nil
}

// sourceNameForExisting reports which source the titles already in a file came
// from. Files written before provenance existed hold Wikipedia titles.
func sourceNameForExisting(s *chaptertitles.Series) string {
	if s.Article != "" {
		return "wikipedia"
	}
	return ""
}

// applyMerge writes a merge result back onto a series record.
func applyMerge(s *chaptertitles.Series, merged sources.Merged) {
	writeTitles(s, merged)
	rebuildSourceRefs(s, merged.Contributions)
}

// sourceNames lists the names of the sources that contributed to a series.
func sourceNames(s *chaptertitles.Series) []string {
	var out []string
	for _, r := range s.Sources {
		if r.Count > 0 {
			out = append(out, r.Name)
		}
	}
	return out
}
