package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/aloglu/bookshelf/internal/siteassets"
)

const generatedCoverManifestName = "cover-manifest.json"

type generatedCoverRecord struct {
	SHA256          string `json:"sha256"`
	Cover           string `json:"cover"`
	CoverSHA256     string `json:"coverSha256"`
	Thumbnail       string `json:"thumbnail"`
	ThumbnailSHA256 string `json:"thumbnailSha256"`
}

type generatedCoverManifest struct {
	Covers map[string]generatedCoverRecord `json:"covers"`
}

type Paths struct {
	Root            string
	DataDir         string
	PublicDir       string
	RootMarker      string
	BooksJSON       string
	ConfigJSON      string
	CoverReportJSON string
	BooksJS         string
	CoversDir       string
	ManualCoversDir string
	IndexHTML       string
}

const (
	PublicationPublished           = "Published"
	PublicationNotPublished        = "Not Published"
	PublicationChangesNotPublished = "Changes Not Published"
)

func NewPaths(root string) Paths {
	root, _ = filepath.Abs(root)
	return Paths{
		Root:            root,
		DataDir:         filepath.Join(root, "data"),
		PublicDir:       filepath.Join(root, "public"),
		RootMarker:      filepath.Join(root, "data", ".bookshelf-root"),
		BooksJSON:       filepath.Join(root, "data", "books.json"),
		ConfigJSON:      filepath.Join(root, "data", "settings.json"),
		CoverReportJSON: filepath.Join(root, "data", "cover-report.json"),
		BooksJS:         filepath.Join(root, "public", "data", "books.js"),
		CoversDir:       filepath.Join(root, "data", "covers"),
		ManualCoversDir: filepath.Join(root, "data", "manual-covers"),
		IndexHTML:       filepath.Join(root, "public", "index.html"),
	}
}

func ResolveRoot() (string, error) {
	return ResolveRootFor("bookshelf")
}

func ResolveRootFor(dataDirectory string) (string, error) {
	if configured := strings.TrimSpace(os.Getenv("BOOKSHELF_INSTALL_DIR")); configured != "" {
		return ResolveRootAt(configured)
	}
	root, err := DefaultRootFor(dataDirectory)
	if err != nil {
		return "", err
	}
	return ResolveRootAt(root)
}

func ResolveRootAt(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("Bookshelf data directory cannot be empty")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	paths := NewPaths(root)
	if err := recoverDataIfNeeded(paths); err != nil {
		return "", err
	}
	if fileExists(paths.BooksJSON) {
		return root, nil
	}
	return "", fmt.Errorf("bookshelf data was not found; expected data/books.json under %s", root)
}

func DefaultRoot() (string, error) {
	return DefaultRootFor("bookshelf")
}

