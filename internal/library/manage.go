package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type ChangeOptions struct {
	FetchCover bool
	Build      bool
}

func Add(ctx context.Context, paths Paths, book Book, options ChangeOptions) (Book, BuildStats, error) {
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
	books = append(books, book)
	if err := Save(paths, books); err != nil {
		return Book{}, BuildStats{}, err
	}
	if !options.Build {
		return book, BuildStats{Books: len(books)}, nil
	}
	stats, err := Build(ctx, paths, BuildOptions{
		FetchCovers: options.FetchCover,
		ProcessOnly: map[string]bool{book.Key(): true},
		FetchOnly:   map[string]bool{book.Key(): true},
	})
	return book, stats, err
}

func Update(ctx context.Context, paths Paths, id string, updates BookPatch, options ChangeOptions) (Book, BuildStats, error) {
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
		current.Published = ParseYear(*updates.Published)
	}
	current = Normalize(current)
	if current.Title == "" {
		return Book{}, BuildStats{}, fmt.Errorf("title is required")
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
	books[index] = current
	if err := Save(paths, books); err != nil {
		return Book{}, BuildStats{}, err
	}
	if !options.Build {
		return current, BuildStats{Books: len(books)}, nil
	}
	stats, err := Build(ctx, paths, BuildOptions{
		FetchCovers: options.FetchCover,
		ProcessOnly: map[string]bool{current.Key(): true},
		FetchOnly:   map[string]bool{current.Key(): true},
	})
	return current, stats, err
}

func Replace(ctx context.Context, paths Paths, id string, replacement Book, options ChangeOptions) (Book, BuildStats, error) {
	books, err := Load(paths)
	if err != nil {
		return Book{}, BuildStats{}, err
	}
	index := FindIndex(books, id)
	if index < 0 {
		return Book{}, BuildStats{}, fmt.Errorf("no book found for %q", id)
	}
	replacement.ID = books[index].ID
	replacement.Cover = books[index].Cover
	replacement.SpineColor = books[index].SpineColor
	replacement.SpineTextColor = books[index].SpineTextColor
	replacement = Normalize(replacement)
	if replacement.Title == "" {
		return Book{}, BuildStats{}, fmt.Errorf("title is required")
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
	books[index] = replacement
	if err := Save(paths, books); err != nil {
		return Book{}, BuildStats{}, err
	}
	if !options.Build {
		return replacement, BuildStats{Books: len(books)}, nil
	}
	stats, err := Build(ctx, paths, BuildOptions{
		FetchCovers: options.FetchCover,
		ProcessOnly: map[string]bool{replacement.Key(): true},
		FetchOnly:   map[string]bool{replacement.Key(): true},
	})
	return replacement, stats, err
}

func Remove(paths Paths, ids []string, removeCovers bool) ([]Book, error) {
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
			if removeCovers {
				_ = os.Remove(filepath.Join(paths.CoversDir, coverFilename(book)))
			}
			continue
		}
		remaining = append(remaining, book)
	}
	if err := Save(paths, remaining); err != nil {
		return nil, err
	}
	if err := SaveGenerated(paths, remaining); err != nil {
		return nil, err
	}
	return removed, nil
}
