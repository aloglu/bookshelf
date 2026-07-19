package library

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	archiveFormat          = "bookshelf"
	archiveVersion         = 1
	archiveMaxMetadata     = 64 << 20
	archiveMaxImage        = 50 << 20
	archiveMaxUncompressed = 8 << 30
)

type ArchiveImportMode string

const (
	ArchiveMerge   ArchiveImportMode = "merge"
	ArchiveReplace ArchiveImportMode = "replace"
)

type ArchiveExportResult struct {
	Books        int
	Covers       int
	ManualCovers int
}

type ArchiveInfo struct {
	Books        int
	Covers       int
	ManualCovers int
	SiteTitle    string
}

type ArchiveImportOptions struct {
	Mode           ArchiveImportMode
	SkipDuplicates bool
	DryRun         bool
	BeforeReplace  func([]Book) (string, error)
}

type ArchiveImportResult struct {
	Imported     int
	Skipped      int
	Covers       int
	ManualCovers int
	Replaced     bool
	SafetyBackup string
}

type archiveManifest struct {
	Format   string `json:"format"`
	Version  int    `json:"version"`
	Books    string `json:"books"`
	Settings string `json:"settings"`
}

type extractedArchive struct {
	root         string
	books        []Book
	config       Config
	covers       map[string]string
	manualCovers map[string]string
}

// InspectArchive fully validates an archive before the caller asks the user
// whether it should be merged or used as a replacement.
func InspectArchive(filename string) (ArchiveInfo, error) {
	archive, err := extractArchive(filename)
	if err != nil {
		return ArchiveInfo{}, err
	}
	defer os.RemoveAll(archive.root)
	return ArchiveInfo{
		Books:        len(archive.books),
		Covers:       len(archive.covers),
		ManualCovers: len(archive.manualCovers),
		SiteTitle:    archive.config.SiteTitle,
	}, nil
}

func EncodeArchive(writer io.Writer, paths Paths, books []Book) (ArchiveExportResult, error) {
	var result ArchiveExportResult
	config, err := LoadConfig(paths)
	if err != nil {
		return result, err
	}
	manifest := archiveManifest{
		Format:   archiveFormat,
		Version:  archiveVersion,
		Books:    "books.json",
		Settings: "settings.json",
	}
	manifestJSON, err := encodeArchiveJSON(manifest, archiveMaxMetadata)
	if err != nil {
		return result, fmt.Errorf("encode archive manifest: %w", err)
	}
	booksJSON, err := encodeArchiveJSON(exportBooks(books), archiveMaxMetadata)
	if err != nil {
		return result, fmt.Errorf("encode archive books: %w", err)
	}
	settingsJSON, err := encodeArchiveJSON(config, archiveMaxMetadata)
	if err != nil {
		return result, fmt.Errorf("encode archive settings: %w", err)
	}

	type archiveImage struct {
		name   string
		source string
	}
	images := make([]archiveImage, 0)
	totalSize := uint64(len(manifestJSON) + len(booksJSON) + len(settingsJSON))
	addImage := func(name, source string, requireJPEG bool) error {
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if info.Size() > archiveMaxImage {
			return fmt.Errorf("%s exceeds the %d MiB archive image limit", source, archiveMaxImage>>20)
		}
		if err := validateArchiveImage(source, requireJPEG); err != nil {
			return fmt.Errorf("validate %s for archive export: %w", source, err)
		}
		size := uint64(info.Size())
		if totalSize > archiveMaxUncompressed-size {
			return fmt.Errorf("archive expands beyond the %d GiB safety limit", archiveMaxUncompressed>>30)
		}
		totalSize += size
		images = append(images, archiveImage{name: name, source: source})
		return nil
	}

	coverNames := make(map[string]bool)
	for _, book := range books {
		if filename := coverFilename(book); filename != "" && fileExists(filepath.Join(paths.CoversDir, filename)) {
			coverNames[filename] = true
		}
	}
	for _, filename := range sortedNames(coverNames) {
		if err := addImage("covers/"+filename, filepath.Join(paths.CoversDir, filename), true); err != nil {
			return result, err
		}
		result.Covers++
	}
	manualNames, err := regularFileNames(paths.ManualCoversDir)
	if err != nil {
		return result, err
	}
	for _, filename := range manualNames {
		if !validManualCoverName(filename) {
			continue
		}
		if err := addImage("manual-covers/"+filename, filepath.Join(paths.ManualCoversDir, filename), false); err != nil {
			return result, err
		}
		result.ManualCovers++
	}

	zipWriter := zip.NewWriter(writer)
	closeWithError := func(inputErr error) (ArchiveExportResult, error) {
		closeErr := zipWriter.Close()
		if inputErr != nil {
			return result, inputErr
		}
		return result, closeErr
	}
	if err := writeArchiveBytes(zipWriter, "manifest.json", manifestJSON); err != nil {
		return closeWithError(err)
	}
	if err := writeArchiveBytes(zipWriter, manifest.Books, booksJSON); err != nil {
		return closeWithError(err)
	}
	if err := writeArchiveBytes(zipWriter, manifest.Settings, settingsJSON); err != nil {
		return closeWithError(err)
	}
	for _, image := range images {
		if err := writeArchiveFile(zipWriter, image.name, image.source); err != nil {
			return closeWithError(err)
		}
	}
	result.Books = len(books)
	return closeWithError(nil)
}

