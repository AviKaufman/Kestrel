package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kunchenguid/firstmate/internal/firstmate"
)

type fakeSource struct {
	tasks []firstmate.Task
	live  []string
}

type deadlineCheckingSource struct{}

func (deadlineCheckingSource) Load(ctx context.Context) ([]firstmate.Task, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("missing deadline")
	}
	return sampleTasks(), nil
}

func (deadlineCheckingSource) LoadLive(ctx context.Context, _ string) ([]string, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("missing deadline")
	}
	return []string{"bounded"}, nil
}

func (source fakeSource) Load(context.Context) ([]firstmate.Task, error) {
	return source.tasks, nil
}

func (source fakeSource) LoadLive(context.Context, string) ([]string, error) {
	return source.live, nil
}

func TestModelKeyboardNavigationAndOutputSwitch(t *testing.T) {
	tasks := sampleTasks()
	model := NewModel("/fake/home", tasks, []string{"live alpha"}, fakeSource{tasks: tasks, live: []string{"live beta"}})
	model = updateModel(t, model, keyPress("j"))
	if model.Selected() != 1 {
		t.Fatalf("j selected = %d, want 1", model.Selected())
	}
	model = updateModel(t, model, specialKey(tea.KeyUp))
	if model.Selected() != 0 {
		t.Fatalf("up selected = %d, want 0", model.Selected())
	}
	model = updateModel(t, model, keyPress("G"))
	if model.Selected() != len(tasks)-1 {
		t.Fatalf("G selected = %d, want bottom", model.Selected())
	}
	model = updateModel(t, model, keyPress("g"))
	if model.Selected() != 0 {
		t.Fatalf("g selected = %d, want top", model.Selected())
	}

	model = updateModel(t, model, specialKey(tea.KeyEnter))
	if model.OutputMode() != LiveMode {
		t.Fatalf("enter mode = %v, want LiveMode", model.OutputMode())
	}
	model = updateModel(t, model, specialKey(tea.KeyEscape))
	if model.OutputMode() != ReportsMode {
		t.Fatalf("esc mode = %v, want ReportsMode", model.OutputMode())
	}
}

func TestReportsNavigationDoesNotReadLiveWorkerOutput(t *testing.T) {
	tasks := sampleTasks()
	model := NewModel("/fake/home", tasks, nil, fakeSource{tasks: tasks})

	_, command := model.Update(keyPress("j"))
	if command != nil {
		t.Fatal("Reports-mode navigation returned a live-read command")
	}
}

func TestInteractiveAdapterCommandsCarryDeadlines(t *testing.T) {
	model := NewModel("/fake/home", sampleTasks(), nil, deadlineCheckingSource{})

	updated, command := model.Update(specialKey(tea.KeyRight))
	if command == nil {
		t.Fatal("right returned nil live-read command")
	}
	message, ok := command().(liveLoadedMsg)
	if !ok {
		t.Fatalf("live command returned %T", message)
	}
	if message.err != nil {
		t.Fatalf("live command error = %v", message.err)
	}
	model = updated.(Model)
	_, command = model.Update(keyPress("r"))
	if command == nil {
		t.Fatal("r returned nil refresh command")
	}
	refresh, ok := command().(fleetLoadedMsg)
	if !ok {
		t.Fatalf("refresh command returned %T", refresh)
	}
	if refresh.err != nil {
		t.Fatalf("refresh command error = %v", refresh.err)
	}
}

func TestModelHelpRefreshAndQuitKeys(t *testing.T) {
	tasks := sampleTasks()
	model := NewModel("/fake/home", tasks, nil, fakeSource{tasks: tasks})

	model = updateModel(t, model, keyPress("?"))
	if !model.HelpVisible() {
		t.Fatal("? did not open help")
	}
	model = updateModel(t, model, specialKey(tea.KeyEscape))
	if model.HelpVisible() {
		t.Fatal("esc did not close help")
	}

	_, refresh := model.Update(keyPress("r"))
	if refresh == nil {
		t.Fatal("r returned nil refresh command")
	}
	_, quit := model.Update(keyPress("q"))
	if quit == nil {
		t.Fatal("q returned nil quit command")
	}
}

func TestModelViewUsesDominantInspectorAndFitsNarrowWidth(t *testing.T) {
	tasks := sampleTasks()
	model := NewModel("/fake/home", tasks, []string{"bounded worker line"}, fakeSource{tasks: tasks})
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 64, Height: 24})
	content := model.View().Content

	for _, expected := range []string{
		"FIRSTMATE TUI",
		"alpha",
		"CURRENT STATE",
		"REPORTS",
		"LIVE",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("view missing %q:\n%s", expected, content)
		}
	}
	for _, line := range strings.Split(content, "\n") {
		if width := lipgloss.Width(line); width > 64 {
			t.Fatalf("rendered line width = %d, want <= 64: %q", width, line)
		}
	}
	if bottomBorders := strings.Count(ansi.Strip(content), "└"); bottomBorders < 3 {
		t.Fatalf("rendered bottom borders = %d, want header, list, and inspector borders:\n%s", bottomBorders, content)
	}
}

func TestModelLiveViewLabelsEventsAsHistory(t *testing.T) {
	tasks := sampleTasks()
	model := NewModel("/fake/home", tasks, []string{"bounded worker line"}, fakeSource{tasks: tasks})
	model = updateModel(t, model, specialKey(tea.KeyRight))
	content := model.View().Content
	if !strings.Contains(content, "STATUS EVENT HISTORY") || !strings.Contains(content, "not current-state truth") {
		t.Fatalf("live view does not label historical events:\n%s", content)
	}
	if !strings.Contains(content, "WORKER CAPTURE") || !strings.Contains(content, "bounded worker line") {
		t.Fatalf("live view missing bounded worker capture:\n%s", content)
	}
}

func sampleTasks() []firstmate.Task {
	return []firstmate.Task{
		{
			Metadata: firstmate.Metadata{
				ID:       "alpha",
				Project:  "/projects/alpha",
				Kind:     "scout",
				Mode:     "local-only",
				Yolo:     "off",
				Harness:  "codex",
				Model:    "gpt-5.5",
				Effort:   "high",
				Worktree: "/worktrees/alpha",
				Window:   "firstmate:fm-alpha",
			},
			Current: firstmate.CurrentState{State: "working", Source: "pane", Detail: "harness busy"},
			Events: []firstmate.StatusEvent{
				{Verb: "working", Note: "setup complete", Raw: "working: setup complete"},
			},
			Report: firstmate.Report{Path: "/fake/home/data/alpha/report.md", Present: true, Content: "# Alpha report\n\nOfficial result."},
		},
		{
			Metadata: firstmate.Metadata{ID: "beta", Kind: "ship"},
			Current:  firstmate.CurrentState{State: "unknown", Source: "none"},
			Report:   firstmate.Report{Path: "/fake/home/data/beta/report.md"},
		},
	}
}

func keyPress(text string) tea.KeyPressMsg {
	runes := []rune(text)
	return tea.KeyPressMsg(tea.Key{Code: runes[0], Text: text})
}

func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

func updateModel(t *testing.T, model Model, message tea.Msg) Model {
	t.Helper()
	updated, _ := model.Update(message)
	result, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want tui.Model", updated)
	}
	return result
}
