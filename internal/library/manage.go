package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ChangeOptions struct {
	Build bool
}

func Add(ctx context.Context, paths Paths, book Book, options ChangeOptions) (Book, BuildStats, error) {
	unlock, err := acquireLibraryLock(ctx, paths)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	defer unlock()
	books, err := Load(paths)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	book = Normalize(book)
	if book.Title == "" {
		return Book{}, BuildStats{}, fmt.Errorf("title is required")
	}
	if FindIndex(books, book.ID) >= 0 {
		return Book{}, BuildStats{}, fmt.Errorf("a book with id %q already exists", book.ID)
	}
	if isbn := CleanISBN(book.ISBN); isbn != "" {
		for _, existing := range books {
			if CleanISBN(existing.ISBN) == isbn {
				return Book{}, BuildStats{}, fmt.Errorf("a book with ISBN %q already exists", isbn)
			}
		}
	}
	if err := ensureUniqueSlug(books, book.Slug, -1); err != nil {
		return Book{}, BuildStats{}, err
	}
	books = append(books, book)
	if problems := Validate(books); len(problems) > 0 {
		return Book{}, BuildStats{}, validationError(problems)
	}
	if err := Save(paths, books); err != nil {
		return Book{}, BuildStats{}, err
	}
	if !options.Build {
		return book, BuildStats{Books: len(books)}, nil
	}
	unlock()
	stats, err := Build(ctx, paths, BuildOptions{
		ProcessOnly: map[string]bool{book.Key(): true},
	})
	return book, stats, savedButUnpublished("book", err)
}

func Update(ctx context.Context, paths Paths, id string, updates BookPatch, options ChangeOptions) (Book, BuildStats, error) {
	unlock, err := acquireLibraryLock(ctx, paths)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	defer unlock()
	books, err := Load(paths)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	index := FindIndex(books, id)
	if index < 0 {
		return Book{}, BuildStats{}, fmt.Errorf("no book found for %q", id)
	}
	current := books[index]
	if updates.Title != nil {
		current.Title = *updates.Title
	}
	if updates.Author != nil {
		current.Author = *updates.Author
	}
	if updates.ISBN != nil {
		current.ISBN = *updates.ISBN
	}
	if updates.Slug != nil {
		current.Slug = *updates.Slug
	}
	if updates.Translator != nil {
		current.Translator = *updates.Translator
	}
	if updates.Publisher != nil {
		current.Publisher = *updates.Publisher
	}
	if updates.Binding != nil {
		current.Binding = *updates.Binding
	}
	if updates.Published != nil {
		current.Published, err = ParseYearInput(*updates.Published)
		if err != nil {
			return Book{}, BuildStats{}, err
		}
	}
	current = Normalize(current)
	if current.Title == "" {
		return Book{}, BuildStats{}, fmt.Errorf("title is required")
	}
	if err := ensureUniqueSlug(books, current.Slug, index); err != nil {
		return Book{}, BuildStats{}, err
	}
	for i, existing := range books {
		if i == index {
			continue
		}
		if existing.ID == current.ID {
			return Book{}, BuildStats{}, fmt.Errorf("a book with id %q already exists", current.ID)
		}
		if isbn := CleanISBN(current.ISBN); isbn != "" && CleanISBN(existing.ISBN) == isbn {
			return Book{}, BuildStats{}, fmt.Errorf("a book with ISBN %q already exists", isbn)
		}
	}
	original := books[index]
	books[index] = current
	if problems := Validate(books); len(problems) > 0 {
		return Book{}, BuildStats{}, validationError(problems)
	}
	rollbackCover, err := renameCoverForBook(paths, original, &current)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	if err := Save(paths, books); err != nil {
		rollbackCover()
		return Book{}, BuildStats{}, err
	}
	if !options.Build {
		return current, BuildStats{Books: len(books)}, nil
	}
	unlock()
	stats, err := Build(ctx, paths, BuildOptions{
		ProcessOnly: map[string]bool{current.Key(): true},
	})
	return current, stats, savedButUnpublished("book", err)
}

func ensureUniqueSlug(books []Book, slug string, except int) error {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return nil
	}
	for index, existing := range books {
		if index != except && strings.EqualFold(existing.Slug, slug) {
			return fmt.Errorf("a book with URL slug %q already exists", slug)
		}
	}
	return nil
}

