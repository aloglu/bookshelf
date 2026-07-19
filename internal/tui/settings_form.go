package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aloglu/bookshelf/internal/library"
)

type settingKind int

const (
	settingChoice settingKind = iota
	settingText
)

type settingOption struct {
	label string
	value string
}

type settingRow struct {
	section string
	label   string
	kind    settingKind
	options []settingOption
}

var settingsRows = []settingRow{
	{section: "Branding", label: "Website title", kind: settingText},
	{section: "Branding", label: "Subtitle", kind: settingText},
	{
		section: "Browsing", label: "Library statistics",
		options: []settingOption{{label: "Show", value: "show"}, {label: "Hide", value: "hide"}},
	},
	{
		section: "Browsing", label: "Random book button",
		options: []settingOption{{label: "Show", value: "show"}, {label: "Hide", value: "hide"}},
	},
	{
		section: "Browsing", label: "Default desktop view",
		options: []settingOption{
			{label: "Shelf", value: string(library.WebsiteViewShelf)},
			{label: "Stacks", value: string(library.WebsiteViewStack)},
			{label: "Coverflow", value: string(library.WebsiteViewCoverflow)},
		},
	},
	{
		section: "Browsing", label: "Shelf scroll speed",
		options: []settingOption{
			{label: "Slow", value: string(library.ScrollSpeedSlow)},
			{label: "Normal", value: string(library.ScrollSpeedNormal)},
			{label: "Fast", value: string(library.ScrollSpeedFast)},
		},
	},
	{
		section: "Browsing", label: "Coverflow scroll speed",
		options: []settingOption{
			{label: "Slow", value: string(library.ScrollSpeedSlow)},
			{label: "Normal", value: string(library.ScrollSpeedNormal)},
			{label: "Fast", value: string(library.ScrollSpeedFast)},
		},
	},
	{
		section: "Browsing", label: "Default sort",
		options: []settingOption{
			{label: "Title", value: string(library.WebsiteSortTitle)},
			{label: "Author", value: string(library.WebsiteSortAuthor)},
			{label: "Year", value: string(library.WebsiteSortYear)},
		},
	},
	{
		section: "Browsing", label: "Sort direction",
		options: []settingOption{
			{label: "Ascending", value: string(library.SortAscending)},
			{label: "Descending", value: string(library.SortDescending)},
		},
	},
	{
		section: "Links", label: "ISBN link sources",
		options: []settingOption{
			{label: "Both", value: string(library.ISBNLinksBoth)},
			{label: "Wikipedia", value: string(library.ISBNLinksWikipedia)},
			{label: "Goodreads", value: string(library.ISBNLinksGoodreads)},
		},
	},
	{
		section: "Links", label: "Default permalink",
		options: []settingOption{
			{label: "Formatted ISBN", value: string(library.PermalinkFormattedISBN)},
			{label: "Compact ISBN", value: string(library.PermalinkCompactISBN)},
			{label: "Title slug", value: string(library.PermalinkTitleSlug)},
		},
	},
	{
		section: "Footer", label: "Footer",
		options: []settingOption{{label: "Show", value: "show"}, {label: "Hide", value: "hide"}},
	},
	{section: "Footer", label: "Footer text", kind: settingText},
}

type settingsModel struct {
	original    library.Config
	config      library.Config
	cursor      int
	candidates  []int
	inputs      map[int]textinput.Model
	width       int
	height      int
	dialog      *decisionModel
	validation  string
	saved       bool
	interrupted bool
}

func newSettingsModel(config library.Config) settingsModel {
	model := settingsModel{
		original:   config,
		config:     config,
		candidates: make([]int, len(settingsRows)),
		inputs:     make(map[int]textinput.Model),
		width:      80,
		height:     24,
	}
	values := map[int]string{
		0:  config.SiteTitle,
		1:  config.SiteSubtitle,
		12: config.FooterText,
	}
	placeholders := map[int]string{
		0:  "Bookshelf",
		1:  "Leave blank to hide",
		12: "Markdown; blank uses the built-in attribution",
	}
	for row := range settingsRows {
		if settingsRows[row].kind == settingText {
			input := textinput.New()
			input.Prompt = ""
			input.SetValue(values[row])
			input.Placeholder = placeholders[row]
			input.SetWidth(50)
			model.inputs[row] = input
		} else {
			model.candidates[row] = model.selectedIndex(row)
		}
	}
	input := model.inputs[0]
	input.Focus()
	model.inputs[0] = input
	return model
}

