package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aloglu/bookshelf/internal/library"
)

var bookFieldLabels = []string{
	"Title (required)",
	"Author",
	"ISBN (hyphens allowed)",
	"URL Slug",
	"Translator",
	"Publisher",
	"Binding",
	"Published Year",
}

type bookFormModel struct {
	inputs      []textinput.Model
	initial     []string
	focus       int
	width       int
	height      int
	title       string
	build       bool
	dialog      *decisionModel
	saved       bool
	cancelled   bool
	existing    *library.Book
	validation  string
	interrupted bool
}

func newBookFormModel(existing *library.Book) bookFormModel {
	values := make([]string, len(bookFieldLabels))
	title := "Add a Book"
	if existing != nil {
		title = "Edit a Book"
		values = []string{
			existing.Title, existing.Author, existing.ISBN, existing.Slug,
			existing.Translator, existing.Publisher, existing.Binding, existing.Year(),
		}
	}
	inputs := make([]textinput.Model, len(values))
	for index, value := range values {
		inputs[index] = textinput.New()
		inputs[index].Prompt = ""
		inputs[index].SetValue(value)
		inputs[index].SetWidth(64)
	}
	inputs[0].Focus()
	return bookFormModel{
		inputs:   inputs,
		initial:  append([]string(nil), values...),
		title:    title,
		build:    true,
		width:    80,
		height:   24,
		existing: existing,
	}
}

func (m bookFormModel) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.inputs[0].Focus())
}

func (m *bookFormModel) dirty() bool {
	for index := range m.inputs {
		if m.inputs[index].Value() != m.initial[index] {
			return true
		}
	}
	return !m.build
}

func (m *bookFormModel) setFocus(next int) tea.Cmd {
	if next < 0 {
		next = 0
	}
	if next > len(m.inputs)+1 {
		next = len(m.inputs) + 1
	}
	m.focus = next
	var commands []tea.Cmd
	for index := range m.inputs {
		if index == m.focus {
			commands = append(commands, m.inputs[index].Focus())
		} else {
			m.inputs[index].Blur()
		}
	}
	return tea.Batch(commands...)
}

func (m *bookFormModel) validate() bool {
	if strings.TrimSpace(m.inputs[0].Value()) == "" {
		m.validation = "Title is required."
		m.setFocus(0)
		return false
	}
	year := strings.TrimSpace(m.inputs[7].Value())
	if year != "" && library.ParseYear(year) == nil {
		m.validation = "Published year must be a four-digit year."
		m.setFocus(7)
		return false
	}
	m.validation = ""
	return true
}

func (m bookFormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		for index := range m.inputs {
			m.inputs[index].SetWidth(min(64, max(20, msg.Width-16)))
		}
		if m.dialog != nil {
			m.dialog.width = msg.Width
		}
	case tea.PasteMsg:
		if m.dialog == nil && m.focus < len(m.inputs) {
			var command tea.Cmd
			m.inputs[m.focus], command = m.inputs[m.focus].Update(msg)
			m.validation = ""
			return m, command
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.cancelled = true
			m.interrupted = true
			return m, tea.Quit
		}
		if m.dialog != nil {
			choice, done, dismissed := m.dialog.handleKey(msg.String())
			if !done {
				return m, nil
			}
			if !dismissed && choice == "discard" {
				m.cancelled = true
				return m, tea.Quit
			}
			m.dialog = nil
			return m, nil
		}
		switch msg.String() {
		case "esc":
			if !m.dirty() {
				m.cancelled = true
				return m, tea.Quit
			}
			dialog := newDecisionModel(DecisionRequest{
				Title:       "Discard Book Changes?",
				Description: "The changes on this screen have not been saved.",
				Options: []DecisionOption{
					{ID: "discard", Label: "Discard", Tone: DecisionDanger},
					{ID: "continue", Label: "Keep Editing"},
				},
				Default: 1,
			})
			dialog.width = m.width
			m.dialog = &dialog
			return m, nil
		case "tab", "down":
			return m, m.setFocus(m.focus + 1)
		case "shift+tab", "up":
			return m, m.setFocus(m.focus - 1)
		case "enter":
			if m.focus < len(m.inputs) {
				return m, m.setFocus(m.focus + 1)
			}
			if m.focus == len(m.inputs) {
				return m, m.setFocus(m.focus + 1)
			}
			if m.validate() {
				m.saved = true
				return m, tea.Quit
			}
			return m, nil
		case "space":
			if m.focus == len(m.inputs) {
				m.build = !m.build
				return m, nil
			}
		case "left":
			if m.focus == len(m.inputs) {
				m.build = true
				return m, nil
			}
		case "right":
			if m.focus == len(m.inputs) {
				m.build = false
				return m, nil
			}
		}
		if m.focus < len(m.inputs) {
			var command tea.Cmd
			m.inputs[m.focus], command = m.inputs[m.focus].Update(msg)
			m.validation = ""
			return m, command
		}
	}
	return m, nil
}

