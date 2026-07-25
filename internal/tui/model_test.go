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

func (deadlineCheckingSource) LoadLive(ctx context.Context, _ firstmate.Task) ([]string, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("missing deadline")
	}
	return []string{"bounded"}, nil
}

func (deadlineCheckingSource) Send(ctx context.Context, _ firstmate.Task, _ string) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing deadline")
	}
	return nil
}

func (source fakeSource) Load(context.Context) ([]firstmate.Task, error) {
	return source.tasks, nil
}

func (source fakeSource) LoadLive(context.Context, firstmate.Task) ([]string, error) {
	return source.live, nil
}

func (source fakeSource) Send(context.Context, firstmate.Task, string) error {
	return nil
}

type composerSource struct {
	fakeSource
	sentTask    firstmate.Task
	sentMessage string
	sendErr     error
}

func (source *composerSource) Send(ctx context.Context, task firstmate.Task, message string) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing deadline")
	}
	source.sentTask = task
	source.sentMessage = message
	return source.sendErr
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

	if strings.Contains(ansi.Strip(model.View().Content), "KEYBOARD HELP") ||
		strings.Contains(ansi.Strip(model.View().Content), "k/up previous") {
		t.Fatalf("help is visible by default:\n%s", model.View().Content)
	}
	model = updateModel(t, model, keyPress("?"))
	if !model.HelpVisible() {
		t.Fatal("? did not open help")
	}
	selected := model.Selected()
	model = updateModel(t, model, keyPress("j"))
	if model.Selected() != selected {
		t.Fatal("navigation changed selection while help was open")
	}
	model = updateModel(t, model, keyPress("?"))
	if model.HelpVisible() {
		t.Fatal("? did not close help")
	}
	model = updateModel(t, model, keyPress("?"))
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

func TestHelpModalOverlaysWithoutChangingFrameGeometry(t *testing.T) {
	model := NewModel("/fake/home", sampleTasks(), nil, fakeSource{tasks: sampleTasks()})
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 92, Height: 26})
	base := model.View().Content
	model = updateModel(t, model, keyPress("?"))
	overlay := model.View().Content

	if !strings.Contains(ansi.Strip(overlay), "KEYBOARD HELP") ||
		!strings.Contains(ansi.Strip(overlay), "FOCUSED MODAL") {
		t.Fatalf("help modal lacks explicit focus:\n%s", overlay)
	}
	if lipgloss.Height(overlay) != lipgloss.Height(base) {
		t.Fatalf("help changed frame height: base=%d overlay=%d", lipgloss.Height(base), lipgloss.Height(overlay))
	}
	for _, line := range strings.Split(overlay, "\n") {
		if width := lipgloss.Width(line); width > 92 {
			t.Fatalf("help line width = %d, want <= 92: %q", width, line)
		}
	}
	if !strings.Contains(ansi.Strip(overlay), "firstmate") {
		t.Fatalf("help displaced the compact header:\n%s", overlay)
	}
}

func TestMainGeometryIsFlushBelowCompactHeader(t *testing.T) {
	model := NewModel("/fake/home", sampleTasks(), nil, fakeSource{tasks: sampleTasks()})
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 28})
	lines := strings.Split(ansi.Strip(model.View().Content), "\n")

	if len(lines) < 3 || !strings.Contains(lines[0], "firstmate") {
		t.Fatalf("first line is not the compact header:\n%s", model.View().Content)
	}
	if strings.TrimSpace(lines[1]) == "" || !strings.HasPrefix(lines[1], "┌") {
		t.Fatalf("main panes are not flush beneath header: line 1=%q", lines[1])
	}
	if strings.Contains(lines[1], "┐ ┌") {
		t.Fatalf("selector and inspector have a gap: %q", lines[1])
	}
	if strings.Contains(ansi.Strip(model.View().Content), "k/up previous") {
		t.Fatalf("permanent key legend remains visible:\n%s", model.View().Content)
	}
}

func TestComposerFocusBlocksNavigationAndRoutesSelectedWorker(t *testing.T) {
	tasks := sampleTasks()
	source := &composerSource{fakeSource: fakeSource{tasks: tasks}}
	model := NewModel("/fake/home", tasks, nil, source)

	model = updateModel(t, model, keyPress("j"))
	if model.Selected() != 1 {
		t.Fatalf("selected = %d, want private worker", model.Selected())
	}
	model = updateModel(t, model, keyPress("i"))
	if !model.ComposerFocused() {
		t.Fatal("i did not focus composer")
	}
	model = updateModel(t, model, keyPress("k"))
	if model.Selected() != 1 || model.Draft() != "k" {
		t.Fatalf("focused k changed selection or missed input: selected=%d draft=%q", model.Selected(), model.Draft())
	}
	model = updateModel(t, model, keyPress(" hello"))

	updated, command := model.Update(specialKey(tea.KeyEnter))
	if command == nil {
		t.Fatal("enter returned nil send command")
	}
	message, ok := command().(sendFinishedMsg)
	if !ok {
		t.Fatalf("send command returned %T", message)
	}
	model = updated.(Model)
	model = updateModel(t, model, message)
	if source.sentTask.Ownership != firstmate.CaptainPrivate || source.sentTask.Target != "private:beta.0" {
		t.Fatalf("sent task = %#v", source.sentTask)
	}
	if source.sentMessage != "k hello" {
		t.Fatalf("sent message = %q", source.sentMessage)
	}
	if model.Draft() != "" || !strings.Contains(model.SendStatus(), "Sent") {
		t.Fatalf("success draft=%q status=%q", model.Draft(), model.SendStatus())
	}
}