func (m settingsModel) Init() tea.Cmd {
	input := m.inputs[0]
	return tea.Batch(tea.RequestBackgroundColor, input.Focus())
}

func (m settingsModel) selectedValue(row int) string {
	switch row {
	case 2:
		if m.config.ShowStatistics {
			return "show"
		}
		return "hide"
	case 3:
		if m.config.ShowRandom {
			return "show"
		}
		return "hide"
	case 4:
		return string(m.config.DefaultView)
	case 5:
		return string(m.config.ShelfScrollSpeed)
	case 6:
		return string(m.config.CoverflowSpeed)
	case 7:
		return string(m.config.DefaultSort)
	case 8:
		return string(m.config.DefaultSortOrder)
	case 9:
		return string(m.config.ISBNLinkSources)
	case 10:
		return string(m.config.PermalinkStyle)
	case 11:
		if m.config.ShowFooter {
			return "show"
		}
		return "hide"
	default:
		return ""
	}
}

func (m settingsModel) selectedIndex(row int) int {
	value := m.selectedValue(row)
	for index, option := range settingsRows[row].options {
		if option.value == value {
			return index
		}
	}
	return 0
}

func (m *settingsModel) selectCandidate() {
	if m.cursor >= len(settingsRows) || settingsRows[m.cursor].kind != settingChoice {
		return
	}
	value := settingsRows[m.cursor].options[m.candidates[m.cursor]].value
	switch m.cursor {
	case 2:
		m.config.ShowStatistics = value == "show"
	case 3:
		m.config.ShowRandom = value == "show"
	case 4:
		m.config.DefaultView = library.WebsiteView(value)
	case 5:
		m.config.ShelfScrollSpeed = library.ScrollSpeed(value)
	case 6:
		m.config.CoverflowSpeed = library.ScrollSpeed(value)
	case 7:
		m.config.DefaultSort = library.WebsiteSort(value)
	case 8:
		m.config.DefaultSortOrder = library.SortDirection(value)
	case 9:
		m.config.ISBNLinkSources = library.ISBNLinkSources(value)
	case 10:
		m.config.PermalinkStyle = library.PermalinkStyle(value)
	case 11:
		m.config.ShowFooter = value == "show"
	}
	m.validation = ""
}

func (m *settingsModel) syncText(row int) {
	input := m.inputs[row]
	switch row {
	case 0:
		m.config.SiteTitle = input.Value()
	case 1:
		m.config.SiteSubtitle = input.Value()
	case 12:
		m.config.FooterText = input.Value()
	}
	m.validation = ""
}

func (m *settingsModel) moveCursor(next int) tea.Cmd {
	if m.cursor < len(settingsRows) {
		if settingsRows[m.cursor].kind == settingChoice {
			m.candidates[m.cursor] = m.selectedIndex(m.cursor)
		} else {
			input := m.inputs[m.cursor]
			input.Blur()
			m.inputs[m.cursor] = input
		}
	}
	m.cursor = max(0, min(len(settingsRows), next))
	if m.cursor < len(settingsRows) {
		if settingsRows[m.cursor].kind == settingChoice {
			m.candidates[m.cursor] = m.selectedIndex(m.cursor)
		} else {
			input := m.inputs[m.cursor]
			command := input.Focus()
			m.inputs[m.cursor] = input
			return command
		}
	}
	return nil
}

