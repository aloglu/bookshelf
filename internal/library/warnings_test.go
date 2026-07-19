package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidationWarningsFindStorageAndISBNProblems(t *testing.T) {
	paths := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "9780441172718"})
	book.CoverFile = "missing.jpg"
	if err := Save(paths, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.CoversDir, "orphan.jpg"), []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.ManualCoversDir, "unknown.png"), []byte("unknown"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.ManualCoversDir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	warnings, err := ValidationWarnings(paths, books)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warnings, "\n")
	for _, expected := range []string{"missing cover", "not referenced", "does not match", "unsupported file type", "suspicious ISBN checksum"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("warnings do not contain %q:\n%s", expected, joined)
		}
	}
}

func TestISBNChecksumValidation(t *testing.T) {
	for _, isbn := range []string{"9780441172719", "0441172717"} {
		if !validISBNChecksum(isbn) {
			t.Fatalf("valid ISBN %q was rejected", isbn)
		}
	}
	for _, isbn := range []string{"9780441172718", "0441172718", "123"} {
		if validISBNChecksum(isbn) {
			t.Fatalf("invalid ISBN %q was accepted", isbn)
		}
	}
}
