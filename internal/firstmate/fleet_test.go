package firstmate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeStateResolver struct {
	states map[string]CurrentState
	err    error
}

func (resolver fakeStateResolver) Resolve(_ context.Context, taskID string) (CurrentState, error) {
	if resolver.err != nil {
		return CurrentState{}, resolver.err
	}
	return resolver.states[taskID], nil
}

type fakeLiveReader struct {
	lines []string
	err   error
}

func (reader fakeLiveReader) Read(_ context.Context, _ string) ([]string, error) {
	return reader.lines, reader.err
}

func TestLoaderJoinsMetadataCurrentStateEventsAndReport(t *testing.T) {
	home := fixtureHome(t)
	writeFile(t, filepath.Join(home.StateDir, "demo.meta"), strings.Join([]string{
		"project=/projects/demo",
		"kind=scout",
		"mode=local-only",
		"yolo=off",
		"harness=codex",
		"model=gpt-5.5",
		"effort=high",
		"worktree=/worktrees/demo",
		"window=firstmate:fm-demo",
	}, "\n")+"\n")
	writeFile(t, filepath.Join(home.StateDir, "demo.status"), "needs-decision: historical gate\nresolved: captain chose A\n")
	reportPath := filepath.Join(home.DataDir, "demo", "report.md")
	writeFile(t, reportPath, "# Official result\n\nThe durable report wins.\n")

	loader := Loader{
		Home: home,
		States: fakeStateResolver{states: map[string]CurrentState{
			"demo": {State: "working", Source: "pane", Detail: "harness busy"},
		}},
		Live:           fakeLiveReader{lines: []string{"bounded live line"}},
		StatusMaxLines: 20,
		StatusMaxBytes: 64 * 1024,
		ReportMaxBytes: 64 * 1024,
	}
	tasks, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("Load() task count = %d, want 1", len(tasks))
	}
	task := tasks[0]
	if task.Current.State != "working" || task.Current.Source != "pane" {
		t.Fatalf("task.Current = %#v, want adapter state", task.Current)
	}
	if task.Events[len(task.Events)-1].Verb != "resolved" {
		t.Fatalf("task.Events = %#v, want historical resolved event", task.Events)
	}
	if !task.Report.Present || task.Report.Path != reportPath || !strings.Contains(task.Report.Content, "durable report") {
		t.Fatalf("task.Report = %#v", task.Report)
	}
	if task.Metadata.Window != "firstmate:fm-demo" {
		t.Fatalf("task.Metadata.Window = %q", task.Metadata.Window)
	}
	live, err := loader.LoadLive(context.Background(), "demo")
	if err != nil {
		t.Fatalf("LoadLive() error = %v", err)
	}
	if len(live) != 1 || live[0] != "bounded live line" {
		t.Fatalf("LoadLive() = %#v", live)
	}
}

func TestLoaderMakesMissingReportExplicit(t *testing.T) {
	home := fixtureHome(t)
	writeFile(t, filepath.Join(home.StateDir, "demo.meta"), "kind=ship\n")

	loader := Loader{
		Home:           home,
		States:         fakeStateResolver{states: map[string]CurrentState{"demo": {State: "unknown", Source: "none"}}},
		Live:           fakeLiveReader{},
		StatusMaxLines: 20,
		StatusMaxBytes: 1024,
		ReportMaxBytes: 1024,
	}
	tasks, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tasks[0].Report.Present {
		t.Fatalf("Report.Present = true, want false for %s", tasks[0].Report.Path)
	}
	if tasks[0].Report.Path != filepath.Join(home.DataDir, "demo", "report.md") {
		t.Fatalf("Report.Path = %q", tasks[0].Report.Path)
	}
}

func TestLoaderSurfacesAdapterFailureWithoutUsingStatusAsCurrentState(t *testing.T) {
	home := fixtureHome(t)
	writeFile(t, filepath.Join(home.StateDir, "demo.meta"), "kind=scout\n")
	writeFile(t, filepath.Join(home.StateDir, "demo.status"), "done: old report\n")

	loader := Loader{
		Home:           home,
		States:         fakeStateResolver{err: errors.New("adapter unavailable")},
		Live:           fakeLiveReader{},
		StatusMaxLines: 20,
		StatusMaxBytes: 1024,
		ReportMaxBytes: 1024,
	}
	tasks, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tasks[0].Current.State != "unknown" || tasks[0].Current.Source != "adapter-error" {
		t.Fatalf("Current = %#v, want unknown adapter-error", tasks[0].Current)
	}
	if !strings.Contains(tasks[0].Current.Detail, "adapter unavailable") {
		t.Fatalf("Current.Detail = %q", tasks[0].Current.Detail)
	}
}

func TestReadReportIsByteBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.md")
	writeFile(t, path, "0123456789abcdef")

	report, err := ReadReport(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Truncated || report.Content != "01234567" {
		t.Fatalf("ReadReport() = %#v, want first 8 bytes and truncation", report)
	}
}

func fixtureHome(t *testing.T) Home {
	t.Helper()
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return Home{Root: root, StateDir: stateDir, DataDir: dataDir}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