func (m settingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		inputWidth := min(52, max(16, msg.Width-35))
		for row, input := range m.inputs {
			input.SetWidth(inputWidth)
			m.inputs[row] = input
		}
		if m.dialog != nil {
			m.dialog.width = msg.Width
		}
	case tea.PasteMsg:
		if m.dialog == nil && m.cursor < len(settingsRows) && settingsRows[m.cursor].kind == settingText {
			input := m.inputs[m.cursor]
			var command tea.Cmd
			input, command = input.Update(msg)
			m.inputs[m.cursor] = input
			m.syncText(m.cursor)
			return m, command
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.interrupted = true
			return m, tea.Quit
		}
		if m.dialog != nil {
			choice, done, dismissed := m.dialog.handleKey(msg.String())
			if !done {
				return m, nil
			}
			if !dismissed && choice == "discard" {
				m.config = m.original
				return m, tea.Quit
			}
			m.dialog = nil
			if m.cursor < len(settingsRows) && settingsRows[m.cursor].kind == settingText {
				input := m.inputs[m.cursor]
				command := input.Focus()
				m.inputs[m.cursor] = input
				return m, command
			}
			return m, nil
		}
		if m.cursor < len(settingsRows) && settingsRows[m.cursor].kind == settingText {
			switch msg.String() {
			case "up", "shift+tab":
				return m, m.moveCursor(m.cursor - 1)
			case "down", "tab", "enter":
				return m, m.moveCursor(m.cursor + 1)
			case "esc":
				return m, m.handleEscape()
			default:
				input := m.inputs[m.cursor]
				var command tea.Cmd
				input, command = input.Update(msg)
				m.inputs[m.cursor] = input
				m.syncText(m.cursor)
				return m, command
			}
		}
		switch msg.String() {
		case "up", "shift+tab", "k":
			return m, m.moveCursor(m.cursor - 1)
		case "down", "tab", "j":
			return m, m.moveCursor(m.cursor + 1)
		case "left", "h":
			if m.cursor < len(settingsRows) {
				m.candidates[m.cursor] = max(0, m.candidates[m.cursor]-1)
			}
		case "right", "l":
			if m.cursor < len(settingsRows) {
				last := len(settingsRows[m.cursor].options) - 1
				m.candidates[m.cursor] = min(last, m.candidates[m.cursor]+1)
			}
		case "space":
			m.selectCandidate()
		case "enter":
			if m.cursor < len(settingsRows) {
				return m, m.moveCursor(m.cursor + 1)
			}
			if err := library.ValidateConfig(m.config); err != nil {
				m.validation = err.Error()
				if strings.TrimSpace(m.config.SiteTitle) == "" {
					return m, m.moveCursor(0)
				}
				return m, nil
			}
			m.saved = true
			return m, tea.Quit
		case "esc":
			return m, m.handleEscape()
		}
	}
	return m, nil
}

func (m *settingsModel) handleEscape() tea.Cmd {
	if m.config == m.original {
		return tea.Quit
	}
	dialog := newDecisionModel(DecisionRequest{
		Title:       "Discard Settings Changes?",
		Description: "The selected settings have not been saved.",
		Options: []DecisionOption{
			{ID: "discard", Label: "Discard", Tone: DecisionDanger},
			{ID: "continue", Label: "Keep Editing"},
		},
		Default: 1,
	})
	dialog.width = m.width
	m.dialog = &dialog
	return nil
}

func (m settingsModel) renderChoiceRow(rowIndex, formWidth int) string {
	row := settingsRows[rowIndex]
	focused := m.cursor == rowIndex
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8D8D8"))
	if focused {
		labelStyle = labelStyle.Foreground(lipgloss.Color("#80EF80")).Bold(true)
	}
	var renderedOptions []string
	selected := m.selectedIndex(rowIndex)
	for index, option := range row.options {
		style := lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#8B8B96"))
		switch {
		case focused && index == m.candidates[rowIndex]:
			style = style.Background(lipgloss.Color("#80EF80")).
				Foreground(lipgloss.Color("#111827")).Bold(true)
		case index == selected:
			style = style.Foreground(lipgloss.Color("#A78BFA")).Bold(true)
		}
		renderedOptions = append(renderedOptions, style.Render(option.label))
	}
	choices := strings.Join(renderedOptions, "  ")
	const labelWidth = 20
	if formWidth >= labelWidth+lipgloss.Width(choices)+3 {
		return labelStyle.Width(labelWidth).Render(row.label) + "   " + choices
	}
	return labelStyle.Render(row.label) + "  " + choices
}

