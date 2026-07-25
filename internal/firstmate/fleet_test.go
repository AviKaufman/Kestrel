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

type fakeAgentStateResolver struct {
	states map[string]string
}

func (resolver fakeAgentStateResolver) ResolveAgentState(_ context.Context, taskID string) (string, error) {
	return resolver.states[taskID], nil
}

type fakeDirectSessionSource struct {
	sessions []DirectSession
}

func (source fakeDirectSessionSource) Discover(context.Context, []Metadata) ([]DirectSession, error) {
	return source.sessions, nil
}

func (source fakeDirectSessionSource) Read(context.Context, string) ([]string, error) {
	return []string{"direct capture"}, nil
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
		Agents:         fakeAgentStateResolver{states: map[string]string{"demo": "alive"}},
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
	live, err := loader.LoadLive(context.Background(), task)
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
		Agents:         fakeAgentStateResolver{states: map[string]string{"demo": "alive"}},
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
		Agents:         fakeAgentStateResolver{states: map[string]string{"demo": "alive"}},
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

func TestLoaderShowsOnlyActiveManagedWorkersAndDirectSessions(t *testing.T) {
	home := fixtureHome(t)
	for _, id := range []string{"active", "done", "failed", "terminal", "missing", "stale"} {
		writeFile(t, filepath.Join(home.StateDir, id+".meta"), strings.Join([]string{
			"window=firstmate:fm-" + id,
			"harness=codex",
			"kind=ship",
		}, "\n")+"\n")
		writeFile(t, filepath.Join(home.StateDir, id+".status"), "working: historical event only\n")
	}

	loader := Loader{
		Home: home,
		States: fakeStateResolver{states: map[string]CurrentState{
			"active":   {State: "working", Source: "pane"},
			"done":     {State: "done", Source: "run-step"},
			"failed":   {State: "failed", Source: "run-step"},
			"terminal": {State: "cancelled", Source: "run-step"},
			"missing":  {State: "working", Source: "pane"},
			"stale":    {State: "unknown", Source: "none"},
		}},
		Agents: fakeAgentStateResolver{states: map[string]string{
			"active": "alive", "done": "alive", "failed": "alive",
			"terminal": "alive", "missing": "missing", "stale": "dead",
		}},
		Direct: fakeDirectSessionSource{sessions: []DirectSession{
			{Target: "private:notes.0", Project: "/projects/notes"},
		}},
		Live:           fakeLiveReader{},
		StatusMaxLines: 20,
		StatusMaxBytes: 1024,
		ReportMaxBytes: 1024,
	}

	tasks, err := loader.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("Load() tasks = %#v, want active managed and private direct session", tasks)
	}
	if tasks[0].Metadata.ID != "active" || tasks[0].Ownership != FirstmateManaged {
		t.Fatalf("tasks[0] = %#v", tasks[0])
	}
	if tasks[1].Metadata.ID != "private:notes.0" || tasks[1].Ownership != CaptainPrivate {
		t.Fatalf("tasks[1] = %#v", tasks[1])
	}
}

func TestLoaderRoutesLiveReadsByOwnership(t *testing.T) {
	loader := Loader{
		Live:   fakeLiveReader{lines: []string{"managed capture"}},
		Direct: fakeDirectSessionSource{},
	}
	managed, err := loader.LoadLive(context.Background(), Task{
		Ownership: FirstmateManaged,
		Metadata:  Metadata{ID: "managed"},
	})
	if err != nil || len(managed) != 1 || managed[0] != "managed capture" {
		t.Fatalf("managed LoadLive() = %#v, %v", managed, err)
	}
	direct, err := loader.LoadLive(context.Background(), Task{
		Ownership: CaptainPrivate,
		Target:    "private:notes.0",
	})
	if err != nil || len(direct) != 1 || direct[0] != "direct capture" {
		t.Fatalf("direct LoadLive() = %#v, %v", direct, err)
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
