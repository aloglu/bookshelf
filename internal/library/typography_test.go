package library

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestNormalizeTypographyUsesContextualEnglishQuotes(t *testing.T) {
	input := `He said "It's 'excellent'." O'Brien wrote about the 1980's.`
	want := `He said “It’s ‘excellent’.” O’Brien wrote about the 1980’s.`
	got := NormalizeTypography(input)
	if got != want {
		t.Fatalf("NormalizeTypography() = %q, want %q", got, want)
	}
	if second := NormalizeTypography(got); second != got {
		t.Fatalf("normalization is not idempotent: %q", second)
	}
}

func TestNormalizeFooterMarkdownPreservesDestinationsAndCode(t *testing.T) {
	input := "Editor's [author's site](https://example.com/o'brien) and `don't`"
	want := "Editor’s [author’s site](https://example.com/o'brien) and `don't`"
	if got := NormalizeFooterMarkdown(input); got != want {
		t.Fatalf("NormalizeFooterMarkdown() = %q, want %q", got, want)
	}
}

func TestNormalizeAppliesOnlyToHumanReadableBookFields(t *testing.T) {
	book := Normalize(Book{
		ID:         "archive's-copy",
		Title:      `"Alice's Archive"`,
		Author:     "O'Brien",
		ISBN:       "978-0-00-00'0000-0",
		Translator: `"T. O'Neil"`,
		Publisher:  "Writer's Press",
		Binding:    "Publisher's hardcover",
	})
	if book.Title != `“Alice’s Archive”` ||
		book.Author != "O’Brien" ||
		book.Translator != "“T. O’Neil”" ||
		book.Publisher != "Writer’s Press" ||
		book.Binding != "Publisher’s hardcover" {
		t.Fatalf("human-readable fields were not normalized: %#v", book)
	}
	if book.ID != "archive's-copy" || book.ISBN != "978-0-00-00'0000-0" {
		t.Fatalf("identifier fields changed: %#v", book)
	}
}

func TestBuildPersistsTypographyMigration(t *testing.T) {
	paths := fixture(t)
	legacyBooks := `[
    {
        "id": "archive-copy",
        "title": "\"Editor's Archive\"",
        "author": "O'Brien"
    }
]`
	if err := os.WriteFile(paths.BooksJSON, []byte(legacyBooks), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyConfig := `{
    "permalinkStyle": "formatted-isbn",
    "siteTitle": "Reader's Shelf",
    "siteSubtitle": "\"Books\" for everyone",
    "footerText": "[Editor's site](https://example.com/o'brien) and ` + "`don't`" + `"
}`
	if err := os.WriteFile(paths.ConfigJSON, []byte(legacyConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), paths, BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	bookData, err := os.ReadFile(paths.BooksJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bookData), `“Editor’s Archive”`) ||
		!strings.Contains(string(bookData), `O’Brien`) ||
		!strings.Contains(string(bookData), `"id": "archive-copy"`) {
		t.Fatalf("books were not migrated safely:\n%s", bookData)
	}
	configData, err := os.ReadFile(paths.ConfigJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configData), `Reader’s Shelf`) ||
		!strings.Contains(string(configData), `“Books” for everyone`) ||
		!strings.Contains(string(configData), `[Editor’s site](https://example.com/o'brien) and `+"`don't`") {
		t.Fatalf("config was not migrated safely:\n%s", configData)
	}
}
