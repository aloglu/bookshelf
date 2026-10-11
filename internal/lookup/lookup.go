// Package lookup finds a book's details by ISBN: Open Library first, then
// Google Books. Everything it returns is a suggestion the owner confirms.
package lookup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound means neither source knows the ISBN.
var ErrNotFound = errors.New("Open Library and Google Books don’t know this ISBN")

// Draft is what a source says about a book.
type Draft struct {
	Title      string `json:"title"`
	Author     string `json:"author"`
	Translator string `json:"translator"`
	Publisher  string `json:"publisher"`
	Published  *int   `json:"published"`
	Pages      *int   `json:"pages"`
	Binding    string `json:"binding"`
	ISBN       string `json:"isbn"`
	CoverURL   string `json:"coverUrl"`
	Source     string `json:"source"` // "Open Library" or "Google Books"
}

// Client looks books up. The zero value is not usable; use New.
type Client struct {
	HTTP        *http.Client
	OpenLibrary string // base URLs, replaceable in tests
	OpenCovers  string
	Google      string
	GoogleKey   string
	UserAgent   string
}

// New makes a client. googleKey may be empty (anonymous Google lookups are
// rate-limited, so a key helps a busy library).
func New(googleKey, version string) *Client {
	return &Client{
		HTTP:        &http.Client{Timeout: 12 * time.Second},
		OpenLibrary: "https://openlibrary.org",
		OpenCovers:  "https://covers.openlibrary.org",
		Google:      "https://www.googleapis.com/books/v1",
		GoogleKey:   googleKey,
		UserAgent:   "Bookshelf/" + version + " (+https://github.com/aloglu/bookshelf)",
	}
}

// Find looks up bare ISBN-13 digits.
func (c *Client) Find(ctx context.Context, isbn string) (Draft, error) {
	d, err := c.openLibrary(ctx, isbn)
	if err == nil {
		return d, nil
	}
	g, gerr := c.google(ctx, isbn)
	if gerr == nil {
		return g, nil
	}
	if errors.Is(err, ErrNotFound) && errors.Is(gerr, ErrNotFound) {
		return Draft{}, ErrNotFound
	}
	return Draft{}, fmt.Errorf("the book sources couldn’t be reached: %v; %v", err, gerr)
}