func ImportArchive(ctx context.Context, paths Paths, filename string, options ArchiveImportOptions) (ArchiveImportResult, error) {
	var result ArchiveImportResult
	unlock, err := acquireLibraryLock(ctx, paths)
	if err != nil {
		return result, err
	}
	defer unlock()
	if options.Mode != ArchiveMerge && options.Mode != ArchiveReplace {
		return result, fmt.Errorf("archive import mode must be merge or replace")
	}
	archive, err := extractArchive(filename)
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(archive.root)

	existing, err := Load(paths)
	if err != nil {
		return result, err
	}
	finalBooks := archive.books
	importedBooks := archive.books
	config := archive.config
	if options.Mode == ArchiveMerge {
		config, err = LoadConfig(paths)
		if err != nil {
			return result, err
		}
		finalBooks, importedBooks, result.Skipped, err = mergeArchiveBooks(existing, archive.books, options.SkipDuplicates)
		if err != nil {
			return result, err
		}
	}
	if problems := Validate(finalBooks); len(problems) > 0 {
		return result, validationError(problems)
	}
	result.Imported = len(importedBooks)
	result.Replaced = options.Mode == ArchiveReplace
	if options.DryRun {
		result.Covers, result.ManualCovers = archiveImageCounts(archive, importedBooks, options.Mode)
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if options.Mode == ArchiveReplace && len(existing) > 0 && options.BeforeReplace != nil {
		result.SafetyBackup, err = options.BeforeReplace(existing)
		if err != nil {
			return result, fmt.Errorf("create safety backup before replacement: %w", err)
		}
	}

	stageRoot, err := os.MkdirTemp(filepath.Dir(paths.DataDir), ".bookshelf-import-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stageRoot)
	stagePaths := NewPaths(stageRoot)
	if err := Initialize(stagePaths); err != nil {
		return result, err
	}
	if options.Mode == ArchiveMerge {
		if err := copyDirectoryFiles(paths.CoversDir, stagePaths.CoversDir); err != nil {
			return result, err
		}
		if err := copyDirectoryFiles(paths.ManualCoversDir, stagePaths.ManualCoversDir); err != nil {
			return result, err
		}
		if fileExists(paths.CoverReportJSON) {
			if err := copyFile(paths.CoverReportJSON, stagePaths.CoverReportJSON); err != nil {
				return result, err
			}
		}
	}
	result.Covers, result.ManualCovers, err = copyArchiveImages(ctx, archive, stagePaths, importedBooks, options.Mode)
	if err != nil {
		return result, err
	}
	if err := Save(stagePaths, finalBooks); err != nil {
		return result, err
	}
	if err := SaveConfig(stagePaths, config); err != nil {
		return result, err
	}
	if err := replaceDirectory(paths.DataDir, stagePaths.DataDir); err != nil {
		return result, err
	}
	loaded, err := Load(paths)
	if err != nil {
		return result, err
	}
	if err := SaveGenerated(paths, loaded); err != nil {
		return result, fmt.Errorf("archive data imported but published website rebuild failed: %w", err)
	}
	return result, nil
}

func encodeArchiveJSON(value any, limit int64) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "    ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	if int64(output.Len()) > limit {
		return nil, fmt.Errorf("metadata exceeds the %d MiB safety limit", limit>>20)
	}
	return output.Bytes(), nil
}

func writeArchiveBytes(zipWriter *zip.Writer, name string, value []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o644)
	output, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = output.Write(value)
	return err
}

func writeArchiveFile(zipWriter *zip.Writer, name, source string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetMode(0o644)
	output, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return err
}

