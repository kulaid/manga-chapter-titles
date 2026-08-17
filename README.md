# manga-chapter-titles

Scrapes **official English manga chapter titles** from Wikipedia into plain JSON
files, so an application can look up "chapter 47 of Berserk" without hitting the
network or parsing wikitext itself.

Wikipedia carries the *licensed* English titles (Viz, Kodansha, Yen Press),
which is what makes it worth scraping — aggregator APIs such as Comick and
MangaDex mostly carry scanlator titles. The trade-off is coverage: only
licensed/notable series have articles, and the newest chapters lag. Treat this
as the highest-quality source to consult first, not as a complete one.

The committed `data/` directory currently holds **338 series and 53,031 chapter
titles** (2.8 MB), built from the 407 articles in Wikipedia's chapter-list
category; the other 69 are volume lists carrying no chapter titles.

## Quick start

```sh
make build                          # build ./bin/wikichapters
./bin/wikichapters fetch "Berserk"  # scrape one series into data/berserk.json
./bin/wikichapters build            # scrape every series Wikipedia lists
./bin/wikichapters lookup "Berserk" -chapter 47
```

## Data format

The dataset lives in `data/`: one file per series plus an index.

```
data/
  index.json
  berserk.json
  chainsaw-man.json
  ...
```

`data/<slug>.json`:

```json
{
  "series": "Chainsaw Man",
  "slug": "chainsaw-man",
  "match_key": "chainsawman",
  "article": "List of Chainsaw Man chapters",
  "source_url": "https://en.wikipedia.org/wiki/List_of_Chainsaw_Man_chapters",
  "scraped_at": "2026-08-17T02:03:55Z",
  "chapter_count": 232,
  "inferred_numbers": 0,
  "chapters": {
    "1": "Dog & Chainsaw",
    "2": "The Place Where Pochita Is",
    "4.5": "An Interlude"
  }
}
```

Chapter numbers are **object keys, as decimal strings** — not array indices.
They are sparse and not always whole numbers, since half-chapters like `4.5` are
common.

`data/index.json` lists every series so a consumer can resolve a name to a file
without reading the whole dataset:

```json
{
  "generated_at": "2026-08-17T02:20:11Z",
  "count": 338,
  "series": [
    {
      "series": "Berserk",
      "slug": "berserk",
      "match_key": "berserk",
      "file": "berserk.json",
      "article": "List of Berserk chapters",
      "chapter_count": 381
    }
  ]
}
```

### `match_key`

Series names differ in punctuation between sources ("Hunter × Hunter" vs
"Hunter x Hunter", "Love Is War" vs "Love is War"). `match_key` is the name
reduced to lowercase alphanumerics with a standalone `x` dropped, so a consumer
can normalise its own title the same way and compare directly. The rule is
`chaptertitles.MatchKey`.

### `inferred_numbers`

Most articles number their chapters explicitly. Some list them as plain bullets,
where the number comes from the entry's position in the list. `inferred_numbers`
counts how many titles in the file were numbered that way — if it is non-zero,
treat that file's numbering as best-effort rather than authoritative.

## Using the data from Go

The `chaptertitles` package is the consumer-facing half of this repo — import it
and point it at a `data/` directory:

```go
import "github.com/kulaid/manga-chapter-titles/chaptertitles"

db, err := chaptertitles.Load("data")
if err != nil {
    return err
}

if title, ok := db.Title("Berserk", 47); ok {
    fmt.Println(title) // "Wounds (1–2)"
}

// Or pull a whole series at once, keyed by chapter number.
titles, ok := db.Titles("Chainsaw Man") // map[float64]string
```

`Load` reads only `index.json`; each series file is loaded on first use and
cached, so touching a few series doesn't pay for the whole corpus. `DB` is safe
for concurrent use.

Series names are matched with `chaptertitles.MatchKey`, so capitalisation and
punctuation don't have to line up — `"Hunter x Hunter"` finds
`"Hunter × Hunter"`.

### Without the Go package

The JSON is plain and self-describing, so any language can read it directly.
Chapter numbers are strings, so format yours the same way
(`strconv.FormatFloat(n, 'f', -1, 64)` in Go, `str(n).rstrip('.0')`-style logic
elsewhere — `1` not `1.0`, `4.5` stays `4.5`).

Vendoring `data/` into a consuming app, or fetching it from this repo at build
time, avoids any runtime dependency on Wikipedia.

## Commands

| Command | What it does |
| --- | --- |
| `build` | Scrapes every article in Wikipedia's chapter-list category and rewrites `data/` |
| `fetch <series\|article\|URL>` | Scrapes one series; accepts a name, an article title, or a full URL |
| `list` | Prints the articles `build` would scrape, without scraping them |
| `lookup <series>` | Reads a series back out of an existing dataset |

Useful flags:

- `-out <dir>` — where to write (default `data`)
- `-delay <duration>` — pause between API requests (default `1s`)
- `-limit <n>` — `build` only; stop after N series, for a smoke test
- `-stdout` — `fetch` only; print the JSON instead of writing a file
- `-chapter <n>` — `lookup` only; print just that chapter's title

Flags may appear before or after the series name.

## How series are discovered

`build` enumerates `Category:Lists of manga volumes and chapters`, which is how
Wikipedia itself indexes these articles. Entries in that category that turn out
to be volume lists with no chapter titles are skipped and reported.

`fetch` instead resolves a name through Wikipedia search, and only accepts a
result whose series name matches exactly once normalised. Wikipedia's search is
fuzzy enough to offer `List of Hunter × Hunter chapters` for "Solo Leveling", so
a loose match would import a different manga's titles entirely. When nothing
matches confidently, `fetch` reports no result rather than guessing.

If the resolved article has no chapter list of its own — common for series
articles, which delegate to a separate page — the linked `List of X chapters`
article is followed once.

## Parsing notes

The chapter lists are hand-written wikitext, and three formats are in use:

- `{{Numbered list|start=N}}` with one entry per pipe (Chainsaw Man)
- `* 012. {{Nihongo|...}}` numbered bullets, including `* 008–011.` ranges that
  share a single title across several chapters (Berserk, Vinland Saga)
- `* {{Nihongo|...}}` plain bullets with no numbers, which continue from the
  previous entry

On top of that the parser unwraps `{{Nihongo}}` and similar templates, resolves
wikilinks, strips refs/comments/HTML, skips non-chapter bullets like
`Bonus Material.`, and splits stray bullets that articles glue onto the previous
entry. Series that restart numbering per arc (Berserk) produce duplicate chapter
numbers; the first title in document order wins.

Every one of those cases is pinned by a test in
`internal/wikipedia/parse_test.go`.

## Politeness

Requests are serialised with a configurable delay (1s default) and send a
descriptive `User-Agent`, per the
[Wikimedia API etiquette](https://www.mediawiki.org/wiki/API:Etiquette).
A full build is ~400 requests, so it takes several minutes by design. Don't
lower `-delay` much; the API starts returning 429 when hit in bursts.

## Licence of the scraped data

Wikipedia text is licensed
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/). The chapter
titles themselves are factual, but if you redistribute the dataset, attribute
Wikipedia — `source_url` in every file points at the article it came from.

## Development

```sh
make test    # go test ./...
make fmt     # gofmt -w .
make build   # ./bin/wikichapters
```

The scraper has no third-party dependencies.
