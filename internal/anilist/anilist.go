// Package anilist resolves a manga series name to its AniList ID.
//
// The Wikipedia articles this tool scrapes carry no cross-site identifiers, so
// consumers would otherwise have to match series by name. An AniList ID gives
// them an exact key instead, and it is the identifier the common manga
// APIs (MangaDex, Comick) already expose.
//
// AniList's search is fuzzy, so a hit is only accepted when one of the entry's
// own titles matches the series name — see Match.
package anilist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kulaid/manga-chapter-titles/chaptertitles"
)

// DefaultUserAgent identifies this tool to AniList. Requests without a
// User-Agent are answered with 403.
const DefaultUserAgent = "manga-chapter-titles/1.0 (https://github.com/kulaid/manga-chapter-titles)"

// DefaultDelay is the pause between API calls. AniList allows 90 requests per
// minute and drops to 30 when degraded; 1.5s stays under both.
const DefaultDelay = 1500 * time.Millisecond

const endpoint = "https://graphql.anilist.co"

// byIDQuery fetches one entry by its exact ID, used to confirm that a
// hand-entered ID is the series the curator meant.
const byIDQuery = `query($id:Int){
  Media(id:$id,type:MANGA){
    id
    title{romaji english native}
    synonyms
  }
}`

// searchQuery asks for several candidates so Match has something to verify
// against rather than trusting the top hit.
const searchQuery = `query($search:String){
  Page(page:1,perPage:5){
    media(search:$search,type:MANGA){
      id
      format
      title{romaji english native}
      synonyms
    }
  }
}`

// Media is one AniList manga entry.
type Media struct {
	ID int `json:"id"`
	// Format separates a comic from a light novel. AniList's type:MANGA covers
	// both, so this is the only thing telling them apart.
	Format string `json:"format"`
	Title  struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
	Synonyms []string `json:"synonyms"`
}

// Names returns every title AniList knows this entry by.
func (m Media) Names() []string {
	names := make([]string, 0, 3+len(m.Synonyms))
	for _, n := range []string{m.Title.Romaji, m.Title.English, m.Title.Native} {
		if n != "" {
			names = append(names, n)
		}
	}
	return append(names, m.Synonyms...)
}

