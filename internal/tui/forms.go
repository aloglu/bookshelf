package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aloglu/bookshelf/internal/library"
)

type BookFormResult struct {
	Book       library.Book
	FetchCover bool
	Build      bool
	Cancelled  bool
}

func ConfirmUninstall(binPath, installDir string, purge bool) (bool, error) {
	title := "Uninstall Bookshelf?"
	description := fmt.Sprintf("Remove the command and generated website?\n\n%s\n%s\n\nEverything under data/ will be kept. To remove it too, cancel and run `bookshelf uninstall --purge`.", binPath, filepath.Join(installDir, "public"))
	affirmative := "Uninstall"
	if purge {
		title = "Delete Bookshelf and all data?"
		description = fmt.Sprintf("This permanently removes the command, books, covers, settings, reports, and generated website.\n\n%s\n%s", binPath, installDir)
		affirmative = "Delete Everything"
	}
	choice, chosen, err := RunDecision(DecisionRequest{
		Title:       title,
		Description: description,
		Borderless:  true,
		Options: []DecisionOption{
			{ID: "confirm", Label: affirmative, Tone: DecisionDanger},
			{ID: "cancel", Label: "Cancel"},
		},
		Default: 1,
	})
	return chosen && choice == "confirm", err
}

func ConfirmRemoval(books []library.Book) (confirmed, removeCovers bool, err error) {
	if len(books) == 0 {
		return false, false, nil
	}
	request := removalDecisionRequest(books)
	choice, chosen, err := RunDecision(request)
	if err != nil || !chosen || choice == "cancel" {
		return false, false, err
	}
	return true, choice == "books-and-covers", nil
}

func removalDecisionRequest(books []library.Book) DecisionRequest {
	names := make([]string, 0, len(books))
	for _, book := range books {
		names = append(names, "• "+book.Title)
	}
	return DecisionRequest{
		Title:       fmt.Sprintf("Remove %d book(s)?", len(books)),
		Description: strings.Join(names, "\n"),
		Options: []DecisionOption{
			{ID: "books-and-covers", Label: "Books + Covers", Tone: DecisionDanger},
			{ID: "books-only", Label: "Books Only", Tone: DecisionDanger},
			{ID: "cancel", Label: "Cancel"},
		},
		Default: 2,
	}
}
