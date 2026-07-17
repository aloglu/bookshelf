package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type Paths struct {
	Root            string
	PublicDir       string
	SourceDir       string
	BooksJSON       string
	ConfigJSON      string
	CoverReportJSON string
	BooksJS         string
	CoversDir       string
	ManualCoversDir string
	IndexHTML       string
}

func NewPaths(root string) Paths {
	root, _ = filepath.Abs(root)
	return Paths{
		Root:            root,
		PublicDir:       filepath.Join(root, "public"),
		SourceDir:       filepath.Join(root, "library"),
		BooksJSON:       filepath.Join(root, "library", "books.json"),
		ConfigJSON:      filepath.Join(root, "library", "config.json"),
		CoverReportJSON: filepath.Join(root, "library", "cover-report.json"),
		BooksJS:         filepath.Join(root, "public", "data", "books.js"),
		CoversDir:       filepath.Join(root, "public", "data", "covers"),
		ManualCoversDir: filepath.Join(root, "library", "manual-covers"),
		IndexHTML:       filepath.Join(root, "public", "index.html"),
	}
}

func ResolveRoot() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("BOOKSHELF_INSTALL_DIR")); configured != "" {
		return filepath.Abs(configured)
	}
	if cwd, err := os.Getwd(); err == nil {
		paths := NewPaths(cwd)
		if fileExists(paths.BooksJSON) && fileExists(paths.IndexHTML) {
			return paths.Root, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(home, ".local", "share", "bookshelf")
	paths := NewPaths(root)
	if fileExists(paths.BooksJSON) && fileExists(paths.IndexHTML) {
		return root, nil
	}
	return "", fmt.Errorf("bookshelf files were not found; expected library/books.json and public/index.html under %s", root)
}

func Ensure(paths Paths) error {
	if !fileExists(paths.BooksJSON) || !fileExists(paths.IndexHTML) {
		return fmt.Errorf("installed bookshelf files are incomplete at %s", paths.Root)
	}
	if err := os.MkdirAll(paths.CoversDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(paths.ManualCoversDir, 0o755)
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
	}
	AssignTitleSlugs(books)
	return books, nil
}

func Save(paths Paths, books []Book) error {
	sourceBooks := make([]Book, len(books))
	copy(sourceBooks, books)
	for index := range sourceBooks {
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
	config, err := LoadConfig(paths)
	if err != nil {
		return err
	}
	publishedBooks := make([]Book, len(books))
	copy(publishedBooks, books)
	AssignTitleSlugs(publishedBooks)
	for index := range publishedBooks {
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
	return atomicWrite(paths.BooksJS, data, 0o644)
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

func GeneratedMatches(source, generated []Book) bool {
	return reflect.DeepEqual(source, generated)
}

func PublicationStatuses(paths Paths, source []Book) map[string]string {
	statuses := make(map[string]string, len(source))
	generated, err := LoadGenerated(paths)
	if err != nil {
		for _, book := range source {
			statuses[book.ID] = "not generated"
		}
		return statuses
	}
	byID := make(map[string]Book, len(generated))
	for _, book := range generated {
		byID[book.ID] = book
	}
	for _, book := range source {
		published, ok := byID[book.ID]
		switch {
		case !ok:
			statuses[book.ID] = "not generated"
		case !reflect.DeepEqual(book, published):
			statuses[book.ID] = "stale"
		case book.Cover == "":
			statuses[book.ID] = "missing cover"
		default:
			statuses[book.ID] = "ready"
		}
	}
	return statuses
}

func FindIndex(books []Book, idOrISBN string) int {
	needle := strings.TrimSpace(idOrISBN)
	needleISBN := CleanISBN(needle)
	for i, book := range books {
		if book.ID == needle || (needleISBN != "" && CleanISBN(book.ISBN) == needleISBN) {
			return i
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

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}
