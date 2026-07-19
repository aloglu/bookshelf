package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTaskProgressEscapeCancelsOperation(t *testing.T) {
	model := newTaskProgressModel(
		context.Background(),
		TaskProgress{Phase: "Restoring library"},
		func(ctx context.Context, _ ProgressReporter) (int, error) {
			return 0, ctx.Err()
		},
	)
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	result := updated.(*taskProgressModel[int])
	if !result.interrupted || result.context.Err() == nil {
		t.Fatal("Escape did not cancel the progress operation")
	}
	if !strings.Contains(result.View().Content, "Cancelling") {
		t.Fatalf("cancel view = %q", result.View().Content)
	}
}

func TestTaskProgressRendersInlinePhaseCounter(t *testing.T) {
	model := newTaskProgressModel(
		context.Background(),
		TaskProgress{Phase: "Checking archive"},
		func(context.Context, ProgressReporter) (int, error) { return 0, nil },
	)
	updated, _ := model.Update(taskProgressUpdate{progress: TaskProgress{
		Phase: "Restoring library", Current: 526, Total: 795, Unit: "files",
	}})
	view := updated.(*taskProgressModel[int]).View()
	if view.AltScreen {
		t.Fatal("task progress unexpectedly uses the alternate screen")
	}
	if !strings.Contains(view.Content, "Restoring library") || !strings.Contains(view.Content, "526 / 795 files") {
		t.Fatalf("progress view = %q", view.Content)
	}
}

func TestRunProgressUsesPlainPathInAccessibleMode(t *testing.T) {
	t.Setenv("BOOKSHELF_ACCESSIBLE", "1")
	result, err := RunProgress(
		context.Background(),
		TaskProgress{Phase: "Checking archive"},
		func(_ context.Context, report ProgressReporter) (int, error) {
			report(TaskProgress{Phase: "Restoring library", Current: 1, Total: 2, Unit: "files"})
			return 42, nil
		},
	)
	if err != nil || result != 42 {
		t.Fatalf("result = %d, error = %v", result, err)
	}
}
