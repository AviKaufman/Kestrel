package firstmate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeHubAdapter struct {
	target     HubTarget
	resolveErr error
	sentTarget HubTarget
	message    string
	sendErr    error
}

func (adapter *fakeHubAdapter) Resolve(context.Context) (HubTarget, error) {
	return adapter.target, adapter.resolveErr
}

func (adapter *fakeHubAdapter) Send(_ context.Context, target HubTarget, message string) error {
	adapter.sentTarget = target
	adapter.message = message
	return adapter.sendErr
}

func TestParseHubTargetAcceptsOneStableRecord(t *testing.T) {
	target, err := ParseHubTarget("tmux\t%9\n")
	if err != nil {
		t.Fatal(err)
	}
	if target.Backend != "tmux" || target.Target != "%9" {
		t.Fatalf("ParseHubTarget() = %#v", target)
	}

	for _, invalid := range []string{
		"",
		"tmux\n",
		"shell\t%9\n",
		"tmux\tunsafe target\n",
		"tmux\t%9\nherdr\tdefault:w1:p2\n",
	} {
		if _, err := ParseHubTarget(invalid); err == nil {
			t.Fatalf("ParseHubTarget(%q) error = nil", invalid)
		}
	}
}

func TestLoaderResolvesAndRoutesHubMessages(t *testing.T) {
	adapter := &fakeHubAdapter{target: HubTarget{Backend: "herdr", Target: "default:w1:p2"}}
	loader := Loader{Hub: adapter}
	target, err := loader.LoadHub(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if target != adapter.target {
		t.Fatalf("LoadHub() = %#v", target)
	}
	if err := loader.SendHub(context.Background(), target, "hello primary"); err != nil {
		t.Fatal(err)
	}
	if adapter.sentTarget != target || adapter.message != "hello primary" {
		t.Fatalf("hub send = target %#v message %q", adapter.sentTarget, adapter.message)
	}
}

func TestLoaderRejectsInvalidHubTargetsAndMessages(t *testing.T) {
	adapter := &fakeHubAdapter{}
	loader := Loader{Hub: adapter}
	tests := []struct {
		target  HubTarget
		message string
	}{
		{target: HubTarget{Backend: "shell", Target: "%9"}, message: "hello"},
		{target: HubTarget{Backend: "tmux", Target: "unsafe target"}, message: "hello"},
		{target: HubTarget{Backend: "tmux", Target: "%9"}, message: " \n "},
		{target: HubTarget{Backend: "tmux", Target: "%9"}, message: strings.Repeat("x", MaxMessageBytes+1)},
	}
	for _, test := range tests {
		if err := loader.SendHub(context.Background(), test.target, test.message); err == nil {
			t.Fatalf("SendHub(%#v, %q) error = nil", test.target, test.message)
		}
	}
	if adapter.message != "" {
		t.Fatalf("invalid send reached adapter: %q", adapter.message)
	}
}

func TestShellHubAdapterUsesExplicitArgvAndPreservesFailures(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, "argv.log")
	script := writeAdapter(t, root, "hub", `case "$1" in
resolve) printf 'tmux\t%%9\n' ;;
send)
  printf '%s\n%s\n%s\n%s\n%s\n' "$#" "$1" "$2" "$3" "$4" > "$ADAPTER_LOG"
  if [ "${FAIL_SEND:-}" = 1 ]; then printf 'hub unavailable\n' >&2; exit 8; fi
  ;;
esac`)
	adapter := ShellHubAdapter{
		Path:     script,
		Home:     Home{Root: "/fake/home", StateDir: "/fake/home/state"},
		ExtraEnv: []string{"ADAPTER_LOG=" + logPath},
	}
	target, err := adapter.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	message := `literal $HOME; $(touch /tmp/must-not-run)`
	if err := adapter.Send(context.Background(), target, message); err != nil {
		t.Fatal(err)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(logBytes) != "4\nsend\ntmux\n%9\n"+message+"\n" {
		t.Fatalf("hub adapter argv = %q", logBytes)
	}

	adapter.ExtraEnv = append(adapter.ExtraEnv, "FAIL_SEND=1")
	err = adapter.Send(context.Background(), target, "retain me")
	if err == nil || !strings.Contains(err.Error(), "hub unavailable") {
		t.Fatalf("hub Send() error = %v", err)
	}
}

func TestLoadHubReturnsResolutionFailure(t *testing.T) {
	loader := Loader{Hub: &fakeHubAdapter{resolveErr: errors.New("ambiguous supervisor")}}
	if _, err := loader.LoadHub(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "ambiguous supervisor") {
		t.Fatalf("LoadHub() error = %v", err)
	}
}