func DefaultRootFor(dataDirectory string) (string, error) {
	dataDirectory = strings.TrimSpace(dataDirectory)
	if dataDirectory == "" || filepath.Base(dataDirectory) != dataDirectory || dataDirectory == "." {
		return "", fmt.Errorf("invalid Bookshelf data directory name %q", dataDirectory)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", dataDirectory), nil
}

func Ensure(paths Paths) error {
	if !fileExists(paths.BooksJSON) {
		return fmt.Errorf("bookshelf data is incomplete at %s", paths.Root)
	}
	if err := os.MkdirAll(paths.CoversDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(paths.ManualCoversDir, 0o755)
}

func Initialize(paths Paths) error {
	unlock, err := acquireLibraryLock(context.Background(), paths)
	if err != nil {
		return err
	}
	defer unlock()
	if err := recoverDataDirectory(paths); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.CoversDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.ManualCoversDir, 0o755); err != nil {
		return err
	}
	if !fileExists(paths.BooksJSON) {
		if err := atomicWrite(paths.BooksJSON, []byte("[]\n"), 0o644); err != nil {
			return err
		}
	}
	if err := atomicWrite(paths.RootMarker, []byte("bookshelf-root-v1\n"), 0o644); err != nil {
		return err
	}
	return Ensure(paths)
}

func OwnsRoot(paths Paths) bool {
	raw, err := os.ReadFile(paths.RootMarker)
	return err == nil && string(raw) == "bookshelf-root-v1\n"
}

func Load(paths Paths) ([]Book, error) {
	raw, err := os.ReadFile(paths.BooksJSON)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	var books []Book
	if err := json.Unmarshal(raw, &books); err != nil {
		return nil, fmt.Errorf("parse %s: %w", paths.BooksJSON, err)
	}
	for i := range books {
		books[i] = Normalize(books[i])
		books[i].Permalink = ""
		books[i].Thumbnail = ""
		filename := coverFilename(books[i])
		if filename != "" && fileExists(filepath.Join(paths.CoversDir, filename)) {
			books[i].Cover = filepath.ToSlash(filepath.Join("data", "covers", filename))
		} else {
			books[i].CoverFile = ""
			books[i].Cover = ""
			books[i].SpineColor = ""
			books[i].SpineTextColor = ""
		}
	}
	AssignTitleSlugs(books)
	return books, nil
}

func Save(paths Paths, books []Book) error {
	sourceBooks := make([]Book, len(books))
	copy(sourceBooks, books)
	for index := range sourceBooks {
		sourceBooks[index].Cover = ""
		sourceBooks[index].Thumbnail = ""
		sourceBooks[index].TitleSlug = ""
		sourceBooks[index].Permalink = ""
	}
	raw, err := json.MarshalIndent(sourceBooks, "", "    ")
	if err != nil {
		return err
	}
	return atomicWrite(paths.BooksJSON, append(raw, '\n'), 0o644)
}

func SaveGenerated(paths Paths, books []Book) error {
	if err := Ensure(paths); err != nil {
		return err
	}
	config, err := LoadConfig(paths)
	if err != nil {
		return err
	}
	publishedBooks := make([]Book, len(books))
	copy(publishedBooks, books)
	AssignTitleSlugs(publishedBooks)

	stage, err := os.MkdirTemp(filepath.Dir(paths.PublicDir), ".bookshelf-public-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copyEmbeddedSite(stage); err != nil {
		return err
	}
	stageCovers := filepath.Join(stage, "data", "covers")
	if err := os.MkdirAll(stageCovers, 0o755); err != nil {
		return err
	}
	stageThumbnails := filepath.Join(stage, "data", "thumbnails")
	if err := os.MkdirAll(stageThumbnails, 0o755); err != nil {
		return err
	}
	previousManifest := loadGeneratedCoverManifest(paths)
	nextManifest := generatedCoverManifest{Covers: make(map[string]generatedCoverRecord)}

	for index := range publishedBooks {
		filename := coverFilename(publishedBooks[index])
		if filename != "" && fileExists(filepath.Join(paths.CoversDir, filename)) {
			source := filepath.Join(paths.CoversDir, filename)
			digest, _, err := fileDigest(source)
			if err != nil {
				return err
			}
			hash := fmt.Sprintf("%x", digest)
			record, reused := reuseGeneratedCover(paths, stage, previousManifest.Covers[filename], hash)
			if !reused {
				var publishable bool
				record, publishable, err = generatePublishedCover(source, filename, stageCovers, stageThumbnails, hash)
				if err != nil {
					return err
				}
				if !publishable {
					publishedBooks[index].Cover = ""
					publishedBooks[index].Thumbnail = ""
					publishedBooks[index].SpineColor = ""
					publishedBooks[index].SpineTextColor = ""
					publishedBooks[index].CoverFile = ""
					publishedBooks[index].Permalink = PreferredPermalink(publishedBooks[index], config.PermalinkStyle)
					continue
				}
			}
			nextManifest.Covers[filename] = record
			publishedBooks[index].Cover = record.Cover
			publishedBooks[index].Thumbnail = record.Thumbnail
		} else {
			publishedBooks[index].Cover = ""
			publishedBooks[index].Thumbnail = ""
			publishedBooks[index].SpineColor = ""
			publishedBooks[index].SpineTextColor = ""
		}
		publishedBooks[index].CoverFile = ""
		publishedBooks[index].Permalink = PreferredPermalink(publishedBooks[index], config.PermalinkStyle)
	}
	raw, err := json.MarshalIndent(publishedBooks, "", "    ")
	if err != nil {
		return err
	}
	configRaw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	data := append([]byte("window.bookshelfConfig = "), configRaw...)
	data = append(data, ';', '\n')
	data = append(data, []byte("window.booksData = ")...)
	data = append(data, raw...)
	data = append(data, ';', '\n')

	if err := atomicWrite(filepath.Join(stage, "data", "books.js"), data, 0o644); err != nil {
		return err
	}
	manifestRaw, err := json.MarshalIndent(nextManifest, "", "    ")
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(stage, "data", generatedCoverManifestName), append(manifestRaw, '\n'), 0o644); err != nil {
		return err
	}
	return replaceDirectory(paths.PublicDir, stage)
}

func generatePublishedCover(source, filename, stageCovers, stageThumbnails, hash string) (generatedCoverRecord, bool, error) {
	generatedFilename := generatedWebCoverFilename(filename)
	record := generatedCoverRecord{
		SHA256:    hash,
		Cover:     filepath.ToSlash(filepath.Join("data", "covers", generatedFilename)),
		Thumbnail: filepath.ToSlash(filepath.Join("data", "thumbnails", generatedFilename)),
	}
	err := generateWebCoverVariants(
		source,
		filepath.Join(stageThumbnails, generatedFilename),
		filepath.Join(stageCovers, generatedFilename),
	)
	if isInvalidCoverSource(err) {
		return generatedCoverRecord{}, false, nil
	}
	if err != nil {
		return generatedCoverRecord{}, false, err
	}
	record, err = finalizeGeneratedCoverRecord(stageCovers, stageThumbnails, record)
	if err != nil {
		return generatedCoverRecord{}, false, err
	}
	return record, true, nil
}

