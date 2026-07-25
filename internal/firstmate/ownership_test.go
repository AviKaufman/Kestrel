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