func TestComposerKeepsDraftOnValidationAndAdapterFailure(t *testing.T) {
	tasks := sampleTasks()
	source := &composerSource{
		fakeSource: fakeSource{tasks: tasks},
		sendErr:    errors.New("adapter refused target"),
	}
	model := NewModel("/fake/home", tasks, nil, source)
	model = updateModel(t, model, keyPress("i"))

	_, command := model.Update(specialKey(tea.KeyEnter))
	if command != nil {
		t.Fatal("empty draft returned a send command")
	}
	model = updateModel(t, model, specialKey(tea.KeyEnter))
	if !strings.Contains(model.SendStatus(), "empty") {
		t.Fatalf("empty status = %q", model.SendStatus())
	}

	model = updateModel(t, model, keyPress("retry this"))
	updated, command := model.Update(specialKey(tea.KeyEnter))
	if command == nil {
		t.Fatal("non-empty draft returned nil command")
	}
	model = updated.(Model)
	model = updateModel(t, model, command())
	if model.Draft() != "retry this" {
		t.Fatalf("failure cleared draft: %q", model.Draft())
	}
	if !strings.Contains(model.SendStatus(), "adapter refused target") {
		t.Fatalf("failure status = %q", model.SendStatus())
	}
	model = updateModel(t, model, specialKey(tea.KeyEscape))
	if model.ComposerFocused() || model.Draft() != "retry this" {
		t.Fatalf("esc focus=%v draft=%q", model.ComposerFocused(), model.Draft())
	}
}

func TestComposerSendCompletionSettlesAfterSelectedWorkerDisappears(t *testing.T) {
	tasks := sampleTasks()
	source := &composerSource{fakeSource: fakeSource{tasks: tasks}}
	model := NewModel("/fake/home", tasks, nil, source)
	model = updateModel(t, model, keyPress("i"))
	model = updateModel(t, model, keyPress("already sent"))

	updated, command := model.Update(specialKey(tea.KeyEnter))
	if command == nil {
		t.Fatal("enter returned nil send command")
	}
	model = updated.(Model)
	model = updateModel(t, model, fleetLoadedMsg{tasks: tasks[1:]})
	model = updateModel(t, model, command())

	if model.sending {
		t.Fatal("send completion left model in sending state after selection changed")
	}
	if model.Draft() != "" || !strings.Contains(model.SendStatus(), "Sent to alpha") {
		t.Fatalf("completion draft=%q status=%q", model.Draft(), model.SendStatus())
	}
}

func TestComposerViewIsVisibleAndOwnershipSpecific(t *testing.T) {
	tasks := sampleTasks()
	model := NewModel("/fake/home", tasks, nil, fakeSource{tasks: tasks})
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 110, Height: 34})
	content := ansi.Strip(model.View().Content)
	for _, expected := range []string{"MESSAGE / Firstmate managed", "i to focus", "REPORTS", "LIVE"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("composer view missing %q:\n%s", expected, content)
		}
	}
}

func TestPrivateReportsStateHasExplicitAbsence(t *testing.T) {
	tasks := sampleTasks()[1:]
	model := NewModel("/fake/home", tasks, nil, fakeSource{tasks: tasks})
	content := ansi.Strip(model.View().Content)
	if !strings.Contains(content, "Direct Codex sessions have no durable Firstmate report") {
		t.Fatalf("private report absence is not explicit:\n%s", content)
	}
}

func TestModelViewUsesDominantInspectorAndFitsNarrowWidth(t *testing.T) {
	tasks := sampleTasks()
	model := NewModel("/fake/home", tasks, []string{"bounded worker line"}, fakeSource{tasks: tasks})
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 64, Height: 24})
	content := model.View().Content

	for _, expected := range []string{
		"firstmate",
		"alpha",
		"Firstmate managed",
		"STATE",
		"REPORTS",
		"LIVE",
		"MESSAGE / Firstmate managed",
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
	if bottomBorders := strings.Count(ansi.Strip(content), "└"); bottomBorders < 2 {
		t.Fatalf("rendered bottom borders = %d, want list and inspector borders:\n%s", bottomBorders, content)
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
			Report:    firstmate.Report{Path: "/fake/home/data/alpha/report.md", Present: true, Content: "# Alpha report\n\nOfficial result."},
			Ownership: firstmate.FirstmateManaged,
			Target:    "alpha",
		},
		{
			Metadata:  firstmate.Metadata{ID: "beta", Kind: "ship"},
			Current:   firstmate.CurrentState{State: "unknown", Source: "none"},
			Report:    firstmate.Report{Path: "/fake/home/data/beta/report.md"},
			Ownership: firstmate.CaptainPrivate,
			Target:    "private:beta.0",
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
