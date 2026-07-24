package firstmate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCurrentStatePreservesStateSourceAndDetail(t *testing.T) {
	got, err := ParseCurrentState("state: working · source: run-step · validating (running)\n")
	if err != nil {
		t.Fatalf("ParseCurrentState() error = %v", err)
	}
	if got.State != "working" || got.Source != "run-step" || got.Detail != "validating (running)" {
		t.Fatalf("ParseCurrentState() = %#v", got)
	}
}

func TestShellAdaptersPassHomeAndExactArguments(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "argv.log")
	stateScript := writeAdapter(t, root, "crew-state", `printf '%s|%s|%s\n' "$FM_HOME" "$1" "$#" > "$ADAPTER_LOG"
printf 'state: working · source: pane · harness busy\n'`)
	peekScript := writeAdapter(t, root, "peek", `printf '%s|%s|%s|%s|%s\n' "$FM_HOME" "$1" "$2" "$#" "$FM_GUARD_READ_ONLY" >> "$ADAPTER_LOG"
printf 'bounded worker output\n'`)
	home := Home{Root: "/fake/home", StateDir: "/fake/home/state", DataDir: "/fake/home/data"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	current, err := (ShellStateResolver{Path: stateScript, Home: home, ExtraEnv: []string{"ADAPTER_LOG=" + logPath}}).Resolve(ctx, "task-a")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if current.Source != "pane" {
		t.Fatalf("Resolve() source = %q, want pane", current.Source)
	}
	lines, err := (ShellLiveReader{Path: peekScript, Home: home, Lines: 12, ExtraEnv: []string{"ADAPTER_LOG=" + logPath}}).Read(ctx, "task-a")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(lines) != 1 || lines[0] != "bounded worker output" {
		t.Fatalf("Read() = %#v", lines)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, "/fake/home|task-a|1") {
		t.Fatalf("state adapter log = %q", logText)
	}
	if !strings.Contains(logText, "/fake/home|task-a|12|2|1") {
		t.Fatalf("peek adapter log = %q", logText)
	}
}

func writeAdapter(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	content := "#!/usr/bin/env bash\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
