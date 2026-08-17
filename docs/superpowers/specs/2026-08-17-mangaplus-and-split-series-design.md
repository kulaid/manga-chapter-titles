# Ranked sources, split-series merging, and incremental updates

Date: 2026-08-17

## Problem

Three defects and one feature, discovered together while adding MangaPlus.

1. **Split series are invisible.** `wikipedia.CategoryMembers` listed only direct
   page members of `Category:Lists of manga volumes and chapters`. Wikipedia
   files long-running series whose chapter list spans several articles under a
   *per-series subcategory* instead. Twelve of the longest manga in existence
   were therefore absent from the dataset: One Piece, Naruto, Bleach, Case
   Closed, Dragon Ball, Inuyasha, Hajime no Ippo, Captain Tsubasa, Kaiji, Major,
   Pokémon Adventures, Saint Seiya. (Fixed in this branch; recursing one level
   takes the enumeration from 407 to 475 articles.)

2. **Split series would fragment.** With those articles now reachable, the six
   One Piece articles become six series — `one-piece`, `one-piece-2`, … — because
   `SeriesNameFromArticle` leaves the trailing range in the name and `usedSlugs`
   then disambiguates the collision. They must collapse into one series.

3. **Nothing can outrank a stored title.** `sources.Merge` treats whatever is
   already in the file as final and lets later sources fill gaps only. There is
   no way to express "this source is better than that one".

4. **MangaPlus is not consulted.** It carries the official licensed English
   titles for Shueisha series.

## Non-goals

- Reading MangaPlus chapter *content*. Only titles.
- Any name-based join. Every source is matched by AniList ID, as today.
- A full dataset rebuild as the delivery mechanism for new series. Adding twelve
  series must not cost a 50-minute re-scrape of the other 341.

## Design

### Source ranking

`sources` gains an explicit rank per source name:

| Rank | Source | Why |
| --- | --- | --- |
| 1 | wikipedia | Licensed English titles, bare (`Dog & Chainsaw`), most consistent. |
| 2 | mangaplus | Official Shueisha titles, but only ~5–8 chapters per series and sometimes ALL CAPS (`RYOMEN SUKUNA`). |
| 3 | comick | Broad coverage, scanlator titles. |
| 4 | mangadex | Fallback. |
| 99 | unknown | Anything unrecognised sorts last. |

`Merge` becomes provenance-aware. It receives stored titles together with the
per-chapter source that produced them (already persisted as `chapter_sources`)
and, for each chapter, keeps the title whose source has the lowest rank number.
A fetched result replaces a stored title only on a **strictly better** rank;
equal ranks keep the stored value so repeated runs do not churn the file.

This yields the required retention property without a special case:

- A stored MangaPlus title survives once MangaPlus stops serving that chapter,
  because no result arrives to displace it. **This is the point of the feature**
  — MangaPlus's free window slides forward, so a title captured today is gone
  from the API within weeks.
- Wikipedia (rank 1) overwrites a stored MangaPlus title.
- Comick and MangaDex can never overwrite one.

### `build` stops overwriting

`build` currently replaces `data/<slug>.json` wholesale, which discards every
aggregator title and is why `build` must never be run without `enrich` behind
it. It will instead load the existing file and merge Wikipedia in at rank 1.
Aggregator titles survive for chapters Wikipedia does not cover, and the
retention property above holds across a rebuild.

### Split-series merging

Two changes:

- `SeriesNameFromArticle` strips a **trailing parenthetical** before trimming
  descriptor words, but only when its contents look like a range or part
  descriptor — digits, `part`, `volume(s)`, `current`, `series`. So
  `List of One Piece chapters (1–186)` → `One Piece`, while a series whose real
  name contains parentheses mid-string is untouched.
- `build` accumulates by slug instead of disambiguating. When a second article
  maps to a slug already written this run, its chapters merge into the existing
  series rather than claiming `-2`. Wikipedia titles from different articles of
  the same series do not overlap in chapter number, so first-wins is sufficient;
  the article list is recorded so provenance stays inspectable.

