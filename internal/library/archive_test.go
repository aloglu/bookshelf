package library

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	var checkProgress []ArchiveProgress
	prepared, err := PrepareArchiveWithProgress(context.Background(), archive, func(progress ArchiveProgress) {
		checkProgress = append(checkProgress, progress)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	assertFinalArchiveProgress(t, checkProgress, "Checking archive", 5, "files")
	info, err := prepared.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Books != 1 || info.Covers != 1 || info.ManualCovers != 1 || info.SiteTitle != "Archive Library" {
		t.Fatalf("archive info = %#v", info)
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
	var importProgress []ArchiveProgress
	imported, err := ImportPreparedArchive(
		context.Background(),
		destination,
		prepared,
		ArchiveImportOptions{
			Mode: ArchiveReplace,
			Progress: func(progress ArchiveProgress) {
				importProgress = append(importProgress, progress)
			},
		},
	)
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
	assertFinalArchiveProgress(t, importProgress, "Restoring library", 4, "files")
	assertFinalArchiveProgress(t, importProgress, "Rebuilding website", 1, "books")
}

func TestArchiveV2PreservesWebsiteVisibilityAndRejectsV1(t *testing.T) {
	source := fixture(t)
	visible := Normalize(Book{ID: "visible", Title: "Visible Book"})
	hidden := Normalize(Book{ID: "hidden", Title: "Hidden Book", WebsiteVisibility: WebsiteHidden})
	if err := Save(source, []Book{visible, hidden}); err != nil {
		t.Fatal(err)
	}
	v2 := filepath.Join(t.TempDir(), "library-v2.bookshelf")
	writeTestArchive(t, v2, source)
	if version := archiveManifestVersion(t, v2); version != archiveVersion {
		t.Fatalf("archive version = %d, want %d", version, archiveVersion)
	}

	v2Destination := fixture(t)
	if _, err := ImportArchive(
		context.Background(), v2Destination, v2, ArchiveImportOptions{Mode: ArchiveReplace},
	); err != nil {
		t.Fatal(err)
	}
	v2Books, err := Load(v2Destination)
	if err != nil {
		t.Fatal(err)
	}
	if v2Books[1].WebsiteVisibility != WebsiteHidden {
		t.Fatalf("v2 hidden visibility = %q", v2Books[1].WebsiteVisibility)
	}

	v1 := filepath.Join(t.TempDir(), "library-v1.bookshelf")
	rewriteArchiveVersion(t, v2, v1, 1)
	v1Destination := fixture(t)
	if _, err := ImportArchive(
		context.Background(), v1Destination, v1, ArchiveImportOptions{Mode: ArchiveReplace},
	); err == nil || !strings.Contains(err.Error(), "unsupported Bookshelf archive format or version") {
		t.Fatalf("v1 import error = %v", err)
	}
}

func TestClosedPreparedArchiveCannotBeImported(t *testing.T) {
	source := fixture(t)
	archive := filepath.Join(t.TempDir(), "library.bookshelf")
	writeTestArchive(t, archive, source)
	prepared, err := PrepareArchive(context.Background(), archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Info(); err == nil {
		t.Fatal("closed archive preparation still returned information")
	}
	if _, err := ImportPreparedArchive(
		context.Background(),
		fixture(t),
		prepared,
		ArchiveImportOptions{Mode: ArchiveReplace},
	); err == nil {
		t.Fatal("closed archive preparation was imported")
	}
}

func TestEncodeArchiveReportsWrittenFiles(t *testing.T) {
	paths := fixture(t)
	var output bytes.Buffer
	var events []ArchiveProgress
	if _, err := EncodeArchiveWithProgress(&output, paths, nil, func(progress ArchiveProgress) {
		events = append(events, progress)
	}); err != nil {
		t.Fatal(err)
	}
	assertFinalArchiveProgress(t, events, "Creating safety backup", 3, "files")
}

func TestArchiveCancellationDuringWebsiteBuildLeavesLibraryUnchanged(t *testing.T) {
	source := fixture(t)
	if err := Save(source, []Book{Normalize(Book{Title: "Imported Book"})}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "library.bookshelf")
	writeTestArchive(t, archive, source)
	prepared, err := PrepareArchive(context.Background(), archive)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()

	destination := fixture(t)
	oldBook := Normalize(Book{Title: "Existing Book"})
	if err := Save(destination, []Book{oldBook}); err != nil {
		t.Fatal(err)
	}
	if err := SaveGenerated(destination, []Book{oldBook}); err != nil {
		t.Fatal(err)
	}
	oldWebsite, err := os.ReadFile(destination.BooksJS)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, err = ImportPreparedArchive(ctx, destination, prepared, ArchiveImportOptions{
		Mode: ArchiveReplace,
		Progress: func(progress ArchiveProgress) {
			if progress.Phase == "Rebuilding website" {
				cancel()
			}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled import error = %v", err)
	}
	books, err := Load(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != oldBook.Title {
		t.Fatalf("cancelled import changed books: %#v", books)
	}
	website, err := os.ReadFile(destination.BooksJS)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(website, oldWebsite) {
		t.Fatal("cancelled import changed the published website")
	}
}

func TestArchiveExportRejectsImagesThatCannotBeImported(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		paths := fixture(t)
		name := filepath.Join(paths.ManualCoversDir, "oversized.png")
		file, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(archiveMaxImage + 1); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if _, err := EncodeArchive(&output, paths, nil); err == nil ||
			!strings.Contains(err.Error(), "50 MiB") {
			t.Fatalf("oversized archive export error = %v", err)
		}
	})

	t.Run("dimensions", func(t *testing.T) {
		paths := fixture(t)
		name := filepath.Join(paths.ManualCoversDir, "too-wide.png")
		output, err := os.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		encodeErr := png.Encode(output, image.NewRGBA(image.Rect(0, 0, maxCoverDimension+1, 1)))
		closeErr := output.Close()
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		var archive bytes.Buffer
		if _, err := EncodeArchive(&archive, paths, nil); err == nil ||
			!strings.Contains(err.Error(), "exceed the safety limit") {
			t.Fatalf("oversized-dimension archive export error = %v", err)
		}
	})

	t.Run("invalid image", func(t *testing.T) {
		paths := fixture(t)
		name := filepath.Join(paths.ManualCoversDir, "invalid.jpg")
		if err := os.WriteFile(name, []byte("not an image"), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if _, err := EncodeArchive(&output, paths, nil); err == nil ||
			!strings.Contains(err.Error(), "validate") {
			t.Fatalf("invalid-image archive export error = %v", err)
		}
	})
}

func TestArchiveReplacementCreatesBackupWhileHoldingLibraryLock(t *testing.T) {
	source := fixture(t)
	replacement := Normalize(Book{Title: "Replacement"})
	if err := Save(source, []Book{replacement}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "replacement.bookshelf")
	writeTestArchive(t, archive, source)

	destination := fixture(t)
	current := Normalize(Book{Title: "Current"})
	if err := Save(destination, []Book{current}); err != nil {
		t.Fatal(err)
	}

	lockAttempted := make(chan struct{})
	lockResult := make(chan error, 1)
	result, err := ImportArchive(context.Background(), destination, archive, ArchiveImportOptions{
		Mode: ArchiveReplace,
		BeforeReplace: func(books []Book) (string, error) {
			if len(books) != 1 || books[0].ID != current.ID {
				return "", fmt.Errorf("backup snapshot = %#v", books)
			}
			go func() {
				close(lockAttempted)
				unlock, lockErr := acquireLibraryLock(context.Background(), destination)
				if lockErr == nil {
					unlock()
				}
				lockResult <- lockErr
			}()
			<-lockAttempted
			select {
			case lockErr := <-lockResult:
				return "", fmt.Errorf("replacement backup did not hold the library lock: %v", lockErr)
			case <-time.After(75 * time.Millisecond):
			}
			return "locked-safety-backup.bookshelf", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.SafetyBackup != "locked-safety-backup.bookshelf" {
		t.Fatalf("safety backup = %q", result.SafetyBackup)
	}
	select {
	case lockErr := <-lockResult:
		if lockErr != nil {
			t.Fatal(lockErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("library lock was not released after archive replacement")
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

func archiveManifestVersion(t *testing.T, filename string) int {
	t.Helper()
	reader, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Name != "manifest.json" {
			continue
		}
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		var manifest archiveManifest
		decodeErr := json.NewDecoder(input).Decode(&manifest)
		closeErr := input.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		return manifest.Version
	}
	t.Fatal("archive is missing manifest.json")
	return 0
}

func rewriteArchiveVersion(t *testing.T, source, destination string, version int) {
	t.Helper()
	reader, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	output, err := os.Create(destination)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(output)
	for _, entry := range reader.File {
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(input)
		closeErr := input.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if entry.Name == "manifest.json" {
			var manifest archiveManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.Version = version
			data, err = json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
		}
		target, err := writer.CreateHeader(&entry.FileHeader)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := target.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertFinalArchiveProgress(t *testing.T, events []ArchiveProgress, phase string, total int, unit string) {
	t.Helper()
	var found *ArchiveProgress
	for index := range events {
		if events[index].Phase == phase {
			found = &events[index]
		}
	}
	if found == nil {
		t.Fatalf("progress never entered %q: %#v", phase, events)
	}
	if found.Current != total || found.Total != total || found.Unit != unit {
		t.Fatalf("final %s progress = %#v, want %d / %d %s", phase, *found, total, total, unit)
	}
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
