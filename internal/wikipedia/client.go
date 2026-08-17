package wikipedia

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultUserAgent identifies this tool to the Wikimedia API. Their policy
// requires a descriptive User-Agent with contact info; requests sent with Go's
// default agent are answered with 403/429.
const DefaultUserAgent = "manga-chapter-titles/1.0 (https://github.com/kulaid/manga-chapter-titles)"

// DefaultDelay is the pause between API calls. Wikimedia asks for serial
// requests from a single client rather than bursts; one per second keeps a full
// corpus build comfortably inside their limits.
const DefaultDelay = time.Second

// Client is a rate-limited MediaWiki API client for one wiki host.
type Client struct {
	Host      string        // e.g. "en.wikipedia.org"
	UserAgent string        // sent on every request
	Delay     time.Duration // minimum pause between requests
	HTTP      *http.Client

	lastRequest time.Time
}

// NewClient returns a Client for the English Wikipedia with polite defaults.
func NewClient() *Client {
	return &Client{
		Host:      "en.wikipedia.org",
		UserAgent: DefaultUserAgent,
		Delay:     DefaultDelay,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}
}

// get performs a rate-limited GET against the wiki's api.php and decodes the
// JSON body into out.
func (c *Client) get(params url.Values, out interface{}) error {
	if wait := c.Delay - time.Since(c.lastRequest); wait > 0 && !c.lastRequest.IsZero() {
		time.Sleep(wait)
	}
	c.lastRequest = time.Now()

	apiURL := fmt.Sprintf("https://%s/w/api.php?%s", c.Host, params.Encode())
	req, err := http.NewRequest("GET", apiURL, nil)
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
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

// parseResponse decodes action=parse with formatversion=2, where wikitext is
// returned as a plain string rather than {"*": "..."}.
type parseResponse struct {
	Parse struct {
		Title    string `json:"title"`
		Wikitext string `json:"wikitext"`
	} `json:"parse"`
	Error *struct {
		Code string `json:"code"`
		Info string `json:"info"`
	} `json:"error"`
}

// Wikitext fetches the raw wikitext of an article, following redirects to the
// canonical page. It returns the resolved article title alongside the text.
func (c *Client) Wikitext(article string) (title, wikitext string, err error) {
	params := url.Values{}
	params.Set("action", "parse")
	params.Set("page", article)
	params.Set("prop", "wikitext")
	params.Set("format", "json")
	params.Set("formatversion", "2")
	params.Set("redirects", "1")

	var data parseResponse
	if err := c.get(params, &data); err != nil {
		return "", "", err
	}
	if data.Error != nil {
		return "", "", fmt.Errorf("%s: %s", data.Error.Code, data.Error.Info)
	}
	if data.Parse.Wikitext == "" {
		return "", "", fmt.Errorf("article %q has no wikitext", article)
	}
	return data.Parse.Title, data.Parse.Wikitext, nil
}

// categoryResponse decodes action=query&list=categorymembers.
type categoryResponse struct {
	Query struct {
		CategoryMembers []struct {
			Title string `json:"title"`
		} `json:"categorymembers"`
	} `json:"query"`
	Continue struct {
		CMContinue string `json:"cmcontinue"`
	} `json:"continue"`
}

// CategoryMembers returns every main-namespace article in a category, following
// continuation until the category is exhausted.
func (c *Client) CategoryMembers(category string) ([]string, error) {
	var titles []string
	cont := ""

	for {
		params := url.Values{}
		params.Set("action", "query")
		params.Set("list", "categorymembers")
		params.Set("cmtitle", category)
		params.Set("cmlimit", "500")
		params.Set("cmnamespace", "0")
		params.Set("format", "json")
		params.Set("formatversion", "2")
		if cont != "" {
			params.Set("cmcontinue", cont)
		}

		var data categoryResponse
		if err := c.get(params, &data); err != nil {
			return titles, err
		}
		for _, m := range data.Query.CategoryMembers {
			titles = append(titles, m.Title)
		}
		if data.Continue.CMContinue == "" {
			return titles, nil
		}
		cont = data.Continue.CMContinue
	}
}

// searchResponse decodes action=query&list=search.
type searchResponse struct {
	Query struct {
		Search []struct {
			Title string `json:"title"`
		} `json:"search"`
	} `json:"query"`
}

// Search returns article titles matching a query, most relevant first.
func (c *Client) Search(query string, limit int) ([]string, error) {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "search")
	params.Set("srsearch", query)
	params.Set("srlimit", fmt.Sprint(limit))
	params.Set("format", "json")
	params.Set("formatversion", "2")

	var data searchResponse
	if err := c.get(params, &data); err != nil {
		return nil, err
	}
	titles := make([]string, 0, len(data.Query.Search))
	for _, hit := range data.Query.Search {
		titles = append(titles, hit.Title)
	}
	return titles, nil
}

// ArticleURL builds the canonical article URL for a page title.
func (c *Client) ArticleURL(title string) string {
	host := c.Host
	if host == "" {
		host = "en.wikipedia.org"
	}
	return fmt.Sprintf("https://%s/wiki/%s", host, strings.ReplaceAll(url.PathEscape(title), "%20", "_"))
}
