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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kulaid/manga-chapter-titles/chaptertitles"
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
	case "fetch":
		err = runFetch(os.Args[2:])
	case "list":
		err = runList(os.Args[2:])
	case "lookup":
		err = runLookup(os.Args[2:])
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
  fetch     Scrape one series by name, article title, or URL
  list      Print the articles that build would scrape
  lookup    Read a series back out of an existing dataset

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
	if err := parseArgs(fs, args); err != nil {
		return err
	}

	client := newClient(*delay)

	fmt.Fprintf(os.Stderr, "Enumerating %s...\n", wikipedia.ChapterListCategory)
	articles, err := client.CategoryMembers(wikipedia.ChapterListCategory)
	if err != nil {
		return fmt.Errorf("listing category: %w", err)
	}
	if *limit > 0 && len(articles) > *limit {
		articles = articles[:*limit]
	}
	fmt.Fprintf(os.Stderr, "Scraping %d articles into %s/ (delay %s)\n\n", len(articles), *out, *delay)

	var entries []chaptertitles.IndexEntry
	usedSlugs := map[string]bool{}
	var skipped, failed int

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

		s := toSeries(article, result, usedSlugs)
		if err := chaptertitles.Write(*out, s); err != nil {
			return fmt.Errorf("writing %s: %w", s.Slug, err)
		}
		usedSlugs[s.Slug] = true

		entries = append(entries, chaptertitles.IndexEntry{
			Series:       s.Series,
			Slug:         s.Slug,
			MatchKey:     s.MatchKey,
			File:         s.Slug + ".json",
			Article:      s.Article,
			ChapterCount: s.ChapterCount,
		})

		note := ""
		if result.Inferred > 0 {
			note = fmt.Sprintf(" (%d inferred)", result.Inferred)
		}
		fmt.Fprintf(os.Stderr, "%s %-60s %d titles%s\n", progress, truncate(article, 60), len(result.Chapters), note)
	}

	if err := chaptertitles.WriteIndex(*out, entries); err != nil {
		return fmt.Errorf("writing index: %w", err)
	}

	total := 0
	for _, e := range entries {
		total += e.ChapterCount
	}
	fmt.Fprintf(os.Stderr, "\nDone: %d series, %d chapter titles, %d skipped, %d failed\n",
		len(entries), total, skipped, failed)
	return nil
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
