// Package mangaplus reads official English chapter titles from Shueisha's
// MangaPlus API.
//
// MangaPlus carries the licensed titles for Shueisha series and stays current,
// which is exactly where Wikipedia is weakest. What it will not do is serve a
// whole series: the endpoint returns only the first four and last few chapters,
// so One Piece yields 8 of its 1190. That is the free window the service is
// built around, not an authentication gap — the response simply contains no
// other chapters, and the web client calls no further endpoint. The source is
// therefore a correction for a handful of chapters per series, most valuably
// the newest ones of an ongoing series.
//
// Because that window slides forward, a title captured today is gone from the
// API within weeks. Retention is handled by the ranked merge in package
// sources: nothing arrives to displace a stored MangaPlus title, so it stays.
package mangaplus

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kulaid/manga-chapter-titles/internal/sources"
)

// DefaultBaseURL is the MangaPlus web API.
const DefaultBaseURL = "https://jumpg-webapi.tokyo-cdn.com"

// site is the origin the API expects its callers to be on.
const site = "https://mangaplus.shueisha.co.jp"

// DefaultDelay is the pause between requests.
const DefaultDelay = 500 * time.Millisecond

// maxBody caps a response read. Title detail payloads are a few kilobytes.
const maxBody = 8 << 20

// Resolver finds the MangaPlus title id for a series.
//
// MangaPlus has no AniList ID, so the join runs through MangaDex: an entry
// confirmed by its links.al carries a links.engtl pointing at the series'
// MangaPlus page. No name matching is introduced by this.
type Resolver interface {
	MangaPlusID(anilistID int, seriesName string) (int, error)
}

// Client reads chapter titles from MangaPlus.
type Client struct {
	HTTP      *http.Client
	BaseURL   string
	UserAgent string
	Delay     time.Duration

	// Resolver supplies a title id for a series not already in TitleIDs.
	Resolver Resolver
	// TitleIDs caches AniList ID -> MangaPlus title id, and is seeded from the
	// dataset so a rebuild does not re-derive ids it already holds.
	TitleIDs map[int]int

	last time.Time
}

// New builds a client. userAgent is ignored in favour of a browser one: the API
// is a website's backend and rejects callers that do not look like the site.
func New(_ string) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: DefaultBaseURL,
		// The API is only served to what looks like the web client.
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
			"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		Delay:    DefaultDelay,
		TitleIDs: map[int]int{},
	}
}

// Client is one of the sources the merge ranks.
var _ sources.Source = (*Client)(nil)

// Name is the provenance recorded for a MangaPlus title.
func (c *Client) Name() string { return "mangaplus" }

// Fetch returns the chapter titles MangaPlus currently serves for a series.
//
// Result.Ref carries the MangaPlus title id, so a caller can persist it and
// skip the MangaDex round trip next time.
func (c *Client) Fetch(anilistID int, seriesName string) (sources.Result, error) {
	titleID, err := c.titleID(anilistID, seriesName)
	if err != nil {
		return sources.Result{}, err
	}
	if titleID == 0 {
		// Not a MangaPlus series. Most of the dataset is not.
		return sources.Result{}, nil
	}

	body, err := c.get(fmt.Sprintf("%s/api/title_detailV3?title_id=%d&clang=eng", c.BaseURL, titleID))
	if err != nil {
		return sources.Result{}, err
	}

	titles, err := decodeTitleDetail(body)
	if err != nil {
		return sources.Result{}, fmt.Errorf("decoding title %d: %w", titleID, err)
	}
	if len(titles) == 0 {
		return sources.Result{Ref: strconv.Itoa(titleID)}, nil
	}

	return sources.Result{
		Found:  true,
		Ref:    strconv.Itoa(titleID),
		URL:    fmt.Sprintf("%s/titles/%d", site, titleID),
		Titles: titles,
	}, nil
}

// titleID looks up a series' MangaPlus id, preferring one already known.
func (c *Client) titleID(anilistID int, seriesName string) (int, error) {
	if id, ok := c.TitleIDs[anilistID]; ok {
		return id, nil
	}
	if c.Resolver == nil {
		return 0, nil
	}
	id, err := c.Resolver.MangaPlusID(anilistID, seriesName)
	if err != nil {
		return 0, err
	}
	if c.TitleIDs == nil {
		c.TitleIDs = map[int]int{}
	}
	// Cached either way: a series with no MangaPlus page is worth remembering
	// as a zero rather than looked up again for every run.
	c.TitleIDs[anilistID] = id
	return id, nil
}

// get performs a rate-limited request carrying the headers the API requires.
func (c *Client) get(rawURL string) ([]byte, error) {
	if wait := c.Delay - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Origin", site)
	req.Header.Set("Referer", site+"/")
	// Mandatory. Without it every request comes back HTTP 200 carrying an
	// "Account Banned" payload — a missing-client-header error rather than a
	// real ban. Any well-formed UUID is accepted; no session is established.
	req.Header.Set("Session-Token", newUUID())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mangaplus: %s returned %s", rawURL, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

// newUUID returns a random RFC 4122 version 4 UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail in practice; a fixed token still satisfies
		// the header requirement, which is all this is for.
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// TitleIDFromEngTL reads a MangaPlus title id out of a MangaDex links.engtl
// value, returning 0 when the link points somewhere else.
//
// The field holds whatever official English edition a series has, which for
// most is a Viz or Kodansha storefront rather than MangaPlus, so the host has
// to be checked before the trailing number is believed.
func TitleIDFromEngTL(link string) int {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || u.Host != "mangaplus.shueisha.co.jp" {
		return 0
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "titles" {
		return 0
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return id
}
