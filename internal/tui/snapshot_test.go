package tui

import (
	"strings"
	"testing"
)

func TestRenderSnapshotIsStableAndComplete(t *testing.T) {
	snapshot := RenderSnapshot("/fake/home", sampleTasks(), []string{"bounded worker line"})

	for _, expected := range []string{
		"FIRSTMATE TUI SNAPSHOT",
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
}

func TestRenderSnapshotShowsEmptyFleetAndAbsentReport(t *testing.T) {
	empty := RenderSnapshot("/fake/home", nil, nil)
	if !strings.Contains(empty, "No task metadata found.") {
		t.Fatalf("empty snapshot = %q", empty)
	}

	tasks := sampleTasks()[1:]
	absent := RenderSnapshot("/fake/home", tasks, nil)
	if !strings.Contains(absent, "No durable report present at /fake/home/data/beta/report.md") {
		t.Fatalf("absent report snapshot:\n%s", absent)
	}
}