func finalizeGeneratedCoverRecord(stageCovers, stageThumbnails string, record generatedCoverRecord) (generatedCoverRecord, error) {
	coverDigest, _, err := fileDigest(filepath.Join(stageCovers, filepath.Base(record.Cover)))
	if err != nil {
		return generatedCoverRecord{}, err
	}
	thumbnailDigest, _, err := fileDigest(filepath.Join(stageThumbnails, filepath.Base(record.Thumbnail)))
	if err != nil {
		return generatedCoverRecord{}, err
	}
	record.CoverSHA256 = fmt.Sprintf("%x", coverDigest)
	record.ThumbnailSHA256 = fmt.Sprintf("%x", thumbnailDigest)
	return record, nil
}

func loadGeneratedCoverManifest(paths Paths) generatedCoverManifest {
	manifest := generatedCoverManifest{Covers: make(map[string]generatedCoverRecord)}
	raw, err := os.ReadFile(filepath.Join(paths.PublicDir, "data", generatedCoverManifestName))
	if err != nil || json.Unmarshal(raw, &manifest) != nil || manifest.Covers == nil {
		manifest.Covers = make(map[string]generatedCoverRecord)
	}
	return manifest
}

func reuseGeneratedCover(paths Paths, stage string, record generatedCoverRecord, hash string) (generatedCoverRecord, bool) {
	if record.SHA256 != hash ||
		!validGeneratedCoverPath(record.Cover, "data/covers/") ||
		!validGeneratedCoverPath(record.Thumbnail, "data/thumbnails/") {
		return generatedCoverRecord{}, false
	}
	for _, relative := range []string{record.Cover, record.Thumbnail} {
		source := filepath.Join(paths.PublicDir, filepath.FromSlash(relative))
		destination := filepath.Join(stage, filepath.FromSlash(relative))
		expectedHash := record.CoverSHA256
		if relative == record.Thumbnail {
			expectedHash = record.ThumbnailSHA256
		}
		digest, _, err := fileDigest(source)
		if err != nil || expectedHash == "" || fmt.Sprintf("%x", digest) != expectedHash ||
			copyFile(source, destination) != nil {
			return generatedCoverRecord{}, false
		}
	}
	return record, true
}

func validGeneratedCoverPath(relative, prefix string) bool {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(relative))
	return clean == relative && strings.HasPrefix(clean, prefix) &&
		strings.EqualFold(filepath.Ext(clean), ".webp")
}

func LoadGenerated(paths Paths) ([]Book, error) {
	raw, err := os.ReadFile(paths.BooksJS)
	if err != nil {
		return nil, err
	}
	const prefix = "window.booksData ="
	text := strings.TrimSpace(string(raw))
	dataIndex := strings.LastIndex(text, prefix)
	if dataIndex < 0 || !strings.HasSuffix(text, ";") {
		return nil, errors.New("generated books file has an invalid format")
	}
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text[dataIndex:], prefix), ";"))
	var books []Book
	if err := json.Unmarshal([]byte(text), &books); err != nil {
		return nil, err
	}
	for i := range books {
		books[i] = Normalize(books[i])
		books[i].Permalink = ""
	}
	AssignTitleSlugs(books)
	return books, nil
}

func Validate(books []Book) []error {
	validated := make([]Book, len(books))
	copy(validated, books)
	AssignTitleSlugs(validated)
	books = validated
	var problems []error
	ids := make(map[string]int)
	isbns := make(map[string]int)
	slugs := make(map[string]int)
	for i, book := range books {
		label := fmt.Sprintf("book %d", i+1)
		if strings.TrimSpace(book.ID) == "" {
			problems = append(problems, fmt.Errorf("%s: missing id", label))
		}
		if strings.TrimSpace(book.Title) == "" {
			problems = append(problems, fmt.Errorf("%s: missing title", label))
		}
		if previous, ok := ids[book.ID]; ok {
			problems = append(problems, fmt.Errorf("%s: duplicate id %q (also book %d)", label, book.ID, previous+1))
		}
		ids[book.ID] = i
		if isbn := CleanISBN(book.ISBN); isbn != "" {
			if previous, ok := isbns[isbn]; ok {
				problems = append(problems, fmt.Errorf("%s: duplicate ISBN %q (also book %d)", label, isbn, previous+1))
			}
			isbns[isbn] = i
		}
		if slug := strings.ToLower(strings.TrimSpace(book.Slug)); slug != "" {
			if previous, ok := slugs[slug]; ok {
				problems = append(problems, fmt.Errorf("%s: duplicate URL slug %q (also book %d)", label, slug, previous+1))
			}
			slugs[slug] = i
			cleanSlug := CleanISBN(slug)
			for otherIndex, other := range books {
				if otherIndex == i {
					continue
				}
				if strings.EqualFold(slug, other.ID) || strings.EqualFold(slug, other.ISBN) ||
					(cleanSlug != "" && cleanSlug == CleanISBN(other.ISBN)) {
					problems = append(problems, fmt.Errorf("%s: URL slug %q conflicts with book %d", label, slug, otherIndex+1))
					break
				}
			}
		}
		if book.Published != nil && *book.Published < 0 {
			problems = append(problems, fmt.Errorf("%s: published must be a non-negative year", label))
		}
	}
	return problems
}

