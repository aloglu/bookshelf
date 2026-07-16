package library

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	}, ChangeOptions{Build: true})
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	updated, _, err := Update(ctx, paths, added.ID, BookPatch{
		Publisher: &empty,
	}, ChangeOptions{Build: true})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Publisher != "" {
		t.Fatalf("publisher = %q", updated.Publisher)
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