type graphQLResponse struct {
	Data struct {
		Page struct {
			Media []Media `json:"media"`
		} `json:"Page"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Client is a rate-limited AniList GraphQL client.
type Client struct {
	UserAgent string
	Delay     time.Duration
	HTTP      *http.Client

	lastRequest time.Time
}

// NewClient returns a Client with polite defaults.
func NewClient() *Client {
	return &Client{
		UserAgent: DefaultUserAgent,
		Delay:     DefaultDelay,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Search returns AniList manga entries matching a name, most relevant first.
func (c *Client) Search(name string) ([]Media, error) {
	body, err := json.Marshal(map[string]interface{}{
		"query":     searchQuery,
		"variables": map[string]string{"search": name},
	})
	if err != nil {
		return nil, fmt.Errorf("encoding query: %w", err)
	}

	var data graphQLResponse
	if err := c.post(body, &data); err != nil {
		return nil, err
	}
	if len(data.Errors) > 0 {
		return nil, fmt.Errorf("AniList error: %s", data.Errors[0].Message)
	}
	return data.Data.Page.Media, nil
}

// post sends a GraphQL request, rate-limited, decoding the reply into out.
func (c *Client) post(body []byte, out interface{}) error {
	// One retry, to ride out the rate limiter rather than losing the series.
	for attempt := 0; attempt < 2; attempt++ {
		if wait := c.Delay - time.Since(c.lastRequest); wait > 0 && !c.lastRequest.IsZero() {
			time.Sleep(wait)
		}
		c.lastRequest = time.Now()

		req, rerr := http.NewRequest("POST", endpoint, bytes.NewReader(body))
		if rerr != nil {
			return fmt.Errorf("creating request: %w", rerr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.UserAgent)

		resp, derr := c.HTTP.Do(req)
		if derr != nil {
			return fmt.Errorf("request failed: %w", derr)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retry := retryAfter(resp)
			resp.Body.Close()
			if attempt == 0 {
				time.Sleep(retry)
				continue
			}
			return fmt.Errorf("rate limited by AniList")
		}
		// A lookup for an id that does not exist comes back 404 with a GraphQL
		// error body, which the caller decodes as "not found" rather than a
		// transport failure.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			resp.Body.Close()
			return fmt.Errorf("AniList returned status %d", resp.StatusCode)
		}

		decErr := json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		if decErr != nil {
			return fmt.Errorf("decoding response: %w", decErr)
		}
		return nil
	}
	return fmt.Errorf("rate limited by AniList")
}

// retryAfter reads the Retry-After header, falling back to a minute — the
// window AniList's per-minute limiter resets on.
func retryAfter(resp *http.Response) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return time.Minute
}

// ByID fetches the manga with an exact AniList ID, reporting false when no such
// manga exists.
//
// Overrides are typed by hand and skip the title verification that automatic
// resolution applies, so a mistyped or misremembered number would otherwise be
// recorded silently — and most wrong IDs are still valid IDs belonging to some
// other manga.
func (c *Client) ByID(id int) (Media, bool, error) {
	body, err := json.Marshal(map[string]interface{}{
		"query":     byIDQuery,
		"variables": map[string]int{"id": id},
	})
	if err != nil {
		return Media{}, false, fmt.Errorf("encoding query: %w", err)
	}

	var out struct {
		Data struct {
			Media *Media `json:"Media"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := c.post(body, &out); err != nil {
		return Media{}, false, err
	}
	// AniList reports an unknown id as a "Not Found" error rather than null.
	if out.Data.Media == nil {
		return Media{}, false, nil
	}
	return *out.Data.Media, true, nil
}

// FindID resolves a series name to its AniList ID, reporting false when no
// candidate can be confirmed as that series.
func (c *Client) FindID(seriesName string) (int, bool, error) {
	candidates, err := c.Search(seriesName)
	if err != nil {
		return 0, false, err
	}
	id, ok := Match(seriesName, candidates)
	return id, ok, nil
}

// Match picks the candidate that is genuinely seriesName, or reports false.
//
// AniList's search is a relevance ranking, not an exact lookup: it answers
// every query with something, so trusting the top hit would attach a confident
// but wrong ID to any series AniList doesn't carry. A candidate therefore has
// to agree on one of its own titles — romaji, English, native, or a synonym —
// once normalised, which absorbs the styling differences between Wikipedia and
// AniList ("Hunter × Hunter" vs "HUNTER×HUNTER", "Oshi no Ko" vs "[Oshi no Ko]").
func Match(seriesName string, candidates []Media) (int, bool) {
	want := chaptertitles.MatchKey(seriesName)
	if want == "" {
		return 0, false
	}

	// Rank the candidates that agree on the name rather than taking the first,
	// because AniList's ordering is relevance and relevance gets this wrong in
	// two distinct ways.
	//
	// A match on a primary title outranks a match on a synonym: "Pantsu
	// Agerune" is a nine-chapter doujin that merely lists "Fairy Tail" among
	// its synonyms, and it must never outrank the series itself. Within the
	// same kind of match, a comic outranks a light novel: AniList's type:MANGA
	// covers both, and it answered "Fairy Tail" with a 29-chapter novel ahead
	// of the 549-chapter manga. A novel is still a valid answer when nothing
	// better matches, since the dataset does carry light novel series.
	best, bestRank := 0, len(matchRanks)
	for _, m := range candidates {
		rank := matchRank(want, m)
		if rank < bestRank {
			best, bestRank = m.ID, rank
		}
		if bestRank == 0 {
			break
		}
	}
	return best, best != 0
}

// matchRanks names the tiers matchRank returns, best first.
var matchRanks = [...]string{"primary/comic", "primary/novel", "synonym/comic", "synonym/novel"}

// matchRank scores how well a candidate answers to want. Lower is better;
// len(matchRanks) means it does not answer to that name at all.
func matchRank(want string, m Media) int {
	primary := false
	synonym := false
	for _, name := range []string{m.Title.Romaji, m.Title.English, m.Title.Native} {
		if name != "" && chaptertitles.MatchKey(name) == want {
			primary = true
			break
		}
	}
	if !primary {
		for _, name := range m.Synonyms {
			if chaptertitles.MatchKey(name) == want {
				synonym = true
				break
			}
		}
	}
	if !primary && !synonym {
		return len(matchRanks)
	}

	// A serial is the only thing a chapter list can join to. A light novel is
	// the wrong medium, and a one-shot has a single chapter, so neither can be
	// right for a multi-chapter series when a serial of the same name exists.
	// Both stay acceptable as a last resort, because the dataset does carry
	// light novel and one-shot entries.
	serial := m.Format != "NOVEL" && m.Format != "ONE_SHOT"
	switch {
	case primary && serial:
		return 0
	case primary:
		return 1
	case serial:
		return 2
	default:
		return 3
	}
}