func GeneratedMatches(paths Paths, source, generated []Book) bool {
	if !reflect.DeepEqual(comparableBooks(source), comparableBooks(generated)) {
		return false
	}
	byID := make(map[string]Book, len(generated))
	for _, book := range generated {
		byID[book.ID] = book
	}
	manifest := loadGeneratedCoverManifest(paths)
	for _, book := range source {
		published, ok := byID[book.ID]
		if !ok || !generatedCoverCurrent(paths, book, published, manifest) {
			return false
		}
	}
	return true
}

func PublicationStatuses(paths Paths, source []Book) map[string]string {
	statuses := make(map[string]string, len(source))
	generated, err := LoadGenerated(paths)
	if err != nil {
		for _, book := range source {
			statuses[book.ID] = PublicationNotPublished
		}
		return statuses
	}
	byID := make(map[string]Book, len(generated))
	for _, book := range generated {
		byID[book.ID] = book
	}
	manifest := loadGeneratedCoverManifest(paths)
	for _, book := range source {
		published, ok := byID[book.ID]
		coverCurrent := ok && generatedCoverCurrent(paths, book, published, manifest)
		book.CoverFile = ""
		published.CoverFile = ""
		book.Cover = ""
		published.Cover = ""
		book.Thumbnail = ""
		published.Thumbnail = ""
		switch {
		case !ok:
			statuses[book.ID] = PublicationNotPublished
		case !coverCurrent || !reflect.DeepEqual(book, published):
			statuses[book.ID] = PublicationChangesNotPublished
		default:
			statuses[book.ID] = PublicationPublished
		}
	}
	return statuses
}

func generatedCoverCurrent(paths Paths, source, published Book, manifest generatedCoverManifest) bool {
	filename := coverFilename(source)
	if filename == "" || !fileExists(filepath.Join(paths.CoversDir, filename)) {
		return published.Cover == "" && published.Thumbnail == ""
	}
	record, ok := manifest.Covers[filename]
	if !ok || record.Cover != published.Cover || record.Thumbnail != published.Thumbnail {
		return false
	}
	sourceDigest, _, err := fileDigest(filepath.Join(paths.CoversDir, filename))
	if err != nil || fmt.Sprintf("%x", sourceDigest) != record.SHA256 {
		return false
	}
	for relative, expected := range map[string]string{
		record.Cover:     record.CoverSHA256,
		record.Thumbnail: record.ThumbnailSHA256,
	} {
		if !validGeneratedCoverPath(relative, "data/") || expected == "" {
			return false
		}
		digest, _, err := fileDigest(filepath.Join(paths.PublicDir, filepath.FromSlash(relative)))
		if err != nil || fmt.Sprintf("%x", digest) != expected {
			return false
		}
	}
	return true
}

func comparableBooks(books []Book) []Book {
	result := append([]Book(nil), books...)
	for index := range result {
		result[index].CoverFile = ""
		result[index].Cover = ""
		result[index].Thumbnail = ""
	}
	return result
}

func FindIndex(books []Book, idOrISBN string) int {
	needle := strings.TrimSpace(idOrISBN)
	for i, book := range books {
		if book.ID == needle {
			return i
		}
	}
	needleISBN := CleanISBN(needle)
	if needleISBN != "" {
		for i, book := range books {
			if CleanISBN(book.ISBN) == needleISBN {
				return i
			}
		}
	}
	return -1
}

func atomicWrite(name string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, name)
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func copyEmbeddedSite(destination string) error {
	return fs.WalkDir(siteassets.Files, "assets", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel("assets", name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := siteassets.Files.ReadFile(name)
		if err != nil {
			return err
		}
		return atomicWrite(target, data, 0o644)
	})
}

func replaceDirectory(destination, stage string) error {
	backup := destination + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	hadDestination := false
	if _, err := os.Stat(destination); err == nil {
		hadDestination = true
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		if hadDestination {
			_ = os.Rename(backup, destination)
		}
		return err
	}
	return os.RemoveAll(backup)
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}
