package comick

import (
	"testing"
	"time"

	"github.com/kulaid/manga-chapter-titles/internal/sources"
)

func entry(chap, title, createdAt string) chapterEntry {
	return chapterEntry{Chap: chap, Title: title, CreatedAt: createdAt}
}

func TestMergePageKeepsNewestTitle(t *testing.T) {
	// Comick holds one entry per scanlation group. The newest upload wins,
	// regardless of the order the API happens to return them in.
	titles := sources.Titles{}
	newest := map[float64]time.Time{}

	mergePage(titles, newest, []chapterEntry{
		entry("1", "Older Translation", "2020-01-01T00:00:00+00:00"),
		entry("1", "Newer Translation", "2024-06-01T00:00:00+00:00"),
		entry("2", "Only One", "2021-01-01T00:00:00+00:00"),
	})

	if titles[1] != "Newer Translation" {
		t.Errorf("chapter 1 = %q, want the newest upload", titles[1])
	}
	if titles[2] != "Only One" {
		t.Errorf("chapter 2 = %q", titles[2])
	}
}

func TestMergePageNewestWinsWhenNewerArrivesFirst(t *testing.T) {
	titles := sources.Titles{}
	newest := map[float64]time.Time{}

	mergePage(titles, newest, []chapterEntry{
		entry("1", "Newer Translation", "2024-06-01T00:00:00+00:00"),
		entry("1", "Older Translation", "2020-01-01T00:00:00+00:00"),
	})

	if titles[1] != "Newer Translation" {
		t.Errorf("chapter 1 = %q, want the newest upload to survive a later older entry", titles[1])
	}
}

func TestMergePageAcrossPages(t *testing.T) {
	// A chapter's alternatives can straddle a page boundary, so the timestamp
	// of the chosen entry has to carry over between calls.
	titles := sources.Titles{}
	newest := map[float64]time.Time{}

	mergePage(titles, newest, []chapterEntry{entry("5", "From Page One", "2024-01-01T00:00:00+00:00")})
	mergePage(titles, newest, []chapterEntry{entry("5", "Older, From Page Two", "2019-01-01T00:00:00+00:00")})

	if titles[5] != "From Page One" {
		t.Errorf("chapter 5 = %q, want the newer entry from the earlier page to hold", titles[5])
	}

	mergePage(titles, newest, []chapterEntry{entry("5", "Newest, From Page Three", "2025-01-01T00:00:00+00:00")})
	if titles[5] != "Newest, From Page Three" {
		t.Errorf("chapter 5 = %q, want a genuinely newer entry to replace it", titles[5])
	}
}

func TestMergePageSkipsUntitledAndUnparseable(t *testing.T) {
	titles := sources.Titles{}
	newest := map[float64]time.Time{}

	mergePage(titles, newest, []chapterEntry{
		entry("1", "", "2024-01-01T00:00:00+00:00"),                     // untitled
		{Chap: "1", Title: nil, CreatedAt: "2025-01-01T00:00:00+00:00"}, // null title
		entry("", "No Chapter Number", "2024-01-01T00:00:00+00:00"),
		entry("abc", "Unparseable", "2024-01-01T00:00:00+00:00"),
		entry("2", "Real", "2024-01-01T00:00:00+00:00"),
	})

	if _, present := titles[1]; present {
		t.Error("an untitled entry was recorded")
	}
	if len(titles) != 1 || titles[2] != "Real" {
		t.Errorf("titles = %v, want only chapter 2", titles)
	}
}

func TestMergePageHandlesDecimalChapters(t *testing.T) {
	titles := sources.Titles{}
	newest := map[float64]time.Time{}
	mergePage(titles, newest, []chapterEntry{entry("4.5", "Interlude", "2024-01-01T00:00:00+00:00")})

	if titles[4.5] != "Interlude" {
		t.Errorf("chapter 4.5 = %q", titles[4.5])
	}
}

func TestChapterTime(t *testing.T) {
	created := "2024-06-01T00:00:00+00:00"
	published := "2020-01-01T00:00:00+00:00"

	if got := chapterTime(created, published); got.Year() != 2024 {
		t.Errorf("chapterTime preferred %v, want the created_at value", got)
	}
	if got := chapterTime("", published); got.Year() != 2020 {
		t.Errorf("chapterTime fell back to %v, want publish_at", got)
	}
	// An undated entry must sort oldest so it never displaces a dated one.
	if got := chapterTime("", ""); !got.IsZero() {
		t.Errorf("chapterTime(\"\", \"\") = %v, want the zero time", got)
	}
	if got := chapterTime("not-a-date", ""); !got.IsZero() {
		t.Errorf("chapterTime with garbage = %v, want the zero time", got)
	}
}

func TestAniListLinkAcceptsBothTypes(t *testing.T) {
	// Comick types this value inconsistently across entries.
	if got := aniListLink(map[string]any{"al": "105778"}); got != "105778" {
		t.Errorf("string form = %q", got)
	}
	if got := aniListLink(map[string]any{"al": float64(105778)}); got != "105778" {
		t.Errorf("number form = %q", got)
	}
	if got := aniListLink(map[string]any{}); got != "" {
		t.Errorf("missing link = %q, want empty", got)
	}
	if got := aniListLink(map[string]any{"al": []any{1}}); got != "" {
		t.Errorf("unexpected type = %q, want empty", got)
	}
}
