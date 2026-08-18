package mangaplus

import (
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return b
}

func TestDecodeTitleDetail_realChainsawManResponse(t *testing.T) {
	// Captured from the live endpoint. MangaPlus serves only the first four and
	// last few chapters of a series, so five of Chainsaw Man's 232 is the whole
	// response, not a truncated one.
	got, err := decodeTitleDetail(fixture(t, "chainsaw_man_title_detail.bin"))
	if err != nil {
		t.Fatalf("decodeTitleDetail: %v", err)
	}

	want := map[float64]string{
		1:   "Dog & Chainsaw",
		2:   "The Place Where Pochita Is",
		3:   "Arrival in Tokyo",
		4:   "Power",
		232: "Thank You, Chainsaw Man",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d chapters, want %d: %v", len(got), len(want), got)
	}
	for num, title := range want {
		if got[num] != title {
			t.Errorf("chapter %v = %q, want %q", num, got[num], title)
		}
	}
}

func TestDecodeTitleDetail_readsChaptersFromEveryGroupTag(t *testing.T) {
	// Chapter lists hang off tags 2, 3 and 4 of a chapter_list_group, and the
	// recent chapters — the ones Wikipedia is most likely to lag on, and so the
	// most valuable — are not all under tag 2.
	got, err := decodeTitleDetail(fixture(t, "one_piece_title_detail.bin"))
	if err != nil {
		t.Fatalf("decodeTitleDetail: %v", err)
	}

	if got[1] != "Romance Dawn" {
		t.Errorf("chapter 1 = %q, want %q", got[1], "Romance Dawn")
	}
	if got[1190] != "Those Whose Death Brings Celebration" {
		t.Errorf("chapter 1190 = %q; the newest chapters must not be missed", got[1190])
	}
	if len(got) != 8 {
		t.Errorf("got %d chapters, want 8", len(got))
	}
}

func TestDecodeTitleDetail_stripsOnlyTheLeadingChapterPrefix(t *testing.T) {
	// "Chapter 3: Enter Zolo: Pirate Hunter" has a colon in the real title, so
	// only the first segment may be removed.
	got, err := decodeTitleDetail(fixture(t, "one_piece_title_detail.bin"))
	if err != nil {
		t.Fatalf("decodeTitleDetail: %v", err)
	}
	if got[3] != "Enter Zolo: Pirate Hunter" {
		t.Errorf("chapter 3 = %q, want %q", got[3], "Enter Zolo: Pirate Hunter")
	}
}

func TestDecodeTitleDetail_garbageIsAnErrorNotAPanic(t *testing.T) {
	if _, err := decodeTitleDetail([]byte{0xff, 0xff, 0xff, 0xff}); err == nil {
		t.Error("decodeTitleDetail(garbage) = nil error, want an error")
	}
}

func TestDecodeTitleDetail_empty(t *testing.T) {
	got, err := decodeTitleDetail(nil)
	if err != nil {
		t.Fatalf("decodeTitleDetail: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d chapters from an empty body, want 0", len(got))
	}
}
