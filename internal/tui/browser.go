package tui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aloglu/bookshelf/internal/library"
)

type Action string

const (
	ActionQuit   Action = "quit"
	ActionSelect Action = "select"
)

type BrowserResult struct {
	Action Action
	IDs    []string
}

type bookItem struct {
	book            library.Book
	status          string
	selected        map[string]bool
	showCoverStatus bool
}

func (i bookItem) FilterValue() string {
	return strings.Join([]string{i.book.Title, i.book.Author, i.book.ISBN, i.book.Slug, i.book.Publisher, i.status}, " ")
}

func (i bookItem) Title() string {
	return i.book.Title
}

func (i bookItem) Description() string {
	return strings.Join(i.descriptionParts(), " · ")
}

func (i bookItem) descriptionParts() []string {
	author := i.book.Author
	if author == "" {
		author = "Unknown author"
	}
	metadata := []string{author}
	if year := i.book.Year(); year != "" {
		metadata = append(metadata, year)
	}
	if i.book.ISBN != "" {
		metadata = append(metadata, i.book.ISBN)
	}
	if i.status != "" {
		metadata = append(metadata, i.status)
	}
	if i.showCoverStatus {
		if i.book.Cover == "" {
			metadata = append(metadata, "✕")
		} else {
			metadata = append(metadata, "✓")
		}
	}
	return metadata
}

type browserModel struct {
	list        list.Model
	selected    map[string]bool
	result      BrowserResult
	width       int
	height      int
	selecting   bool
	multi       bool
	pageTitle   string
	interrupted bool
}

func newBrowserModel(books []library.Book, statuses map[string]string) browserModel {
	sort.SliceStable(books, func(i, j int) bool {
		return strings.ToLower(books[i].Title) < strings.ToLower(books[j].Title)
	})
	selected := make(map[string]bool)
	items := make([]list.Item, 0, len(books))
	for _, book := range books {
		items = append(items, bookItem{book: book, status: statuses[book.ID], selected: selected})
	}
	delegate := newBookDelegate(false, false)
	model := list.New(items, delegate, 80, 24)
	model.SetShowTitle(false)
	model.SetShowStatusBar(false)
	model.DisableQuitKeybindings()
	model.Filter = wordFilter
	model.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back/quit")),
		}
	}
	return browserModel{
		list:      model,
		selected:  selected,
		pageTitle: "Bookshelf · List",
		result:    BrowserResult{Action: ActionQuit},
	}
}

func newBookSelectorModel(books []library.Book, statuses map[string]string, initial []string, title string, multi, showCoverStatus bool) browserModel {
	model := newBrowserModel(books, statuses)
	model.selecting = true
	model.multi = multi
	model.pageTitle = title
	if showCoverStatus {
		items := model.list.Items()
		for index, item := range items {
			book := item.(bookItem)
			book.showCoverStatus = true
			items[index] = book
		}
		model.list.SetItems(items)
	}
	model.list.SetDelegate(newBookDelegate(true, multi))
	for _, id := range initial {
		model.selected[id] = true
	}
	model.list.AdditionalShortHelpKeys = func() []key.Binding {
		bindings := []key.Binding{
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
		if multi {
			bindings = append(bindings, key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "select all")))
		}
		return bindings
	}
	return model
}

func (m browserModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m browserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.list.Styles = list.DefaultStyles(msg.IsDark())
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, max(10, msg.Height-2))
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.interrupted = true
			return m, tea.Quit
		}
		if !m.list.SettingFilter() {
			if m.selecting {
				switch msg.String() {
				case "q", "esc":
					m.result = BrowserResult{Action: ActionQuit}
					return m, tea.Quit
				case "space":
					if m.multi {
						m.toggleCurrent()
					}
					return m, nil
				case "a":
					if m.multi {
						for _, item := range m.list.Items() {
							if book, ok := item.(bookItem); ok {
								m.selected[book.book.ID] = true
							}
						}
					}
					return m, nil
				case "enter":
					ids := m.selectedIDs()
					if !m.multi || len(ids) == 0 {
						if item, ok := m.list.SelectedItem().(bookItem); ok {
							ids = []string{item.book.ID}
						}
					}
					m.result = BrowserResult{Action: ActionSelect, IDs: ids}
					return m, tea.Quit
				}
			}
			switch msg.String() {
			case "q", "esc":
				m.result = BrowserResult{Action: ActionQuit}
				return m, tea.Quit
			}
		}
	}
	var command tea.Cmd
	m.list, command = m.list.Update(msg)
	return m, command
}

func (m *browserModel) toggleCurrent() {
	if item, ok := m.list.SelectedItem().(bookItem); ok {
		m.selected[item.book.ID] = !m.selected[item.book.ID]
		if !m.selected[item.book.ID] {
			delete(m.selected, item.book.ID)
		}
	}
}

func (m browserModel) View() tea.View {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E8E8E8")).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#80EF80")).Bold(true)
	countLabel := "Books"
	if len(m.list.Items()) == 1 {
		countLabel = "Book"
	}
	header := titleStyle.Render(m.pageTitle) +
		metaStyle.Render(fmt.Sprintf(" · %d %s", len(m.list.Items()), countLabel))
	if m.selecting && m.multi {
		header += metaStyle.Render(" · ") +
			selectedStyle.Render(fmt.Sprintf("%d Selected", len(m.selected)))
	}
	content := lipgloss.NewStyle().PaddingLeft(2).Render(header) + browserHeaderGap(m.list.FilterState()) + m.list.View()
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Bookshelf"
	return view
}

