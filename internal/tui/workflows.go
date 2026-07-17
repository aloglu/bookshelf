package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/aloglu/bookshelf/internal/library"
)

type CoverWorkflowResult struct {
	IDs       []string
	Source    library.CoverSource
	URL       string
	Confirmed bool
}

type coverWorkflowModel struct {
	books       []library.Book
	picker      browserModel
	source      *coverSourceModel
	selectedIDs []string
	result      CoverWorkflowResult
	interrupted bool
}

func newCoverWorkflowModel(books []library.Book, statuses map[string]string, initial []string) coverWorkflowModel {
	return coverWorkflowModel{
		books:  books,
		picker: newBookSelectorModel(books, statuses, initial, "Bookshelf · Covers", true),
	}
}

func (m coverWorkflowModel) Init() tea.Cmd { return m.picker.Init() }

func (m coverWorkflowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.source != nil {
		updated, command := m.source.Update(msg)
		source := updated.(coverSourceModel)
		m.source = &source
		if source.interrupted {
			m.interrupted = true
			return m, tea.Quit
		}
		if source.result.Confirmed {
			m.result = CoverWorkflowResult{
				IDs:       append([]string(nil), m.selectedIDs...),
				Source:    source.result.Source,
				URL:       source.result.URL,
				Confirmed: true,
			}
			return m, tea.Quit
		}
		if command != nil && !source.enterURL {
			if key, ok := msg.(tea.KeyPressMsg); ok && (key.String() == "esc" || key.String() == "q") {
				m.source = nil
				m.picker.result = BrowserResult{Action: ActionQuit}
				return m, nil
			}
		}
		return m, command
	}
	updated, command := m.picker.Update(msg)
	m.picker = updated.(browserModel)
	if m.picker.interrupted {
		m.interrupted = true
		return m, tea.Quit
	}
	if m.picker.result.Action == ActionSelect {
		m.selectedIDs = append([]string(nil), m.picker.result.IDs...)
		selected := selectedBooks(m.books, m.selectedIDs)
		source := newCoverSourceModel(selected)
		m.source = &source
		return m, source.Init()
	}
	return m, command
}

func (m coverWorkflowModel) View() tea.View {
	if m.source != nil {
		return m.source.View()
	}
	return m.picker.View()
}

func RunCoverWorkflow(books []library.Book, statuses map[string]string, initial []string) (CoverWorkflowResult, error) {
	final, err := tea.NewProgram(newCoverWorkflowModel(books, statuses, initial)).Run()
	if err != nil {
		return CoverWorkflowResult{}, err
	}
	model, ok := final.(coverWorkflowModel)
	if !ok {
		return CoverWorkflowResult{}, fmt.Errorf("unexpected cover workflow result %T", final)
	}
	if model.interrupted {
		return CoverWorkflowResult{}, ErrInterrupted
	}
	return model.result, nil
}

type EditWorkflowResult struct {
	Original  library.Book
	Form      BookFormResult
	Confirmed bool
}

type editWorkflowModel struct {
	books       []library.Book
	picker      browserModel
	form        *bookFormModel
	original    library.Book
	result      EditWorkflowResult
	width       int
	height      int
	interrupted bool
}

func newEditWorkflowModel(books []library.Book, statuses map[string]string) editWorkflowModel {
	return editWorkflowModel{
		books:  books,
		picker: newBookSelectorModel(books, statuses, nil, "Bookshelf · Edit", false),
	}
}

func (m editWorkflowModel) Init() tea.Cmd { return m.picker.Init() }

func (m editWorkflowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
	}
	if m.form != nil {
		updated, command := m.form.Update(msg)
		form := updated.(bookFormModel)
		m.form = &form
		if form.interrupted {
			m.interrupted = true
			return m, tea.Quit
		}
		if form.saved {
			result, err := bookFormResult(form, &m.original)
			if err != nil {
				return m, tea.Quit
			}
			m.result = EditWorkflowResult{Original: m.original, Form: result, Confirmed: true}
			return m, tea.Quit
		}
		if form.cancelled {
			m.form = nil
			m.picker.result = BrowserResult{Action: ActionQuit}
			return m, nil
		}
		return m, command
	}
	updated, command := m.picker.Update(msg)
	m.picker = updated.(browserModel)
	if m.picker.interrupted {
		m.interrupted = true
		return m, tea.Quit
	}
	if m.picker.result.Action == ActionSelect {
		selected := selectedBooks(m.books, m.picker.result.IDs)
		if len(selected) == 1 {
			m.original = selected[0]
			form := newBookFormModel(&m.original)
			if m.width > 0 && m.height > 0 {
				updated, _ := form.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
				form = updated.(bookFormModel)
			}
			m.form = &form
			return m, form.Init()
		}
	}
	return m, command
}

