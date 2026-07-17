package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type DecisionTone int

const (
	DecisionNormal DecisionTone = iota
	DecisionDanger
)

type DecisionOption struct {
	ID    string
	Label string
	Tone  DecisionTone
}

type DecisionRequest struct {
	Title       string
	Description string
	Options     []DecisionOption
	Default     int
	EscapeLabel string
	Vertical    bool
	Borderless  bool
}

type decisionModel struct {
	request     DecisionRequest
	cursor      int
	width       int
	result      string
	chosen      bool
	interrupted bool
}

func newDecisionModel(request DecisionRequest) decisionModel {
	cursor := request.Default
	if cursor < 0 || cursor >= len(request.Options) {
		cursor = len(request.Options) - 1
	}
	return decisionModel{request: request, cursor: max(0, cursor), width: 72}
}

func (m decisionModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m decisionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.interrupted = true
			return m, tea.Quit
		}
		choice, done, dismissed := m.handleKey(msg.String())
		if done {
			m.result = choice
			m.chosen = !dismissed
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *decisionModel) handleKey(key string) (choice string, done, dismissed bool) {
	switch key {
	case "up", "left", "shift+tab", "k", "h":
		if len(m.request.Options) > 0 {
			m.cursor = max(0, m.cursor-1)
		}
	case "down", "right", "tab", "j", "l":
		if len(m.request.Options) > 0 {
			m.cursor = min(len(m.request.Options)-1, m.cursor+1)
		}
	case "enter":
		if len(m.request.Options) > 0 {
			return m.request.Options[m.cursor].ID, true, false
		}
	case "esc", "q":
		return "", true, true
	}
	return "", false, false
}

func (m decisionModel) View() tea.View {
	view := tea.NewView(renderDecision(m.request, m.cursor, m.width))
	view.AltScreen = true
	view.WindowTitle = "Bookshelf"
	return view
}

func renderDecision(request DecisionRequest, cursor, width int) string {
	dialogWidth := min(68, max(38, width-6))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E8E8E8"))
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B7B7B7")).Width(dialogWidth - 6)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8D8D8"))
	selectedStyle := normalStyle.Bold(true).Foreground(lipgloss.Color("#A78BFA"))
	dangerStyle := normalStyle.Foreground(lipgloss.Color("#F87171"))
	selectedDangerStyle := dangerStyle.Bold(true)
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))

	var content strings.Builder
	content.WriteString(titleStyle.Render(request.Title))
	if strings.TrimSpace(request.Description) != "" {
		content.WriteString("\n\n")
		content.WriteString(bodyStyle.Render(request.Description))
	}
	content.WriteString("\n\n")
	if request.Vertical {
		for index, option := range request.Options {
			prefix := "  "
			style := normalStyle
			if option.Tone == DecisionDanger {
				style = dangerStyle
			}
			if index == cursor {
				prefix = "› "
				if option.Tone == DecisionDanger {
					style = selectedDangerStyle
				} else {
					style = selectedStyle
				}
			}
			content.WriteString(style.Render(prefix + option.Label))
			content.WriteByte('\n')
		}
	} else {
		buttons := make([]string, 0, len(request.Options))
		for index, option := range request.Options {
			style := lipgloss.NewStyle().
				Padding(0, 2).
				Foreground(lipgloss.Color("#D8D8D8")).
				Background(lipgloss.Color("#292932"))
			if option.Tone == DecisionDanger {
				style = style.Foreground(lipgloss.Color("#FCA5A5"))
			}
			if index == cursor {
				color := lipgloss.Color("#8B5CF6")
				if option.Tone == DecisionDanger {
					color = lipgloss.Color("#EF4444")
				}
				style = style.Background(color).Foreground(lipgloss.Color("#111827")).Bold(true)
			}
			buttons = append(buttons, style.Render(option.Label))
		}
		content.WriteString(strings.Join(buttons, "  "))
		content.WriteByte('\n')
	}
	content.WriteString("\n")
	escapeLabel := request.EscapeLabel
	if escapeLabel == "" {
		escapeLabel = "cancel"
	}
	directions := "←/→"
	if request.Vertical {
		directions = "↑/↓"
	}
	content.WriteString(helpStyle.Render(directions + " choose  •  enter confirm  •  esc " + escapeLabel))

	rendered := strings.TrimRight(content.String(), "\n")
	if request.Borderless {
		return renderBorderlessPanel(rendered)
	}
	return renderDialogBox(rendered, dialogWidth)
}

func renderDialogBox(content string, width int) string {
	box := lipgloss.NewStyle().
		Width(width).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5B5B66")).
		Render(content)
	return fmt.Sprintf("\n%s", box)
}

func renderBorderlessPanel(content string) string {
	return "\n" + lipgloss.NewStyle().Padding(1, 3).Render(content)
}

func RunDecision(request DecisionRequest) (choice string, chosen bool, err error) {
	if len(request.Options) == 0 {
		return "", false, fmt.Errorf("decision requires at least one option")
	}
	final, err := tea.NewProgram(newDecisionModel(request)).Run()
	if err != nil {
		return "", false, err
	}
	model, ok := final.(decisionModel)
	if !ok {
		return "", false, fmt.Errorf("unexpected decision result %T", final)
	}
	if model.interrupted {
		return "", false, ErrInterrupted
	}
	return model.result, model.chosen, nil
}
