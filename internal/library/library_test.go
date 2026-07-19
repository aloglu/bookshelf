package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"sync"
	"testing"
)

func fixture(t *testing.T) Paths {
	t.Helper()
	root := t.TempDir()
	paths := NewPaths(root)
	if err := Initialize(paths); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestResolveRootRecoversInterruptedDataReplacement(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Recovered Book"})
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	staleStage := filepath.Join(paths.Root, ".bookshelf-import-abandoned")
	if err := os.MkdirAll(staleStage, 0o755); err != nil {
		t.Fatal(err)
	}
	previous := paths.DataDir + ".previous"
	if err := os.Rename(paths.DataDir, previous); err != nil {
		t.Fatal(err)
	}

	root, err := ResolveRootAt(paths.Root)
	if err != nil {
		t.Fatal(err)
	}
	if root != paths.Root {
		t.Fatalf("resolved root = %q, want %q", root, paths.Root)
	}
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != book.Title {
		t.Fatalf("recovered books = %#v", books)
	}
	if _, err := os.Stat(previous); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous data survived recovery: %v", err)
	}
	if _, err := os.Stat(staleStage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale import stage survived recovery: %v", err)
	}
}

func TestInitializeRecoversBeforeCreatingFreshData(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Do Not Replace"})
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(paths.DataDir, paths.DataDir+".previous"); err != nil {
		t.Fatal(err)
	}

	if err := Initialize(paths); err != nil {
		t.Fatal(err)
	}
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != book.Title {
		t.Fatalf("initialize replaced recovered books: %#v", books)
	}
}

func TestRecoveryRefusesAmbiguousIncompleteCurrentData(t *testing.T) {
	paths := fixture(t)
	previousPaths := withDataDirectory(paths, paths.DataDir+".previous")
	if err := Initialize(previousPaths); err != nil {
		t.Fatal(err)
	}
	if err := Save(previousPaths, []Book{Normalize(Book{Title: "Previous Book"})}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.BooksJSON); err != nil {
		t.Fatal(err)
	}

	_, err := ResolveRootAt(paths.Root)
	if err == nil || !strings.Contains(err.Error(), "both") || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("recovery error = %v", err)
	}
	if _, err := os.Stat(paths.DataDir); err != nil {
		t.Fatalf("current data was removed: %v", err)
	}
	if _, err := os.Stat(previousPaths.DataDir); err != nil {
		t.Fatalf("previous data was removed: %v", err)
	}
}

func TestResolveRootNeverUsesCurrentDirectoryData(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BOOKSHELF_INSTALL_DIR", "")

	installedRoot := filepath.Join(home, ".local", "share", "bookshelf")
	if err := Initialize(NewPaths(installedRoot)); err != nil {
		t.Fatal(err)
	}
	repositoryRoot := t.TempDir()
	if err := Initialize(NewPaths(repositoryRoot)); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repositoryRoot)

	root, err := ResolveRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root != installedRoot {
		t.Fatalf("resolved root = %q, want installed root %q", root, installedRoot)
	}
}

func TestResolveRootHonorsExplicitInstallDirectory(t *testing.T) {
	explicitRoot := t.TempDir()
	t.Setenv("BOOKSHELF_INSTALL_DIR", explicitRoot)
	if err := Initialize(NewPaths(explicitRoot)); err != nil {
		t.Fatal(err)
	}

	root, err := ResolveRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root != explicitRoot {
		t.Fatalf("resolved root = %q, want explicit root %q", root, explicitRoot)
	}
}

func TestResolveRootAtRejectsEmptyDirectory(t *testing.T) {
	if _, err := ResolveRootAt(""); err == nil {
		t.Fatal("accepted an empty data directory")
	}
}

func TestResolveRootUsesSelectedDataProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BOOKSHELF_INSTALL_DIR", "")

	productionRoot := filepath.Join(home, ".local", "share", "bookshelf")
	developmentRoot := filepath.Join(home, ".local", "share", "bookshelf-dev")
	if err := Initialize(NewPaths(productionRoot)); err != nil {
		t.Fatal(err)
	}
	if err := Initialize(NewPaths(developmentRoot)); err != nil {
		t.Fatal(err)
	}

	root, err := ResolveRootFor("bookshelf-dev")
	if err != nil {
		t.Fatal(err)
	}
	if root != developmentRoot {
		t.Fatalf("resolved development root = %q, want %q", root, developmentRoot)
	}
}

