package firstmate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordingSender struct {
	target  string
	message string
	err     error
}

func (sender *recordingSender) Send(_ context.Context, target, message string) error {
	sender.target = target
	sender.message = message
	return sender.err
}

func TestLoaderRoutesMessagesByExplicitOwnership(t *testing.T) {
	managed := &recordingSender{}
	direct := &recordingSender{}
	loader := Loader{ManagedSend: managed, DirectSend: direct}

	err := loader.Send(context.Background(), Task{
		Ownership: FirstmateManaged,
		Metadata:  Metadata{ID: "task-a"},
		Target:    "task-a",
	}, "managed message")
	if err != nil {
		t.Fatal(err)
	}
	if managed.target != "task-a" || managed.message != "managed message" || direct.target != "" {
		t.Fatalf("managed sender = %#v, direct sender = %#v", managed, direct)
	}

	err = loader.Send(context.Background(), Task{
		Ownership: CaptainPrivate,
		Metadata:  Metadata{ID: "private:notes.0"},
		Target:    "private:notes.0",
	}, "direct message")
	if err != nil {
		t.Fatal(err)
	}
	if direct.target != "private:notes.0" || direct.message != "direct message" {
		t.Fatalf("direct sender = %#v", direct)
	}
}

func TestLoaderRejectsInvalidTargetsEmptyAndOversizedMessages(t *testing.T) {
	sender := &recordingSender{}
	loader := Loader{ManagedSend: sender, DirectSend: sender}
	tests := []struct {
		name    string
		task    Task
		message string
	}{
		{
			name:    "invalid managed target",
			task:    Task{Ownership: FirstmateManaged, Metadata: Metadata{ID: "../escape"}, Target: "../escape"},
			message: "hello",
		},
		{
			name:    "invalid direct target",
			task:    Task{Ownership: CaptainPrivate, Target: "not a target"},
			message: "hello",
		},
		{
			name:    "empty",
			task:    Task{Ownership: FirstmateManaged, Metadata: Metadata{ID: "task-a"}, Target: "task-a"},
			message: " \n\t ",
		},
		{
			name:    "oversized",
			task:    Task{Ownership: FirstmateManaged, Metadata: Metadata{ID: "task-a"}, Target: "task-a"},
			message: strings.Repeat("x", MaxMessageBytes+1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := loader.Send(context.Background(), test.task, test.message); err == nil {
				t.Fatal("Send() error = nil")
			}
		})
	}
	if sender.target != "" {
		t.Fatalf("sender invoked for invalid input: %#v", sender)
	}
}

func TestShellMessageSenderUsesExactArgumentArrayAndBoundsFailures(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "argv.log")
	script := writeAdapter(t, root, "send", `printf '%s\n%s\n%s\n%s\n' "$#" "$1" "$2" "$FM_HOME" > "$ADAPTER_LOG"
if [ "${FAIL_SEND:-}" = 1 ]; then printf 'adapter failed\n' >&2; exit 9; fi`)
	home := Home{Root: "/fake/home", StateDir: "/fake/home/state"}
	sender := ShellMessageSender{
		Path: script, Home: home, ExtraEnv: []string{"ADAPTER_LOG=" + logPath},
	}
	message := `literal $HOME; $(touch /tmp/must-not-run)`
	if err := sender.Send(context.Background(), "task-a", message); err != nil {
		t.Fatal(err)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(logBytes) != "2\ntask-a\n"+message+"\n/fake/home\n" {
		t.Fatalf("adapter argv = %q", logBytes)
	}

	sender.ExtraEnv = append(sender.ExtraEnv, "FAIL_SEND=1")
	err = sender.Send(context.Background(), "task-a", "retry me")
	if err == nil || !strings.Contains(err.Error(), "adapter failed") {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestLoaderPreservesAdapterFailure(t *testing.T) {
	managed := &recordingSender{err: errors.New("submission failed")}
	loader := Loader{ManagedSend: managed}
	err := loader.Send(context.Background(), Task{
		Ownership: FirstmateManaged,
		Metadata:  Metadata{ID: "task-a"},
		Target:    "task-a",
	}, "keep this draft")
	if err == nil || !strings.Contains(err.Error(), "submission failed") {
		t.Fatalf("Send() error = %v", err)
	}
}