func extractArchive(filename string) (*extractedArchive, error) {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, fmt.Errorf("open Bookshelf archive: %w", err)
	}
	defer reader.Close()
	entries := make(map[string]*zip.File, len(reader.File))
	var total uint64
	for _, entry := range reader.File {
		name := entry.Name
		if !validArchiveEntry(name, entry.FileInfo().IsDir()) {
			return nil, fmt.Errorf("archive contains an unsupported or unsafe path %q", name)
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("archive contains a symbolic link %q", name)
		}
		if _, exists := entries[name]; exists {
			return nil, fmt.Errorf("archive contains duplicate entry %q", name)
		}
		entries[name] = entry
		total += entry.UncompressedSize64
		if total > archiveMaxUncompressed {
			return nil, fmt.Errorf("archive expands beyond the %d GiB safety limit", archiveMaxUncompressed>>30)
		}
	}
	manifestEntry := entries["manifest.json"]
	if manifestEntry == nil {
		return nil, fmt.Errorf("archive is missing manifest.json")
	}
	var manifest archiveManifest
	if err := decodeArchiveJSON(manifestEntry, archiveMaxMetadata, &manifest); err != nil {
		return nil, fmt.Errorf("read archive manifest: %w", err)
	}
	if manifest.Format != archiveFormat || manifest.Version != archiveVersion ||
		manifest.Books != "books.json" || manifest.Settings != "settings.json" {
		return nil, fmt.Errorf("unsupported Bookshelf archive format or version")
	}
	var books []Book
	if err := decodeArchiveJSON(entries[manifest.Books], archiveMaxMetadata, &books); err != nil {
		return nil, fmt.Errorf("read archive books: %w", err)
	}
	for index := range books {
		books[index] = Normalize(books[index])
		books[index].Cover = ""
		books[index].Thumbnail = ""
		books[index].TitleSlug = ""
		books[index].Permalink = ""
		if books[index].CoverFile != "" && coverFilename(books[index]) == "" {
			return nil, fmt.Errorf("archive book %q has an invalid cover filename", books[index].Title)
		}
	}
	AssignTitleSlugs(books)
	if problems := Validate(books); len(problems) > 0 {
		return nil, validationError(problems)
	}
	config := DefaultConfig()
	if err := decodeArchiveJSON(entries[manifest.Settings], archiveMaxMetadata, &config); err != nil {
		return nil, fmt.Errorf("read archive settings: %w", err)
	}
	config = NormalizeConfig(config)
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("validate archive settings: %w", err)
	}

	root, err := os.MkdirTemp("", ".bookshelf-archive-")
	if err != nil {
		return nil, err
	}
	cleanup := func(inputErr error) (*extractedArchive, error) {
		os.RemoveAll(root)
		return nil, inputErr
	}
	extracted := &extractedArchive{
		root:         root,
		books:        books,
		config:       config,
		covers:       make(map[string]string),
		manualCovers: make(map[string]string),
	}
	for name, entry := range entries {
		if entry.FileInfo().IsDir() || name == "manifest.json" || name == manifest.Books || name == manifest.Settings {
			continue
		}
		if entry.UncompressedSize64 > archiveMaxImage {
			return cleanup(fmt.Errorf("archive image %q exceeds the %d MiB safety limit", name, archiveMaxImage>>20))
		}
		destination := filepath.Join(root, filepath.FromSlash(name))
		if err := extractArchiveFile(entry, destination); err != nil {
			return cleanup(err)
		}
		if err := validateArchiveImage(destination, strings.HasPrefix(name, "covers/")); err != nil {
			return cleanup(fmt.Errorf("validate %q: %w", name, err))
		}
		base := path.Base(name)
		if strings.HasPrefix(name, "covers/") {
			extracted.covers[base] = destination
		} else {
			extracted.manualCovers[base] = destination
		}
	}
	for _, book := range books {
		if filename := coverFilename(book); filename != "" {
			if extracted.covers[filename] == "" {
				return cleanup(fmt.Errorf("archive is missing cover image %q for %q", filename, book.Title))
			}
		}
	}
	return extracted, nil
}

func decodeArchiveJSON(entry *zip.File, limit int64, destination any) error {
	if entry == nil {
		return fmt.Errorf("required file is missing")
	}
	if entry.UncompressedSize64 > uint64(limit) {
		return fmt.Errorf("%s exceeds the metadata safety limit", entry.Name)
	}
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	decoder := json.NewDecoder(io.LimitReader(input, limit+1))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("unexpected trailing data")
	}
	return nil
}

func validArchiveEntry(name string, directory bool) bool {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") {
		if !(directory && path.Clean(name)+"/" == name) {
			return false
		}
	}
	if directory {
		return name == "covers/" || name == "manual-covers/"
	}
	if name == "manifest.json" || name == "books.json" || name == "settings.json" {
		return true
	}
	parent, base := path.Split(name)
	if base == "" || strings.Contains(base, "/") {
		return false
	}
	switch parent {
	case "covers/":
		return strings.EqualFold(path.Ext(base), ".jpg")
	case "manual-covers/":
		return validManualCoverName(base)
	default:
		return false
	}
}

