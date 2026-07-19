package tui

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

func TestAccessibleBookFormUsesLinePromptsWithoutStartingBubbleTea(t *testing.T) {
	t.Setenv("BOOKSHELF_ACCESSIBLE", "1")
	previousInput, previousOutput := accessibleInput, accessibleOutput
	accessibleInput = strings.NewReader("Dune\nFrank Herbert\n978-0-441-17271-9\n\n\nAce\nPaperback\nedition-1965\n1965\n\ny\n")
	var output bytes.Buffer
	accessibleOutput = &output
	t.Cleanup(func() {
		accessibleInput = previousInput
		accessibleOutput = previousOutput
	})

	result, err := RunBookForm(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Book.Title != "Dune" || result.Book.Author != "Frank Herbert" ||
		result.Book.ISBN != "978-0-441-17271-9" || !result.Build {
		t.Fatalf("accessible form result = %#v", result)
	}
	if !strings.Contains(output.String(), "Title:") ||
		!strings.Contains(output.String(), "Published year must be exactly four digits.") ||
		!strings.Contains(output.String(), "Update published website") {
		t.Fatalf("accessible prompts were not written: %q", output.String())
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

func TestCoverProgressCancelDecisionIsBorderless(t *testing.T) {
	request := coverCancelDecisionRequest(3)
	if !request.Borderless {
		t.Fatal("cover progress cancel decision has a border")
	}
}

func TestCoverProgressStopsWhenParentContextIsCancelled(t *testing.T) {
	paths := library.NewPaths(t.TempDir())
	if err := library.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	book := library.Normalize(library.Book{Title: "Dune"})
	session, err := library.NewCoverFetchSession(paths, []library.Book{book}, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	model := newCoverProgressModel(ctx, session, library.CoverSourceAutomatic)
	model.inFlight = true
	cancel()

	updated, command := model.Update(coverFetchedMsg{canceled: true})
	got := updated.(*coverProgressModel)
	if !got.interrupted || got.stopAction != "discard" || command == nil {
		t.Fatalf("cancelled model = interrupted:%v action:%q command:%v", got.interrupted, got.stopAction, command)
	}
	finished := command()
	updated, quit := got.Update(finished)
	got = updated.(*coverProgressModel)
	if !got.completed || got.kept || quit == nil {
		t.Fatalf("finished cancelled model = completed:%v kept:%v quit:%v", got.completed, got.kept, quit)
	}
}

func TestTerminalDetectionRequiresInputAndOutputTerminals(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()

	for _, test := range []struct {
		name      string
		inputTTY  bool
		outputTTY bool
		want      bool
	}{
		{name: "both", inputTTY: true, outputTTY: true, want: true},
		{name: "input only", inputTTY: true, outputTTY: false},
		{name: "output only", inputTTY: false, outputTTY: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := isTerminalPair(input, output, func(fd uintptr) bool {
				if fd == input.Fd() {
					return test.inputTTY
				}
				return test.outputTTY
			})
			if got != test.want {
				t.Fatalf("isTerminalPair() = %v, want %v", got, test.want)
			}
		})
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
	model.cursor = 10
	model.candidates[10] = 2
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(settingsModel)
	if model.config.PermalinkStyle != library.PermalinkTitleSlug {
		t.Fatalf("active style = %q", model.config.PermalinkStyle)
	}
}

func TestSettingsSaveDoesNotCommitHighlightedCandidate(t *testing.T) {
	model := newSettingsModel(library.DefaultConfig())
	model.cursor = 10
	model.candidates[10] = 2
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

func TestSettingsSelectsIndependentScrollSpeeds(t *testing.T) {
	model := newSettingsModel(library.DefaultConfig())
	model.cursor = 5
	model.candidates[5] = 0
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(settingsModel)
	model.cursor = 6
	model.candidates[6] = 2
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(settingsModel)
	if model.config.ShelfScrollSpeed != library.ScrollSpeedSlow {
		t.Fatalf("shelf scroll speed = %q", model.config.ShelfScrollSpeed)
	}
	if model.config.CoverflowSpeed != library.ScrollSpeedFast {
		t.Fatalf("Coverflow scroll speed = %q", model.config.CoverflowSpeed)
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
	if !model.build {
		t.Fatal("update website data does not default to enabled")
	}
	if model.dirty() {
		t.Fatal("untouched form is dirty")
	}
}

func TestBookFormAcceptsBracketedPaste(t *testing.T) {
	model := newBookFormModel(nil)
	updated, _ := model.Update(tea.PasteMsg{Content: "The Left Hand of Darkness"})
	model = updated.(bookFormModel)
	if model.inputs[0].Value() != "The Left Hand of Darkness" {
		t.Fatalf("pasted title = %q", model.inputs[0].Value())
	}
}

func TestSettingsTextInputAcceptsBracketedPaste(t *testing.T) {
	model := newSettingsModel(library.DefaultConfig())
	updated, _ := model.Update(tea.PasteMsg{Content: " Library"})
	model = updated.(settingsModel)
	if model.config.SiteTitle != "Bookshelf Library" {
		t.Fatalf("pasted website title = %q", model.config.SiteTitle)
	}
}

func TestBookFormUsesSpaceForTogglesAndDoesNotWrap(t *testing.T) {
	model := newBookFormModel(nil)
	model.setFocus(len(model.inputs) + 1)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(bookFormModel)
	if model.build {
		t.Fatal("space did not toggle the focused publish option")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	model = updated.(bookFormModel)
	if !model.build {
		t.Fatal("left did not choose Yes for the horizontal option")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model = updated.(bookFormModel)
	if model.build {
		t.Fatal("right did not choose No for the horizontal option")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(bookFormModel)
	if model.focus != len(model.inputs)+2 {
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

func TestBookFormVisibilityChangeForcesWebsiteUpdate(t *testing.T) {
	model := newBookFormModel(nil)
	model.build = false
	model.setFocus(len(model.inputs))
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(bookFormModel)
	if model.visibility != library.WebsiteHidden {
		t.Fatalf("visibility = %q, want hidden", model.visibility)
	}
	if !model.build {
		t.Fatal("visibility change did not force a website update")
	}
}

func TestVisibilityWorkflowSelectsBooksAndConfirmsAction(t *testing.T) {
	book := library.Normalize(library.Book{ID: "dune", Title: "Dune"})
	model := newVisibilityWorkflowModel([]library.Book{book})
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model = updated.(visibilityWorkflowModel)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(visibilityWorkflowModel)
	if model.dialog == nil {
		t.Fatal("selected books did not open the visibility decision")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	model = updated.(visibilityWorkflowModel)
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(visibilityWorkflowModel)
	if command == nil || !model.result.Confirmed ||
		model.result.Visibility != library.WebsiteHidden ||
		len(model.result.IDs) != 1 || model.result.IDs[0] != book.ID {
		t.Fatalf("visibility workflow result = %#v, command nil = %v", model.result, command == nil)
	}
}

func TestCoverSelectorEnterUsesHighlightedBook(t *testing.T) {
	book := library.Normalize(library.Book{Title: "Dune", Author: "Frank Herbert"})
	model := newBookSelectorModel([]library.Book{book}, nil, nil, "Bookshelf · Covers", true, true)
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

func TestBookDescriptionUsesCompactSeparatorsAndCoverIcon(t *testing.T) {
	published := 2016
	book := library.Book{
		Title:     "Ulysses",
		Author:    "James Joyce",
		ISBN:      "978-0143108245",
		Cover:     "data/covers/978-0143108245.jpg",
		Published: &published,
	}
	item := bookItem{
		book:            book,
		showCoverStatus: true,
	}
	if got, want := item.Description(), "James Joyce · 2016 · 978-0143108245 · Has Cover"; got != want {
		t.Fatalf("description = %q, want %q", got, want)
	}
	item.book.Cover = ""
	if !strings.HasSuffix(item.Description(), " · Cover Missing") {
		t.Fatalf("missing-cover description = %q", item.Description())
	}
}

func TestCoverStatusesUseNeutralAndAttentionColors(t *testing.T) {
	base := lipgloss.NewStyle()
	covered := renderBookDescription(bookItem{
		book:            library.Book{Author: "Author", Cover: "data/covers/book.jpg"},
		showCoverStatus: true,
	}, 80, base)
	missing := renderBookDescription(bookItem{
		book:            library.Book{Author: "Author"},
		showCoverStatus: true,
	}, 80, base)
	if !strings.Contains(covered, base.Render("Has Cover")) {
		t.Fatalf("covered status is missing: %q", covered)
	}
	if !strings.Contains(missing, lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render("Cover Missing")) {
		t.Fatalf("missing status is not red: %q", missing)
	}
	if strings.Contains(covered, library.PublicationPublished) || strings.Contains(missing, library.PublicationPublished) {
		t.Fatalf("cover rows included publication status: %q / %q", covered, missing)
	}
}

func TestWordFilterRequiresEveryWholeQueryToken(t *testing.T) {
	targets := []string{
		"A Portrait of the Artist as a Young Man James Joyce",
		"Mars: The Pristine Beauty of the Red Planet",
		"The Upside of Irrationality: The Unexpected Benefits of Defying Logic",
	}
	ranks := wordFilter("portrait of", targets)
	if len(ranks) != 1 || ranks[0].Index != 0 {
		t.Fatalf("ranks = %#v", ranks)
	}
	ranks = wordFilter("joyce portrait", targets)
	if len(ranks) != 1 || ranks[0].Index != 0 {
		t.Fatalf("cross-field ranks = %#v", ranks)
	}
}

func TestFilteredListsLeaveOneBlankHeaderLine(t *testing.T) {
	if got := browserHeaderGap(list.Filtering); got != "\n\n" {
		t.Fatalf("filtering header gap = %q", got)
	}
	if got := browserHeaderGap(list.Unfiltered); got != "\n" {
		t.Fatalf("unfiltered header gap = %q", got)
	}
}

func TestSelectAllOnlySelectsVisibleFilteredBooks(t *testing.T) {
	books := []library.Book{
		library.Normalize(library.Book{Title: "Dune"}),
		library.Normalize(library.Book{Title: "Neuromancer"}),
		library.Normalize(library.Book{Title: "The Left Hand of Darkness"}),
	}
	model := newBookSelectorModel(books, nil, nil, "Bookshelf · Remove", true, false)
	model.list.SetFilterText("dune")
	model.list.SetFilterState(list.FilterApplied)

	updated, _ := model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	model = updated.(browserModel)
	if len(model.selected) != 1 || !model.selected[books[0].ID] {
		t.Fatalf("select all selected %#v instead of only the visible filtered book", model.selected)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	model = updated.(browserModel)
	if len(model.selected) != 0 {
		t.Fatalf("second select-all toggle did not clear the visible selection: %#v", model.selected)
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

func TestPublicationStatusAppearsOnlyInListWorkflow(t *testing.T) {
	book := library.Normalize(library.Book{Title: "Dune", Cover: "data/covers/dune.jpg"})
	statuses := map[string]string{book.ID: library.PublicationPublished}
	listModel := newBrowserModel([]library.Book{book}, statuses)
	if got := listModel.list.Items()[0].(bookItem).status; got != library.PublicationPublished {
		t.Fatalf("list publication status = %q", got)
	}
	editModel := newEditWorkflowModel([]library.Book{book})
	if got := editModel.picker.list.Items()[0].(bookItem).status; got != "" {
		t.Fatalf("edit publication status = %q", got)
	}
	removeModel := newRemoveWorkflowModel([]library.Book{book})
	if got := removeModel.picker.list.Items()[0].(bookItem).status; got != "" {
		t.Fatalf("remove publication status = %q", got)
	}
	coverModel := newCoverWorkflowModel([]library.Book{book}, nil)
	coverItem := coverModel.picker.list.Items()[0].(bookItem)
	if coverItem.status != "" || !strings.HasSuffix(coverItem.Description(), "Has Cover") {
		t.Fatalf("cover item = %#v, description = %q", coverItem, coverItem.Description())
	}
}

func TestEditWorkflowPassesTerminalSizeToForm(t *testing.T) {
	book := library.Normalize(library.Book{Title: "Dune"})
	model := newEditWorkflowModel([]library.Book{book})
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
	model := newCoverWorkflowModel([]library.Book{book}, nil)
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
	model := newRemoveWorkflowModel([]library.Book{book})
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(removeWorkflowModel)
	if model.dialog == nil || command != nil {
		t.Fatal("remove picker did not open its inline confirmation")
	}
}
