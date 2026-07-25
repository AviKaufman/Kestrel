package firstmate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShellAgentStateResolverUsesExactTaskArgument(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "argv.log")
	script := writeAdapter(t, root, "agent-state", `printf '%s|%s|%s\n' "$FM_HOME" "$1" "$#" > "$ADAPTER_LOG"
printf 'alive\n'`)
	home := Home{Root: "/fake/home", StateDir: "/fake/home/state", DataDir: "/fake/home/data"}

	state, err := (ShellAgentStateResolver{
		Path: script, Home: home, ExtraEnv: []string{"ADAPTER_LOG=" + logPath},
	}).ResolveAgentState(context.Background(), "task-a")
	if err != nil {
		t.Fatal(err)
	}
	if state != "alive" {
		t.Fatalf("ResolveAgentState() = %q", state)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(logBytes) != "/fake/home|task-a|1\n" {
		t.Fatalf("adapter argv = %q", logBytes)
	}
}

func TestShellDirectSessionSourceParsesStableBoundedRecords(t *testing.T) {
	root := t.TempDir()
	script := writeAdapter(t, root, "direct", `case "$1" in
list)
  printf 'private:notes.0\t/projects/notes\n'
  printf 'private:work.1\t/projects/work\n'
  ;;
peek)
  printf 'direct line one\ndirect line two\n'
  ;;
esac`)
	source := ShellDirectSessionSource{
		Path:  script,
		Home:  Home{Root: "/fake/home", StateDir: "/fake/home/state"},
		Lines: 1,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sessions, err := source.Discover(ctx, []Metadata{{ID: "managed", Window: "firstmate:fm-managed"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].Target != "private:notes.0" || sessions[1].Project != "/projects/work" {
		t.Fatalf("Discover() = %#v", sessions)
	}
	lines, err := source.Read(ctx, "private:notes.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "direct line two") {
		t.Fatalf("Read() = %#v", lines)
	}
}

func TestParseDirectSessionsRejectsMalformedOrDuplicateTargets(t *testing.T) {
	for name, input := range map[string]string{
		"malformed": "not-a-record\n",
		"invalid":   "bad target\t/projects/demo\n",
		"duplicate": "private:demo.0\t/a\nprivate:demo.0\t/b\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDirectSessions(input); err == nil {
				t.Fatalf("ParseDirectSessions(%q) error = nil", input)
			}
		})
	}
}

func TestShellDirectSessionSourceCreatesWithExactArguments(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "project")
	if err := os.Mkdir(workdir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "argv.log")
	script := writeAdapter(t, root, "direct-create", `printf '%s|%s|%s|%s\n' "$1" "$2" "$3" "$#" > "$ADAPTER_LOG"
printf 'private:codex-notes.0\t%s\n' "$3"`)
	source := ShellDirectSessionSource{
		Path:     script,
		Home:     Home{Root: "/fake/home", StateDir: "/fake/home/state"},
		ExtraEnv: []string{"ADAPTER_LOG=" + logPath},
	}

	session, err := source.Create(context.Background(), "notes", workdir)
	if err != nil {
		t.Fatal(err)
	}
	if session.Target != "private:codex-notes.0" || session.Project != workdir {
		t.Fatalf("Create() = %#v", session)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(logBytes) != "create|notes|"+workdir+"|3\n" {
		t.Fatalf("adapter argv = %q", logBytes)
	}
}

func TestShellDirectSessionSourceRejectsInvalidCreatedTarget(t *testing.T) {
	root := t.TempDir()
	script := writeAdapter(t, root, "direct-create-invalid", `printf 'not-a-target\t%s\n' "$3"`)
	source := ShellDirectSessionSource{
		Path: script,
		Home: Home{Root: "/fake/home", StateDir: "/fake/home/state"},
	}
	if _, err := source.Create(context.Background(), "notes", root); err == nil {
		t.Fatal("Create() accepted invalid adapter output")
	}
}

func TestCreatePrivateValidatesLabelPathAndAdapterRecord(t *testing.T) {
	workdir := t.TempDir()
	creator := &recordingPrivateCreator{
		session: DirectSession{Target: "private:codex-notes.0", Project: workdir},
	}
	loader := Loader{DirectCreate: creator, PrivateWorkdir: workdir}

	session, err := loader.CreatePrivate(context.Background(), "notes-2")
	if err != nil || session != creator.session {
		t.Fatalf("CreatePrivate() = %#v, %v", session, err)
	}
	if creator.label != "notes-2" || creator.workdir != workdir {
		t.Fatalf("creator args = %q, %q", creator.label, creator.workdir)
	}

	for _, test := range []struct {
		name    string
		label   string
		workdir string
	}{
		{name: "unsafe label", label: "bad;label", workdir: workdir},
		{name: "relative path", label: "notes", workdir: "relative"},
		{name: "missing path", label: "notes", workdir: filepath.Join(workdir, "missing")},
	} {
		t.Run(test.name, func(t *testing.T) {
			testLoader := loader
			testLoader.PrivateWorkdir = test.workdir
			if _, err := testLoader.CreatePrivate(context.Background(), test.label); err == nil {
				t.Fatal("CreatePrivate() error = nil")
			}
		})
	}

	creator.session.Project = filepath.Join(workdir, "other")
	if _, err := loader.CreatePrivate(context.Background(), "notes"); err == nil {
		t.Fatal("CreatePrivate() accepted mismatched adapter project")
	}
}

type recordingPrivateCreator struct {
	label   string
	workdir string
	session DirectSession
}

func (creator *recordingPrivateCreator) Create(_ context.Context, label, workdir string) (DirectSession, error) {
	creator.label = label
	creator.workdir = workdir
	return creator.session, nil
}