func extractArchiveFile(entry *zip.File, destination string) error {
	input, err := entry.Open()
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, io.LimitReader(input, archiveMaxImage+1))
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func validateArchiveImage(filename string, requireJPEG bool) error {
	input, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer input.Close()
	config, format, err := image.DecodeConfig(input)
	if err != nil {
		return err
	}
	if err := validateCoverDimensions(config); err != nil {
		return err
	}
	if requireJPEG && format != "jpeg" {
		return fmt.Errorf("stored covers must be JPEG images")
	}
	return nil
}

func mergeArchiveBooks(existing, candidates []Book, skipDuplicates bool) ([]Book, []Book, int, error) {
	ids := make(map[string]bool, len(existing)+len(candidates))
	isbns := make(map[string]bool, len(existing)+len(candidates))
	for _, book := range existing {
		ids[book.ID] = true
		if isbn := CleanISBN(book.ISBN); isbn != "" {
			isbns[isbn] = true
		}
	}
	imported := make([]Book, 0, len(candidates))
	skipped := 0
	for _, book := range candidates {
		isbn := CleanISBN(book.ISBN)
		if ids[book.ID] || (isbn != "" && isbns[isbn]) {
			if skipDuplicates {
				skipped++
				continue
			}
			return nil, nil, skipped, fmt.Errorf("archive book %q duplicates an existing ID or ISBN", book.Title)
		}
		ids[book.ID] = true
		if isbn != "" {
			isbns[isbn] = true
		}
		imported = append(imported, book)
	}
	combined := append(append([]Book(nil), existing...), imported...)
	return combined, imported, skipped, nil
}

func copyArchiveImages(ctx context.Context, archive *extractedArchive, paths Paths, books []Book, mode ArchiveImportMode) (int, int, error) {
	coverCount := 0
	manualCount := 0
	for _, book := range books {
		if err := ctx.Err(); err != nil {
			return coverCount, manualCount, err
		}
		filename := coverFilename(book)
		if filename == "" {
			continue
		}
		if err := copyWithoutConflict(archive.covers[filename], filepath.Join(paths.CoversDir, filename)); err != nil {
			return coverCount, manualCount, err
		}
		coverCount++
	}
	for filename, source := range archive.manualCovers {
		if mode == ArchiveMerge && !manualCoverMatchesAny(filename, books) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return coverCount, manualCount, err
		}
		if err := copyWithoutConflict(source, filepath.Join(paths.ManualCoversDir, filename)); err != nil {
			return coverCount, manualCount, err
		}
		manualCount++
	}
	return coverCount, manualCount, nil
}

func archiveImageCounts(archive *extractedArchive, books []Book, mode ArchiveImportMode) (int, int) {
	covers := 0
	for _, book := range books {
		if filename := coverFilename(book); filename != "" && archive.covers[filename] != "" {
			covers++
		}
	}
	manuals := len(archive.manualCovers)
	if mode == ArchiveMerge {
		manuals = 0
		for filename := range archive.manualCovers {
			if manualCoverMatchesAny(filename, books) {
				manuals++
			}
		}
	}
	return covers, manuals
}

func manualCoverMatchesAny(filename string, books []Book) bool {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	for _, book := range books {
		for _, candidate := range []string{formattedISBNToken(book.ISBN), CleanISBN(book.ISBN), safeToken(book.ID)} {
			if candidate != "" && strings.EqualFold(stem, candidate) {
				return true
			}
		}
	}
	return false
}

func copyWithoutConflict(source, destination string) error {
	if source == "" {
		return fmt.Errorf("archive image source is missing")
	}
	if fileExists(destination) {
		same, err := sameFileContents(source, destination)
		if err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("image %q conflicts with an existing file", filepath.Base(destination))
		}
		return nil
	}
	return copyFile(source, destination)
}

func sameFileContents(first, second string) (bool, error) {
	firstHash, firstSize, err := fileDigest(first)
	if err != nil {
		return false, err
	}
	secondHash, secondSize, err := fileDigest(second)
	if err != nil {
		return false, err
	}
	return firstSize == secondSize && firstHash == secondHash, nil
}

func fileDigest(filename string) ([sha256.Size]byte, int64, error) {
	var digest [sha256.Size]byte
	input, err := os.Open(filename)
	if err != nil {
		return digest, 0, err
	}
	defer input.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, input)
	if err != nil {
		return digest, 0, err
	}
	copy(digest[:], hash.Sum(nil))
	return digest, size, nil
}

func copyDirectoryFiles(source, destination string) error {
	names, err := regularFileNames(source)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := copyFile(filepath.Join(source, name), filepath.Join(destination, name)); err != nil {
			return err
		}
	}
	return nil
}

func regularFileNames(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func validManualCoverName(filename string) bool {
	if filepath.Base(filename) != filename {
		return false
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".bmp":
		return true
	default:
		return false
	}
}

func sortedNames(values map[string]bool) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
