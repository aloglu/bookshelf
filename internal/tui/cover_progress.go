package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aloglu/bookshelf/internal/library"
)

type coverFetchedMsg struct {
	outcome  library.CoverFetchOutcome
	canceled bool
}

type coverFinishedMsg struct {
	summary     library.CoverFetchSummary
	kept        bool
	reportPath  string
	reportCount int
	err         error
}

type coverProgressModel struct {
	parent      context.Context
	session     *library.CoverFetchSession
	source      library.CoverSource
	books       []library.Book
	index       int
	width       int
	spinner     spinner.Model
	cancel      context.CancelFunc
	inFlight    bool
	dialog      *decisionModel
	stopAction  string
	summary     library.CoverFetchSummary
	kept        bool
	completed   bool
	back        bool
	reportPath  string
	reportCount int
	err         error
	interrupted bool
}

func newCoverProgressModel(ctx context.Context, session *library.CoverFetchSession, source library.CoverSource) *coverProgressModel {
	indicator := spinner.New()
	indicator.Spinner = spinner.Dot
	return &coverProgressModel{
		parent:  ctx,
		session: session,
		source:  source,
		books:   session.Books(),
		width:   80,
		spinner: indicator,
	}
}

func (m *coverProgressModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.startNext())
}

func (m *coverProgressModel) startNext() tea.Cmd {
	if m.index >= len(m.books) {
		return m.finish(true)
	}
	requestContext, cancel := context.WithCancel(m.parent)
	m.cancel = cancel
	m.inFlight = true
	index := m.index
	return func() tea.Msg {
		outcome := m.session.Fetch(requestContext, index, m.source)
		return coverFetchedMsg{outcome: outcome, canceled: requestContext.Err() != nil}
	}
}

func (m *coverProgressModel) finish(keep bool) tea.Cmd {
	m.inFlight = true
	return func() tea.Msg {
		if keep {
			summary, err := m.session.Commit()
			if err != nil {
				return coverFinishedMsg{summary: summary, kept: true, err: err}
			}
			reportPath, reportCount, err := m.session.WriteReport()
			return coverFinishedMsg{
				summary: summary, kept: true, reportPath: reportPath,
				reportCount: reportCount, err: err,
			}
		}
		err := m.session.Discard()
		return coverFinishedMsg{kept: false, err: err}
	}
}

