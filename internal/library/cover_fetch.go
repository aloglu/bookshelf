package library

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type CoverSource string

const (
	CoverSourceAutomatic   CoverSource = "automatic"
	CoverSourceGoodreads   CoverSource = "goodreads"
	CoverSourceOpenLibrary CoverSource = "openlibrary"
	CoverSourceGoogle      CoverSource = "google"
	CoverSourceManual      CoverSource = "manual"
	CoverSourceURL         CoverSource = "url"
)

func ParseCoverSource(value string) (CoverSource, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto", "automatic":
		return CoverSourceAutomatic, nil
	case "goodreads", "gr":
		return CoverSourceGoodreads, nil
	case "openlibrary", "open-library", "ol":
		return CoverSourceOpenLibrary, nil
	case "google", "google-books", "googlebooks":
		return CoverSourceGoogle, nil
	case "manual", "local":
		return CoverSourceManual, nil
	case "url", "custom-url", "custom":
		return CoverSourceURL, nil
	default:
		return "", fmt.Errorf("unknown cover source %q", value)
	}
}

func CoverSourceLabel(source CoverSource) string {
	switch source {
	case CoverSourceAutomatic:
		return "Automatic"
	case CoverSourceGoodreads:
		return "Goodreads"
	case CoverSourceOpenLibrary:
		return "Open Library"
	case CoverSourceGoogle:
		return "Google Books"
	case CoverSourceManual:
		return "Manual covers"
	case CoverSourceURL:
		return "Custom URL"
	default:
		return string(source)
	}
}

type CoverFetchStatus string

const (
	CoverFetchDownloaded CoverFetchStatus = "downloaded"
	CoverFetchSkipped    CoverFetchStatus = "skipped"
	CoverFetchNotFound   CoverFetchStatus = "not-found"
	CoverFetchFailed     CoverFetchStatus = "failed"
)

type CoverFetchOutcome struct {
	Book       Book
	Source     CoverSource
	Status     CoverFetchStatus
	Message    string
	stagedPath string
}

type CoverFetchSummary struct {
	Total      int
	Downloaded int
	Skipped    int
	NotFound   int
	Failed     int
	Colored    int
}

type coverFetchConfig struct {
	client              *http.Client
	goodreadsBookURL    string
	openLibraryCoverURL string
	googleVolumesURL    string
	goodreadsDelay      func(context.Context) error
}

