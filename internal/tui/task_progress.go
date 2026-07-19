package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type TaskProgress struct {
	Phase   string
	Current int
	Total   int
	Unit    string
}

type ProgressReporter func(TaskProgress)

type taskProgressResult[T any] struct {
	value T
	err   error
}

type taskProgressUpdate struct {
	progress TaskProgress
}

type taskProgressModel[T any] struct {
	cancel      context.CancelFunc
	operation   func(context.Context, ProgressReporter) (T, error)
	context     context.Context
	events      chan tea.Msg
	progress    TaskProgress
	spinner     spinner.Model
	result      T
	err         error
	interrupted bool
}

func newTaskProgressModel[T any](
	ctx context.Context,
	initial TaskProgress,
	operation func(context.Context, ProgressReporter) (T, error),
) *taskProgressModel[T] {
	taskContext, cancel := context.WithCancel(ctx)
	indicator := spinner.New()
	indicator.Spinner = spinner.Dot
	return &taskProgressModel[T]{
		cancel:    cancel,
		operation: operation,
		context:   taskContext,
		events:    make(chan tea.Msg, 64),
		progress:  normalizeTaskProgress(initial),
		spinner:   indicator,
	}
}

func normalizeTaskProgress(progress TaskProgress) TaskProgress {
	progress.Phase = strings.TrimSpace(progress.Phase)
	progress.Unit = strings.TrimSpace(progress.Unit)
	if progress.Current < 0 {
		progress.Current = 0
	}
	if progress.Total < 0 {
		progress.Total = 0
	}
	if progress.Total > 0 && progress.Current > progress.Total {
		progress.Current = progress.Total
	}
	return progress
}

func (model *taskProgressModel[T]) Init() tea.Cmd {
	return tea.Batch(model.spinner.Tick, model.runOperation(), model.waitForEvent())
}

func (model *taskProgressModel[T]) runOperation() tea.Cmd {
	return func() tea.Msg {
		report := func(progress TaskProgress) {
			select {
			case model.events <- taskProgressUpdate{progress: normalizeTaskProgress(progress)}:
			case <-model.context.Done():
			}
		}
		value, err := model.operation(model.context, report)
		select {
		case model.events <- taskProgressResult[T]{value: value, err: err}:
		case <-model.context.Done():
			model.events <- taskProgressResult[T]{value: value, err: err}
		}
		return nil
	}
}

func (model *taskProgressModel[T]) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		return <-model.events
	}
}

func (model *taskProgressModel[T]) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case spinner.TickMsg:
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(message)
		return model, command
	case tea.KeyPressMsg:
		switch message.String() {
		case "esc", "ctrl+c":
			if !model.interrupted {
				model.interrupted = true
				model.progress = TaskProgress{Phase: "Cancelling…"}
				model.cancel()
			}
		}
	case taskProgressUpdate:
		if !model.interrupted {
			model.progress = message.progress
		}
		return model, model.waitForEvent()
	case taskProgressResult[T]:
		model.result = message.value
		model.err = message.err
		return model, tea.Quit
	}
	return model, nil
}

func (model *taskProgressModel[T]) View() tea.View {
	spinnerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#875FFF"))
	phaseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8D8D8"))
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))

	phase := model.progress.Phase
	if phase == "" {
		phase = "Working…"
	}
	content := spinnerStyle.Render(model.spinner.View()) + " " + phaseStyle.Render(phase)
	if model.progress.Total > 0 {
		count := fmt.Sprintf("%d / %d", model.progress.Current, model.progress.Total)
		if model.progress.Unit != "" {
			count += " " + model.progress.Unit
		}
		content += countStyle.Render(" · " + count)
	}

	view := tea.NewView(content)
	view.WindowTitle = "Bookshelf"
	return view
}

func RunProgress[T any](
	ctx context.Context,
	initial TaskProgress,
	operation func(context.Context, ProgressReporter) (T, error),
) (T, error) {
	var zero T
	if operation == nil {
		return zero, fmt.Errorf("progress operation is required")
	}
	initial = normalizeTaskProgress(initial)
	if !IsTerminal() || AccessibleMode() {
		lastPhase := ""
		report := func(progress TaskProgress) {
			progress = normalizeTaskProgress(progress)
			if progress.Phase == "" || progress.Phase == lastPhase {
				return
			}
			fmt.Fprintln(os.Stderr, progress.Phase)
			lastPhase = progress.Phase
		}
		report(initial)
		return operation(ctx, report)
	}

	model := newTaskProgressModel(ctx, initial, operation)
	defer model.cancel()
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return zero, err
	}
	result, ok := final.(*taskProgressModel[T])
	if !ok {
		return zero, fmt.Errorf("unexpected progress result %T", final)
	}
	if result.interrupted {
		if closer, ok := any(result.result).(io.Closer); ok {
			_ = closer.Close()
		}
		return zero, ErrInterrupted
	}
	return result.result, result.err
}
