package firstmate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHomeUsesExplicitPathBeforeEnvironment(t *testing.T) {
	explicit := makeHome(t, "explicit")
	fromEnvironment := makeHome(t, "environment")
	t.Setenv("FM_HOME", fromEnvironment)

	home, err := ResolveHome(explicit)
	if err != nil {
		t.Fatalf("ResolveHome() error = %v", err)
	}
	if home.Root != explicit {
		t.Fatalf("ResolveHome() root = %q, want %q", home.Root, explicit)
	}
}

func TestResolveHomeUsesEnvironmentBeforeWorkingDirectory(t *testing.T) {
	fromEnvironment := makeHome(t, "environment")
	t.Setenv("FM_HOME", fromEnvironment)

	home, err := ResolveHome("")
	if err != nil {
		t.Fatalf("ResolveHome() error = %v", err)
	}
	if home.Root != fromEnvironment {
		t.Fatalf("ResolveHome() root = %q, want %q", home.Root, fromEnvironment)
	}
}

func TestResolveHomeRejectsMissingStateDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("FM_HOME", "")

	_, err := ResolveHome(root)
	if err == nil {
		t.Fatal("ResolveHome() error = nil, want missing state directory error")
	}
	if !strings.Contains(err.Error(), "state directory") {
		t.Fatalf("ResolveHome() error = %q, want state directory context", err)
	}
}

func makeHome(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}
