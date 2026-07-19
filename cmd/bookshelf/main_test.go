package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aloglu/bookshelf/internal/library"
)

func TestCommandUsageDescribesBatchAdd(t *testing.T) {
	var output bytes.Buffer
	if !commandUsage(&output, "add") {
		t.Fatal("add help was not recognized")
	}
	for _, expected := range []string{"--from FILE", "--skip-duplicates", "--dry-run", "JSON", "CSV"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("add help does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestAddCommandRejectsMalformedPublishedYear(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	err := addCommand(context.Background(), paths, []string{
		"--title", "Dune",
		"--published", "edition-1965",
		"--no-build",
	})
	if err == nil || !strings.Contains(err.Error(), "exactly four digits") {
		t.Fatalf("add error = %v", err)
	}
	books, loadErr := library.Load(paths)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(books) != 0 {
		t.Fatalf("invalid year saved a book: %#v", books)
	}
}

func TestBuildSkippedNoticeExplainsHowToPublish(t *testing.T) {
	var output bytes.Buffer
	printBuildSkippedNotice(&output)
	if got := output.String(); got != "Published website not updated. Run `bookshelf build` when ready.\n" {
		t.Fatalf("build-skipped notice = %q", got)
	}
}

func TestVisibilityCommandRequiresAnActionAndPublishesTheChange(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	book := library.Normalize(library.Book{ID: "dune", Title: "Dune"})
	if err := library.Save(paths, []library.Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := library.SaveGenerated(paths, []library.Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := visibilityCommand(context.Background(), paths, []string{book.ID}); err == nil ||
		!strings.Contains(err.Error(), "choose --hide or --show") {
		t.Fatalf("missing-action error = %v", err)
	}
	if err := visibilityCommand(context.Background(), paths, []string{"--hide", book.ID}); err != nil {
		t.Fatal(err)
	}
	books, err := library.Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := library.LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if books[0].WebsiteVisibility != library.WebsiteHidden || len(generated) != 0 {
		t.Fatalf("stored = %#v, generated = %#v", books, generated)
	}
}

func TestEditIsTheOnlyPublicEditingCommand(t *testing.T) {
	var output bytes.Buffer
	if !commandUsage(&output, "edit") {
		t.Fatal("edit help was not recognized")
	}
	if commandUsage(&output, "update") {
		t.Fatal("obsolete update command is still public")
	}
}

func TestCommandsWithoutArgumentsRejectUnexpectedValues(t *testing.T) {
	if err := run(context.Background(), []string{"version", "unexpected"}); err == nil {
		t.Fatal("version accepted an unexpected argument")
	}
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	if err := validateCommand(paths, []string{"unexpected"}); err == nil {
		t.Fatal("validate accepted an unexpected argument")
	}
}

func TestCoverOptionsSupportAllSourcesAndReplacement(t *testing.T) {
	options, err := parseCoversArgs([]string{"--all", "--source", "goodreads", "--replace"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.all || !options.replace || options.source != library.CoverSourceGoodreads {
		t.Fatalf("options = %#v", options)
	}
	if _, err := parseCoversArgs([]string{"--all", "9780441172719"}); err == nil {
		t.Fatal("--all with a book ID was accepted")
	}
	custom, err := parseCoversArgs([]string{"9780441172719", "--url", "https://example.com/cover.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if custom.source != library.CoverSourceURL || custom.url == "" {
		t.Fatalf("custom URL options = %#v", custom)
	}
}

func TestCustomCoverURLAlwaysAllowsIntentionalReplacement(t *testing.T) {
	if !coverReplacementAllowed(library.CoverSourceURL, false) {
		t.Fatal("custom cover URL did not allow replacing the current cover")
	}
	if coverReplacementAllowed(library.CoverSourceAutomatic, false) {
		t.Fatal("automatic cover fetch replaced a current cover without --replace")
	}
	if !coverReplacementAllowed(library.CoverSourceAutomatic, true) {
		t.Fatal("--replace was ignored for automatic cover fetching")
	}
}

func TestCoverOptionsSupportMissingOnlyRetries(t *testing.T) {
	options, err := parseCoversArgs([]string{"--missing", "--source", "automatic"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.missing || options.source != library.CoverSourceAutomatic {
		t.Fatalf("options = %#v", options)
	}
	for _, args := range [][]string{
		{"--missing", "--all"},
		{"--missing", "dune"},
		{"--missing", "--replace"},
		{"--missing", "--source", "url"},
		{"--missing", "--url", "https://example.com/cover.jpg"},
	} {
		if _, err := parseCoversArgs(args); err == nil {
			t.Fatalf("incompatible options were accepted: %v", args)
		}
	}
}

func TestCoverOptionsSupportAttentionWorkflow(t *testing.T) {
	options, err := parseCoversArgs([]string{"--attention", "--source", "automatic"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.attention || options.source != library.CoverSourceAutomatic {
		t.Fatalf("options = %#v", options)
	}
	for _, args := range [][]string{
		{"--attention", "--all"},
		{"--attention", "--missing"},
		{"--attention", "dune"},
		{"--attention", "--replace"},
		{"--attention", "--source", "url"},
	} {
		if _, err := parseCoversArgs(args); err == nil {
			t.Fatalf("incompatible options were accepted: %v", args)
		}
	}
}

func TestLegacyFetchCoversFlagsAreRemoved(t *testing.T) {
	var output bytes.Buffer
	for _, command := range []string{"add", "import", "build", "edit"} {
		output.Reset()
		if !commandUsage(&output, command) {
			t.Fatalf("%s help was not recognized", command)
		}
		if strings.Contains(output.String(), "--fetch-covers") {
			t.Fatalf("%s help still advertises --fetch-covers", command)
		}
	}
	if err := buildCommand(context.Background(), library.Paths{}, []string{"--fetch-covers"}); err == nil {
		t.Fatal("build accepted the removed --fetch-covers option")
	}
	if err := importCommand(context.Background(), library.Paths{}, []string{"--fetch-covers"}); err == nil {
		t.Fatal("import accepted the removed --fetch-covers option")
	}
}

func TestBookshelfArchiveCommandsRequireExplicitNonInteractiveConflictMode(t *testing.T) {
	source := library.NewPaths(t.TempDir())
	if err := library.Initialize(source); err != nil {
		t.Fatal(err)
	}
	if err := library.Save(source, []library.Book{library.Normalize(library.Book{Title: "Dune"})}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "library.bookshelf")
	if err := exportCommand(context.Background(), source, []string{archive}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(archive); err != nil || info.Size() == 0 {
		t.Fatalf("archive was not created: info = %#v, error = %v", info, err)
	}

	destination := library.NewPaths(t.TempDir())
	if err := library.Initialize(destination); err != nil {
		t.Fatal(err)
	}
	if err := library.Save(destination, []library.Book{library.Normalize(library.Book{Title: "Foundation"})}); err != nil {
		t.Fatal(err)
	}
	err := importCommand(context.Background(), destination, []string{archive})
	if err == nil || !strings.Contains(err.Error(), "--merge or --replace") {
		t.Fatalf("non-interactive archive conflict error = %v", err)
	}
	if err := importCommand(context.Background(), destination, []string{archive, "--replace"}); err != nil {
		t.Fatal(err)
	}
	books, err := library.Load(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "Dune" {
		t.Fatalf("archive replacement result = %#v", books)
	}
}

func TestStatusSummarizesCoverAndPublicationState(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	covered := library.Normalize(library.Book{Title: "Dune"})
	covered.CoverFile = covered.ID + ".jpg"
	writeValidJPEG(t, filepath.Join(paths.CoversDir, covered.CoverFile))
	missing := library.Normalize(library.Book{Title: "Foundation"})
	if err := library.Save(paths, []library.Book{covered, missing}); err != nil {
		t.Fatal(err)
	}
	books, err := library.Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := library.SaveGenerated(paths, books); err != nil {
		t.Fatal(err)
	}
	status, err := collectStatus(paths)
	if err != nil {
		t.Fatal(err)
	}
	if status.Books != 2 || status.Covers != 1 || status.MissingCovers != 1 ||
		status.Published != 2 || status.Website != "Current" {
		t.Fatalf("status = %#v", status)
	}
}

func TestBooksMissingCoversUsesLoadedCoverState(t *testing.T) {
	books := []library.Book{
		{ID: "covered", Title: "Covered", Cover: "data/covers/covered.jpg"},
		{ID: "missing", Title: "Missing"},
	}
	missing := booksMissingCovers(books)
	if len(missing) != 1 || missing[0].ID != "missing" {
		t.Fatalf("missing books = %#v", missing)
	}
}

func TestUpgradeDoesNotDownloadInstallerWhenCurrent(t *testing.T) {
	var installerRequests atomic.Int32
	previousClient := httpClient
	httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"tag_name":"v1.0.0"}`
		if strings.Contains(request.URL.Path, "installer") {
			installerRequests.Add(1)
			body = "#!/bin/sh\nexit 0\n"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	t.Cleanup(func() { httpClient = previousClient })
	t.Setenv("BOOKSHELF_LATEST_RELEASE_URL", "https://example.test/latest")
	t.Setenv("BOOKSHELF_INSTALLER_URL", "https://example.test/installer")
	previousVersion := version
	version = "v1.0.0"
	t.Cleanup(func() { version = previousVersion })

	if err := upgradeCommand(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := installerRequests.Load(); got != 0 {
		t.Fatalf("installer requests = %d, want 0", got)
	}
}

func TestUpgradeRequiresConfirmationBeforeDownload(t *testing.T) {
	var installerRequests atomic.Int32
	previousClient := httpClient
	httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"tag_name":"v1.1.0"}`
		if strings.Contains(request.URL.Path, "installer") {
			installerRequests.Add(1)
			body = "#!/bin/sh\nexit 0\n"
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	t.Cleanup(func() { httpClient = previousClient })
	t.Setenv("BOOKSHELF_LATEST_RELEASE_URL", "https://example.test/latest")
	t.Setenv("BOOKSHELF_INSTALLER_URL", "https://example.test/installer")
	previousVersion := version
	version = "v1.0.0"
	t.Cleanup(func() { version = previousVersion })

	err := upgradeCommand(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v, want non-interactive confirmation guidance", err)
	}
	if got := installerRequests.Load(); got != 0 {
		t.Fatalf("installer requests = %d, want 0", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestVersionComparisonAllowsOptionalVPrefix(t *testing.T) {
	if !sameVersion("1.0.0", "v1.0.0") {
		t.Fatal("equivalent release versions did not match")
	}
}

func TestSyncDataOnlyWritesGeneratedIndex(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	books := []library.Book{library.Normalize(library.Book{
		Title:          "Dune",
		Author:         "Frank Herbert",
		ISBN:           "978-0-441-17271-9",
		CoverFile:      "9780441172719.jpg",
		Cover:          "data/covers/dune.jpg",
		SpineColor:     "#123456",
		SpineTextColor: "#ffffff",
	})}
	if err := library.Save(paths, books); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.CoversDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeValidJPEG(t, filepath.Join(paths.CoversDir, "9780441172719.jpg"))
	sourceBefore, err := os.ReadFile(paths.BooksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncDataCommand(context.Background(), paths); err != nil {
		t.Fatal(err)
	}
	sourceAfter, err := os.ReadFile(paths.BooksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatal("synchronization modified the source library")
	}
	generated, err := library.LoadGenerated(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 1 || generated[0].SpineColor != "#123456" {
		t.Fatalf("generated books = %#v", generated)
	}
}

func TestSyncDataWaitForLibraryLockIsCancellable(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	release := holdLibraryLock(t, paths)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := syncDataCommand(ctx, paths); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sync lock error = %v", err)
	}
}

func TestSettingsCommandPublishesPermalinkPreference(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	if err := library.Save(paths, []library.Book{library.Normalize(library.Book{Title: "Dune"})}); err != nil {
		t.Fatal(err)
	}
	if err := settingsCommand(context.Background(), paths, []string{
		"--statistics", "hide",
		"--default-view", "coverflow",
		"--shelf-scroll-speed", "slow",
		"--coverflow-scroll-speed", "fast",
		"--default-sort", "author",
		"--sort-direction", "descending",
		"--site-title", "My Library",
		"--site-subtitle", "Books worth sharing",
		"--random", "hide",
		"--isbn-links", "wikipedia",
		"--footer", "show",
		"--footer-text", "Built with Bookshelf",
		"compact",
	}); err != nil {
		t.Fatal(err)
	}
	config, err := library.LoadConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if config.PermalinkStyle != library.PermalinkCompactISBN {
		t.Fatalf("permalink style = %q", config.PermalinkStyle)
	}
	if config.ShowStatistics || config.DefaultView != library.WebsiteViewCoverflow || config.DefaultSort != library.WebsiteSortAuthor {
		t.Fatalf("website config = %#v", config)
	}
	if config.DefaultSortOrder != library.SortDescending ||
		config.ShelfScrollSpeed != library.ScrollSpeedSlow ||
		config.CoverflowSpeed != library.ScrollSpeedFast ||
		config.SiteTitle != "My Library" ||
		config.SiteSubtitle != "Books worth sharing" ||
		config.ShowRandom ||
		config.ISBNLinkSources != library.ISBNLinksWikipedia ||
		!config.ShowFooter ||
		config.FooterText != "Built with Bookshelf" {
		t.Fatalf("additional website config = %#v", config)
	}
	raw, err := os.ReadFile(paths.BooksJS)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"permalinkStyle":"compact-isbn"`) {
		t.Fatalf("published setting missing:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"showStatistics":false`) ||
		!strings.Contains(string(raw), `"defaultView":"coverflow"`) ||
		!strings.Contains(string(raw), `"shelfScrollSpeed":"slow"`) ||
		!strings.Contains(string(raw), `"coverflowScrollSpeed":"fast"`) ||
		!strings.Contains(string(raw), `"defaultSort":"author"`) ||
		!strings.Contains(string(raw), `"defaultSortOrder":"descending"`) ||
		!strings.Contains(string(raw), `"siteTitle":"My Library"`) ||
		!strings.Contains(string(raw), `"showRandom":false`) ||
		!strings.Contains(string(raw), `"isbnLinkSources":"wikipedia"`) ||
		!strings.Contains(string(raw), `"footerText":"Built with Bookshelf"`) {
		t.Fatalf("published website settings missing:\n%s", raw)
	}
}

func TestSettingsChangesMergeOntoLatestConfig(t *testing.T) {
	original := library.DefaultConfig()
	desired := original
	desired.ShowFooter = false
	latest := original
	latest.SiteTitle = "Changed Elsewhere"

	applyConfigFields(&latest, desired, changedConfigFields(original, desired))
	if latest.SiteTitle != "Changed Elsewhere" {
		t.Fatalf("unrelated concurrent title was overwritten: %#v", latest)
	}
	if latest.ShowFooter {
		t.Fatalf("requested footer change was not applied: %#v", latest)
	}
}

func TestPreviewHandlerServesGeneratedWebsite(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := os.MkdirAll(paths.PublicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.IndexHTML, []byte("<!doctype html><title>Bookshelf Preview</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	response := httptest.NewRecorder()
	previewHandler(paths).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Bookshelf Preview") {
		t.Fatalf("preview response = %d %q", response.Code, response.Body.String())
	}
}

func TestSettingsCommandReportsSavedSettingsWhenPublishingFails(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(paths.Root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(paths.Root, 0o755) })

	err := settingsCommand(context.Background(), paths, []string{"--site-title", "Saved Title"})
	if err == nil || !strings.Contains(err.Error(), "settings were saved") ||
		!strings.Contains(err.Error(), "`bookshelf build`") {
		t.Fatalf("settings error = %v", err)
	}
	config, loadErr := library.LoadConfig(paths)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if config.SiteTitle != "Saved Title" {
		t.Fatalf("saved title = %q", config.SiteTitle)
	}
}

func TestSettingsWaitForLibraryLockIsCancellable(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	release := holdLibraryLock(t, paths)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := settingsCommand(ctx, paths, []string{"--site-title", "Blocked Title"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("settings lock error = %v", err)
	}
	release()
	config, err := library.LoadConfig(paths)
	if err != nil {
		t.Fatal(err)
	}
	if config.SiteTitle == "Blocked Title" {
		t.Fatal("settings were saved without acquiring the library lock")
	}
}

func TestExportCommandInfersCSVAndProtectsExistingFiles(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	book := library.Normalize(library.Book{Title: "Příliš hlučná samota", Author: "Bohumil Hrabal"})
	if err := library.Save(paths, []library.Book{book}); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "books.csv")
	if err := exportCommand(context.Background(), paths, []string{destination}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) || !strings.Contains(string(raw), book.Title) {
		t.Fatalf("CSV export = %q", raw)
	}
	if err := exportCommand(context.Background(), paths, []string{destination}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("existing export error = %v", err)
	}
	if err := exportCommand(context.Background(), paths, []string{destination, "--force"}); err != nil {
		t.Fatal(err)
	}
}

func TestExportCommandExplainsFilenameFormats(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	err := exportCommand(context.Background(), paths, []string{filepath.Join(t.TempDir(), "library")})
	if err == nil ||
		!strings.Contains(err.Error(), ".bookshelf") ||
		!strings.Contains(err.Error(), ".json") ||
		!strings.Contains(err.Error(), ".csv") {
		t.Fatalf("extensionless export error = %v", err)
	}
}

func TestExportWaitForLibraryLockIsCancellable(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	release := holdLibraryLock(t, paths)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	destination := filepath.Join(t.TempDir(), "library.bookshelf")
	err := exportCommand(ctx, paths, []string{destination})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("export lock error = %v", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled export destination error = %v", statErr)
	}
	release()
}

func TestCommitExportFileDoesNotClobberLateDestination(t *testing.T) {
	directory := t.TempDir()
	tempName := filepath.Join(directory, ".export.tmp")
	destination := filepath.Join(directory, "library.csv")
	if err := os.WriteFile(tempName, []byte("new export"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("created concurrently"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := commitExportFile(tempName, destination, false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("late destination error = %v", err)
	}
	raw, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != "created concurrently" {
		t.Fatalf("late destination was overwritten: %q", raw)
	}
}

func TestSafetyBackupRetentionKeepsNewestFiveAndUnrelatedFiles(t *testing.T) {
	directory := t.TempDir()
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 7; index++ {
		name := filepath.Join(directory, fmt.Sprintf("before-replace-20260101-00000%d.bookshelf", index))
		if err := os.WriteFile(name, []byte("archive"), 0o644); err != nil {
			t.Fatal(err)
		}
		modified := start.Add(time.Duration(index) * time.Minute)
		if err := os.Chtimes(name, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(directory, "my-library.bookshelf")
	if err := os.WriteFile(unrelated, []byte("user export"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := pruneSafetyArchives(directory, safetyBackupRetention); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, filepath.Join(directory, "before-replace-20260101-000000.bookshelf"))
	assertMissing(t, filepath.Join(directory, "before-replace-20260101-000001.bookshelf"))
	for index := 2; index < 7; index++ {
		assertExists(t, filepath.Join(directory, fmt.Sprintf("before-replace-20260101-00000%d.bookshelf", index)))
	}
	assertExists(t, unrelated)
}

func TestUninstallPreservesDataByDefault(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)
	if err := uninstallCommand(context.Background(), []string{"--force", "--yes"}); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, binPath)
	assertMissing(t, filepath.Join(installDir, "public"))
	assertExists(t, filepath.Join(installDir, "data", "books.json"))
	assertExists(t, filepath.Join(installDir, "data", "covers", "dune.jpg"))
	for _, completionPath := range legacyCompletionPaths(t) {
		assertExists(t, completionPath)
	}
}

func TestUninstallPreservesRememberedDataLocationByDefault(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	hintPath, err := installRootHintPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hintPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hintPath, []byte(installDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOOKSHELF_INSTALL_DIR", "")

	if err := uninstallCommand(context.Background(), []string{"--force", "--yes"}); err != nil {
		t.Fatal(err)
	}
	assertExists(t, hintPath)
	got, err := preferredInstallRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != installDir {
		t.Fatalf("remembered root = %q, want %q", got, installDir)
	}
}

func TestUninstallPurgeDeletesAllData(t *testing.T) {
	for _, flag := range []string{"--purge", "--delete-data"} {
		t.Run(flag, func(t *testing.T) {
			binPath, installDir := uninstallFixture(t)
			t.Setenv("BOOKSHELF_BIN_PATH", binPath)
			t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)
			hintPath, err := installRootHintPath()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(hintPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(hintPath, []byte(installDir+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := uninstallCommand(context.Background(), []string{"--force", "--yes", flag}); err != nil {
				t.Fatal(err)
			}
			assertMissing(t, binPath)
			assertMissing(t, installDir)
			assertMissing(t, hintPath)
		})
	}
}

func TestUninstallPurgeWaitForLibraryLockIsCancellable(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)
	release := holdLibraryLock(t, library.NewPaths(installDir))
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := uninstallCommand(ctx, []string{"--force", "--yes", "--purge"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("uninstall lock error = %v", err)
	}
	assertExists(t, binPath)
	assertExists(t, filepath.Join(installDir, "data", "books.json"))
	release()
}

func TestUninstallPurgePreservesUnrelatedFilesInMixedRoot(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)

	unrelatedFile := filepath.Join(installDir, "personal.txt")
	unrelatedDirectory := filepath.Join(installDir, "other-project")
	for name, contents := range map[string]string{
		unrelatedFile: "keep me",
		filepath.Join(unrelatedDirectory, "notes.txt"):                             "keep this too",
		filepath.Join(installDir, "backups", "old.bookshelf"):                      "backup",
		filepath.Join(installDir, "data.previous", "books.json"):                   "[]",
		filepath.Join(installDir, "public.previous", "index.html"):                 "old site",
		filepath.Join(installDir, ".bookshelf-public-stage", "index.html"):         "staged site",
		filepath.Join(installDir, ".bookshelf-import-stage", "data", "books.json"): "[]",
		filepath.Join(installDir, ".bookshelf.lock"):                               "",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := uninstallCommand(context.Background(), []string{"--force", "--yes", "--purge"}); err != nil {
		t.Fatal(err)
	}

	assertMissing(t, binPath)
	for _, name := range []string{
		"data",
		"public",
		"backups",
		"data.previous",
		"public.previous",
		".bookshelf-public-stage",
		".bookshelf-import-stage",
		".bookshelf.lock",
	} {
		assertMissing(t, filepath.Join(installDir, name))
	}
	assertExists(t, installDir)
	assertExists(t, unrelatedFile)
	assertExists(t, filepath.Join(unrelatedDirectory, "notes.txt"))
}

func TestUninstallRequiresConfirmationWithoutTerminal(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)
	err := uninstallCommand(context.Background(), []string{"--force"})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v", err)
	}
	assertExists(t, binPath)
	assertExists(t, filepath.Join(installDir, "public"))
}

func TestUninstallRefusesUnrecognizedDataDirectory(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)
	if err := os.Remove(library.NewPaths(installDir).RootMarker); err != nil {
		t.Fatal(err)
	}
	err := uninstallCommand(context.Background(), []string{"--force", "--yes", "--purge"})
	if err == nil || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("error = %v", err)
	}
	assertExists(t, binPath)
	assertExists(t, installDir)
}

func TestUninstallRefusesHomeDirectory(t *testing.T) {
	binPath, _ := uninstallFixture(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := library.Initialize(library.NewPaths(home)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", home)
	err = uninstallCommand(context.Background(), []string{"--force", "--yes", "--purge"})
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("error = %v", err)
	}
	assertExists(t, binPath)
	assertExists(t, home)
}

func TestUninstallRefusesSymbolicLinkDataDirectory(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	link := filepath.Join(filepath.Dir(installDir), "linked-bookshelf")
	if err := os.Symlink(installDir, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", link)
	err := uninstallCommand(context.Background(), []string{"--force", "--yes", "--purge"})
	if err == nil || !strings.Contains(err.Error(), "symbolic-link") {
		t.Fatalf("error = %v", err)
	}
	assertExists(t, binPath)
	assertExists(t, installDir)
	assertExists(t, link)
}

func TestPreferredInstallRootReadsRememberedLocation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BOOKSHELF_INSTALL_DIR", "")
	name, err := installRootHintPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(root+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := preferredInstallRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("preferred root = %q, want %q", got, root)
	}
}

func uninstallFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	binPath := filepath.Join(root, "bin", "bookshelf")
	installDir := filepath.Join(root, "share", "bookshelf")
	for _, directory := range []string{
		filepath.Dir(binPath),
		filepath.Join(installDir, "data", "covers"),
		filepath.Join(installDir, "data", "manual-covers"),
		filepath.Join(installDir, "public"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := library.Initialize(library.NewPaths(installDir)); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		binPath: "binary",
		filepath.Join(installDir, "data", "books.json"):         "[]\n",
		filepath.Join(installDir, "data", "settings.json"):      `{"permalinkStyle":"formatted-isbn"}`,
		filepath.Join(installDir, "data", "covers", "dune.jpg"): "cover",
		filepath.Join(installDir, "public", "index.html"):       "<!doctype html>",
	} {
		if err := os.WriteFile(name, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, completionPath := range legacyCompletionPaths(t) {
		if err := os.MkdirAll(filepath.Dir(completionPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(completionPath, []byte("user-owned completion\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return binPath, installDir
}

func legacyCompletionPaths(t *testing.T) []string {
	t.Helper()
	return []string{
		filepath.Join(os.Getenv("XDG_DATA_HOME"), "bash-completion", "completions", "bookshelf"),
		filepath.Join(os.Getenv("XDG_DATA_HOME"), "zsh", "site-functions", "_bookshelf"),
		filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "fish", "completions", "bookshelf.fish"),
	}
}

func holdLibraryLock(t *testing.T, paths library.Paths) func() {
	t.Helper()
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- library.WithLibraryLock(context.Background(), paths, func() error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	released := false
	return func() {
		if released {
			return
		}
		released = true
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

func assertExists(t *testing.T, name string) {
	t.Helper()
	if _, err := os.Stat(name); err != nil {
		t.Fatalf("expected %s to exist: %v", name, err)
	}
}

func assertMissing(t *testing.T, name string) {
	t.Helper()
	if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be absent, stat error = %v", name, err)
	}
}

func writeValidJPEG(t *testing.T, name string) {
	t.Helper()
	output, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewRGBA(image.Rect(0, 0, 12, 18))
	for y := 0; y < picture.Bounds().Dy(); y++ {
		for x := 0; x < picture.Bounds().Dx(); x++ {
			picture.Set(x, y, color.RGBA{R: 80, G: 40, B: 120, A: 255})
		}
	}
	encodeErr := jpeg.Encode(output, picture, &jpeg.Options{Quality: 85})
	closeErr := output.Close()
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
