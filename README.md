# manga-chapter-titles

Builds a **manga chapter title dataset** as plain JSON files, so an application
can look up "chapter 47 of Berserk" without hitting the network.

This is a **database** of chapter titles. Scraping is how most of the data gets
in; it is not what makes a title correct. Where every source is wrong, the right
answer is to write the title down by hand — see
[curated titles](#curated-titles).

Each source has a **rank**, and for every chapter the best-ranked title wins:

| Rank | Source | What it gives | Trade-off |
| --- | --- | --- | --- |
| 0 | **Curated** | Titles a person decided, in `overrides.json` | Only what someone has written |
| 1 | **Wikipedia** | The *licensed* English titles (Viz, Kodansha, Yen Press) | Only licensed/notable series; lags recent chapters |
| 2 | **MangaPlus** | Official Shueisha titles, and current | Only ~5–8 chapters per series; sometimes ALL CAPS |
| 3 | **Comick** | Broad coverage, names chapters MangaDex leaves untitled | Scanlator titles |
| 4 | **MangaDex** | Fills what the others miss | Scanlator titles, many entries untitled |

A freshly fetched title replaces a stored one only on a **strictly better**
rank. Equal ranks keep what is already on file, so re-running a source over its
own titles never churns the dataset — and a title whose source has stopped
serving that chapter is **retained**, because nothing arrives to displace it.

That retention is the point of MangaPlus. Its free window slides forward, so a
title captured today is gone from the API within weeks; once stored, only
Wikipedia or a curator can overwrite it.

Where several scanlation groups have named the same chapter, the **newest
upload wins** — later groups are usually re-translations or fixes.

Every source is joined on an **ID, never on a name**. Most join on the AniList
ID directly; MangaPlus has none of its own, so it joins through the
`links.engtl` of a MangaDex entry already confirmed by its AniList ID. A series
a source does not carry contributes nothing rather than something plausible
from a different manga.

The committed `data/` directory currently holds **368 series and 73,664 chapter
titles** (5.3 MB), built from the 475 articles in Wikipedia's chapter-list
category and its per-series subcategories. Of the rest, 34 are further parts of
a series already counted — the six One Piece articles are one series — and 74
are volume lists carrying no chapter titles.

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
  "anilist_id": 105778,
  "article": "List of Chainsaw Man chapters",
  "source_url": "https://en.wikipedia.org/wiki/List_of_Chainsaw_Man_chapters",
  "scraped_at": "2026-08-17T02:03:55Z",
  "chapter_count": 232,
  "inferred_numbers": 0,
  "sources": [
    {"name": "wikipedia", "ref": "List of Chainsaw Man chapters", "count": 232},
    {"name": "comick", "ref": "02-chainsaw-man", "count": 0}
  ],
  "chapters": {
    "1": "Dog & Chainsaw",
    "2": "The Place Where Pochita Is",
    "4.5": "An Interlude"
  },
  "chapter_sources": {
    "1": "wikipedia",
    "2": "wikipedia",
    "4.5": "comick"
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
      "anilist_id": 30002,
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

### `sources` and `chapter_sources`

`sources` lists every source consulted, in merge order, with the identifier it
matched and how many titles it contributed that no higher-priority source
already had. `chapter_sources` records the origin of each individual title.

Neither is needed to use the data — `chapters` is self-contained — but they let
a curator see *why* a title reads the way it does, and spot a series that is
running entirely on scanlator titles.

### `mangaplus_id`

The series' MangaPlus title id, present when it has one — most series do not,
since MangaPlus carries Shueisha titles only. It is recorded so the MangaDex
round trip that derived it happens once per series rather than once per run.

### `anilist_id`

The series' [AniList](https://anilist.co) manga ID, present in both the index
and the series file, and omitted when it could not be confirmed. Prefer it over
`match_key` whenever you already hold an AniList ID — it is exact, whereas a
name match can miss when two sources title a series differently ("Attack on
Titan" vs "Shingeki no Kyojin").

IDs come from AniList's search API, but its search is a relevance ranking that
answers *every* query with something, so a hit is only accepted when one of the
entry's own titles — romaji, English, native, or a synonym — matches the series
name once normalised. Anything unconfirmed is left absent rather than guessed.

Some series can never resolve automatically and that is fine — see
[Fixing what automatic resolution misses](#fixing-what-automatic-resolution-misses).

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

// Exact lookup when you already have an AniList ID — preferred over the name.
if title, ok := db.TitleByAniListID(30002, 47); ok {
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
| `enrich` | Fills and corrects chapter titles from MangaPlus, Comick and MangaDex, in place |
| `fetch <series\|article\|URL>` | Scrapes one series; accepts a name, an article title, or a full URL |
| `list` | Prints the articles `build` would scrape, without scraping them |
| `lookup <series>` | Reads a series back out of an existing dataset |
| `anilist` | Backfills missing AniList IDs into an existing dataset, without re-scraping Wikipedia |
| `missing` | Lists series whose AniList ID could not be resolved automatically |
| `override` | Records a hand-verified AniList ID that automatic lookup cannot find |

Useful flags:

- `-out <dir>` — where to write (default `data`)
- `-delay <duration>` — pause between API requests (default `1s`)
- `-limit <n>` — `build` only; stop after N series, for a smoke test
- `-stdout` — `fetch` only; print the JSON instead of writing a file
- `-chapter <n>` — `lookup` only; print just that chapter's title
- `-anilist=false` — `build` only; skip AniList lookups (saves ~1.5s per series)
- `-only <series>` — `enrich` only; restrict to one series, for spot checks
- `-comick=false` / `-mangadex=false` / `-mangaplus=false` — `enrich` only; disable a source
- `-force` — `anilist` only; re-resolve series that already have an ID

Flags may appear before or after the series name.

### Series Wikipedia files elsewhere

`build` and `add` work from `Category:Lists of manga volumes and chapters`, and
not every series is in it: one whose chapters are listed on its own article
rather than a separate `List of X chapters` page — Gachiakuta, say — appears
nowhere in that category and can only be added by name:

```sh
./bin/wikichapters fetch "Gachiakuta"      # writes data/gachiakuta.json and its index row
./bin/wikichapters enrich -only "Gachiakuta"
```

`fetch` registers the series in `index.json`, which is the only thing consumers
read to resolve a name, and a rebuild keeps the index row of every series it did
not scrape — so a hand-fetched series is not dropped by the next `build`.

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

An ongoing series also keeps its newest chapters in a bulleted section headed
*Chapters not yet in tankōbon format* (a few articles word it *…in volume
format*), because no volume collects them yet. Those bullets sit in no template
parameter, so they are read as a chapter list of their own, ending at the next
heading — the section that follows is usually an episode list, whose bullets are
not chapters. Around 120 articles carry one, and without this the most recent
licensed titles of every ongoing series were missing.

On top of that the parser unwraps `{{Nihongo}}` and similar templates, resolves
wikilinks, strips refs/comments/HTML, skips non-chapter bullets like
`Bonus Material.`, and splits stray bullets that articles glue onto the previous
entry. Series that restart numbering per arc (Berserk) produce duplicate chapter
numbers; the first title in document order wins.

Every one of those cases is pinned by a test in
`internal/wikipedia/parse_test.go`.

## Fixing what automatic resolution misses

This is a curation tool, not an automatic one. The scraper's job is to get the
easy 90% right and to *refuse* the rest rather than guess; the remainder are
corrected by hand, once, and kept.

Wikipedia abbreviates some titles in ways AniList does not share — it says
"KonoSuba" where AniList has "Kono Subarashii Sekai ni Shukufuku wo!", and
AniList answers that query with spin-offs (`Megumin Anthology`, `Kappore`).
Accepting a fuzzy match there would attach a *different work's* ID, which is
worse than no ID at all. So those are left empty and fixed manually:

```sh
./bin/wikichapters missing                      # what still needs an ID
./bin/wikichapters override "KonoSuba" -anilist 85702     -note "Wikipedia abbreviates; AniList search returns spin-offs"
```

`override` writes to **`overrides.json`** at the repo root and applies the
correction to `data/` immediately, so you don't need to rebuild to use it.

It also **verifies the ID against AniList before recording it** and stores the
title that ID resolves to. A typo or a half-remembered number is otherwise
indistinguishable from a correct one — 86994 is a perfectly valid AniList ID,
it just happens to be *No Guns Life* — and an override bypasses the title check
that automatic resolution applies. Pass `-force` to record an ID anyway.

The file lives outside `data/` deliberately: it is the record of every decision
a person made, kept separate from everything a machine derived. Overrides are
re-applied after automatic resolution on every build, which makes them
permanent. They also short-circuit the AniList lookup entirely — a
hand-verified ID is never second-guessed.

`overrides.json` is committed, hand-editable, and keyed by Wikipedia article
title, the one identifier that doesn't change when a series name, slug or match
key is adjusted:

```json
{
  "overrides": {
    "List of KonoSuba chapters": {
      "series": "KonoSuba",
      "anilist_id": 85702,
      "anilist_title": "Kono Subarashii Sekai ni Shukufuku wo!",
      "note": "Wikipedia abbreviates; AniList search returns spin-offs"
    }
  }
}
```

### Curated titles

An override may also carry a `chapters` map, which is how a wrong or missing
*title* is fixed rather than a wrong ID. Curated titles rank above every
scraper, so nothing automatic can overwrite one, and they are re-applied on
every build.

```json
{
  "overrides": {
    "List of Vinland Saga chapters": {
      "series": "Vinland Saga",
      "anilist_id": 30642,
      "chapters": {
        "175.5": "Assassin's Creed: Valhalla x Vinland Saga",
        "191.5": "Omake"
      }
    }
  }
}
```

Use this whenever the sources are wrong, and not only as a last resort. Two
cases come up often:

- **Wikipedia has a hole.** Fairy Tail's article names 295 of 545 chapters.
  The rest are not going to appear on their own.
- **Wikipedia has regressed.** An article can lose chapters it once listed —
  Berserk's stops at 382 although 383–386 have been published and were captured
  earlier. A rebuild replaces Wikipedia's share of a series wholesale, so a
  title only Wikipedia carried disappears with it unless it is curated.

An empty value **deletes** a chapter, which is how to say that a title on file
is wrong and no source has a right one.

Keys must be canonical, the way `FormatChapterNumber` writes them: chapter 0.10
is stored `"0.1"`. Any other spelling of the same number is removed rather than
left to sit beside it, since consumers parse both back to the same value.

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
