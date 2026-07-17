package tui

import (
	"fmt"
	"net/url"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aloglu/bookshelf/internal/library"
)

type CoverSourceResult struct {
	Source    library.CoverSource
	URL       string
	Confirmed bool
}

type coverSourceModel struct {
	decision    decisionModel
	input       textinput.Model
	enterURL    bool
	result      CoverSourceResult
	errText     string
	width       int
	interrupted bool
}

func newCoverSourceModel(books []library.Book) coverSourceModel {
	options := []DecisionOption{
		{ID: string(library.CoverSourceAutomatic), Label: "Automatic"},
		{ID: string(library.CoverSourceGoodreads), Label: "Goodreads — slower, often the most accurate"},
		{ID: string(library.CoverSourceOpenLibrary), Label: "Open Library"},
		{ID: string(library.CoverSourceGoogle), Label: "Google Books"},
	}
	if len(books) == 1 {
		options = append(options, DecisionOption{ID: string(library.CoverSourceURL), Label: "Custom image URL"})
	}
	options = append(options, DecisionOption{ID: string(library.CoverSourceManual), Label: "Apply matching files from manual-covers"})
	description := fmt.Sprintf("%d books selected.\n\nAutomatic tries Goodreads, Open Library, then Google Books.", len(books))
	if len(books) == 1 {
		description = "Selected: " + books[0].Title
		if strings.TrimSpace(books[0].Author) != "" {
			description += " — " + books[0].Author
		}
		description += "\n\nAutomatic tries Goodreads, Open Library, then Google Books."
	}
	input := textinput.New()
	input.Placeholder = "https://example.com/cover.jpg"
	input.Prompt = ""
	input.SetWidth(64)
	return coverSourceModel{
		decision: newDecisionModel(DecisionRequest{
			Title:       "Choose a Cover Source",
			Description: description,
			Options:     options,
			Default:     0,
			EscapeLabel: "back",
			Vertical:    true,
			Borderless:  true,
		}),
		input: input,
		width: 80,
	}
}

func (m coverSourceModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m coverSourceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.decision.width = msg.Width
		m.input.SetWidth(min(64, max(24, msg.Width-12)))
	case tea.PasteMsg:
		if m.enterURL {
			var command tea.Cmd
			m.input, command = m.input.Update(msg)
			m.errText = ""
			return m, command
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.interrupted = true
			return m, tea.Quit
		}
		if m.enterURL {
			switch msg.String() {
			case "esc":
				m.enterURL = false
				m.errText = ""
				m.input.Blur()
				return m, nil
			case "enter":
				value := strings.TrimSpace(m.input.Value())
				parsed, err := url.ParseRequestURI(value)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
					m.errText = "Enter a valid HTTP or HTTPS URL."
					return m, nil
				}
				m.result = CoverSourceResult{Source: library.CoverSourceURL, URL: value, Confirmed: true}
				return m, tea.Quit
			}
			var command tea.Cmd
			m.input, command = m.input.Update(msg)
			m.errText = ""
			return m, command
		}
		choice, done, dismissed := m.decision.handleKey(msg.String())
		if !done {
			return m, nil
		}
		if dismissed {
			return m, tea.Quit
		}
		if choice == string(library.CoverSourceURL) {
			m.enterURL = true
			return m, m.input.Focus()
		}
		source, err := library.ParseCoverSource(choice)
		if err != nil {
			return m, tea.Quit
		}
		m.result = CoverSourceResult{Source: source, Confirmed: true}
		return m, tea.Quit
	}
	return m, nil
}

func (m coverSourceModel) View() tea.View {
	if !m.enterURL {
		view := tea.NewView(renderDecision(m.decision.request, m.decision.cursor, m.width))
		view.AltScreen = true
		view.WindowTitle = "Bookshelf Covers"
		return view
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E8E8E8")).Render("Custom Cover URL")
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#B7B7B7")).Render("Direct image URL")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777")).Render("enter continue  •  esc back")
	input := lipgloss.NewStyle().
		Width(min(60, max(22, m.width-16))).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#8B5CF6")).
		Render(m.input.View())
	body := title + "\n\n" + label + "\n" + input
	if m.errText != "" {
		body += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(m.errText)
	}
	body += "\n\n" + help
	view := tea.NewView(renderBorderlessPanel(body))
	view.AltScreen = true
	view.WindowTitle = "Bookshelf Covers"
	return view
}

func ChooseCoverSource(books []library.Book) (CoverSourceResult, error) {
	final, err := tea.NewProgram(newCoverSourceModel(books)).Run()
	if err != nil {
		return CoverSourceResult{}, err
	}
	model, ok := final.(coverSourceModel)
	if !ok {
		return CoverSourceResult{}, fmt.Errorf("unexpected cover source result %T", final)
	}
	if model.interrupted {
		return CoverSourceResult{}, ErrInterrupted
	}
	return model.result, nil
}

func fullScreenView(content, title string) tea.View {
	view := tea.NewView("\n" + lipgloss.NewStyle().Padding(1, 3).Render(content))
	view.AltScreen = true
	view.WindowTitle = title
	return view
}
