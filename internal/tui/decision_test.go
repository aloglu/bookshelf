package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/aloglu/bookshelf/internal/library"
)

func TestDecisionEscapeDismissesWithoutChoosing(t *testing.T) {
	model := newDecisionModel(DecisionRequest{
		Title: "Delete everything?",
		Options: []DecisionOption{
			{ID: "delete", Label: "Delete everything", Tone: DecisionDanger},
			{ID: "cancel", Label: "Cancel"},
		},
		Default: 1,
	})
	choice, done, dismissed := model.handleKey("esc")
	if !done || !dismissed || choice != "" {
		t.Fatalf("choice = %q, done = %v, dismissed = %v", choice, done, dismissed)
	}
}

func TestCoverProgressCountsFailedBooks(t *testing.T) {
	bar := renderCoverProgress([]library.CoverFetchOutcome{
		{Status: library.CoverFetchFailed},
	}, 2, 10)
	if filled := strings.Count(bar, "█"); filled != 5 {
		t.Fatalf("filled progress cells = %d, want 5; bar = %q", filled, bar)
	}
	if remaining := strings.Count(bar, "░"); remaining != 5 {
		t.Fatalf("remaining progress cells = %d, want 5; bar = %q", remaining, bar)
	}
}

func TestCoverProgressPreservesOutcomeOrder(t *testing.T) {
	outcomes := []library.CoverFetchOutcome{
		{Status: library.CoverFetchDownloaded},
		{Status: library.CoverFetchSkipped},
		{Status: library.CoverFetchDownloaded},
		{Status: library.CoverFetchFailed},
		{Status: library.CoverFetchDownloaded},
	}
	got := coverProgressCells(outcomes, 5, 5)
	for index, want := range []library.CoverFetchStatus{
		library.CoverFetchDownloaded,
		library.CoverFetchSkipped,
		library.CoverFetchDownloaded,
		library.CoverFetchFailed,
		library.CoverFetchDownloaded,
	} {
		if got[index] != want {
			t.Fatalf("cell %d = %q, want %q", index, got[index], want)
		}
	}
}

func TestDecisionUsesSafeDefault(t *testing.T) {
	model := newDecisionModel(DecisionRequest{
		Title: "Delete everything?",
		Options: []DecisionOption{
			{ID: "delete", Label: "Delete everything", Tone: DecisionDanger},
			{ID: "cancel", Label: "Cancel"},
		},
		Default: 1,
	})
	choice, done, dismissed := model.handleKey("enter")
	if !done || dismissed || choice != "cancel" {
		t.Fatalf("choice = %q, done = %v, dismissed = %v", choice, done, dismissed)
	}
}

func TestSettingsEscapeWithoutChangesExitsImmediately(t *testing.T) {
	model := newSettingsModel(library.DefaultConfig())
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	got := updated.(settingsModel)
	if got.dialog != nil {
		t.Fatal("escape opened a discard dialog without a settings change")
	}
	if command == nil {
		t.Fatal("escape did not quit settings")
	}
}

func TestSettingsSelectsOnlyOnSpace(t *testing.T) {
	model := newSettingsModel(library.DefaultConfig())
	model.cursor = 8
	model.candidates[8] = 2
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(settingsModel)
	if model.config.PermalinkStyle != library.PermalinkTitleSlug {
		t.Fatalf("active style = %q", model.config.PermalinkStyle)
	}
}

func TestSettingsSaveDoesNotCommitHighlightedCandidate(t *testing.T) {
	model := newSettingsModel(library.DefaultConfig())
	model.cursor = 8
	model.candidates[8] = 2
	model.moveCursor(len(settingsRows))
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(settingsModel)
	if command == nil || !model.saved {
		t.Fatal("settings were not saved")
	}
	if model.config.PermalinkStyle != library.PermalinkFormattedISBN {
		t.Fatalf("highlighted but unselected style was saved: %q", model.config.PermalinkStyle)
	}
}

