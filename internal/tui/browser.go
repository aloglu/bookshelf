package tui

import (
	"fmt"
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
	ActionQuit     Action = "quit"
	ActionAdd      Action = "add"
	ActionEdit     Action = "edit"
	ActionRemove   Action = "remove"
	ActionBuild    Action = "build"
	ActionValidate Action = "validate"
	ActionCovers   Action = "covers"
)

type BrowserResult struct {
	Action Action
	IDs    []string
}

type bookItem struct {
	book     library.Book
	status   string
	selected map[string]bool
}

func (i bookItem) FilterValue() string {
	return strings.Join([]string{i.book.Title, i.book.Author, i.book.ISBN, i.book.Publisher, i.status}, " ")
}

func (i bookItem) Title() string {
	marker := "○"
	if i.selected[i.book.ID] {
		marker = "●"
	}
	return fmt.Sprintf("%s  %s", marker, i.book.Title)
}

func (i bookItem) Description() string {
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
	metadata = append(metadata, i.status)
	return strings.Join(metadata, "  ·  ")
}

type browserModel struct {
	list     list.Model
	selected map[string]bool
	result   BrowserResult
	width    int
	height   int
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
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	model := list.New(items, delegate, 80, 24)
	model.Title = fmt.Sprintf("Bookshelf  ·  %d books", len(books))
	model.SetStatusBarItemName("book", "books")
	model.DisableQuitKeybindings()
	model.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select")),
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
			key.NewBinding(key.WithKeys("e", "enter"), key.WithHelp("e", "edit")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove")),
			key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build")),
		}
	}
	return browserModel{
		list:     model,
		selected: selected,
		result:   BrowserResult{Action: ActionQuit},
	}
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
		if !m.list.SettingFilter() {
			switch msg.String() {
			case "ctrl+c", "q":
				m.result = BrowserResult{Action: ActionQuit}
				return m, tea.Quit
			case "space":
				if item, ok := m.list.SelectedItem().(bookItem); ok {
					m.selected[item.book.ID] = !m.selected[item.book.ID]
					if !m.selected[item.book.ID] {
						delete(m.selected, item.book.ID)
					}
				}
				return m, nil
			case "a":
				m.result = BrowserResult{Action: ActionAdd}
				return m, tea.Quit
			case "e", "enter":
				if item, ok := m.list.SelectedItem().(bookItem); ok {
					m.result = BrowserResult{Action: ActionEdit, IDs: []string{item.book.ID}}
					return m, tea.Quit
				}
			case "d":
				ids := m.selectedIDs()
				if len(ids) == 0 {
					if item, ok := m.list.SelectedItem().(bookItem); ok {
						ids = []string{item.book.ID}
					}
				}
				if len(ids) > 0 {
					m.result = BrowserResult{Action: ActionRemove, IDs: ids}
					return m, tea.Quit
				}
			case "b":
				m.result = BrowserResult{Action: ActionBuild}
				return m, tea.Quit
			case "v":
				m.result = BrowserResult{Action: ActionValidate}
				return m, tea.Quit
			case "c":
				ids := m.selectedIDs()
				if len(ids) == 0 {
					if item, ok := m.list.SelectedItem().(bookItem); ok {
						ids = []string{item.book.ID}
					}
				}
				m.result = BrowserResult{Action: ActionCovers, IDs: ids}
				return m, tea.Quit
			}
		}
	}
	var command tea.Cmd
	m.list, command = m.list.Update(msg)
	return m, command
}

func (m browserModel) View() tea.View {
	content := m.list.View()
	if len(m.selected) > 0 {
		content += "\n" + lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Render(fmt.Sprintf("%d selected", len(m.selected)))
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Bookshelf"
	return view
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
	if len(books) == 0 {
		return BrowserResult{Action: ActionAdd}, nil
	}
	final, err := tea.NewProgram(newBrowserModel(books, statuses)).Run()
	if err != nil {
		return BrowserResult{}, err
	}
	model, ok := final.(browserModel)
	if !ok {
		return BrowserResult{}, fmt.Errorf("unexpected browser result %T", final)
	}
	return model.result, nil
}

func IsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