func defaultCoverFetchConfig() coverFetchConfig {
	return coverFetchConfig{
		client:              &http.Client{Timeout: 25 * time.Second},
		goodreadsBookURL:    "https://www.goodreads.com/book/isbn/%s",
		openLibraryCoverURL: openLibraryURL,
		googleVolumesURL:    "https://www.googleapis.com/books/v1/volumes",
		goodreadsDelay: func(ctx context.Context) error {
			delay := time.Duration(1000+rand.IntN(2001)) * time.Millisecond
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

type CoverFetchSession struct {
	paths    Paths
	books    []Book
	replace  bool
	stageDir string
	config   coverFetchConfig
	outcomes []CoverFetchOutcome

	goodreadsBlocked bool
	customURL        string
}

func NewCoverFetchSession(paths Paths, books []Book, replace bool) (*CoverFetchSession, error) {
	return newCoverFetchSession(paths, books, replace, defaultCoverFetchConfig())
}

func newCoverFetchSession(paths Paths, books []Book, replace bool, config coverFetchConfig) (*CoverFetchSession, error) {
	if err := Ensure(paths); err != nil {
		return nil, err
	}
	stageDir, err := os.MkdirTemp(paths.DataDir, ".cover-fetch-")
	if err != nil {
		return nil, err
	}
	return &CoverFetchSession{
		paths:    paths,
		books:    append([]Book(nil), books...),
		replace:  replace,
		stageDir: stageDir,
		config:   config,
	}, nil
}

func (s *CoverFetchSession) Books() []Book {
	return append([]Book(nil), s.books...)
}

func (s *CoverFetchSession) Outcomes() []CoverFetchOutcome {
	return append([]CoverFetchOutcome(nil), s.outcomes...)
}

func (s *CoverFetchSession) CoverDirectory() string {
	return s.paths.CoversDir
}

func (s *CoverFetchSession) SetCustomURL(value string) {
	s.customURL = strings.TrimSpace(value)
}

func CoverPath(paths Paths, book Book) string {
	filename := coverFilename(book)
	if filename == "" {
		filename = preferredCoverFilename(book)
	}
	return filepath.Join(paths.CoversDir, filename)
}

func (s *CoverFetchSession) Fetch(ctx context.Context, index int, source CoverSource) CoverFetchOutcome {
	book := s.books[index]
	outcome := CoverFetchOutcome{Book: book, Source: source}
	existingFilename := coverFilename(book)
	if !s.replace && existingFilename != "" && fileExists(filepath.Join(s.paths.CoversDir, existingFilename)) {
		outcome.Status = CoverFetchSkipped
		outcome.Message = "cover already exists"
		return outcome
	}
	if source != CoverSourceGoogle && source != CoverSourceURL && CleanISBN(book.ISBN) == "" {
		if source == CoverSourceAutomatic {
			source = CoverSourceGoogle
		} else {
			outcome.Status = CoverFetchNotFound
			outcome.Message = "this source requires an ISBN"
			return outcome
		}
	}
	stagePath := filepath.Join(s.stageDir, fmt.Sprintf("%04d-%s.jpg", index, safeToken(book.ID)))
	sources := []CoverSource{source}
	if source == CoverSourceAutomatic {
		sources = []CoverSource{CoverSourceGoodreads, CoverSourceOpenLibrary, CoverSourceGoogle}
	}
	var lastMessage string
	hadError := false
	for _, candidate := range sources {
		if candidate == CoverSourceGoodreads && s.goodreadsBlocked {
			lastMessage = "Goodreads paused after blocking automated requests"
			continue
		}
		found, message, err := s.fetchFrom(ctx, book, candidate, stagePath)
		if err != nil {
			if ctx.Err() != nil {
				outcome.Status = CoverFetchFailed
				outcome.Message = ctx.Err().Error()
				return outcome
			}
			lastMessage = err.Error()
			hadError = true
			continue
		}
		if found {
			outcome.Source = candidate
			outcome.Status = CoverFetchDownloaded
			outcome.Message = "downloaded"
			outcome.stagedPath = stagePath
			return outcome
		}
		lastMessage = message
	}
	outcome.Status = CoverFetchNotFound
	if hadError {
		outcome.Status = CoverFetchFailed
	}
	outcome.Message = lastMessage
	if outcome.Message == "" {
		outcome.Message = "no cover found"
	}
	return outcome
}

func (s *CoverFetchSession) Record(outcome CoverFetchOutcome) {
	s.outcomes = append(s.outcomes, outcome)
}

func (s *CoverFetchSession) Summary() CoverFetchSummary {
	summary := CoverFetchSummary{Total: len(s.outcomes)}
	for _, outcome := range s.outcomes {
		switch outcome.Status {
		case CoverFetchDownloaded:
			summary.Downloaded++
		case CoverFetchSkipped:
			summary.Skipped++
		case CoverFetchNotFound:
			summary.NotFound++
		case CoverFetchFailed:
			summary.Failed++
		}
	}
	return summary
}

func (s *CoverFetchSession) WriteReport() (string, int, error) {
	type reportEntry struct {
		ID      string           `json:"id"`
		Title   string           `json:"title"`
		Author  string           `json:"author,omitempty"`
		ISBN    string           `json:"isbn,omitempty"`
		Status  CoverFetchStatus `json:"status"`
		Source  CoverSource      `json:"source"`
		Message string           `json:"message,omitempty"`
	}
	entries := make([]reportEntry, 0)
	for _, outcome := range s.outcomes {
		if outcome.Status == CoverFetchDownloaded {
			continue
		}
		entries = append(entries, reportEntry{
			ID:      outcome.Book.ID,
			Title:   outcome.Book.Title,
			Author:  outcome.Book.Author,
			ISBN:    outcome.Book.ISBN,
			Status:  outcome.Status,
			Source:  outcome.Source,
			Message: outcome.Message,
		})
	}
	if len(entries) == 0 {
		if err := os.Remove(s.paths.CoverReportJSON); err != nil && !os.IsNotExist(err) {
			return s.paths.CoverReportJSON, 0, err
		}
		return s.paths.CoverReportJSON, 0, nil
	}
	raw, err := json.MarshalIndent(entries, "", "    ")
	if err != nil {
		return s.paths.CoverReportJSON, 0, err
	}
	if err := atomicWrite(s.paths.CoverReportJSON, append(raw, '\n'), 0o644); err != nil {
		return s.paths.CoverReportJSON, 0, err
	}
	return s.paths.CoverReportJSON, len(entries), nil
}

func (s *CoverFetchSession) Commit() (CoverFetchSummary, error) {
	summary := s.Summary()
	books, err := Load(s.paths)
	if err != nil {
		return summary, err
	}
	originalBooks := append([]Book(nil), books...)
	type movedCover struct {
		destination string
		backup      string
	}
	moved := make([]movedCover, 0, summary.Downloaded)
	rollback := func() {
		for index := len(moved) - 1; index >= 0; index-- {
			_ = os.Remove(moved[index].destination)
			if moved[index].backup != "" {
				_ = os.Rename(moved[index].backup, moved[index].destination)
			}
		}
		_ = Save(s.paths, originalBooks)
		_ = SaveGenerated(s.paths, originalBooks)
	}

	for _, outcome := range s.outcomes {
		if outcome.Status != CoverFetchDownloaded {
			continue
		}
		index := FindIndex(books, outcome.Book.ID)
		if index < 0 {
			rollback()
			return summary, fmt.Errorf("book %q was removed during cover fetching", outcome.Book.Title)
		}
		filename := coverFilename(books[index])
		if filename == "" {
			filename = preferredCoverFilename(books[index])
		}
		destination := filepath.Join(s.paths.CoversDir, filename)
		backup := ""
		if fileExists(destination) {
			backup = filepath.Join(s.stageDir, "backup-"+filepath.Base(destination))
			if err := os.Rename(destination, backup); err != nil {
				rollback()
				return summary, err
			}
		}
		if err := os.Rename(outcome.stagedPath, destination); err != nil {
			if backup != "" {
				_ = os.Rename(backup, destination)
			}
			rollback()
			return summary, err
		}
		moved = append(moved, movedCover{destination: destination, backup: backup})
		books[index].CoverFile = filename
		books[index].Cover = filepath.ToSlash(filepath.Join("data", "covers", filepath.Base(destination)))
		background, foreground, paletteErr := extractPalette(destination)
		if paletteErr == nil {
			books[index].SpineColor = background
			books[index].SpineTextColor = foreground
			summary.Colored++
		}
	}
	if err := Save(s.paths, books); err != nil {
		rollback()
		return summary, err
	}
	if err := SaveGenerated(s.paths, books); err != nil {
		rollback()
		return summary, err
	}
	if err := os.RemoveAll(s.stageDir); err != nil {
		return summary, err
	}
	return summary, nil
}

func (s *CoverFetchSession) Discard() error {
	return os.RemoveAll(s.stageDir)
}

func (s *CoverFetchSession) fetchFrom(ctx context.Context, book Book, source CoverSource, destination string) (bool, string, error) {
	switch source {
	case CoverSourceGoodreads:
		if err := s.config.goodreadsDelay(ctx); err != nil {
			return false, "", err
		}
		imageURL, blocked, err := s.goodreadsImageURL(ctx, CleanISBN(book.ISBN))
		if blocked {
			s.goodreadsBlocked = true
			return false, "Goodreads temporarily blocked automated requests", nil
		}
		if err != nil || imageURL == "" {
			return false, "not found on Goodreads", err
		}
		return s.downloadJPEG(ctx, imageURL, destination)
	case CoverSourceOpenLibrary:
		imageURL := fmt.Sprintf(s.config.openLibraryCoverURL, CleanISBN(book.ISBN))
		return s.downloadJPEG(ctx, imageURL, destination)
	case CoverSourceGoogle:
		imageURL, err := s.googleImageURL(ctx, book)
		if err != nil || imageURL == "" {
			return false, "not found on Google Books", err
		}
		return s.downloadJPEG(ctx, imageURL, destination)
	case CoverSourceURL:
		if s.customURL == "" {
			return false, "", fmt.Errorf("custom cover URL is empty")
		}
		return s.downloadJPEG(ctx, s.customURL, destination)
	default:
		return false, "", fmt.Errorf("unsupported cover source %q", source)
	}
}

var (
	goodreadsOGImage = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	goodreadsOGAlt   = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)
	goodreadsCover   = regexp.MustCompile(`(?i)(?:class=["'][^"']*bookCover[^"']*["']|id=["']coverImage["'])[^>]+src=["']([^"']+)["']`)
	goodreadsResize  = regexp.MustCompile(`\._S[XY]\d+_`)
)

func (s *CoverFetchSession) goodreadsImageURL(ctx context.Context, isbn string) (string, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(s.config.goodreadsBookURL, isbn), nil)
	if err != nil {
		return "", false, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/124 Safari/537.36")
	response, err := s.config.client.Do(request)
	if err != nil {
		return "", false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusServiceUnavailable {
		return "", true, nil
	}
	if response.StatusCode != http.StatusOK {
		return "", false, nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return "", false, err
	}
	lower := bytes.ToLower(raw)
	if bytes.Contains(lower, []byte("captcha")) || bytes.Contains(lower, []byte("robot check")) {
		return "", true, nil
	}
	for _, pattern := range []*regexp.Regexp{goodreadsOGImage, goodreadsOGAlt, goodreadsCover} {
		if match := pattern.FindSubmatch(raw); len(match) == 2 {
			value := html.UnescapeString(string(match[1]))
			value = goodreadsResize.ReplaceAllString(value, "")
			return value, false, nil
		}
	}
	return "", false, nil
}

func (s *CoverFetchSession) googleImageURL(ctx context.Context, book Book) (string, error) {
	query := ""
	if isbn := CleanISBN(book.ISBN); isbn != "" {
		query = "isbn:" + isbn
	} else {
		query = "intitle:" + book.Title
		if book.Author != "" {
			query += " inauthor:" + book.Author
		}
	}
	values := url.Values{"q": {query}, "maxResults": {"5"}, "printType": {"books"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.config.googleVolumesURL+"?"+values.Encode(), nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "BookshelfCLI/2")
	response, err := s.config.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", nil
	}
	var result struct {
		Items []struct {
			VolumeInfo struct {
				ImageLinks map[string]string `json:"imageLinks"`
			} `json:"volumeInfo"`
		} `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
		return "", err
	}
	for _, item := range result.Items {
		for _, size := range []string{"extraLarge", "large", "medium", "small", "thumbnail", "smallThumbnail"} {
			if imageURL := item.VolumeInfo.ImageLinks[size]; imageURL != "" {
				return strings.Replace(imageURL, "http://", "https://", 1), nil
			}
		}
	}
	return "", nil
}

func (s *CoverFetchSession) downloadJPEG(ctx context.Context, imageURL, destination string) (bool, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return false, "", err
	}
	request.Header.Set("User-Agent", "BookshelfCLI/2")
	response, err := s.config.client.Do(request)
	if err != nil {
		return false, "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, "cover not found", nil
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 20<<20))
	if err != nil {
		return false, "", err
	}
	if len(raw) < 1000 {
		return false, "cover image was empty", nil
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return false, "cover response was not an image", nil
	}
	file, err := os.Create(destination)
	if err != nil {
		return false, "", err
	}
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 90}); err != nil {
		file.Close()
		_ = os.Remove(destination)
		return false, "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(destination)
		return false, "", err
	}
	return true, "downloaded", nil
}
