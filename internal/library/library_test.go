package library

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func fixture(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	paths := NewPaths(root)
	if err := os.MkdirAll(filepath.Dir(paths.BooksJSON), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.BooksJS), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.BooksJSON, []byte("[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.IndexHTML, []byte("<!doctype html>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestNormalizeAndCleanISBN(t *testing.T) {
	book := Normalize(Book{
		Title:  "  Dune  ",
		Author: " Frank Herbert ",
		ISBN:   "978-0-441-17271-9",
	})
	if book.ID != "9780441172719" {
		t.Fatalf("ID = %q", book.ID)
	}
	if book.Title != "Dune" || book.Author != "Frank Herbert" {
		t.Fatalf("book was not trimmed: %#v", book)
	}
}

func TestGoodreadsCoverIsStagedUntilCommit(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", Author: "Frank Herbert", ISBN: "978-0-441-17271-9"})
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	imageData := testCoverJPEG(t)
	client := &http.Client{Transport: libraryRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		status := http.StatusOK
		contentType := "text/html"
		body := []byte(`<meta property="og:image" content="https://covers.test/cover._SX98_.jpg">`)
		switch request.URL.Path {
		case "/cover.jpg":
			body = imageData
			contentType = "image/jpeg"
		}
		return &http.Response{
			StatusCode: status,
			Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
			Header:     http.Header{"Content-Type": {contentType}},
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    request,
		}, nil
	})}
	config := defaultCoverFetchConfig()
	config.client = client
	config.goodreadsBookURL = "https://goodreads.test/book/%s"
	config.goodreadsDelay = func(context.Context) error { return nil }
	session, err := newCoverFetchSession(paths, []Book{book}, true, config)
	if err != nil {
		t.Fatal(err)
	}
	outcome := session.Fetch(context.Background(), 0, CoverSourceGoodreads)
	if outcome.Status != CoverFetchDownloaded || outcome.Source != CoverSourceGoodreads {
		t.Fatalf("outcome = %#v", outcome)
	}
	destination := filepath.Join(paths.CoversDir, coverFilename(book))
	if fileExists(destination) {
		t.Fatal("cover was published before commit")
	}
	session.Record(outcome)
	summary, err := session.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Downloaded != 1 || !fileExists(destination) {
		t.Fatalf("summary = %#v, destination exists = %v", summary, fileExists(destination))
	}
}

func TestDiscardCoverSessionPreservesExistingCover(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.CoversDir, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(paths.CoversDir, coverFilename(book))
	original := []byte("existing cover")
	if err := os.WriteFile(destination, original, 0o644); err != nil {
		t.Fatal(err)
	}
	imageData := testCoverJPEG(t)
	client := &http.Client{Transport: libraryRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": {"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(imageData)),
			Request:    request,
		}, nil
	})}
	config := defaultCoverFetchConfig()
	config.client = client
	config.openLibraryCoverURL = "https://openlibrary.test/%s"
	session, err := newCoverFetchSession(paths, []Book{book}, true, config)
	if err != nil {
		t.Fatal(err)
	}
	outcome := session.Fetch(context.Background(), 0, CoverSourceOpenLibrary)
	if outcome.Status != CoverFetchDownloaded {
		t.Fatalf("outcome = %#v", outcome)
	}
	session.Record(outcome)
	if err := session.Discard(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("discard changed the existing cover")
	}
}

type libraryRoundTripFunc func(*http.Request) (*http.Response, error)

func (function libraryRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestAutomaticCoverUsesGoogleForBookWithoutISBN(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "An Old Book", Author: "A. Writer"})
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	imageData := testCoverJPEG(t)
	client := &http.Client{Transport: libraryRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := imageData
		contentType := "image/jpeg"
		if request.URL.Path == "/volumes" {
			body = []byte(`{"items":[{"volumeInfo":{"imageLinks":{"large":"https://covers.test/old-book.jpg"}}}]}`)
			contentType = "application/json"
			if !strings.Contains(request.URL.Query().Get("q"), "intitle:An Old Book") {
				t.Errorf("query = %q", request.URL.Query().Get("q"))
			}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": {contentType}},
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    request,
		}, nil
	})}
	config := defaultCoverFetchConfig()
	config.client = client
	config.googleVolumesURL = "https://google.test/volumes"
	config.goodreadsDelay = func(context.Context) error { return nil }
	session, err := newCoverFetchSession(paths, []Book{book}, false, config)
	if err != nil {
		t.Fatal(err)
	}
	outcome := session.Fetch(context.Background(), 0, CoverSourceAutomatic)
	if outcome.Status != CoverFetchDownloaded || outcome.Source != CoverSourceGoogle {
		t.Fatalf("outcome = %#v", outcome)
	}
	if err := session.Discard(); err != nil {
		t.Fatal(err)
	}
}

