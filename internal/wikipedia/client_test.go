package wikipedia

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// categoryServer serves action=query&list=categorymembers from a fixture map
// keyed by "<category>|<cmnamespace>". Anything not in the map comes back
// empty, which is what the real API does for a category with no subcategories.
func categoryServer(t *testing.T, fixture map[string][]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		key := q.Get("cmtitle") + "|" + q.Get("cmnamespace")
		var members []map[string]string
		for _, title := range fixture[key] {
			members = append(members, map[string]string{"title": title})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"query": map[string]any{"categorymembers": members},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	c.APIBase = srv.URL
	c.Delay = 0
	return c
}

func TestCategoryMembers_includesSubcategoryArticles(t *testing.T) {
	// The live category splits long series like One Piece into a per-series
	// subcategory. Listing only direct members loses them entirely.
	c := categoryServer(t, map[string][]string{
		"Category:Lists|0":  {"List of Zetman chapters"},
		"Category:Lists|14": {"Category:One Piece chapter lists"},
		"Category:One Piece chapter lists|0": {
			"List of One Piece chapters (1–186)",
			"List of One Piece chapters (187–388)",
		},
	})

	got, err := c.CategoryMembers("Category:Lists")
	if err != nil {
		t.Fatalf("CategoryMembers: %v", err)
	}
	want := []string{
		"List of Zetman chapters",
		"List of One Piece chapters (1–186)",
		"List of One Piece chapters (187–388)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestCategoryMembers_deduplicatesAcrossSubcategories(t *testing.T) {
	// An article listed both directly and inside a subcategory must be
	// scraped once, or build would write the same series twice.
	c := categoryServer(t, map[string][]string{
		"Category:Lists|0":                {"List of Naruto chapters"},
		"Category:Lists|14":               {"Category:Naruto chapter lists"},
		"Category:Naruto chapter lists|0": {"List of Naruto chapters", "List of Naruto chapters (Part II)"},
	})

	got, err := c.CategoryMembers("Category:Lists")
	if err != nil {
		t.Fatalf("CategoryMembers: %v", err)
	}
	want := []string{"List of Naruto chapters", "List of Naruto chapters (Part II)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestCategoryMembers_doesNotRecurseBeyondOneLevel(t *testing.T) {
	// Wikipedia's category graph contains cycles. One level is all the layout
	// needs, and it makes unbounded descent impossible by construction.
	c := categoryServer(t, map[string][]string{
		"Category:Lists|0":  {"Direct article"},
		"Category:Lists|14": {"Category:Level1"},
		"Category:Level1|0": {"Level 1 article"},
		// Would be reached only by a second level of descent.
		"Category:Level1|14": {"Category:Level2"},
		"Category:Level2|0":  {"Level 2 article"},
	})

	got, err := c.CategoryMembers("Category:Lists")
	if err != nil {
		t.Fatalf("CategoryMembers: %v", err)
	}
	want := []string{"Direct article", "Level 1 article"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestCategoryMembers_followsContinuation(t *testing.T) {
	// cmcontinue paging has to keep working for the direct-member listing,
	// which is the path that returns ~400 articles in production.
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		resp := map[string]any{"query": map[string]any{"categorymembers": []map[string]string{}}}
		if q.Get("cmnamespace") == "0" {
			page++
			if page == 1 {
				resp = map[string]any{
					"query":    map[string]any{"categorymembers": []map[string]string{{"title": "First"}}},
					"continue": map[string]string{"cmcontinue": "next"},
				}
			} else if q.Get("cmcontinue") == "next" {
				resp = map[string]any{
					"query": map[string]any{"categorymembers": []map[string]string{{"title": "Second"}}},
				}
			}
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	c.APIBase = srv.URL
	c.Delay = 0

	got, err := c.CategoryMembers("Category:Lists")
	if err != nil {
		t.Fatalf("CategoryMembers: %v", err)
	}
	if want := []string{"First", "Second"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCategoryMembers_subcategoryFailureIsReported(t *testing.T) {
	// Enumeration runs once, before ~400 article fetches. Silently dropping a
	// subcategory here would quietly erase a whole series from the dataset, so
	// this has to fail loudly instead.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("cmtitle") == "Category:Broken" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var members []map[string]string
		if q.Get("cmnamespace") == "14" {
			members = []map[string]string{{"title": "Category:Broken"}}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"query": map[string]any{"categorymembers": members},
		})
	}))
	t.Cleanup(srv.Close)

	c := NewClient()
	c.APIBase = srv.URL
	c.Delay = 0

	if _, err := c.CategoryMembers("Category:Lists"); err == nil {
		t.Fatal("expected an error when a subcategory listing fails, got nil")
	}
}

func TestClientEndpoint_defaultsToHost(t *testing.T) {
	c := NewClient()
	if got, want := c.endpoint(), "https://en.wikipedia.org/w/api.php"; got != want {
		t.Errorf("endpoint() = %q, want %q", got, want)
	}
}