func (m *coverProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		if m.dialog != nil {
			m.dialog.width = msg.Width
		}
	case spinner.TickMsg:
		if m.completed {
			return m, nil
		}
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(msg)
		return m, command
	case tea.KeyPressMsg:
		if m.completed {
			switch msg.String() {
			case "ctrl+c":
				m.interrupted = true
				return m, tea.Quit
			case "esc":
				m.back = true
				return m, tea.Quit
			case "enter":
				return m, tea.Quit
			}
			return m, nil
		}
		if m.dialog != nil {
			if msg.String() == "ctrl+c" {
				m.interrupted = true
				if m.cancel != nil {
					m.cancel()
				}
				m.stopAction = "discard"
				if !m.inFlight {
					return m, m.finish(false)
				}
				return m, nil
			}
			choice, done, dismissed := m.dialog.handleKey(msg.String())
			if !done {
				return m, nil
			}
			if dismissed || choice == "continue" {
				m.dialog = nil
				m.stopAction = ""
				if m.parent.Err() != nil {
					m.parent = context.Background()
				}
				if !m.inFlight {
					return m, m.startNext()
				}
				return m, nil
			}
			m.dialog = nil
			m.stopAction = choice
			if !m.inFlight {
				return m, m.finish(choice == "keep")
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			m.interrupted = true
			if m.cancel != nil {
				m.cancel()
			}
			m.stopAction = "discard"
			if !m.inFlight {
				return m, m.finish(false)
			}
			return m, nil
		case "esc", "q":
			if m.cancel != nil {
				m.cancel()
			}
			request := DecisionRequest{
				Title:       "Stop Fetching Covers?",
				Description: fmt.Sprintf("%d cover(s) fetched during this session.", m.session.Summary().Downloaded),
				Options: []DecisionOption{
					{ID: "keep", Label: "Keep + Stop"},
					{ID: "discard", Label: "Discard + Stop", Tone: DecisionDanger},
					{ID: "continue", Label: "Continue"},
				},
				Default: 2,
			}
			dialog := newDecisionModel(request)
			dialog.width = m.width
			m.dialog = &dialog
			return m, nil
		}
	case coverFetchedMsg:
		m.inFlight = false
		if !msg.canceled {
			m.session.Record(msg.outcome)
			m.index++
		}
		if m.stopAction != "" {
			return m, m.finish(m.stopAction == "keep")
		}
		if m.dialog != nil {
			return m, nil
		}
		return m, m.startNext()
	case coverFinishedMsg:
		m.inFlight = false
		m.summary = msg.summary
		m.kept = msg.kept
		m.completed = true
		m.reportPath = msg.reportPath
		m.reportCount = msg.reportCount
		m.err = msg.err
		if msg.err != nil || !msg.kept {
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

func (m *coverProgressModel) View() tea.View {
	if m.dialog != nil {
		view := tea.NewView(renderDecision(m.dialog.request, m.dialog.cursor, m.width))
		view.AltScreen = true
		view.WindowTitle = "Bookshelf Covers"
		return view
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E8E8E8"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8B8B96"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8D8D8"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	book := library.Book{}
	if len(m.books) > 0 {
		index := min(m.index, len(m.books)-1)
		book = m.books[index]
	}
	summary := m.session.Summary()
	var content strings.Builder
	title := "Fetching Covers"
	if m.completed {
		title = "Cover Fetch Complete"
	}
	content.WriteString(titleStyle.Render(title))
	content.WriteString("\n\n")
	content.WriteString(renderCoverProgress(m.session.Outcomes(), len(m.books), min(64, max(24, m.width-12))))
	processed := m.index
	if m.completed {
		processed = len(m.books)
	}
	content.WriteString(fmt.Sprintf("  %d / %d", processed, len(m.books)))
	content.WriteString("\n\n")
	if m.completed {
		content.WriteString(valueStyle.Render(fmt.Sprintf("%d book(s) processed.", m.summary.Total)))
		if m.summary.Downloaded > 0 {
			content.WriteString("\n")
			content.WriteString(labelStyle.Render("Covers saved in: "))
			content.WriteString(valueStyle.Render(m.session.CoverDirectory()))
		}
		if m.reportCount > 0 {
			content.WriteString("\n")
			content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).
				Render(fmt.Sprintf("%d book(s) need attention.", m.reportCount)))
			content.WriteString("\n")
			content.WriteString(labelStyle.Render("Report: "))
			content.WriteString(valueStyle.Render(m.reportPath))
		}
		content.WriteString("\n\n")
	} else if m.inFlight && m.index < len(m.books) {
		content.WriteString(m.spinner.View())
		content.WriteString(" ")
	}
	if !m.completed {
		content.WriteString(labelStyle.Render("Current: "))
		content.WriteString(valueStyle.Render(book.Title))
	}
	if !m.completed && book.Author != "" {
		content.WriteString("\n")
		content.WriteString(labelStyle.Render("Author:  "))
		content.WriteString(valueStyle.Render(book.Author))
	}
	if !m.completed {
		content.WriteString("\n")
		content.WriteString(labelStyle.Render("Source:  "))
		content.WriteString(valueStyle.Render(library.CoverSourceLabel(m.source)))
		content.WriteString("\n\n")
	}
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#8B5CF6")).Render(fmt.Sprintf("Downloaded %d", summary.Downloaded)))
	content.WriteString("  ·  ")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#737373")).Render(fmt.Sprintf("Skipped %d", summary.Skipped)))
	content.WriteString("  ·  ")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render(fmt.Sprintf("Not found %d", summary.NotFound)))
	content.WriteString("  ·  ")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(fmt.Sprintf("Failed %d", summary.Failed)))
	content.WriteString("\n\n")
	if m.completed {
		content.WriteString(helpStyle.Render("enter done  •  esc back"))
	} else {
		content.WriteString(helpStyle.Render("esc cancel"))
	}

	view := tea.NewView("\n" + lipgloss.NewStyle().Padding(1, 3).Render(content.String()))
	view.AltScreen = true
	view.WindowTitle = "Bookshelf Covers"
	return view
}

func renderCoverProgress(outcomes []library.CoverFetchOutcome, total, width int) string {
	if total <= 0 {
		return strings.Repeat("░", width)
	}
	var result strings.Builder
	for cell, status := range coverProgressCells(outcomes, total, width) {
		event := cell * total / width
		if event >= len(outcomes) {
			result.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563")).Render("░"))
			continue
		}
		color := "#4B5563"
		switch status {
		case library.CoverFetchDownloaded:
			color = "#8B5CF6"
		case library.CoverFetchSkipped:
			color = "#737373"
		case library.CoverFetchNotFound:
			color = "#F59E0B"
		case library.CoverFetchFailed:
			color = "#EF4444"
		}
		result.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("█"))
	}
	return result.String()
}

func coverProgressCells(outcomes []library.CoverFetchOutcome, total, width int) []library.CoverFetchStatus {
	cells := make([]library.CoverFetchStatus, width)
	for cell := range cells {
		event := cell * total / width
		if event < len(outcomes) {
			cells[cell] = outcomes[event].Status
		}
	}
	return cells
}

func RunCoverProgress(ctx context.Context, session *library.CoverFetchSession, source library.CoverSource) (library.CoverFetchSummary, bool, bool, error) {
	model := newCoverProgressModel(ctx, session, source)
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		_ = session.Discard()
		return library.CoverFetchSummary{}, false, false, err
	}
	result, ok := final.(*coverProgressModel)
	if !ok {
		_ = session.Discard()
		return library.CoverFetchSummary{}, false, false, fmt.Errorf("unexpected cover progress result %T", final)
	}
	if result.interrupted {
		return library.CoverFetchSummary{}, false, false, ErrInterrupted
	}
	return result.summary, result.kept, result.back, result.err
}