func browserHeaderGap(filterState list.FilterState) string {
	if filterState != list.Unfiltered {
		return "\n\n"
	}
	return "\n"
}

type bookDelegate struct {
	base list.DefaultDelegate
}

func (d bookDelegate) Height() int  { return d.base.Height() }
func (d bookDelegate) Spacing() int { return d.base.Spacing() }
func (d bookDelegate) Update(msg tea.Msg, model *list.Model) tea.Cmd {
	return d.base.Update(msg, model)
}
func (d bookDelegate) ShortHelp() []key.Binding  { return d.base.ShortHelp() }
func (d bookDelegate) FullHelp() [][]key.Binding { return d.base.FullHelp() }
func (d bookDelegate) Render(writer io.Writer, model list.Model, index int, item list.Item) {
	book, ok := item.(bookItem)
	if !ok {
		return
	}
	marker := "  "
	if book.selected[book.book.ID] {
		marker = lipgloss.NewStyle().Foreground(lipgloss.Color("#80EF80")).Bold(true).Render("✓ ")
	}
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8D8D8"))
	descriptionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	cursor := "  "
	if index == model.Index() {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#8B5CF6")).Render("│ ")
		titleStyle = titleStyle.Foreground(lipgloss.Color("#A78BFA")).Bold(true)
		descriptionStyle = descriptionStyle.Foreground(lipgloss.Color("#8B7CB8"))
	}
	available := max(8, model.Width()-4)
	title := truncateListText(book.book.Title, max(4, available-2))
	description := renderBookDescription(book, available, descriptionStyle)
	fmt.Fprintf(writer, "%s%s%s\n%s  %s",
		cursor, marker, titleStyle.Render(title),
		cursor, description)
}

func renderBookDescription(book bookItem, width int, base lipgloss.Style) string {
	plain := book.Description()
	if lipgloss.Width(plain) > width {
		return base.Render(truncateListText(plain, width))
	}
	parts := book.descriptionParts()
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		style := base
		switch part {
		case library.PublicationNotPublished:
			style = style.Foreground(lipgloss.Color("#F59E0B"))
		case library.PublicationChangesNotPublished:
			style = style.Foreground(lipgloss.Color("#EF4444"))
		case "✓":
			style = style.Foreground(lipgloss.Color("#80EF80")).Bold(true)
		case "✕":
			style = style.Foreground(lipgloss.Color("#EF4444")).Bold(true)
		}
		rendered = append(rendered, style.Render(part))
	}
	return strings.Join(rendered, base.Render(" · "))
}

func wordFilter(term string, targets []string) []list.Rank {
	words := strings.Fields(strings.ToLower(term))
	if len(words) == 0 {
		ranks := make([]list.Rank, len(targets))
		for index := range targets {
			ranks[index] = list.Rank{Index: index}
		}
		return ranks
	}
	ranks := make([]list.Rank, 0, len(targets))
	for index, target := range targets {
		searchable := strings.ToLower(target)
		matches := true
		for _, word := range words {
			if !strings.Contains(searchable, word) {
				matches = false
				break
			}
		}
		if matches {
			ranks = append(ranks, list.Rank{Index: index})
		}
	}
	return ranks
}

func truncateListText(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	var result strings.Builder
	for _, character := range value {
		candidate := result.String() + string(character)
		if lipgloss.Width(candidate) >= width {
			break
		}
		result.WriteRune(character)
	}
	return result.String() + "…"
}

func newBookDelegate(selecting, multi bool) bookDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(1)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("#A78BFA"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("#8B7CB8"))
	var bindings []key.Binding
	if selecting && multi {
		bindings = []key.Binding{
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "continue")),
		}
	} else if selecting {
		bindings = []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit")),
		}
	}
	delegate.ShortHelpFunc = func() []key.Binding { return bindings }
	delegate.FullHelpFunc = func() [][]key.Binding { return [][]key.Binding{bindings} }
	return bookDelegate{base: delegate}
}

func (m browserModel) selectedIDs() []string {
	ids := make([]string, 0, len(m.selected))
	for id := range m.selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func RunBrowser(books []library.Book, statuses map[string]string) (BrowserResult, error) {
	final, err := tea.NewProgram(newBrowserModel(books, statuses)).Run()
	if err != nil {
		return BrowserResult{}, err
	}
	model, ok := final.(browserModel)
	if !ok {
		return BrowserResult{}, fmt.Errorf("unexpected browser result %T", final)
	}
	if model.interrupted {
		return BrowserResult{}, ErrInterrupted
	}
	return model.result, nil
}

func RunBookSelector(books []library.Book, statuses map[string]string, initial []string, title string, multi, showCoverStatus bool) ([]string, bool, error) {
	if len(books) == 0 {
		return nil, false, nil
	}
	final, err := tea.NewProgram(newBookSelectorModel(books, statuses, initial, title, multi, showCoverStatus)).Run()
	if err != nil {
		return nil, false, err
	}
	model, ok := final.(browserModel)
	if !ok {
		return nil, false, fmt.Errorf("unexpected selector result %T", final)
	}
	if model.interrupted {
		return nil, false, ErrInterrupted
	}
	return model.result.IDs, model.result.Action == ActionSelect, nil
}

func IsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
