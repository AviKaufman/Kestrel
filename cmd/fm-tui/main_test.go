package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSnapshotAgainstFakeHomeAndInjectedAdapters(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	home := filepath.Join(root, "tests", "fixtures", "tui-home")
	stateAdapter := filepath.Join(root, "tests", "fixtures", "tui-bin", "fm-crew-state.sh")
	peekAdapter := filepath.Join(root, "tests", "fixtures", "tui-bin", "fm-peek.sh")
	agentAdapter := filepath.Join(root, "tests", "fixtures", "tui-bin", "fm-tui-agent-state.sh")
	directAdapter := filepath.Join(root, "tests", "fixtures", "tui-bin", "fm-tui-direct.sh")
	sendAdapter := filepath.Join(root, "tests", "fixtures", "tui-bin", "fm-send.sh")

	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"--snapshot",
		"--home", home,
		"--root", root,
		"--crew-state", stateAdapter,
		"--agent-state", agentAdapter,
		"--peek", peekAdapter,
		"--direct", directAdapter,
		"--send", sendAdapter,
	}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("run() exit = %d, stderr = %q", exitCode, stderr.String())
	}
	for _, expected := range []string{
		"FIRSTMATE TUI SNAPSHOT",
		"task id: demo",
		"current state: working",
		"current source: pane",
		"# Demo report",
		"working: setup complete",
		"bounded fake capture",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("snapshot missing %q:\n%s", expected, stdout.String())
		}
	}
}

func TestRunReportsClearHomeError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"--snapshot", "--home", filepath.Join(t.TempDir(), "missing")}, &stdout, &stderr)
	if exitCode == 0 {
		t.Fatal("run() exit = 0, want failure")
	}
	if !strings.Contains(stderr.String(), "FM_HOME") {
		t.Fatalf("stderr = %q, want FM_HOME context", stderr.String())
	}
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
