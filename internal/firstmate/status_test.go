package firstmate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadStatusEventsReturnsBoundedNewestEventsInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.status")
	content := "working: one\n\nworking: two\nblocked: three\ndone: four\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	events, err := ReadStatusEvents(path, 2, 1024)
	if err != nil {
		t.Fatalf("ReadStatusEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ReadStatusEvents() len = %d, want 2", len(events))
	}
	if events[0].Verb != "blocked" || events[0].Note != "three" {
		t.Fatalf("events[0] = %#v, want blocked: three", events[0])
	}
	if events[1].Raw != "done: four" {
		t.Fatalf("events[1].Raw = %q, want done: four", events[1].Raw)
	}
}

func TestReadStatusEventsDoesNotPromoteLastEventToCurrentState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.status")
	if err := os.WriteFile(path, []byte("needs-decision: old gate\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	events, err := ReadStatusEvents(path, 20, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if events[0].Verb != "needs-decision" {
		t.Fatalf("event verb = %q, want historical needs-decision", events[0].Verb)
	}
}

func TestReadStatusEventsMarksByteTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.status")
	if err := os.WriteFile(path, []byte("working: oldest\nworking: middle\ndone: newest\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	events, err := ReadStatusEvents(path, 20, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || !events[0].Truncated {
		t.Fatalf("ReadStatusEvents() = %#v, want first retained event marked truncated", events)
	}
}