func (m settingsModel) renderTextRow(rowIndex, formWidth int) string {
	row := settingsRows[rowIndex]
	focused := m.cursor == rowIndex
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8D8D8"))
	if focused {
		labelStyle = labelStyle.Foreground(lipgloss.Color("#80EF80")).Bold(true)
	}
	input := m.inputs[rowIndex]
	const labelWidth = 20
	if formWidth >= 45 {
		return labelStyle.Width(labelWidth).Render(row.label) + "   " + input.View()
	}
	return labelStyle.Render(row.label) + "  " + input.View()
}

type settingsViewEntry struct {
	row     int
	section string
}

func settingsEntries() []settingsViewEntry {
	entries := make([]settingsViewEntry, 0, len(settingsRows)*2+1)
	section := ""
	for row, setting := range settingsRows {
		if setting.section != section {
			if section != "" {
				entries = append(entries, settingsViewEntry{row: -2})
			}
			section = setting.section
			entries = append(entries, settingsViewEntry{row: -1, section: section})
		}
		entries = append(entries, settingsViewEntry{row: row})
	}
	entries = append(entries, settingsViewEntry{row: -2})
	entries = append(entries, settingsViewEntry{row: len(settingsRows)})
	return entries
}

func (m settingsModel) View() tea.View {
	if m.dialog != nil {
		view := tea.NewView(renderDecision(m.dialog.request, m.dialog.cursor, m.width))
		view.AltScreen = true
		view.WindowTitle = "Bookshelf Settings"
		return view
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E8E8E8"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#67C6C2"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	formWidth := min(88, max(30, m.width-10))
	entries := settingsEntries()
	focusEntry := 0
	for index, entry := range entries {
		if entry.row == m.cursor {
			focusEntry = index
			break
		}
	}
	available := max(5, m.height-9)
	start := max(0, focusEntry-available/2)
	end := min(len(entries), start+available)
	start = max(0, end-available)

	var visible []string
	for _, entry := range entries[start:end] {
		switch {
		case entry.row == -2:
			visible = append(visible, "")
		case entry.row == -1:
			visible = append(visible, sectionStyle.Render(entry.section))
		case entry.row == len(settingsRows):
			saveStyle := lipgloss.NewStyle().Padding(0, 2).
				Foreground(lipgloss.Color("#A78BFA")).Bold(true)
			if m.cursor == len(settingsRows) {
				saveStyle = saveStyle.Background(lipgloss.Color("#80EF80")).
					Foreground(lipgloss.Color("#111827"))
			}
			visible = append(visible, saveStyle.Render("Save Settings"))
		case settingsRows[entry.row].kind == settingText:
			visible = append(visible, m.renderTextRow(entry.row, formWidth))
		default:
			visible = append(visible, m.renderChoiceRow(entry.row, formWidth))
		}
	}
	body := titleStyle.Render("Settings") + "\n\n"
	if start > 0 {
		body += helpStyle.Render(fmt.Sprintf("↑ %d earlier setting(s)", start)) + "\n"
	}
	body += strings.Join(visible, "\n")
	if end < len(entries) {
		body += "\n" + helpStyle.Render(fmt.Sprintf("↓ %d more setting(s)", len(entries)-end))
	}
	if m.validation != "" {
		body += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(m.validation)
	}
	body += "\n\n" + helpStyle.Render("↑/↓/tab move  •  ←/→ choose  •  space select  •  enter continue/save  •  esc back")
	return fullScreenView(body, "Bookshelf Settings")
}

func RunSettingsForm(config library.Config) (library.Config, bool, error) {
	if AccessibleMode() {
		return runAccessibleSettingsForm(config)
	}
	final, err := tea.NewProgram(newSettingsModel(config)).Run()
	if err != nil {
		return library.Config{}, false, err
	}
	model, ok := final.(settingsModel)
	if !ok {
		return library.Config{}, false, fmt.Errorf("unexpected settings result %T", final)
	}
	if model.interrupted {
		return library.Config{}, false, ErrInterrupted
	}
	return model.config, model.saved, nil
}