func (m editWorkflowModel) View() tea.View {
	if m.form != nil {
		return m.form.View()
	}
	return m.picker.View()
}

func RunEditWorkflow(books []library.Book, statuses map[string]string) (EditWorkflowResult, error) {
	final, err := tea.NewProgram(newEditWorkflowModel(books, statuses)).Run()
	if err != nil {
		return EditWorkflowResult{}, err
	}
	model, ok := final.(editWorkflowModel)
	if !ok {
		return EditWorkflowResult{}, fmt.Errorf("unexpected edit workflow result %T", final)
	}
	if model.interrupted {
		return EditWorkflowResult{}, ErrInterrupted
	}
	return model.result, nil
}

type RemoveWorkflowResult struct {
	IDs          []string
	RemoveCovers bool
	Confirmed    bool
}

type removeWorkflowModel struct {
	books       []library.Book
	picker      browserModel
	dialog      *decisionModel
	selectedIDs []string
	result      RemoveWorkflowResult
	interrupted bool
}

func newRemoveWorkflowModel(books []library.Book, statuses map[string]string) removeWorkflowModel {
	return removeWorkflowModel{
		books:  books,
		picker: newBookSelectorModel(books, statuses, nil, "Bookshelf · Remove", true),
	}
}

func (m removeWorkflowModel) Init() tea.Cmd { return m.picker.Init() }

func (m removeWorkflowModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.dialog != nil {
		if size, ok := msg.(tea.WindowSizeMsg); ok {
			m.dialog.width = size.Width
			return m, nil
		}
		if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
			m.interrupted = true
			return m, tea.Quit
		}
		choice, done, dismissed := m.dialog.handleKey(keyString(msg))
		if !done {
			return m, nil
		}
		if dismissed || choice == "cancel" {
			m.dialog = nil
			m.picker.result = BrowserResult{Action: ActionQuit}
			return m, nil
		}
		m.result = RemoveWorkflowResult{
			IDs:          append([]string(nil), m.selectedIDs...),
			RemoveCovers: choice == "books-and-covers",
			Confirmed:    true,
		}
		return m, tea.Quit
	}
	updated, command := m.picker.Update(msg)
	m.picker = updated.(browserModel)
	if m.picker.interrupted {
		m.interrupted = true
		return m, tea.Quit
	}
	if m.picker.result.Action == ActionSelect {
		m.selectedIDs = append([]string(nil), m.picker.result.IDs...)
		selected := selectedBooks(m.books, m.selectedIDs)
		dialog := newDecisionModel(removalDecisionRequest(selected))
		dialog.width = m.picker.width
		m.dialog = &dialog
		return m, nil
	}
	return m, command
}

func (m removeWorkflowModel) View() tea.View {
	if m.dialog != nil {
		view := tea.NewView(renderDecision(m.dialog.request, m.dialog.cursor, m.dialog.width))
		view.AltScreen = true
		view.WindowTitle = "Bookshelf Remove"
		return view
	}
	return m.picker.View()
}

func RunRemoveWorkflow(books []library.Book, statuses map[string]string) (RemoveWorkflowResult, error) {
	final, err := tea.NewProgram(newRemoveWorkflowModel(books, statuses)).Run()
	if err != nil {
		return RemoveWorkflowResult{}, err
	}
	model, ok := final.(removeWorkflowModel)
	if !ok {
		return RemoveWorkflowResult{}, fmt.Errorf("unexpected remove workflow result %T", final)
	}
	if model.interrupted {
		return RemoveWorkflowResult{}, ErrInterrupted
	}
	return model.result, nil
}

func selectedBooks(books []library.Book, ids []string) []library.Book {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	selected := make([]library.Book, 0, len(ids))
	for _, book := range books {
		if wanted[book.ID] {
			selected = append(selected, book)
		}
	}
	return selected
}

func keyString(msg tea.Msg) string {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		return key.String()
	}
	return ""
}