`usedSlugs` disambiguation is retained for genuinely distinct series that
happen to slugify identically.

### `internal/mangaplus`

Implements `sources.Source`.

- Endpoint `GET https://jumpg-webapi.tokyo-cdn.com/api/title_detailV3?title_id=<id>&clang=eng`.
- Headers: Chrome `User-Agent`, `Origin`/`Referer` of the site, and a
  `Session-Token` holding a freshly generated UUID. That header is what the API
  requires; without it every request returns HTTP 200 carrying an "Account
  Banned" payload, which is a missing-client-header error and not a real ban.
- Response is protobuf. Decoded with
  `google.golang.org/protobuf/encoding/protowire` — official module, no
  `protoc-gen-go` codegen step, and only two nested string fields are needed.
- Shape: `title_detail_view` → `chapter_list_group` (tag 28) → chapter lists at
  tags **2, 3 and 4** → chapter with `name` (tag 3, e.g. `#001`) and `sub_title`
  (tag 4, e.g. `Chapter 1: Dog & Chainsaw`). Aidoku's own model reads only tags
  2 and 4 and so misses the recent chapters, which live in tag 3.
- Entries with no `sub_title` carry no title and are skipped.
- Titles are stripped of a leading `(Chapter|Episode|Ch.) <number>:` prefix,
  matched only when the pre-colon segment has that shape, so
  `Chapter 3: Enter Zolo: Pirate Hunter` → `Enter Zolo: Pirate Hunter`. The
  number may be spelled out (`Episode One:`, used by Kaiju No. 8).
- Rate limit 500ms between requests.

**Known ceiling:** the endpoint returns only the first ~4 and last ~4 chapters —
the free window. One Piece yields 8 of 1190. This is MangaPlus's business model,
not an auth gap: the response contains two `chapter_list_group`s covering those
buckets and the rest are absent, and the web client calls no other endpoint. The
feature is therefore a correction source for a handful of chapters per series,
most valuably the newest chapters of ongoing series where Wikipedia lags.

### MangaPlus title ID

New `Series.MangaPlusID` (`mangaplus_id`), mirrored into the index. Resolved
from MangaDex `links.engtl` — an entry already confirmed by its `al` field, so
no name matching is introduced — and reused thereafter, the same pattern as the
AniList ID reuse in `ae2db32`.

### Incremental updates

A full `build` costs ~17 minutes and a full `enrich` ~30. Adding twelve series
must not require either. New command:

```
wikichapters add            # scrape + enrich only series absent from the dataset
wikichapters add -series X  # target one series by name
```

It enumerates the category, filters to articles whose series is not already in
the index, scrapes those, then enriches exactly those slugs. Adding the twelve
missing series costs roughly a minute.

## Testing

- `sources`: rank table; merge keeps the better rank; equal rank keeps stored;
  a stored MangaPlus title survives a run where MangaPlus returns nothing; a
  Wikipedia title displaces a stored MangaPlus one.
- `wikipedia`: subcategory recursion, dedup, one-level bound, continuation,
  loud failure on a broken subcategory (done); trailing-parenthetical stripping
  in `SeriesNameFromArticle`.
- `mangaplus`: protowire decode against a captured real response fixture;
  prefix stripping including the spelled-out `Episode One:` form; chapters
  without `sub_title` skipped.
- `build`: an existing file's aggregator titles survive a re-scrape; a second
  article for the same series merges instead of claiming a `-2` slug.
- `add`: series already present are skipped.

## Risks

- Recursion adds ~68 articles, many of them volume lists that yield no chapter
  titles and are skipped. Expected, not an error.
- MangaPlus's free window slides, so which chapters it can supply changes over
  time. That is precisely why retention is rank-based rather than
  last-writer-wins.
- The `engtl` link is absent for non-Shueisha series; those simply get no
  MangaPlus titles.