func (m bookFormModel) View() tea.View {
	if m.dialog != nil {
		view := tea.NewView(renderDecision(m.dialog.request, m.dialog.cursor, m.width))
		view.AltScreen = true
		view.WindowTitle = "Bookshelf"
		return view
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E8E8E8"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	activeLabel := labelStyle.Foreground(lipgloss.Color("#A78BFA")).Bold(true)
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#67C6C2"))
	formWidth := min(70, max(30, m.width-10))

	var rows []string
	for index := range m.inputs {
		label := labelStyle
		borderColor := lipgloss.Color("#454550")
		if index == m.focus {
			label = activeLabel
			borderColor = lipgloss.Color("#8B5CF6")
		}
		section := ""
		switch index {
		case 0:
			section = sectionStyle.Render("Book Details") + "\n"
		case 3:
			section = sectionStyle.Render("Website") + "\n"
		case 4:
			section = sectionStyle.Render("Edition & Publication") + "\n"
		}
		field := lipgloss.NewStyle().
			Width(formWidth).
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Render(m.inputs[index].View())
		rows = append(rows, section+label.Render(bookFieldLabels[index])+"\n"+field)
	}
	toggle := func(index int, label string, value bool) string {
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8D8D8"))
		if m.focus == index {
			labelStyle = labelStyle.Foreground(lipgloss.Color("#A78BFA")).Bold(true)
		}
		button := func(text string, selected bool) string {
			style := lipgloss.NewStyle().Padding(0, 2).Foreground(lipgloss.Color("#8B8B96"))
			if selected {
				color := lipgloss.Color("#80EF80")
				foreground := lipgloss.Color("#102015")
				if text == "No" {
					color = lipgloss.Color("#EF4444")
					foreground = lipgloss.Color("#FFFFFF")
				}
				style = style.Background(color).Foreground(foreground).Bold(true)
			}
			return style.Render(text)
		}
		choices := button("Yes", value) + "  " + button("No", !value)
		const labelWidth = 48
		if formWidth >= labelWidth+19 {
			return labelStyle.Width(labelWidth).Render(label) + "    " + choices
		}
		return labelStyle.Render(label) + "\n  " + choices
	}
	afterSaving := sectionStyle.Render("After Saving") + "\n" +
		toggle(len(m.inputs), "Update published website data after saving", m.build)
	rows = append(rows, afterSaving)
	saveStyle := lipgloss.NewStyle().
		Padding(0, 2).
		Foreground(lipgloss.Color("#A78BFA")).
		Bold(true)
	saveButton := "Save Book"
	if m.focus == len(m.inputs)+1 {
		saveStyle = saveStyle.
			Background(lipgloss.Color("#8B5CF6")).
			Foreground(lipgloss.Color("#111827"))
	}
	rows = append(rows, saveStyle.Render(saveButton))

	available := max(2, (m.height-8)/5)
	viewportFocus := m.focus
	start, end := 0, len(rows)
	if len(rows) > available {
		start = max(0, viewportFocus-available/2)
		end = min(len(rows), start+available)
		start = max(0, end-available)
	}
	body := titleStyle.Render(m.title) + "\n\n"
	if start > 0 {
		body += helpStyle.Render(fmt.Sprintf("↑ %d earlier field(s)", start)) + "\n"
	}
	body += strings.Join(rows[start:end], "\n\n")
	if end < len(rows) {
		body += "\n" + helpStyle.Render(fmt.Sprintf("↓ %d more field(s)", len(rows)-end))
	}
	if m.validation != "" {
		body += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(m.validation)
	}
	body += "\n\n" + helpStyle.Render("↑/↓/tab move  •  ←/→ choose  •  space toggle  •  enter continue/save  •  esc back")
	return fullScreenView(body, "Bookshelf")
}

func RunBookForm(existing *library.Book) (BookFormResult, error) {
	if AccessibleMode() {
		return newAccessiblePrompter().bookForm(existing)
	}
	final, err := tea.NewProgram(newBookFormModel(existing)).Run()
	if err != nil {
		return BookFormResult{}, err
	}
	model, ok := final.(bookFormModel)
	if !ok {
		return BookFormResult{}, fmt.Errorf("unexpected book form result %T", final)
	}
	if model.interrupted {
		return BookFormResult{}, ErrInterrupted
	}
	if !model.saved {
		return BookFormResult{Cancelled: true}, nil
	}
	return bookFormResult(model, existing)
}

func bookFormResult(model bookFormModel, existing *library.Book) (BookFormResult, error) {
	input := library.BookInput{
		Title: model.inputs[0].Value(), Author: model.inputs[1].Value(),
		ISBN: model.inputs[2].Value(), Slug: model.inputs[3].Value(),
		Translator: model.inputs[4].Value(), Publisher: model.inputs[5].Value(),
		Binding: model.inputs[6].Value(), Published: model.inputs[7].Value(),
	}
	book := library.FromInput(input)
	if existing != nil {
		book.ID = existing.ID
		book.Cover = existing.Cover
		book.SpineColor = existing.SpineColor
		book.SpineTextColor = existing.SpineTextColor
		book = library.Normalize(book)
	}
	return BookFormResult{Book: book, Build: model.build}, nil
}
