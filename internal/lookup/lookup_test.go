package lookup

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fake(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for prefix, body := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				w.Write([]byte(body))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOpenLibrary(t *testing.T) {
	srv := fake(t, map[string]string{
		"/isbn/9780679723424.json": `{"title":"Pale fire","publishers":["Vintage Books"],"publish_date":"1989","number_of_pages":315,"covers":[11203725]}`,
		"/search.json":             `{"docs":[{"title":"Pale Fire","author_name":["Vladimir Nabokov"],"first_publish_year":1962}]}`,
	})
	c := New("", "test")
	c.OpenLibrary, c.OpenCovers = srv.URL, "https://covers.openlibrary.org"
	d, err := c.Find(context.Background(), "9780679723424")
	if err != nil {
		t.Fatal(err)
	}
	if d.Title != "Pale Fire" || d.Author != "Vladimir Nabokov" || *d.Published != 1962 || *d.Pages != 315 || d.Publisher != "Vintage Books" || d.Source != "Open Library" {
		t.Fatalf("%+v", d)
	}
	if d.CoverURL != "https://covers.openlibrary.org/b/id/11203725-L.jpg" || !AllowedCover(d.CoverURL) {
		t.Fatalf("cover %q", d.CoverURL)
	}
}

func TestFallsBackToGoogle(t *testing.T) {
	ol := fake(t, nil)
	g := fake(t, map[string]string{"/volumes": `{"items":[{"volumeInfo":{"title":"Middlemarch","authors":["George Eliot"],"publisher":"Penguin","publishedDate":"2003-04-29","pageCount":880,"imageLinks":{"thumbnail":"http://books.google.com/books/content?id=x&zoom=1&edge=curl"}}}]}`})
	c := New("", "test")
	c.OpenLibrary, c.Google = ol.URL, g.URL
	d, err := c.Find(context.Background(), "9780141439549")
	if err != nil {
		t.Fatal(err)
	}
	if d.Source != "Google Books" || d.Author != "George Eliot" || *d.Pages != 880 || d.CoverURL != "https://books.google.com/books/content?id=x&zoom=0" {
		t.Fatalf("%+v", d)
	}
}

func TestNotFound(t *testing.T) {
	ol, g := fake(t, nil), fake(t, map[string]string{"/volumes": `{"totalItems":0}`})
	c := New("", "test")
	c.OpenLibrary, c.Google = ol.URL, g.URL
	if _, err := c.Find(context.Background(), "9780000000002"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestAllowedCover(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://covers.openlibrary.org/b/id/1-L.jpg":     true,
		"https://ia800100.us.archive.org/view/x.jpg":      true,
		"https://books.google.com/books/content?id=x":     true,
		"http://covers.openlibrary.org/b/id/1-L.jpg":      false, // not HTTPS
		"https://evil.example/covers.openlibrary.org.jpg": false,
		"https://covers.openlibrary.org.evil.example/x":   false,
		"https://user@covers.openlibrary.org/x":           false,
		"https://127.0.0.1/x":                             false,
	} {
		if AllowedCover(raw) != want {
			t.Errorf("AllowedCover(%q) != %v", raw, want)
		}
	}
}

func TestTranslator(t *testing.T) {
	if got := translator("by Gabriel García Márquez ; translated from the Spanish by Gregory Rabassa.", nil); got != "Gregory Rabassa" {
		t.Fatalf("by statement: %q", got)
	}
	if got := translator("", []string{"Breon Mitchell (Translator)"}); got != "Breon Mitchell" {
		t.Fatalf("contributions: %q", got)
	}
}
