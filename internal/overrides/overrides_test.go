package overrides

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	// Nothing corrected yet is the normal starting state, not a failure.
	f, err := Load(filepath.Join(t.TempDir(), "overrides.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f.Len() != 0 {
		t.Errorf("Len = %d, want 0", f.Len())
	}
	if _, ok := f.Get("List of KonoSuba chapters"); ok {
		t.Error("Get reported a correction in an empty file")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")

	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f.Set("List of KonoSuba chapters", Entry{
		Series:    "KonoSuba",
		AniListID: 86994,
		Note:      "AniList search returns spin-offs",
	})
	if err := f.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	e, ok := reloaded.Get("List of KonoSuba chapters")
	if !ok {
		t.Fatal("correction did not survive the round trip")
	}
	if e.AniListID != 86994 {
		t.Errorf("AniListID = %d, want 86994", e.AniListID)
	}
	if e.Note == "" {
		t.Error("note was lost; it records why the correction exists")
	}
}

func TestSetReplacesPreviousCorrection(t *testing.T) {
	f := &File{}
	f.Set("List of X chapters", Entry{AniListID: 1})
	f.Set("List of X chapters", Entry{AniListID: 2})

	if f.Len() != 1 {
		t.Errorf("Len = %d, want 1", f.Len())
	}
	if e, _ := f.Get("List of X chapters"); e.AniListID != 2 {
		t.Errorf("AniListID = %d, want 2 (the later correction)", e.AniListID)
	}
}

func TestGetTrimsWhitespace(t *testing.T) {
	f := &File{}
	f.Set("  List of X chapters  ", Entry{AniListID: 7})
	if e, ok := f.Get("List of X chapters"); !ok || e.AniListID != 7 {
		t.Errorf("Get() = %+v, %v; want the entry set with surrounding spaces", e, ok)
	}
}

func TestArticlesAreSorted(t *testing.T) {
	f := &File{}
	f.Set("List of Zeta chapters", Entry{AniListID: 1})
	f.Set("List of Alpha chapters", Entry{AniListID: 2})

	got := f.Articles()
	if len(got) != 2 || got[0] != "List of Alpha chapters" {
		t.Errorf("Articles() = %v, want sorted order", got)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overrides.json")

	f := &File{}
	f.Set("List of X chapters", Entry{AniListID: 1})
	if err := f.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	leftovers, _ := filepath.Glob(filepath.Join(dir, ".overrides-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("overrides file missing after save: %v", err)
	}
}

func TestNilFileIsSafe(t *testing.T) {
	var f *File
	if f.Len() != 0 {
		t.Error("nil file reported corrections")
	}
	if _, ok := f.Get("anything"); ok {
		t.Error("nil file reported a correction")
	}
	if f.Articles() != nil {
		t.Error("nil file returned articles")
	}
}