func (c *Client) get(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

func (c *Client) openLibrary(ctx context.Context, isbn string) (Draft, error) {
	var ed struct {
		Title          string   `json:"title"`
		Publishers     []string `json:"publishers"`
		PublishDate    string   `json:"publish_date"`
		Pages          int      `json:"number_of_pages"`
		PhysicalFormat string   `json:"physical_format"`
		Covers         []int    `json:"covers"`
		ByStatement    string   `json:"by_statement"`
		Contributions  []string `json:"contributions"`
	}
	if err := c.get(ctx, c.OpenLibrary+"/isbn/"+isbn+".json", &ed); err != nil {
		return Draft{}, err
	}
	// The search index has the author's name, the year the work first
	// appeared, and the title with its usual capitals.
	var search struct {
		Docs []struct {
			Title     string   `json:"title"`
			Authors   []string `json:"author_name"`
			FirstYear int      `json:"first_publish_year"`
			Pages     int      `json:"number_of_pages_median"`
		} `json:"docs"`
	}
	_ = c.get(ctx, c.OpenLibrary+"/search.json?isbn="+isbn+"&fields=title,author_name,first_publish_year,number_of_pages_median", &search)

	d := Draft{ISBN: isbn, Source: "Open Library", Title: ed.Title, Binding: ed.PhysicalFormat}
	if len(ed.Publishers) > 0 {
		d.Publisher = ed.Publishers[0]
	}
	if ed.Pages > 0 {
		d.Pages = &ed.Pages
	}
	if y := year(ed.PublishDate); y > 0 {
		d.Published = &y
	}
	if len(search.Docs) > 0 {
		doc := search.Docs[0]
		// The index has the work's title, which may be in its original language
		// ("Cien años de soledad" for an English edition); it is only used when
		// it is the edition's title with better capitals.
		// An edition title may also carry its series ("Middlemarch (Penguin
		// Classics)"), dropped when what remains is the work's title.
		if doc.Title != "" && (strings.EqualFold(doc.Title, d.Title) || strings.EqualFold(doc.Title, withoutSeries(d.Title))) {
			d.Title = doc.Title
		}
		if len(doc.Authors) > 0 {
			d.Author = doc.Authors[0]
		}
		if doc.FirstYear > 0 {
			d.Published = &doc.FirstYear
		}
		if d.Pages == nil && doc.Pages > 0 {
			d.Pages = &doc.Pages
		}
	}
	d.Translator = translator(ed.ByStatement, ed.Contributions)
	if len(ed.Covers) > 0 && ed.Covers[0] > 0 {
		d.CoverURL = c.OpenCovers + "/b/id/" + strconv.Itoa(ed.Covers[0]) + "-L.jpg"
	}
	if d.Title == "" {
		return Draft{}, ErrNotFound
	}
	return d, nil
}

func (c *Client) google(ctx context.Context, isbn string) (Draft, error) {
	q := url.Values{"q": {"isbn:" + isbn}}
	if c.GoogleKey != "" {
		q.Set("key", c.GoogleKey)
	}
	var res struct {
		Items []struct {
			Info struct {
				Title         string   `json:"title"`
				Authors       []string `json:"authors"`
				Publisher     string   `json:"publisher"`
				PublishedDate string   `json:"publishedDate"`
				PageCount     int      `json:"pageCount"`
				Images        struct {
					Thumbnail string `json:"thumbnail"`
				} `json:"imageLinks"`
			} `json:"volumeInfo"`
		} `json:"items"`
	}
	if err := c.get(ctx, c.Google+"/volumes?"+q.Encode(), &res); err != nil {
		return Draft{}, err
	}
	if len(res.Items) == 0 || res.Items[0].Info.Title == "" {
		return Draft{}, ErrNotFound
	}
	v := res.Items[0].Info
	d := Draft{ISBN: isbn, Source: "Google Books", Title: v.Title, Publisher: v.Publisher}
	if len(v.Authors) > 0 {
		d.Author = v.Authors[0]
	}
	if y := year(v.PublishedDate); y > 0 {
		d.Published = &y
	}
	if v.PageCount > 0 {
		d.Pages = &v.PageCount
	}
	if t := v.Images.Thumbnail; t != "" {
		// Ask for the largest size Google will give, over HTTPS.
		t = strings.Replace(t, "http://", "https://", 1)
		t = strings.Replace(t, "&edge=curl", "", 1)
		d.CoverURL = strings.Replace(t, "zoom=1", "zoom=0", 1)
	}
	return d, nil
}

var trailingSeries = regexp.MustCompile(`\s*\([^()]*\)\s*$`)

func withoutSeries(title string) string { return trailingSeries.ReplaceAllString(title, "") }

var yearPattern = regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`)

func year(s string) int {
	m := yearPattern.FindString(s)
	y, _ := strconv.Atoi(m)
	return y
}

var translatedBy = regexp.MustCompile(`(?i)translated(?: from the [a-z]+)?(?: and [a-z ]+)? by ([^;.\[\]]+)`)

// translator picks a translator out of Open Library's credit lines, if one is named.
func translator(byStatement string, contributions []string) string {
	for _, c := range contributions {
		if name, ok := strings.CutSuffix(strings.TrimSpace(c), "(Translator)"); ok {
			return strings.TrimSpace(name)
		}
	}
	if m := translatedBy.FindStringSubmatch(byStatement); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// CoverHosts are the only places covers are fetched from, so the server can
// never be used to fetch an arbitrary address.
var CoverHosts = []string{"covers.openlibrary.org", "archive.org", "books.google.com", "books.googleusercontent.com"}

// AllowedCover reports whether a cover URL may be fetched.
func AllowedCover(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range CoverHosts {
		if host == h || (h == "archive.org" && strings.HasSuffix(host, ".archive.org")) { // Open Library covers redirect to archive.org mirrors
			return true
		}
	}
	return false
}

// FetchCover downloads a cover from an allowed host. Open Library's covers
// redirect to archive.org, which is allowed; nothing else is followed.
func (c *Client) FetchCover(ctx context.Context, raw string) (io.ReadCloser, error) {
	if !AllowedCover(raw) {
		return nil, errors.New("covers can only be fetched from Open Library and Google Books")
	}
	client := *c.HTTP
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !AllowedCover(req.URL.String()) {
			return errors.New("the cover redirected somewhere it may not")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("the cover couldn’t be downloaded (%s)", resp.Status)
	}
	return resp.Body, nil
}
