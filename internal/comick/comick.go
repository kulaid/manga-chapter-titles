// Package comick fetches English chapter titles from the Comick API.
//
// Comick aggregates many scanlation groups and often names chapters that
// MangaDex leaves untitled, so it is worth consulting even where MangaDex has
// the series. Like MangaDex it carries scanlator titles, so it ranks below
// Wikipedia's licensed ones.
package comick

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

// DefaultDelay is the pause between requests. Comick publishes no rate limit,
// so this is deliberately conservative.
const DefaultDelay = 500 * time.Millisecond

const apiBase = "https://api.comick.dev"

// Client is a rate-limited Comick API client.
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
func (c *Client) Name() string { return "comick" }

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
	req.Header.Set("Referer", "https://comick.dev/")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Comick returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type searchResult struct {
	HID   string `json:"hid"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type comicResponse struct {
	Comic struct {
		HID      string         `json:"hid"`
		Slug     string         `json:"slug"`
		Links    map[string]any `json:"links"`
		Title    string         `json:"title"`
		MDTitles []struct {
			Title string `json:"title"`
		} `json:"md_titles"`
	} `json:"comic"`
}

// aniListLink reads the AniList id out of a comic's links map. Comick types the
// value inconsistently across entries — sometimes a JSON string, sometimes a
// number — so both are accepted.
func aniListLink(links map[string]any) string {
	v, ok := links["al"]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// findByAniList searches by name, then confirms each candidate against the
// AniList id. The name only narrows the search; the id decides the match.
func (c *Client) findByAniList(anilistID int, seriesName string) (hid, slug string, err error) {
	params := url.Values{}
	params.Set("q", seriesName)
	params.Set("limit", "10")

	var results []searchResult
	if err := c.get(apiBase+"/v1.0/search?"+params.Encode(), &results); err != nil {
		return "", "", err
	}

	want := strconv.Itoa(anilistID)
	for _, r := range results {
		if r.Slug == "" {
			continue
		}
		var detail comicResponse
		if err := c.get(fmt.Sprintf("%s/comic/%s/?tachiyomi=true", apiBase, url.PathEscape(r.Slug)), &detail); err != nil {
			// One unreadable candidate shouldn't abandon the whole search.
			continue
		}
		if aniListLink(detail.Comic.Links) == want {
			return detail.Comic.HID, detail.Comic.Slug, nil
		}
	}
	return "", "", nil
}

type chapterEntry struct {
	Chap      string `json:"chap"`
	Title     any    `json:"title"` // string or null
	Lang      string `json:"lang"`
	CreatedAt string `json:"created_at"`
	PublishAt string `json:"publish_at"`
}

type chaptersResponse struct {
	Chapters []chapterEntry `json:"chapters"`
	Total    int            `json:"total"`
}

// mergePage folds one page of chapter entries into the running result, keeping
// the newest-uploaded title for each chapter number. newest carries the chosen
// entry's timestamp across pages, since a chapter's alternatives can straddle a
// page boundary.
func mergePage(titles sources.Titles, newest map[float64]time.Time, entries []chapterEntry) {
	for _, ch := range entries {
		num, err := strconv.ParseFloat(strings.TrimSpace(ch.Chap), 64)
		if err != nil {
			continue
		}
		title, _ := ch.Title.(string)
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}

		when := chapterTime(ch.CreatedAt, ch.PublishAt)
		if prev, seen := newest[num]; seen && !when.After(prev) {
			continue
		}
		titles[num] = title
		newest[num] = when
	}
}

// chapterTime reads an entry's timestamp, preferring when it was added to
// Comick and falling back to its publish date. An unparseable or missing value
// sorts oldest so it never displaces a dated entry.
func chapterTime(createdAt, publishAt string) time.Time {
	for _, v := range []string{createdAt, publishAt} {
		if v == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// chapterTitles pages through a comic's English chapters.
//
// Comick holds one entry per scanlation group, so a chapter number appears
// several times with different titles. The newest upload wins: later groups
// tend to be re-translations or corrections of earlier ones, and Comick returns
// entries newest-first only within a page, so the choice has to be made on the
// timestamp rather than on arrival order.
func (c *Client) chapterTitles(hid string) (sources.Titles, error) {
	titles := make(sources.Titles)
	newest := make(map[float64]time.Time)
	const limit = 300

	for page := 1; ; page++ {
		rawURL := fmt.Sprintf("%s/comic/%s/chapters?lang=en&tachiyomi=true&limit=%d&page=%d",
			apiBase, url.PathEscape(hid), limit, page)

		var resp chaptersResponse
		if err := c.get(rawURL, &resp); err != nil {
			return titles, err
		}

		mergePage(titles, newest, resp.Chapters)

		if len(resp.Chapters) == 0 || page*limit >= resp.Total {
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

	hid, slug, err := c.findByAniList(anilistID, seriesName)
	if err != nil {
		return sources.Result{}, err
	}
	if hid == "" {
		return sources.Result{}, nil
	}

	titles, err := c.chapterTitles(hid)
	result := sources.Result{
		Found:  len(titles) > 0 || err == nil,
		Ref:    slug,
		URL:    "https://comick.dev/comic/" + slug,
		Titles: titles,
	}
	return result, err
}
