// Package mangadex fetches English chapter titles from the MangaDex API.
//
// MangaDex covers far more series than Wikipedia and stays current with ongoing
// releases, but its titles come from scanlation groups rather than the licensed
// English release. It is therefore a gap-filler: consulted after Wikipedia, and
// only for chapters no better source has named.
package mangadex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kulaid/manga-chapter-titles/internal/sources"
)

// DefaultDelay keeps requests inside MangaDex's published rate limit
// (5 requests/second across the API); one every 250ms leaves ample headroom.
const DefaultDelay = 250 * time.Millisecond

const apiBase = "https://api.mangadex.org"

// Client is a rate-limited MangaDex API client.
type Client struct {
	UserAgent string
	Delay     time.Duration
	HTTP      *http.Client

	lastRequest time.Time
}

// New returns a Client with polite defaults.
func New(userAgent string) *Client {
	return &Client{
		UserAgent: userAgent,
		Delay:     DefaultDelay,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Name identifies this source in provenance records.
func (c *Client) Name() string { return "mangadex" }

func (c *Client) get(rawURL string, out interface{}) error {
	if wait := c.Delay - time.Since(c.lastRequest); wait > 0 && !c.lastRequest.IsZero() {
		time.Sleep(wait)
	}
	c.lastRequest = time.Now()

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MangaDex returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type mangaSearchResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Links map[string]string `json:"links"`
		} `json:"attributes"`
	} `json:"data"`
}

// findByAniList searches for a series by name and returns the MangaDex ID of
// the entry whose AniList link is exactly anilistID.
//
// The name is only a way to narrow the search; the AniList link is what decides
// the match, so a near-miss on the name yields nothing rather than the wrong
// series.
func (c *Client) findByAniList(anilistID int, seriesName string) (string, error) {
	params := url.Values{}
	params.Set("title", seriesName)
	params.Set("limit", "10")
	for _, r := range []string{"safe", "suggestive", "erotica", "pornographic"} {
		params.Add("contentRating[]", r)
	}

	var resp mangaSearchResponse
	if err := c.get(apiBase+"/manga?"+params.Encode(), &resp); err != nil {
		return "", err
	}

	want := strconv.Itoa(anilistID)
	for _, m := range resp.Data {
		if m.Attributes.Links["al"] == want {
			return m.ID, nil
		}
	}
	return "", nil
}

type chapterResponse struct {
	Data []struct {
		Attributes struct {
			Chapter string `json:"chapter"`
			Title   string `json:"title"`
		} `json:"attributes"`
	} `json:"data"`
	Total int `json:"total"`
}

// chapterTitles pages through a series' English chapters.
//
// MangaDex holds one entry per scanlation group, so a chapter number appears
// several times with different titles and many empty ones. Results are ordered
// oldest-first and the first non-empty title for a number wins, which favours
// the earliest — usually the most established — translation.
func (c *Client) chapterTitles(mangaID string) (sources.Titles, error) {
	titles := make(sources.Titles)
	const limit = 100

	for offset := 0; ; offset += limit {
		params := url.Values{}
		params.Set("manga", mangaID)
		params.Add("translatedLanguage[]", "en")
		params.Set("limit", strconv.Itoa(limit))
		params.Set("offset", strconv.Itoa(offset))
		params.Set("order[chapter]", "asc")

		var resp chapterResponse
		if err := c.get(apiBase+"/chapter?"+params.Encode(), &resp); err != nil {
			return titles, err
		}

		for _, ch := range resp.Data {
			num, err := strconv.ParseFloat(strings.TrimSpace(ch.Attributes.Chapter), 64)
			if err != nil {
				continue
			}
			title := strings.TrimSpace(ch.Attributes.Title)
			if title == "" {
				continue
			}
			if _, exists := titles[num]; !exists {
				titles[num] = title
			}
		}

		if len(resp.Data) == 0 || offset+limit >= resp.Total {
			break
		}
		// MangaDex caps offset at 10000; stop rather than loop forever.
		if offset+limit >= 10000 {
			break
		}
	}

	return titles, nil
}

// Fetch implements sources.Source.
func (c *Client) Fetch(anilistID int, seriesName string) (sources.Result, error) {
	if anilistID == 0 {
		return sources.Result{}, nil
	}

	id, err := c.findByAniList(anilistID, seriesName)
	if err != nil {
		return sources.Result{}, err
	}
	if id == "" {
		return sources.Result{}, nil
	}

	titles, err := c.chapterTitles(id)
	if err != nil {
		// Partial results are still worth keeping; report the error alongside.
		return sources.Result{
			Found:  len(titles) > 0,
			Ref:    id,
			URL:    "https://mangadex.org/title/" + id,
			Titles: titles,
		}, err
	}

	return sources.Result{
		Found:  true,
		Ref:    id,
		URL:    "https://mangadex.org/title/" + id,
		Titles: titles,
	}, nil
}