func TestCustomURLCoverSupportsBookWithoutISBN(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "An Old Book"})
	imageData := testCoverJPEG(t)
	config := defaultCoverFetchConfig()
	config.client = &http.Client{Transport: libraryRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://example.test/custom-cover.jpg" {
			t.Errorf("cover URL = %q", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": {"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(imageData)),
			Request:    request,
		}, nil
	})}
	session, err := newCoverFetchSession(paths, []Book{book}, false, config)
	if err != nil {
		t.Fatal(err)
	}
	session.SetCustomURL("https://example.test/custom-cover.jpg")
	outcome := session.Fetch(context.Background(), 0, CoverSourceURL)
	if outcome.Status != CoverFetchDownloaded || outcome.Source != CoverSourceURL {
		t.Fatalf("outcome = %#v", outcome)
	}
	if err := session.Discard(); err != nil {
		t.Fatal(err)
	}
}

func TestCoverReportListsEveryUnresolvedBook(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Missing Cover", ISBN: "978-0-00-000000-0"})
	session, err := newCoverFetchSession(paths, []Book{book}, false, defaultCoverFetchConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Discard()
	session.Record(CoverFetchOutcome{
		Book:    book,
		Source:  CoverSourceGoodreads,
		Status:  CoverFetchNotFound,
		Message: "no cover found",
	})
	path, count, err := session.WriteReport()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || path != paths.CoverReportJSON {
		t.Fatalf("path = %q, count = %d", path, count)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Missing Cover", "not-found", "goodreads"} {
		if !strings.Contains(string(raw), expected) {
			t.Fatalf("report does not contain %q:\n%s", expected, raw)
		}
	}
}

func testCoverJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 300, 500))
	for y := 0; y < 500; y++ {
		for x := 0; x < 300; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestSlugifyTransliteratesTurkishTitles(t *testing.T) {
	title := "100. Yılında Cumhuriyet’in Popüler Kültür Haritası 2 (1950-1980)"
	want := "100-yilinda-cumhuriyetin-populer-kultur-haritasi-2-1950-1980"
	if got := Slugify(title); got != want {
		t.Fatalf("Slugify() = %q, want %q", got, want)
	}
}

func TestSlugifyTransliteratesMultipleWritingSystems(t *testing.T) {
	allowed := regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	for _, title := range []string{
		"Cien años de soledad",
		"Příliš hlučná samota",
		"Преступление и наказание",
		"影師",
		"موسم الهجرة إلى الشمال",
	} {
		slug := Slugify(title)
		if !allowed.MatchString(slug) || strings.HasPrefix(slug, "book-") {
			t.Errorf("Slugify(%q) = %q", title, slug)
		}
	}
}

func TestCustomSlugIsNormalizedAndUnique(t *testing.T) {
	paths := fixture(t)
	ctx := context.Background()
	first, _, err := Add(ctx, paths, Book{Title: "Dune", ISBN: "978-0-441-17271-9", Slug: "My Dune Copy"}, ChangeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "my-dune-copy" {
		t.Fatalf("slug = %q", first.Slug)
	}
	if _, _, err := Add(ctx, paths, Book{Title: "Dune Messiah", Slug: "MY DUNE COPY"}, ChangeOptions{}); err == nil {
		t.Fatal("duplicate URL slug was accepted")
	}
}

func TestISBNLessCustomSlugBecomesStableID(t *testing.T) {
	book := FromInput(BookInput{Title: "An Old Book", Slug: "Archive Copy 1924"})
	if book.Slug != "archive-copy-1924" || book.ID != "archive-copy-1924" {
		t.Fatalf("book = %#v", book)
	}
	book.Title = "A Newly Corrected Title"
	book = Normalize(book)
	if book.ID != "archive-copy-1924" {
		t.Fatalf("ID changed after title edit: %q", book.ID)
	}
}

func TestConfigDefaultsPersistsAndIsPublished(t *testing.T) {
	paths := fixture(t)
	config, err := LoadConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if config.PermalinkStyle != PermalinkFormattedISBN {
		t.Fatalf("default style = %q", config.PermalinkStyle)
	}
	if !config.ShowStatistics || config.DefaultView != WebsiteViewShelf || config.DefaultSort != WebsiteSortTitle {
		t.Fatalf("default website config = %#v", config)
	}
	if config.DefaultSortOrder != SortAscending ||
		config.SiteTitle != "Bookshelf" ||
		!config.ShowRandom ||
		config.ISBNLinkSources != ISBNLinksBoth ||
		!config.ShowFooter {
		t.Fatalf("additional website defaults = %#v", config)
	}
	config.PermalinkStyle = PermalinkTitleSlug
	if err := SaveConfig(paths, config); err != nil {
		t.Fatal(err)
	}
	books := []Book{Normalize(Book{Title: "Příliš hlučná samota", ISBN: "978-80-00-00000-0"})}
	AssignTitleSlugs(books)
	if err := SaveGenerated(paths, books); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(paths.BooksJS)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"permalinkStyle":"title-slug"`) ||
		!strings.Contains(string(raw), `"defaultSortOrder":"ascending"`) ||
		!strings.Contains(string(raw), `"siteTitle":"Bookshelf"`) ||
		!strings.Contains(string(raw), `"showRandom":true`) ||
		!strings.Contains(string(raw), `"isbnLinkSources":"both"`) ||
		!strings.Contains(string(raw), `"showFooter":true`) {
		t.Fatalf("published config missing:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"titleSlug": "prilis-hlucna-samota"`) {
		t.Fatalf("published title slug missing:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"permalink": "prilis-hlucna-samota"`) {
		t.Fatalf("preferred permalink missing:\n%s", raw)
	}
}

func TestLoadConfigAddsWebsiteDefaultsToLegacyConfig(t *testing.T) {
	paths := fixture(t)
	if err := os.WriteFile(paths.ConfigJSON, []byte(`{"permalinkStyle":"title-slug"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if config.PermalinkStyle != PermalinkTitleSlug ||
		!config.ShowStatistics ||
		config.DefaultView != WebsiteViewShelf ||
		config.DefaultSort != WebsiteSortTitle ||
		config.DefaultSortOrder != SortAscending ||
		config.SiteTitle != "Bookshelf" ||
		config.SiteSubtitle == "" ||
		!config.ShowRandom ||
		config.ISBNLinkSources != ISBNLinksBoth ||
		!config.ShowFooter {
		t.Fatalf("migrated config = %#v", config)
	}
}

func TestParsePermalinkStyleShortNames(t *testing.T) {
	for input, want := range map[string]PermalinkStyle{
		"isbn":      PermalinkFormattedISBN,
		"formatted": PermalinkFormattedISBN,
		"compact":   PermalinkCompactISBN,
		"title":     PermalinkTitleSlug,
	} {
		got, err := ParsePermalinkStyle(input)
		if err != nil {
			t.Fatalf("ParsePermalinkStyle(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParsePermalinkStyle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPreferredPermalinkStylesAndCustomOverride(t *testing.T) {
	book := Normalize(Book{Title: "The Trial", ISBN: "978-0-00-123456-7"})
	if got := PreferredPermalink(book, PermalinkFormattedISBN); got != "978-0-00-123456-7" {
		t.Fatalf("formatted ISBN permalink = %q", got)
	}
	if got := PreferredPermalink(book, PermalinkCompactISBN); got != "9780001234567" {
		t.Fatalf("compact ISBN permalink = %q", got)
	}
	if got := PreferredPermalink(book, PermalinkTitleSlug); got != "the-trial" {
		t.Fatalf("title permalink = %q", got)
	}
	book.Slug = "kafka-trial"
	for _, style := range []PermalinkStyle{PermalinkFormattedISBN, PermalinkCompactISBN, PermalinkTitleSlug} {
		if got := PreferredPermalink(book, style); got != "kafka-trial" {
			t.Fatalf("custom permalink for %s = %q", style, got)
		}
	}
}

func TestDuplicateTitleSlugsReceiveStableSuffixes(t *testing.T) {
	books := []Book{
		Normalize(Book{ID: "edition-one", Title: "Collected Poems"}),
		Normalize(Book{ID: "edition-two", Title: "Collected Poems"}),
	}
	AssignTitleSlugs(books)
	if books[0].TitleSlug == books[1].TitleSlug || !strings.HasPrefix(books[0].TitleSlug, "collected-poems-") {
		t.Fatalf("title slugs = %q, %q", books[0].TitleSlug, books[1].TitleSlug)
	}
	first := books[0].TitleSlug
	AssignTitleSlugs(books)
	if books[0].TitleSlug != first {
		t.Fatalf("title slug was not stable: %q → %q", first, books[0].TitleSlug)
	}
}

func TestGeneratedComparisonDetectsSameLengthStaleness(t *testing.T) {
	paths := fixture(t)
	source := []Book{Normalize(Book{ID: "dune", Title: "Dune"})}
	stale := []Book{Normalize(Book{ID: "foundation", Title: "Foundation"})}
	if err := Save(paths, source); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, stale); err != nil {
		t.Fatal(err)
	}
	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if GeneratedMatches(source, generated) {
		t.Fatal("same-length but different generated data was treated as current")
	}
	if got := PublicationStatuses(paths, source)["dune"]; got != "not generated" {
		t.Fatalf("status = %q", got)
	}
}

func TestAddBuildAndBatchRemove(t *testing.T) {
	paths := fixture(t)
	ctx := context.Background()
	first, _, err := Add(ctx, paths, Book{Title: "Dune", Author: "Frank Herbert"}, ChangeOptions{Build: true})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := Add(ctx, paths, Book{Title: "Foundation", Author: "Isaac Asimov"}, ChangeOptions{Build: true})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 2 {
		t.Fatalf("generated books = %d", len(generated))
	}
	removed, err := Remove(paths, []string{first.ID, second.ID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed = %d", len(removed))
	}
	remaining, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining = %d", len(remaining))
	}
}

func TestUpdateCanClearOptionalFields(t *testing.T) {
	paths := fixture(t)
	ctx := context.Background()
	added, _, err := Add(ctx, paths, Book{
		Title:     "Dune",
		Author:    "Frank Herbert",
		Publisher: "Ace",
		Slug:      "dune-special-edition",
	}, ChangeOptions{Build: true})
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	updated, _, err := Update(ctx, paths, added.ID, BookPatch{
		Publisher: &empty,
		Slug:      &empty,
	}, ChangeOptions{Build: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Publisher != "" {
		t.Fatalf("publisher = %q", updated.Publisher)
	}
	if updated.Slug != "" {
		t.Fatalf("slug = %q", updated.Slug)
	}
}

func TestValidateRejectsDuplicates(t *testing.T) {
	books := []Book{
		Normalize(Book{ID: "same", Title: "One", ISBN: "978-1"}),
		Normalize(Book{ID: "same", Title: "Two", ISBN: "9781"}),
	}
	if problems := Validate(books); len(problems) != 2 {
		t.Fatalf("problems = %d, want 2: %v", len(problems), problems)
	}
}

func TestLegacyStringAndNumericFieldsLoad(t *testing.T) {
	var books []Book
	raw := []byte(`[{
		"id": 42,
		"title": "Legacy Book",
		"isbn": 9780441172719,
		"published": "Published in 1965",
		"cover": null
	}]`)
	if err := json.Unmarshal(raw, &books); err != nil {
		t.Fatal(err)
	}
	if books[0].ID != "42" || books[0].ISBN != "9780441172719" {
		t.Fatalf("legacy identifiers were not converted: %#v", books[0])
	}
	if books[0].Published == nil || *books[0].Published != 1965 {
		t.Fatalf("published = %v", books[0].Published)
	}
}

func TestDecodeImportJSONAndCSV(t *testing.T) {
	jsonBooks, err := DecodeImport(strings.NewReader(`{"books":[{"title":"Dune","author":"Frank Herbert"}]}`), "json")
	if err != nil {
		t.Fatal(err)
	}
	if len(jsonBooks) != 1 || jsonBooks[0].Title != "Dune" {
		t.Fatalf("JSON books = %#v", jsonBooks)
	}

	csvBooks, err := DecodeImport(strings.NewReader("\ufeffTitle,Author,ISBN,Published Year\nFoundation,Isaac Asimov,9780553293357,1951\n"), "csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(csvBooks) != 1 || csvBooks[0].ISBN != "9780553293357" || csvBooks[0].Year() != "1951" {
		t.Fatalf("CSV books = %#v", csvBooks)
	}
}

func TestBatchImportDryRunAndSkipDuplicates(t *testing.T) {
	paths := fixture(t)
	ctx := context.Background()
	if _, _, err := Add(ctx, paths, Book{Title: "Dune", ISBN: "9780441172719"}, ChangeOptions{}); err != nil {
		t.Fatal(err)
	}
	candidates := []Book{
		{Title: "Dune Again", ISBN: "978-0-441-17271-9"},
		{Title: "Foundation", ISBN: "9780553293357"},
	}
	dryRun, err := Import(ctx, paths, candidates, ImportOptions{SkipDuplicates: true, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dryRun.Imported != 1 || dryRun.Skipped != 1 {
		t.Fatalf("dry-run result = %#v", dryRun)
	}
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("dry-run changed library: %d books", len(books))
	}

	result, err := Import(ctx, paths, candidates, ImportOptions{SkipDuplicates: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Skipped != 1 {
		t.Fatalf("import result = %#v", result)
	}
	books, err = Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("books after import = %d", len(books))
	}
}