func Replace(ctx context.Context, paths Paths, id string, replacement Book, options ChangeOptions) (Book, BuildStats, error) {
	unlock, err := acquireLibraryLock(ctx, paths)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	defer unlock()
	books, err := Load(paths)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	index := FindIndex(books, id)
	if index < 0 {
		return Book{}, BuildStats{}, fmt.Errorf("no book found for %q", id)
	}
	replacement.ID = books[index].ID
	replacement.CoverFile = books[index].CoverFile
	replacement.Cover = books[index].Cover
	replacement.SpineColor = books[index].SpineColor
	replacement.SpineTextColor = books[index].SpineTextColor
	replacement = Normalize(replacement)
	if replacement.Title == "" {
		return Book{}, BuildStats{}, fmt.Errorf("title is required")
	}
	if err := ensureUniqueSlug(books, replacement.Slug, index); err != nil {
		return Book{}, BuildStats{}, err
	}
	for i, existing := range books {
		if i == index {
			continue
		}
		if existing.ID == replacement.ID {
			return Book{}, BuildStats{}, fmt.Errorf("a book with id %q already exists", replacement.ID)
		}
		if isbn := CleanISBN(replacement.ISBN); isbn != "" && CleanISBN(existing.ISBN) == isbn {
			return Book{}, BuildStats{}, fmt.Errorf("a book with ISBN %q already exists", isbn)
		}
	}
	original := books[index]
	books[index] = replacement
	if problems := Validate(books); len(problems) > 0 {
		return Book{}, BuildStats{}, validationError(problems)
	}
	rollbackCover, err := renameCoverForBook(paths, original, &replacement)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	if err := Save(paths, books); err != nil {
		rollbackCover()
		return Book{}, BuildStats{}, err
	}
	if !options.Build {
		return replacement, BuildStats{Books: len(books)}, nil
	}
	unlock()
	stats, err := Build(ctx, paths, BuildOptions{
		ProcessOnly: map[string]bool{replacement.Key(): true},
	})
	return replacement, stats, savedButUnpublished("book", err)
}

func Remove(ctx context.Context, paths Paths, ids []string, removeCovers bool) ([]Book, error) {
	unlock, err := acquireLibraryLock(ctx, paths)
	if err != nil {
		return nil, err
	}
	defer unlock()
	books, err := Load(paths)
	if err != nil {
		return nil, err
	}
	removeIndexes := make(map[int]bool)
	for _, id := range ids {
		index := FindIndex(books, id)
		if index < 0 {
			return nil, fmt.Errorf("no book found for %q", id)
		}
		removeIndexes[index] = true
	}
	removed := make([]Book, 0, len(removeIndexes))
	remaining := make([]Book, 0, len(books)-len(removeIndexes))
	for i, book := range books {
		if removeIndexes[i] {
			removed = append(removed, book)
			continue
		}
		remaining = append(remaining, book)
	}
	stage, err := os.MkdirTemp(paths.DataDir, ".remove-covers-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	type stagedCover struct {
		original string
		staged   string
	}
	var staged []stagedCover
	restoreCovers := func() error {
		var restoreErr error
		for index := len(staged) - 1; index >= 0; index-- {
			if err := os.Rename(staged[index].staged, staged[index].original); err != nil && !errors.Is(err, os.ErrNotExist) {
				restoreErr = errors.Join(restoreErr, err)
			}
		}
		return restoreErr
	}
	if removeCovers {
		keptCoverFiles := make(map[string]bool, len(remaining))
		for _, book := range remaining {
			if filename := coverFilename(book); filename != "" {
				keptCoverFiles[filename] = true
			}
		}
		moved := make(map[string]bool)
		for _, book := range removed {
			filename := coverFilename(book)
			if filename == "" || keptCoverFiles[filename] || moved[filename] {
				continue
			}
			original := filepath.Join(paths.CoversDir, filename)
			if !fileExists(original) {
				continue
			}
			stagedName := filepath.Join(stage, filename)
			if err := os.Rename(original, stagedName); err != nil {
				return nil, errors.Join(err, restoreCovers())
			}
			staged = append(staged, stagedCover{original: original, staged: stagedName})
			moved[filename] = true
		}
	}
	if err := Save(paths, remaining); err != nil {
		return nil, errors.Join(err, restoreCovers())
	}
	if err := SaveGenerated(paths, remaining); err != nil {
		restoreErr := restoreCovers()
		saveErr := Save(paths, books)
		publishErr := SaveGenerated(paths, books)
		return nil, errors.Join(err, restoreErr, saveErr, publishErr)
	}
	return removed, nil
}

func renameCoverForBook(paths Paths, previous Book, current *Book) (func(), error) {
	oldFilename := coverFilename(previous)
	if oldFilename == "" {
		return func() {}, nil
	}
	newFilename := preferredCoverFilename(*current)
	if newFilename == oldFilename {
		current.CoverFile = oldFilename
		return func() {}, nil
	}
	source := filepath.Join(paths.CoversDir, oldFilename)
	if !fileExists(source) {
		current.CoverFile = ""
		current.Cover = ""
		current.SpineColor = ""
		current.SpineTextColor = ""
		return func() {}, nil
	}
	destination := filepath.Join(paths.CoversDir, newFilename)
	if fileExists(destination) {
		return nil, fmt.Errorf("cannot rename cover to %q because that file already exists", newFilename)
	}
	if err := os.Rename(source, destination); err != nil {
		return nil, fmt.Errorf("rename cover for %q: %w", current.Title, err)
	}
	current.CoverFile = newFilename
	current.Cover = filepath.ToSlash(filepath.Join("data", "covers", newFilename))
	return func() {
		_ = os.Rename(destination, source)
	}, nil
}
