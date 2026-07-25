package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderSnapshotIsStableAndComplete(t *testing.T) {
	hub := availableHub()
	hub.History = []string{"captain: status", "firstmate: under way"}
	snapshot := RenderSnapshot("/fake/home", hub, sampleTasks(), []string{"bounded worker line"})

	for _, expected := range []string{
		"FIRSTMATE TUI SNAPSHOT",
		"firstmate | [1 HUB] | 2 MANAGED 1 | 3 PRIVATE 1 | n new | ? help",
		"active destination: Firstmate hub",
		"Firstmate hub | send route: current primary supervisor",
		"Managed workers: 1 | send route: fm-send.sh",
		"Private Codex: 1 | send route: fm-tui-direct.sh",
		"private create: n | launch route: fm-tui-direct.sh create",
		"HUB CONVERSATION (bounded; read-only)",
		"firstmate: under way",
		"home: /fake/home",
		"tasks: 2",
		"Firstmate managed",
		"Captain private / Direct Codex",
		"> alpha",
		"  beta",
		"task id: alpha",
		"ownership: Firstmate managed",
		"current state: working",
		"current source: pane",
		"project: /projects/alpha",
		"kind: scout",
		"mode: local-only",
		"yolo: off",
		"harness: codex",
		"model: gpt-5.5",
		"effort: high",
		"worktree: /worktrees/alpha",
		"window: firstmate:fm-alpha",
		"COMPOSER",
		"send route: Firstmate managed via fm-send.sh",
		"REPORTS",
		"# Alpha report",
		"STATUS EVENT HISTORY (bounded; not current-state truth)",
		"working: setup complete",
		"WORKER CAPTURE (bounded; read-only)",
		"bounded worker line",
	} {
		if !strings.Contains(snapshot, expected) {
			t.Fatalf("snapshot missing %q:\n%s", expected, snapshot)
		}
	}
	if strings.Contains(snapshot, "\x1b[") {
		t.Fatalf("snapshot contains ANSI escapes: %q", snapshot)
	}
	if strings.Contains(snapshot, "KEYBOARD HELP") {
		t.Fatalf("snapshot includes the hidden help modal:\n%s", snapshot)
	}
}

func TestRenderSnapshotShowsEmptyFleetAndAbsentReport(t *testing.T) {
	empty := RenderSnapshot("/fake/home", availableHub(), nil, nil)
	if !strings.Contains(empty, "No task metadata found.") {
		t.Fatalf("empty snapshot = %q", empty)
	}
	for _, expected := range []string{
		"Firstmate hub | send route: current primary supervisor",
		"Managed workers: 0 | No active Firstmate-managed workers.",
		"Private Codex: 0 | No active private Codex threads.",
		"private create: n",
	} {
		if !strings.Contains(empty, expected) {
			t.Fatalf("empty snapshot missing %q:\n%s", expected, empty)
		}
	}

	tasks := sampleTasks()[1:]
	absent := RenderSnapshot("/fake/home", availableHub(), tasks, nil)
	if !strings.Contains(absent, "Direct Codex sessions have no durable Firstmate report.") {
		t.Fatalf("absent report snapshot:\n%s", absent)
	}
}

func TestRenderSnapshotSanitizesExternalTerminalControls(t *testing.T) {
	tasks := sampleTasks()
	tasks[0].Report.Content = "report \x1b[2Jkept"
	tasks[0].Events[0].Raw = "event \x1b]0;owned\x07kept"
	hub := availableHub()
	hub.History = []string{"hub \x1b[31mstyled\x1b[0m\b kept"}
	hub.Err = errors.New("failure \x1b[3Jkept")

	snapshot := RenderSnapshot("/fake/\rhome", hub, tasks, []string{"live \x1b]52;c;payload\x07kept"})
	for _, forbidden := range []string{"\x1b", "\x07", "\x08", "\r"} {
		if strings.Contains(snapshot, forbidden) {
			t.Fatalf("snapshot contains terminal control %q: %q", forbidden, snapshot)
		}
	}
	for _, expected := range []string{"report kept", "event kept", "hub styled kept", "failure kept", "live kept"} {
		if !strings.Contains(snapshot, expected) {
			t.Fatalf("snapshot lost sanitized content %q:\n%s", expected, snapshot)
		}
	}
}
