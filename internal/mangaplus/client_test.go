package mangaplus

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTitleIDFromEngTL(t *testing.T) {
	tests := []struct {
		link string
		want int
	}{
		{"https://mangaplus.shueisha.co.jp/titles/100037", 100037},
		{"https://mangaplus.shueisha.co.jp/titles/100020/", 100020},
		// Most series' official English edition is not MangaPlus, and the
		// trailing number of a Viz URL is not a MangaPlus title id.
		{"https://www.viz.com/tatsuki-fujimoto-before-chainsaw-man", 0},
		{"https://mangaplus.shueisha.co.jp/titles/notanumber", 0},
		{"https://mangaplus.shueisha.co.jp/", 0},
		{"", 0},
		{"not a url at all %%%", 0},
	}
	for _, tt := range tests {
		if got := TitleIDFromEngTL(tt.link); got != tt.want {
			t.Errorf("TitleIDFromEngTL(%q) = %d, want %d", tt.link, got, tt.want)
		}
	}
}

// stubResolver stands in for the MangaDex round trip.
type stubResolver struct {
	id    int
	calls int
}

func (s *stubResolver) MangaPlusID(int, string) (int, error) {
	s.calls++
	return s.id, nil
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New("")
	c.BaseURL = srv.URL
	c.Delay = 0
	return c, srv
}

func TestFetch_sendsTheHeadersTheAPIRequires(t *testing.T) {
	// Without a Session-Token the API answers 200 with an "Account Banned"
	// payload, so its absence is a silent, total failure.
	var gotToken, gotUA, gotOrigin string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("Session-Token")
		gotUA = r.Header.Get("User-Agent")
		gotOrigin = r.Header.Get("Origin")
		b, _ := os.ReadFile(filepath.Join("testdata", "chainsaw_man_title_detail.bin"))
		w.Write(b)
	})
	c.Resolver = &stubResolver{id: 100037}

	if _, err := c.Fetch(105778, "Chainsaw Man"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(gotToken) != 36 {
		t.Errorf("Session-Token = %q, want a UUID", gotToken)
	}
	if gotUA == "" || gotOrigin != site {
		t.Errorf("User-Agent = %q, Origin = %q", gotUA, gotOrigin)
	}
}

func TestFetch_returnsTitlesAndTheTitleIDAsRef(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("title_id"); got != "100037" {
			t.Errorf("title_id = %q, want 100037", got)
		}
		if got := r.URL.Query().Get("clang"); got != "eng" {
			t.Errorf("clang = %q, want eng", got)
		}
		b, _ := os.ReadFile(filepath.Join("testdata", "chainsaw_man_title_detail.bin"))
		w.Write(b)
	})
	c.Resolver = &stubResolver{id: 100037}

	got, err := c.Fetch(105778, "Chainsaw Man")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !got.Found {
		t.Fatal("Found = false, want true")
	}
	// Ref carries the id so the caller can persist it and skip the lookup.
	if got.Ref != "100037" {
		t.Errorf("Ref = %q, want %q", got.Ref, "100037")
	}
	if got.Titles[1] != "Dog & Chainsaw" {
		t.Errorf("chapter 1 = %q", got.Titles[1])
	}
}

func TestFetch_seriesWithNoMangaPlusPageIsNotFound(t *testing.T) {
	// Most of the dataset is not Shueisha. That is not an error.
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("the API must not be called for a series with no title id")
	})
	c.Resolver = &stubResolver{id: 0}

	got, err := c.Fetch(1, "Some Non-Shueisha Series")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Found {
		t.Error("Found = true, want false")
	}
}

func TestFetch_reusesAKnownTitleIDWithoutResolving(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := os.ReadFile(filepath.Join("testdata", "chainsaw_man_title_detail.bin"))
		w.Write(b)
	})
	res := &stubResolver{id: 100037}
	c.Resolver = res
	c.TitleIDs = map[int]int{105778: 100037}

	if _, err := c.Fetch(105778, "Chainsaw Man"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.calls != 0 {
		t.Errorf("resolver called %d times, want 0 for a known id", res.calls)
	}
}

func TestFetch_resolvesOnlyOncePerSeries(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := os.ReadFile(filepath.Join("testdata", "chainsaw_man_title_detail.bin"))
		w.Write(b)
	})
	res := &stubResolver{id: 0}
	c.Resolver = res

	for i := 0; i < 3; i++ {
		if _, err := c.Fetch(999, "Repeatedly Asked"); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
	}
	if res.calls != 1 {
		t.Errorf("resolver called %d times, want 1 (a miss is cached too)", res.calls)
	}
}

func TestFetch_httpErrorIsReported(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	})
	c.Resolver = &stubResolver{id: 100037}

	if _, err := c.Fetch(105778, "Chainsaw Man"); err == nil {
		t.Error("Fetch() = nil error, want an error on 503")
	}
}

func TestFetch_rateLimits(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := os.ReadFile(filepath.Join("testdata", "chainsaw_man_title_detail.bin"))
		w.Write(b)
	})
	c.Resolver = &stubResolver{id: 100037}
	c.Delay = 60 * time.Millisecond

	start := time.Now()
	for i := 0; i < 3; i++ {
		if _, err := c.Fetch(105778, "Chainsaw Man"); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed < 120*time.Millisecond {
		t.Errorf("three requests took %s, want the delay to be honoured", elapsed)
	}
}
