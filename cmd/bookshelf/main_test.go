package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

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

func TestEditIsTheOnlyPublicEditingCommand(t *testing.T) {
	var output bytes.Buffer
	if !commandUsage(&output, "edit") {
		t.Fatal("edit help was not recognized")
	}
	if commandUsage(&output, "update") {
		t.Fatal("obsolete update command is still public")
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
	if err := os.WriteFile(filepath.Join(paths.CoversDir, "9780441172719.jpg"), []byte("cover"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(paths.BooksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncDataCommand(paths); err != nil {
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

func TestSettingsCommandPublishesPermalinkPreference(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	if err := library.Save(paths, []library.Book{library.Normalize(library.Book{Title: "Dune"})}); err != nil {
		t.Fatal(err)
	}
	if err := settingsCommand(paths, []string{
		"--statistics", "hide",
		"--default-view", "coverflow",
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
		!strings.Contains(string(raw), `"defaultSort":"author"`) ||
		!strings.Contains(string(raw), `"defaultSortOrder":"descending"`) ||
		!strings.Contains(string(raw), `"siteTitle":"My Library"`) ||
		!strings.Contains(string(raw), `"showRandom":false`) ||
		!strings.Contains(string(raw), `"isbnLinkSources":"wikipedia"`) ||
		!strings.Contains(string(raw), `"footerText":"Built with Bookshelf"`) {
		t.Fatalf("published website settings missing:\n%s", raw)
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
	if err := exportCommand(paths, []string{destination}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) || !strings.Contains(string(raw), book.Title) {
		t.Fatalf("CSV export = %q", raw)
	}
	if err := exportCommand(paths, []string{destination}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("existing export error = %v", err)
	}
	if err := exportCommand(paths, []string{destination, "--force"}); err != nil {
		t.Fatal(err)
	}
}

func TestUninstallPreservesDataByDefault(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)
	if err := uninstallCommand([]string{"--force", "--yes"}); err != nil {
		t.Fatal(err)
	}
	assertMissing(t, binPath)
	assertMissing(t, filepath.Join(installDir, "public"))
	assertExists(t, filepath.Join(installDir, "data", "books.json"))
	assertExists(t, filepath.Join(installDir, "data", "covers", "dune.jpg"))
	for _, completionPath := range completionPaths(filepath.Dir(filepath.Dir(binPath))) {
		assertMissing(t, completionPath)
	}
}

func TestUninstallPurgeDeletesAllData(t *testing.T) {
	for _, flag := range []string{"--purge", "--delete-data"} {
		t.Run(flag, func(t *testing.T) {
			binPath, installDir := uninstallFixture(t)
			t.Setenv("BOOKSHELF_BIN_PATH", binPath)
			t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)
			if err := uninstallCommand([]string{"--force", "--yes", flag}); err != nil {
				t.Fatal(err)
			}
			assertMissing(t, binPath)
			assertMissing(t, installDir)
		})
	}
}

func TestUninstallRequiresConfirmationWithoutTerminal(t *testing.T) {
	binPath, installDir := uninstallFixture(t)
	t.Setenv("BOOKSHELF_BIN_PATH", binPath)
	t.Setenv("BOOKSHELF_INSTALL_DIR", installDir)
	err := uninstallCommand([]string{"--force"})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v", err)
	}
	assertExists(t, binPath)
	assertExists(t, filepath.Join(installDir, "public"))
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
	for _, completionPath := range completionPaths(root) {
		if err := os.MkdirAll(filepath.Dir(completionPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(completionPath, []byte("completion\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return binPath, installDir
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
