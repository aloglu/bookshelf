package library

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBookshelfArchiveReplaceRoundTripIncludesImagesAndSettings(t *testing.T) {
	source := fixture(t)
	book := Normalize(Book{Title: "Dune", Author: "Frank Herbert", ISBN: "978-0-441-17271-9"})
	book.CoverFile = preferredCoverFilename(book)
	writeTestJPEG(t, filepath.Join(source.CoversDir, book.CoverFile))
	writeTestPNG(t, filepath.Join(source.ManualCoversDir, "978-0-441-17271-9.png"))
	if err := Save(source, []Book{book}); err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.SiteTitle = "Archive Library"
	if err := SaveConfig(source, config); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "library.bookshelf")
	result := writeTestArchive(t, archive, source)
	if result.Books != 1 || result.Covers != 1 || result.ManualCovers != 1 {
		t.Fatalf("export result = %#v", result)
	}
	assertArchiveEntries(t, archive, []string{
		"manifest.json",
		"books.json",
		"settings.json",
		"covers/" + book.CoverFile,
		"manual-covers/978-0-441-17271-9.png",
	})

	destination := fixture(t)
	if err := Save(destination, []Book{Normalize(Book{Title: "Old Book"})}); err != nil {
		t.Fatal(err)
	}
	writeTestJPEG(t, filepath.Join(destination.CoversDir, "obsolete.jpg"))
	imported, err := ImportArchive(context.Background(), destination, archive, ArchiveImportOptions{Mode: ArchiveReplace})
	if err != nil {
		t.Fatal(err)
	}
	if !imported.Replaced || imported.Imported != 1 || imported.Covers != 1 || imported.ManualCovers != 1 {
		t.Fatalf("import result = %#v", imported)
	}
	books, err := Load(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "Dune" || books[0].Cover == "" {
		t.Fatalf("restored books = %#v", books)
	}
	if !fileExists(filepath.Join(destination.CoversDir, book.CoverFile)) ||
		!fileExists(filepath.Join(destination.ManualCoversDir, "978-0-441-17271-9.png")) {
		t.Fatal("archive images were not restored")
	}
	if fileExists(filepath.Join(destination.CoversDir, "obsolete.jpg")) {
		t.Fatal("replace import retained an old cover")
	}
	restoredConfig, err := LoadConfig(destination)
	if err != nil {
		t.Fatal(err)
	}
	if restoredConfig.SiteTitle != "Archive Library" {
		t.Fatalf("restored site title = %q", restoredConfig.SiteTitle)
	}
	if !fileExists(destination.BooksJS) {
		t.Fatal("published website data was not rebuilt")
	}
}

func TestBookshelfArchiveMergeKeepsCurrentSettingsAndCopiesMatchingImages(t *testing.T) {
	source := fixture(t)
	dune := Normalize(Book{Title: "Dune", ISBN: "9780441172719"})
	dune.CoverFile = preferredCoverFilename(dune)
	writeTestJPEG(t, filepath.Join(source.CoversDir, dune.CoverFile))
	writeTestPNG(t, filepath.Join(source.ManualCoversDir, dune.ID+".png"))
	writeTestPNG(t, filepath.Join(source.ManualCoversDir, "unrelated.png"))
	if err := Save(source, []Book{dune}); err != nil {
		t.Fatal(err)
	}
	sourceConfig := DefaultConfig()
	sourceConfig.SiteTitle = "Source"
	if err := SaveConfig(source, sourceConfig); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "merge.bookshelf")
	writeTestArchive(t, archive, source)

	destination := fixture(t)
	foundation := Normalize(Book{Title: "Foundation", ISBN: "9780553293357"})
	foundation.CoverFile = preferredCoverFilename(foundation)
	writeTestJPEG(t, filepath.Join(destination.CoversDir, foundation.CoverFile))
	if err := Save(destination, []Book{foundation}); err != nil {
		t.Fatal(err)
	}
	destinationConfig := DefaultConfig()
	destinationConfig.SiteTitle = "Destination"
	if err := SaveConfig(destination, destinationConfig); err != nil {
		t.Fatal(err)
	}
	result, err := ImportArchive(context.Background(), destination, archive, ArchiveImportOptions{Mode: ArchiveMerge})
	if err != nil {
		t.Fatal(err)
	}
	if result.Replaced || result.Imported != 1 || result.Covers != 1 || result.ManualCovers != 1 {
		t.Fatalf("merge result = %#v", result)
	}
	books, err := Load(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("merged books = %#v", books)
	}
	config, err := LoadConfig(destination)
	if err != nil {
		t.Fatal(err)
	}
	if config.SiteTitle != "Destination" {
		t.Fatalf("merge replaced current settings with %q", config.SiteTitle)
	}
	if !fileExists(filepath.Join(destination.ManualCoversDir, dune.ID+".png")) {
		t.Fatal("matching manual cover was not merged")
	}
	if !fileExists(filepath.Join(destination.CoversDir, foundation.CoverFile)) {
		t.Fatal("merge removed an existing cover")
	}
	if fileExists(filepath.Join(destination.ManualCoversDir, "unrelated.png")) {
		t.Fatal("unrelated manual cover was merged")
	}
}

func TestBookshelfArchiveMergeDuplicatePolicyAndDryRun(t *testing.T) {
	source := fixture(t)
	book := Normalize(Book{Title: "Dune", ISBN: "9780441172719"})
	if err := Save(source, []Book{book}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "duplicate.bookshelf")
	writeTestArchive(t, archive, source)

	destination := fixture(t)
	if err := Save(destination, []Book{book}); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportArchive(context.Background(), destination, archive, ArchiveImportOptions{Mode: ArchiveMerge}); err == nil {
		t.Fatal("duplicate archive merge succeeded without skip policy")
	}
	result, err := ImportArchive(context.Background(), destination, archive, ArchiveImportOptions{
		Mode:           ArchiveMerge,
		SkipDuplicates: true,
		DryRun:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 0 || result.Skipped != 1 {
		t.Fatalf("dry-run result = %#v", result)
	}
	books, err := Load(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("dry run changed destination: %#v", books)
	}
}

func TestBookshelfArchiveRejectsUnsafeZipPaths(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "unsafe.bookshelf")
	output, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(output)
	entry, err := writer.Create("../outside.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("unsafe")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	if _, err := ImportArchive(context.Background(), destination, filename, ArchiveImportOptions{Mode: ArchiveReplace}); err == nil {
		t.Fatal("unsafe archive path was accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(destination.Root), "outside.jpg")); !os.IsNotExist(err) {
		t.Fatal("unsafe archive wrote outside its staging directory")
	}
}

func writeTestArchive(t *testing.T, filename string, paths Paths) ArchiveExportResult {
	t.Helper()
	books, err := Load(paths)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	result, encodeErr := EncodeArchive(output, paths, books)
	closeErr := output.Close()
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	return result
}

func assertArchiveEntries(t *testing.T, filename string, expected []string) {
	t.Helper()
	reader, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	found := make(map[string]bool)
	for _, entry := range reader.File {
		found[entry.Name] = true
	}
	for _, name := range expected {
		if !found[name] {
			t.Fatalf("archive is missing %q", name)
		}
	}
	if len(found) != len(expected) {
		t.Fatalf("archive entries = %#v, expected only %#v", found, expected)
	}
}

func writeTestJPEG(t *testing.T, filename string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 80, G: 40, B: 20, A: 255})
		}
	}
	if err := jpeg.Encode(output, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTestPNG(t *testing.T, filename string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