func TestSettingsCandidateResetsWhenLeavingRow(t *testing.T) {
	model := newSettingsModel(library.DefaultConfig())
	model.cursor = 4
	model.candidates[4] = 1
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(settingsModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	model = updated.(settingsModel)
	if model.candidates[4] != model.selectedIndex(4) {
		t.Fatalf("desktop view candidate = %d, selected = %d", model.candidates[4], model.selectedIndex(4))
	}
	if model.config.DefaultView != library.WebsiteViewShelf {
		t.Fatalf("unselected desktop view changed to %q", model.config.DefaultView)
	}
}

func TestBookFormStartsWithSensibleSaveDefaults(t *testing.T) {
	model := newBookFormModel(nil)
	if !model.fetchCover || !model.build {
		t.Fatalf("defaults: fetch cover = %v, update website data = %v", model.fetchCover, model.build)
	}
	if model.dirty() {
		t.Fatal("untouched form is dirty")
	}
}

func TestBookFormUsesSpaceForTogglesAndDoesNotWrap(t *testing.T) {
	model := newBookFormModel(nil)
	model.setFocus(len(model.inputs))
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(bookFormModel)
	if model.fetchCover {
		t.Fatal("space did not toggle the focused cover option")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	model = updated.(bookFormModel)
	if !model.fetchCover {
		t.Fatal("left did not choose Yes for the horizontal option")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model = updated.(bookFormModel)
	if model.fetchCover {
		t.Fatal("right did not choose No for the horizontal option")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(bookFormModel)
	if model.focus != len(model.inputs)+1 {
		t.Fatalf("enter focus = %d, want next option", model.focus)
	}
	model.setFocus(len(model.inputs) + 2)
	model.setFocus(len(model.inputs) + 3)
	if model.focus != len(model.inputs)+2 {
		t.Fatalf("focus wrapped past Save Book to %d", model.focus)
	}
	model.setFocus(-1)
	if model.focus != 0 {
		t.Fatalf("focus wrapped above Title to %d", model.focus)
	}
}

func TestCoverSelectorEnterUsesHighlightedBook(t *testing.T) {
	book := library.Normalize(library.Book{Title: "Dune", Author: "Frank Herbert"})
	model := newBookSelectorModel([]library.Book{book}, nil, nil, "Bookshelf · Covers", true)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(browserModel)
	if command == nil || model.result.Action != ActionSelect || len(model.result.IDs) != 1 || model.result.IDs[0] != book.ID {
		t.Fatalf("result = %#v, command nil = %v", model.result, command == nil)
	}
}

func TestBookItemTitleDoesNotEmbedSelectionMarkup(t *testing.T) {
	book := library.Normalize(library.Book{Title: "Dune"})
	selected := map[string]bool{}
	item := bookItem{book: book, selected: selected}
	if item.Title() != "Dune" {
		t.Fatalf("unselected title = %q", item.Title())
	}
	selected[book.ID] = true
	if item.Title() != "Dune" {
		t.Fatalf("selected title = %q", item.Title())
	}
}

func TestListViewIsReadOnly(t *testing.T) {
	books := []library.Book{
		library.Normalize(library.Book{Title: "Dune"}),
		library.Normalize(library.Book{Title: "Neuromancer"}),
	}
	model := newBrowserModel(books, nil)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(browserModel)
	if len(model.selected) != 0 || model.result.Action != ActionQuit {
		t.Fatalf("list accepted an action: selected = %d, result = %#v", len(model.selected), model.result)
	}
}

func TestEditWorkflowPassesTerminalSizeToForm(t *testing.T) {
	book := library.Normalize(library.Book{Title: "Dune"})
	model := newEditWorkflowModel([]library.Book{book}, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 42})
	model = updated.(editWorkflowModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(editWorkflowModel)
	if model.form == nil {
		t.Fatal("edit form was not opened")
	}
	if model.form.width != 120 || model.form.height != 42 {
		t.Fatalf("edit form size = %dx%d, want 120x42", model.form.width, model.form.height)
	}
}

func TestCoverSourceCustomURLStaysInTheSameModel(t *testing.T) {
	model := newCoverSourceModel([]library.Book{{Title: "Dune"}})
	for model.decision.request.Options[model.decision.cursor].ID != string(library.CoverSourceURL) {
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model = updated.(coverSourceModel)
	}
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(coverSourceModel)
	if !model.enterURL || command == nil {
		t.Fatal("custom URL did not transition to the inline URL input")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(coverSourceModel)
	if model.enterURL {
		t.Fatal("escape did not return from URL input to source choices")
	}
}

func TestCoverSourceShowsSelectionContextWithoutBackItem(t *testing.T) {
	books := []library.Book{{Title: "Dune"}, {Title: "Neuromancer"}}
	model := newCoverSourceModel(books)
	if !strings.Contains(model.decision.request.Description, "2 books selected") {
		t.Fatalf("description = %q", model.decision.request.Description)
	}
	if model.decision.request.EscapeLabel != "back" {
		t.Fatalf("escape label = %q", model.decision.request.EscapeLabel)
	}
	for _, option := range model.decision.request.Options {
		if option.ID == "cancel" || option.Label == "Back" {
			t.Fatalf("back was rendered as a source option: %#v", option)
		}
	}
}

func TestCoverWorkflowTransitionsWithoutQuittingTheProgram(t *testing.T) {
	book := library.Normalize(library.Book{Title: "Dune"})
	model := newCoverWorkflowModel([]library.Book{book}, nil, nil)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(coverWorkflowModel)
	if model.source == nil || command == nil {
		t.Fatal("cover picker did not transition to source selection")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(coverWorkflowModel)
	if model.source != nil {
		t.Fatal("source escape did not return to the cover picker")
	}
}

func TestRemoveWorkflowUsesInlineConfirmation(t *testing.T) {
	book := library.Normalize(library.Book{Title: "Dune"})
	model := newRemoveWorkflowModel([]library.Book{book}, nil)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(removeWorkflowModel)
	if model.dialog == nil || command != nil {
		t.Fatal("remove picker did not open its inline confirmation")
	}
}
