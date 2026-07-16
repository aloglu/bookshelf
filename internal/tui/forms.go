package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"

	"github.com/aloglu/bookshelf/internal/library"
)

type BookFormResult struct {
	Book       library.Book
	FetchCover bool
	Build      bool
}

type BuildFormResult struct {
	Confirmed       bool
	FetchCovers     bool
	RecomputeColors bool
}

func RunBookForm(existing *library.Book) (BookFormResult, error) {
	var input library.BookInput
	fetchCover := false
	build := true
	title := "Add a book"
	if existing != nil {
		title = "Edit book"
		input = library.BookInput{
			Title:      existing.Title,
			Author:     existing.Author,
			ISBN:       existing.ISBN,
			Translator: existing.Translator,
			Publisher:  existing.Publisher,
			Binding:    existing.Binding,
			Published:  existing.Year(),
		}
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Value(&input.Title).
				Validate(func(value string) error {
					if strings.TrimSpace(value) == "" {
						return fmt.Errorf("title is required")
					}
					return nil
				}),
			huh.NewInput().Title("Author").Value(&input.Author),
			huh.NewInput().Title("ISBN").Description("Hyphens are allowed").Value(&input.ISBN),
			huh.NewInput().Title("Translator").Value(&input.Translator),
		).Title(title),
		huh.NewGroup(
			huh.NewInput().Title("Publisher").Value(&input.Publisher),
			huh.NewInput().Title("Binding").Value(&input.Binding),
			huh.NewInput().
				Title("Published year").
				Value(&input.Published).
				Validate(func(value string) error {
					if strings.TrimSpace(value) != "" && library.ParseYear(value) == nil {
						return fmt.Errorf("enter a four-digit year")
					}
					return nil
				}),
			huh.NewConfirm().Title("Fetch a missing cover?").Value(&fetchCover),
			huh.NewConfirm().Title("Refresh published data after saving?").Affirmative("Yes").Negative("Later").Value(&build),
		),
	).
		WithTheme(huh.ThemeFunc(huh.ThemeCharm)).
		WithAccessible(os.Getenv("BOOKSHELF_ACCESSIBLE") != "")
	if err := form.Run(); err != nil {
		return BookFormResult{}, err
	}
	book := library.FromInput(input)
	if existing != nil {
		book.ID = existing.ID
		book.Cover = existing.Cover
		book.SpineColor = existing.SpineColor
		book.SpineTextColor = existing.SpineTextColor
		book = library.Normalize(book)
	}
	return BookFormResult{Book: book, FetchCover: fetchCover, Build: build}, nil
}

func ConfirmRemoval(books []library.Book) (confirmed, removeCovers bool, err error) {
	if len(books) == 0 {
		return false, false, nil
	}
	names := make([]string, 0, len(books))
	for _, book := range books {
		names = append(names, "• "+book.Title)
	}
	confirmed = false
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(fmt.Sprintf("Remove %d book(s)?", len(books))).
				Description(strings.Join(names, "\n")),
			huh.NewConfirm().
				Title("Also remove associated published covers?").
				Affirmative("Remove covers").
				Negative("Keep covers").
				Value(&removeCovers),
			huh.NewConfirm().
				Title("Confirm removal").
				Affirmative("Remove").
				Negative("Cancel").
				Value(&confirmed),
		),
	).
		WithTheme(huh.ThemeFunc(huh.ThemeCharm)).
		WithAccessible(os.Getenv("BOOKSHELF_ACCESSIBLE") != "")
	err = form.Run()
	return confirmed, removeCovers, err
}

func RunBuildForm() (BuildFormResult, error) {
	result := BuildFormResult{Confirmed: true}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Fetch missing covers from Open Library?").
				Value(&result.FetchCovers),
			huh.NewConfirm().
				Title("Recompute all spine colors?").
				Value(&result.RecomputeColors),
			huh.NewConfirm().
				Title("Build the published library now?").
				Affirmative("Build").
				Negative("Cancel").
				Value(&result.Confirmed),
		).Title("Build / refresh library"),
	).
		WithTheme(huh.ThemeFunc(huh.ThemeCharm)).
		WithAccessible(os.Getenv("BOOKSHELF_ACCESSIBLE") != "")
	return result, form.Run()
}