func TestDefaultRootRejectsPathAsProfileName(t *testing.T) {
	if _, err := DefaultRootFor("../bookshelf"); err == nil {
		t.Fatal("accepted a path as a data-directory profile")
	}
}

func TestGeneratedLibraryPreservesTitleCapitalization(t *testing.T) {
	paths := fixture(t)
	title := "NASA and iPhone · İstanbul"
	book := Normalize(Book{Title: title, Author: "e.e. cummings"})
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 1 || generated[0].Title != title {
		t.Fatalf("generated title = %q, want %q", generated[0].Title, title)
	}
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

func TestParseYearRejectsEmbeddedAndMalformedDigits(t *testing.T) {
	for _, input := range []string{"1965", " 1965 "} {
		year, err := ParseYearInput(input)
		if err != nil || year == nil || *year != 1965 {
			t.Fatalf("ParseYearInput(%q) = %v, %v", input, year, err)
		}
	}
	for _, input := range []string{
		"Published in 1965",
		"edition-1965",
		"19650",
		"9780441172719",
		"nineteen sixty-five",
	} {
		if year, err := ParseYearInput(input); err == nil || year != nil {
			t.Fatalf("ParseYearInput(%q) = %v, %v; want an error", input, year, err)
		}
	}
	if year, err := ParseYearInput(" "); err != nil || year != nil {
		t.Fatalf("blank year = %v, %v", year, err)
	}
}

func TestInvalidPublishedYearDoesNotClearExistingValue(t *testing.T) {
	paths := fixture(t)
	ctx := context.Background()
	year := 1965
	book := Normalize(Book{ID: "dune", Title: "Dune", Published: &year})
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	invalid := "edition-2024"
	if _, _, err := Update(ctx, paths, book.ID, BookPatch{
		Published: &invalid,
	}, ChangeOptions{}); err == nil {
		t.Fatal("invalid update year was accepted")
	}
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Published == nil || *books[0].Published != year {
		t.Fatalf("invalid update changed stored year: %#v", books)
	}
}

func TestFindIndexPrefersExactIDOverEarlierISBNAlias(t *testing.T) {
	books := []Book{
		{ID: "first-book", Title: "ISBN Owner", ISBN: "978-0-441-17271-9"},
		{ID: "9780441172719", Title: "Exact ID Owner"},
	}
	if index := FindIndex(books, "9780441172719"); index != 1 {
		t.Fatalf("compact exact ID resolved to index %d, want 1", index)
	}
	if index := FindIndex(books, "978-0-441-17271-9"); index != 0 {
		t.Fatalf("formatted ISBN alias resolved to index %d, want 0", index)
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
	destination := filepath.Join(paths.CoversDir, preferredCoverFilename(book))
	if fileExists(destination) {
		t.Fatal("cover was published before commit")
	}
	session.Record(outcome)
	summary, err := session.Commit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if summary.Downloaded != 1 || !fileExists(destination) {
		t.Fatalf("summary = %#v, destination exists = %v", summary, fileExists(destination))
	}
}

func TestGeneratedSiteIsRebuiltFromTemplatesAndDurableCovers(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
	durable := filepath.Join(paths.CoversDir, book.CoverFile)
	writeTestJPEG(t, durable)
	if err := os.MkdirAll(paths.PublicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.PublicDir, "obsolete.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}

	if fileExists(filepath.Join(paths.PublicDir, "obsolete.txt")) {
		t.Fatal("obsolete generated file survived rebuild")
	}
	if !fileExists(paths.IndexHTML) {
		t.Fatal("website template was not published")
	}
	indexHTML, err := os.ReadFile(paths.IndexHTML)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(indexHTML, []byte(`<script src="http`)) {
		t.Fatal("published website depends on an externally hosted runtime script")
	}
	for _, asset := range []string{"css/bookshelf.css", "js/bookshelf.js", "fonts/peachi.woff2"} {
		if !fileExists(filepath.Join(paths.PublicDir, filepath.FromSlash(asset))) {
			t.Fatalf("embedded website asset %q was not published", asset)
		}
	}
	script, err := os.ReadFile(filepath.Join(paths.PublicDir, "js", "bookshelf.js"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(script, []byte("innerHTML")) {
		t.Fatal("published website uses innerHTML; book metadata must be rendered through safe DOM APIs")
	}
	webFilename := generatedWebCoverFilename(book.CoverFile)
	if !fileExists(filepath.Join(paths.PublicDir, "data", "covers", webFilename)) {
		t.Fatal("detail cover was not generated")
	}
	if !fileExists(filepath.Join(paths.PublicDir, "data", "thumbnails", webFilename)) {
		t.Fatal("website thumbnail was not generated")
	}
	if fileExists(filepath.Join(paths.PublicDir, "data", "covers", book.CoverFile)) {
		t.Fatal("full-resolution durable cover was copied into the generated website")
	}
	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 1 ||
		generated[0].Cover != "data/covers/"+webFilename ||
		generated[0].Thumbnail != "data/thumbnails/"+webFilename {
		t.Fatalf("generated cover paths = %#v", generated)
	}
	source, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if got := PublicationStatuses(paths, source)[book.ID]; got != PublicationPublished {
		t.Fatalf("published cover status = %q", got)
	}
}

func TestGeneratedSiteProcessesDistinctCoversAndDuplicateReferences(t *testing.T) {
	paths := fixture(t)
	books := make([]Book, 0, 9)
	for index := range 8 {
		book := Normalize(Book{Title: fmt.Sprintf("Book %d", index+1)})
		book.CoverFile = book.ID + ".jpg"
		writeTestJPEG(t, filepath.Join(paths.CoversDir, book.CoverFile))
		books = append(books, book)
	}
	duplicate := Normalize(Book{Title: "Shared Cover"})
	duplicate.CoverFile = books[0].CoverFile
	books = append(books, duplicate)

	var progress []int
	if err := SaveGeneratedWithContext(context.Background(), paths, books, func(current, total int) {
		if total != len(books) {
			t.Fatalf("progress total = %d, want %d", total, len(books))
		}
		progress = append(progress, current)
	}); err != nil {
		t.Fatal(err)
	}
	if len(progress) == 0 || progress[len(progress)-1] != len(books) {
		t.Fatalf("final progress = %#v", progress)
	}
	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != len(books) {
		t.Fatalf("generated books = %d, want %d", len(generated), len(books))
	}
	for index, book := range generated {
		if book.Cover == "" || book.Thumbnail == "" {
			t.Fatalf("book %d has no generated cover: %#v", index, book)
		}
	}
	if generated[len(generated)-1].Cover != generated[0].Cover {
		t.Fatal("books sharing a durable cover did not share its generated variant")
	}
}

func TestSourceLibraryDoesNotPersistPublishedCoverPaths(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{
		Title:     "Dune",
		ISBN:      "978-0-441-17271-9",
		CoverFile: "9780441172719.jpg",
		Cover:     "data/covers/9780441172719.jpg",
	})
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(paths.BooksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"cover"`) {
		t.Fatalf("source library contains a generated cover path:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"coverFile": "9780441172719.jpg"`) {
		t.Fatalf("source library does not contain its durable cover reference:\n%s", raw)
	}
}

func TestGeneratedCoverManifestReusesOnlyVerifiedAssets(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
	writeTestJPEG(t, filepath.Join(paths.CoversDir, book.CoverFile))
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}

	manifest := loadGeneratedCoverManifest(paths)
	record, ok := manifest.Covers[book.CoverFile]
	if !ok || record.SHA256 == "" || record.CoverSHA256 == "" || record.ThumbnailSHA256 == "" {
		t.Fatalf("generated cover manifest record = %#v", record)
	}
	stage := t.TempDir()
	if _, reused := reuseGeneratedCover(paths, stage, record, record.SHA256); !reused {
		t.Fatal("unchanged generated cover was not reusable")
	}
	if !fileExists(filepath.Join(stage, filepath.FromSlash(record.Cover))) ||
		!fileExists(filepath.Join(stage, filepath.FromSlash(record.Thumbnail))) {
		t.Fatal("reused generated cover variants were not copied into the stage")
	}

	if err := os.WriteFile(filepath.Join(paths.PublicDir, filepath.FromSlash(record.Thumbnail)), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, reused := reuseGeneratedCover(paths, t.TempDir(), record, record.SHA256); reused {
		t.Fatal("tampered generated cover was reused")
	}
}

func TestISBNChangeRenamesCoverButKeepsReadableFilename(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
	oldPath := filepath.Join(paths.CoversDir, book.CoverFile)
	if err := os.WriteFile(oldPath, []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}

	newISBN := "978-0-441-17270-2"
	updated, _, err := Update(context.Background(), paths, book.ID, BookPatch{ISBN: &newISBN}, ChangeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CoverFile != "978-0-441-17270-2.jpg" {
		t.Fatalf("cover filename = %q", updated.CoverFile)
	}
	if fileExists(oldPath) {
		t.Fatal("old ISBN cover filename still exists")
	}
	if !fileExists(filepath.Join(paths.CoversDir, updated.CoverFile)) {
		t.Fatal("cover was not renamed to the new ISBN")
	}
}

func TestISBNCoverFilenamePreservesEnteredHyphenation(t *testing.T) {
	formatted := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	if got := preferredCoverFilename(formatted); got != "978-0-441-17271-9.jpg" {
		t.Fatalf("formatted ISBN cover filename = %q", got)
	}
	compact := Normalize(Book{Title: "Dune", ISBN: "9780441172719"})
	if got := preferredCoverFilename(compact); got != "9780441172719.jpg" {
		t.Fatalf("compact ISBN cover filename = %q", got)
	}
}

func TestBuildReconcilesCoverFilenameWithISBNHyphenation(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = "9780441172719.jpg"
	oldPath := filepath.Join(paths.CoversDir, book.CoverFile)
	if err := os.WriteFile(oldPath, []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), paths, BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	want := "978-0-441-17271-9.jpg"
	if fileExists(oldPath) || !fileExists(filepath.Join(paths.CoversDir, want)) {
		t.Fatal("build did not reconcile the cover filename")
	}
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if books[0].CoverFile != want {
		t.Fatalf("saved cover filename = %q", books[0].CoverFile)
	}
}

func TestISBNLessCoverUsesInternalBookIDFilename(t *testing.T) {
	book := Normalize(Book{Title: "An Old Book", Author: "A. Writer"})
	if got, want := preferredCoverFilename(book), book.ID+".jpg"; got != want {
		t.Fatalf("cover filename = %q, want %q", got, want)
	}
}

func TestDiscardCoverSessionPreservesExistingCover(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
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

func TestCoverAttentionBooksFiltersResolvedAndRemovedEntries(t *testing.T) {
	paths := fixture(t)
	books := []Book{
		{ID: "missing", Title: "Missing"},
		{ID: "resolved", Title: "Resolved", CoverFile: "resolved.jpg", Cover: "data/covers/resolved.jpg"},
	}
	report := []CoverReportEntry{
		{ID: "missing", Title: "Missing", Status: CoverFetchNotFound},
		{ID: "resolved", Title: "Resolved", Status: CoverFetchFailed},
		{ID: "removed", Title: "Removed", Status: CoverFetchFailed},
		{ID: "missing", Title: "Duplicate", Status: CoverFetchSkipped},
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.CoverReportJSON, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	attention, err := CoverAttentionBooks(paths, books)
	if err != nil {
		t.Fatal(err)
	}
	if len(attention) != 1 || attention[0].ID != "missing" {
		t.Fatalf("attention = %#v", attention)
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

func TestSlugifyFallbackIsDeterministic(t *testing.T) {
	const title = "📚✨"
	const want = "book-96d58a79dd2c"
	for attempt := 0; attempt < 5; attempt++ {
		if got := Slugify(title); got != want {
			t.Fatalf("Slugify(%q) = %q, want %q", title, got, want)
		}
	}
	book := Normalize(Book{Title: title})
	if book.ID != want || book.TitleSlug != want {
		t.Fatalf("normalized fallback identifiers = ID %q, title slug %q", book.ID, book.TitleSlug)
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

func TestSlugIdentifierConflictsDoNotChangeSourceLibrary(t *testing.T) {
	paths := fixture(t)
	ctx := context.Background()
	books := []Book{
		Normalize(Book{ID: "isbn-owner", Title: "ISBN Owner", ISBN: "978-0-441-17271-9"}),
		Normalize(Book{ID: "editable", Title: "Editable Book"}),
	}
	if err := Save(paths, books); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(paths.BooksJSON)
	if err != nil {
		t.Fatal(err)
	}
	assertUnchanged := func() {
		t.Helper()
		after, err := os.ReadFile(paths.BooksJSON)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("rejected slug conflict changed source library:\n%s", after)
		}
	}
	conflictingSlug := "9780441172719"

	if _, _, err := Add(ctx, paths, Book{
		ID: "new-book", Title: "New Book", Slug: conflictingSlug,
	}, ChangeOptions{Build: true}); err == nil || !strings.Contains(err.Error(), "conflicts with book") {
		t.Fatalf("add conflict error = %v", err)
	}
	assertUnchanged()

	if _, _, err := Update(ctx, paths, "editable", BookPatch{
		Slug: &conflictingSlug,
	}, ChangeOptions{Build: true}); err == nil || !strings.Contains(err.Error(), "conflicts with book") {
		t.Fatalf("update conflict error = %v", err)
	}
	assertUnchanged()

	if _, _, err := Replace(ctx, paths, "editable", Book{
		Title: "Replacement", Slug: conflictingSlug,
	}, ChangeOptions{Build: true}); err == nil || !strings.Contains(err.Error(), "conflicts with book") {
		t.Fatalf("replace conflict error = %v", err)
	}
	assertUnchanged()
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
	if GeneratedMatches(paths, source, generated) {
		t.Fatal("same-length but different generated data was treated as current")
	}
	if got := PublicationStatuses(paths, source)["dune"]; got != PublicationNotPublished {
		t.Fatalf("status = %q", got)
	}
}

func TestPublicationStatusesDistinguishUnpublishedChanges(t *testing.T) {
	paths := fixture(t)
	published := Normalize(Book{ID: "dune", Title: "Dune"})
	if err := Save(paths, []Book{published}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, []Book{published}); err != nil {
		t.Fatal(err)
	}
	changed := published
	changed.Title = "Dune: Revised"
	if got := PublicationStatuses(paths, []Book{changed})[changed.ID]; got != PublicationChangesNotPublished {
		t.Fatalf("changed status = %q", got)
	}
	if got := PublicationStatuses(paths, []Book{published})[published.ID]; got != PublicationPublished {
		t.Fatalf("published status = %q", got)
	}
}

func TestGeneratedWebsiteExcludesHiddenBooksAndTheirCoverAssets(t *testing.T) {
	paths := fixture(t)
	visible := Normalize(Book{ID: "visible", Title: "Collected Poems", ISBN: "978-0-00-000000-1"})
	hidden := Normalize(Book{
		ID:                "hidden",
		Title:             "Collected Poems",
		ISBN:              "978-0-00-000000-2",
		WebsiteVisibility: WebsiteHidden,
	})
	visible.CoverFile = preferredCoverFilename(visible)
	hidden.CoverFile = preferredCoverFilename(hidden)
	writeTestJPEG(t, filepath.Join(paths.CoversDir, visible.CoverFile))
	writeTestJPEG(t, filepath.Join(paths.CoversDir, hidden.CoverFile))

	if err := SaveGenerated(paths, []Book{visible, hidden}); err != nil {
		t.Fatal(err)
	}
	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 1 || generated[0].ID != visible.ID {
		t.Fatalf("generated books = %#v", generated)
	}
	all := []Book{visible, hidden}
	AssignTitleSlugs(all)
	if generated[0].TitleSlug != all[0].TitleSlug {
		t.Fatalf("visible title slug = %q, want globally assigned %q", generated[0].TitleSlug, all[0].TitleSlug)
	}
	hiddenWebCover := filepath.Join(paths.PublicDir, "data", "covers", generatedWebCoverFilename(hidden.CoverFile))
	if _, err := os.Stat(hiddenWebCover); !os.IsNotExist(err) {
		t.Fatalf("hidden generated cover exists or returned unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.CoversDir, hidden.CoverFile)); err != nil {
		t.Fatalf("hidden durable cover was removed: %v", err)
	}
}

func TestWebsiteVisibilityStatusesDistinguishHiddenAndPendingChanges(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{ID: "dune", Title: "Dune"})
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	hidden := book
	hidden.WebsiteVisibility = WebsiteHidden
	if got := PublicationStatuses(paths, []Book{hidden})[book.ID]; got != PublicationVisibilityPending {
		t.Fatalf("unpublished hide status = %q", got)
	}
	if err := SaveGenerated(paths, []Book{hidden}); err != nil {
		t.Fatal(err)
	}
	if got := PublicationStatuses(paths, []Book{hidden})[book.ID]; got != PublicationHidden {
		t.Fatalf("published hide status = %q", got)
	}
}

func TestSetWebsiteVisibilityPublishesAtomicallyAndPreservesCover(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{ID: "dune", Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
	coverPath := filepath.Join(paths.CoversDir, book.CoverFile)
	writeTestJPEG(t, coverPath)
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}

	changed, err := SetWebsiteVisibility(
		context.Background(), paths, []string{book.ID}, WebsiteHidden, VisibilityChangeOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].WebsiteVisibility != WebsiteHidden {
		t.Fatalf("hidden changes = %#v", changed)
	}
	stored, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if stored[0].WebsiteVisibility != WebsiteHidden || len(generated) != 0 {
		t.Fatalf("stored = %#v, generated = %#v", stored, generated)
	}
	if _, err := os.Stat(coverPath); err != nil {
		t.Fatalf("durable cover was not preserved: %v", err)
	}

	if _, err := SetWebsiteVisibility(
		context.Background(), paths, []string{book.ID}, WebsiteVisible, VisibilityChangeOptions{},
	); err != nil {
		t.Fatal(err)
	}
	generated, err = LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 1 || generated[0].ID != book.ID {
		t.Fatalf("restored generated books = %#v", generated)
	}
}

func TestSetWebsiteVisibilityCancellationLeavesLibraryAndWebsiteUnchanged(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{ID: "dune", Title: "Dune"})
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(paths.PublicDir, "data", "books.js"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err = SetWebsiteVisibility(ctx, paths, []string{book.ID}, WebsiteHidden, VisibilityChangeOptions{
		Progress: func(_, _ int) {
			cancel()
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("visibility cancellation error = %v", err)
	}
	stored, loadErr := Load(paths)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(stored) != 1 || stored[0].WebsiteVisibility != WebsiteVisible {
		t.Fatalf("library changed after cancellation: %#v", stored)
	}
	after, readErr := os.ReadFile(filepath.Join(paths.PublicDir, "data", "books.js"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("generated website changed after cancellation")
	}
}

func TestPublicationStatusDetectsChangedDurableCoverContents(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
	coverPath := filepath.Join(paths.CoversDir, book.CoverFile)
	writeTestJPEG(t, coverPath)
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	source, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if got := PublicationStatuses(paths, source)[book.ID]; got != PublicationPublished {
		t.Fatalf("initial publication status = %q", got)
	}

	writeTestPNG(t, coverPath)
	source, err = Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if got := PublicationStatuses(paths, source)[book.ID]; got != PublicationChangesNotPublished {
		t.Fatalf("changed-cover publication status = %q", got)
	}
	generated, err := LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if GeneratedMatches(paths, source, generated) {
		t.Fatal("changed durable cover was treated as published")
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
	removed, err := Remove(context.Background(), paths, []string{first.ID, second.ID}, false)
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

func TestRemoveRollsBackMetadataAndCoverWhenPublishingFails(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
	coverPath := filepath.Join(paths.CoversDir, book.CoverFile)
	writeTestJPEG(t, coverPath)
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(paths.Root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(paths.Root, 0o755) })

	if _, err := Remove(context.Background(), paths, []string{book.ID}, true); err == nil {
		t.Fatal("remove succeeded despite an unwritable publication directory")
	}
	remaining, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != book.ID {
		t.Fatalf("library was not rolled back: %#v", remaining)
	}
	if !fileExists(coverPath) {
		t.Fatal("durable cover was not restored")
	}
}

func TestBuildRestoresDurableCoverWhenPublishingFails(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
	coverPath := filepath.Join(paths.CoversDir, book.CoverFile)
	writeTestJPEG(t, coverPath)
	originalCover, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, filepath.Join(paths.ManualCoversDir, "978-0-441-17271-9.png"))
	if err := os.Chmod(paths.Root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(paths.Root, 0o755) })

	if _, err := Build(context.Background(), paths, BuildOptions{RecomputeColors: true}); err == nil {
		t.Fatal("build succeeded despite an unwritable publication directory")
	}
	restoredCover, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restoredCover, originalCover) {
		t.Fatal("durable cover contents were not rolled back")
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
	if err := json.Unmarshal([]byte(`[{"title":"Invalid","published":"edition-1965"}]`), &books); err == nil {
		t.Fatal("invalid embedded JSON year was accepted")
	}
}

func TestDecodeImportJSONAndCSV(t *testing.T) {
	jsonBooks, err := DecodeImport(strings.NewReader(`{"books":[{"title":"Dune","author":"Frank Herbert"}]}`), "json")
	if err != nil {
		t.Fatal(err)
	}
	if len(jsonBooks) != 1 || jsonBooks[0].Title != "Dune" ||
		Normalize(jsonBooks[0]).WebsiteVisibility != WebsiteVisible {
		t.Fatalf("JSON books = %#v", jsonBooks)
	}

	csvBooks, err := DecodeImport(strings.NewReader("\ufeffTitle,Author,ISBN,Published Year,Website Visibility\nFoundation,Isaac Asimov,9780553293357,1951,hidden\n"), "csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(csvBooks) != 1 || csvBooks[0].ISBN != "9780553293357" ||
		csvBooks[0].Year() != "1951" || csvBooks[0].WebsiteVisibility != WebsiteHidden {
		t.Fatalf("CSV books = %#v", csvBooks)
	}
	if _, err := DecodeImport(strings.NewReader("Title,Published Year\nBad Year,edition-1951\n"), "csv"); err == nil {
		t.Fatal("invalid CSV year was accepted")
	}
	if _, err := DecodeImport(strings.NewReader("Title,Website Visibility\nDune,private\n"), "csv"); err == nil {
		t.Fatal("invalid CSV website visibility was accepted")
	}
}

func TestDecodeImportReportsMetadataSizeLimit(t *testing.T) {
	if _, err := decodeImportWithLimit(strings.NewReader("[]"), "json", 2); err != nil {
		t.Fatalf("exact-limit import failed: %v", err)
	}

	_, err := decodeImportWithLimit(strings.NewReader("[] "), "json", 2)
	if err == nil || err.Error() != "import metadata exceeds the 2-byte size limit" {
		t.Fatalf("oversized import error = %v", err)
	}

	_, err = decodeImportWithLimit(strings.NewReader("Title\nBook\n"), "csv", 6)
	if err == nil || err.Error() != "import metadata exceeds the 6-byte size limit" {
		t.Fatalf("oversized CSV import error = %v", err)
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

func TestConcurrentAddsDoNotLoseBooks(t *testing.T) {
	paths := fixture(t)
	const total = 30
	start := make(chan struct{})
	errors := make(chan error, total)
	var workers sync.WaitGroup
	for index := 0; index < total; index++ {
		index := index
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, _, err := Add(
				context.Background(),
				paths,
				Book{Title: fmt.Sprintf("Concurrent Book %02d", index)},
				ChangeOptions{},
			)
			errors <- err
		}()
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != total {
		t.Fatalf("stored %d books after %d concurrent adds", len(books), total)
	}
}

func TestSavedChangesReportPublishingFailureAsPartialSuccess(t *testing.T) {
	paths := fixture(t)
	ctx := context.Background()
	if err := os.WriteFile(paths.ConfigJSON, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertPartialSuccess := func(err error, subject string) {
		t.Helper()
		var partial *SavedButUnpublishedError
		if !errors.As(err, &partial) {
			t.Fatalf("error type = %T, want SavedButUnpublishedError: %v", err, err)
		}
		if partial.Subject != subject || !strings.Contains(err.Error(), "`bookshelf build`") {
			t.Fatalf("partial-success error = %v", err)
		}
	}

	added, _, err := Add(ctx, paths, Book{Title: "Added Book"}, ChangeOptions{Build: true})
	assertPartialSuccess(err, "book")
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "Added Book" {
		t.Fatalf("add was not saved: %#v", books)
	}

	updatedTitle := "Updated Book"
	updated, _, err := Update(ctx, paths, added.ID, BookPatch{Title: &updatedTitle}, ChangeOptions{Build: true})
	assertPartialSuccess(err, "book")
	books, err = Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if books[0].Title != updated.Title {
		t.Fatalf("update was not saved: %#v", books)
	}

	replaced, _, err := Replace(ctx, paths, updated.ID, Book{Title: "Replaced Book"}, ChangeOptions{Build: true})
	assertPartialSuccess(err, "book")
	books, err = Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if books[0].Title != replaced.Title {
		t.Fatalf("replacement was not saved: %#v", books)
	}

	result, err := Import(ctx, paths, []Book{{Title: "Imported Book"}}, ImportOptions{Build: true})
	assertPartialSuccess(err, "imported books")
	if result.Imported != 1 {
		t.Fatalf("import result = %#v", result)
	}
	books, err = Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("import was not saved: %#v", books)
	}
}
