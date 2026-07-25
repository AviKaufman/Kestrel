package firstmate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const adapterOutputLimit = 256 * 1024

// CurrentState is parsed only from fm-crew-state.sh output.
type CurrentState struct {
	State  string
	Source string
	Detail string
	Raw    string
}

// ParseCurrentState parses the stable first line emitted by fm-crew-state.sh.
func ParseCurrentState(output string) (CurrentState, error) {
	raw := strings.TrimSpace(strings.SplitN(output, "\n", 2)[0])
	const statePrefix = "state: "
	const sourceSeparator = " · source: "
	if !strings.HasPrefix(raw, statePrefix) {
		return CurrentState{}, fmt.Errorf("current-state adapter returned an invalid line: %q", raw)
	}
	stateAndRest := strings.TrimPrefix(raw, statePrefix)
	state, rest, found := strings.Cut(stateAndRest, sourceSeparator)
	if !found || strings.TrimSpace(state) == "" {
		return CurrentState{}, fmt.Errorf("current-state adapter returned an invalid line: %q", raw)
	}
	source, detail, _ := strings.Cut(rest, " · ")
	if strings.TrimSpace(source) == "" {
		return CurrentState{}, fmt.Errorf("current-state adapter returned an invalid source: %q", raw)
	}
	return CurrentState{
		State:  strings.TrimSpace(state),
		Source: strings.TrimSpace(source),
		Detail: strings.TrimSpace(detail),
		Raw:    raw,
	}, nil
}

// ShellStateResolver invokes the existing current-state owner.
type ShellStateResolver struct {
	Path     string
	Home     Home
	ExtraEnv []string
}

func (resolver ShellStateResolver) Resolve(ctx context.Context, taskID string) (CurrentState, error) {
	stdout, stderr, truncated, err := runBounded(
		ctx,
		resolver.Path,
		[]string{taskID},
		adapterEnvironment(resolver.Home, resolver.ExtraEnv),
	)
	if err != nil {
		return CurrentState{}, fmt.Errorf("resolve current state for %s: %w: %s", taskID, err, strings.TrimSpace(stderr))
	}
	if truncated {
		return CurrentState{}, fmt.Errorf("resolve current state for %s: adapter output exceeded %d bytes", taskID, adapterOutputLimit)
	}
	return ParseCurrentState(stdout)
}

// ShellLiveReader invokes fm-peek.sh with an explicit line bound.
type ShellLiveReader struct {
	Path     string
	Home     Home
	Lines    int
	ExtraEnv []string
}

// ShellAgentStateResolver invokes the existing recovery-grade endpoint adapter.
type ShellAgentStateResolver struct {
	Path     string
	Home     Home
	ExtraEnv []string
}

func (resolver ShellAgentStateResolver) ResolveAgentState(ctx context.Context, taskID string) (string, error) {
	stdout, stderr, truncated, err := runBounded(
		ctx,
		resolver.Path,
		[]string{taskID},
		adapterEnvironment(resolver.Home, resolver.ExtraEnv),
	)
	if err != nil {
		return "", fmt.Errorf("resolve agent state for %s: %w: %s", taskID, err, strings.TrimSpace(stderr))
	}
	if truncated {
		return "", fmt.Errorf("resolve agent state for %s: adapter output exceeded %d bytes", taskID, adapterOutputLimit)
	}
	state := strings.TrimSpace(stdout)
	switch state {
	case "alive", "dead", "missing", "ambiguous", "unreadable":
		return state, nil
	default:
		return "", fmt.Errorf("resolve agent state for %s: invalid adapter output %q", taskID, state)
	}
}

// ShellDirectSessionSource invokes the bounded direct Codex tmux adapter.
type ShellDirectSessionSource struct {
	Path     string
	Home     Home
	Lines    int
	ExtraEnv []string
}

// ShellMessageSender executes one adapter with target and message as separate
// arguments, optionally after a fixed adapter subcommand.
type ShellMessageSender struct {
	Path       string
	Home       Home
	PrefixArgs []string
	ExtraEnv   []string
}

// ShellHubAdapter invokes the supervisor-target adapter with bounded output.
type ShellHubAdapter struct {
	Path     string
	Home     Home
	Lines    int
	ExtraEnv []string
}

func (adapter ShellHubAdapter) Resolve(ctx context.Context) (HubTarget, error) {
	stdout, stderr, truncated, err := runBounded(
		ctx,
		adapter.Path,
		[]string{"resolve"},
		adapterEnvironment(adapter.Home, adapter.ExtraEnv),
	)
	if err != nil {
		return HubTarget{}, fmt.Errorf("resolve Firstmate hub: %w: %s", err, strings.TrimSpace(stderr))
	}
	if truncated {
		return HubTarget{}, fmt.Errorf("resolve Firstmate hub: adapter output exceeded %d bytes", adapterOutputLimit)
	}
	return ParseHubTarget(stdout)
}

func (adapter ShellHubAdapter) Send(ctx context.Context, target HubTarget, message string) error {
	_, stderr, truncated, err := runBounded(
		ctx,
		adapter.Path,
		[]string{"send", target.Backend, target.Target, message},
		adapterEnvironment(adapter.Home, adapter.ExtraEnv),
	)
	if err != nil {
		return fmt.Errorf("send to Firstmate hub %s: %w: %s", target.Target, err, strings.TrimSpace(stderr))
	}
	if truncated {
		return fmt.Errorf("send to Firstmate hub %s: adapter output exceeded %d bytes", target.Target, adapterOutputLimit)
	}
	return nil
}

func (adapter ShellHubAdapter) Read(ctx context.Context, target HubTarget) ([]string, error) {
	lines := adapter.Lines
	if lines <= 0 {
		lines = 40
	}
	stdout, stderr, truncated, err := runBounded(
		ctx,
		adapter.Path,
		[]string{"history", target.Backend, target.Target, strconv.Itoa(lines)},
		adapterEnvironment(adapter.Home, adapter.ExtraEnv),
	)
	if err != nil {
		return nil, fmt.Errorf("read Firstmate hub history %s: %w: %s", target.Target, err, strings.TrimSpace(stderr))
	}
	rawLines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(rawLines) == 1 && rawLines[0] == "" {
		rawLines = nil
	}
	if len(rawLines) > lines {
		rawLines = rawLines[len(rawLines)-lines:]
		truncated = true
	}
	if truncated && len(rawLines) > 0 {
		rawLines[0] = "[earlier hub history omitted] " + rawLines[0]
	}
	return rawLines, nil
}

func (sender ShellMessageSender) Send(ctx context.Context, target, message string) error {
	args := make([]string, 0, len(sender.PrefixArgs)+2)
	args = append(args, sender.PrefixArgs...)
	args = append(args, target, message)
	_, stderr, truncated, err := runBounded(
		ctx,
		sender.Path,
		args,
		adapterEnvironment(sender.Home, sender.ExtraEnv),
	)
	if err != nil {
		return fmt.Errorf("send to %s: %w: %s", target, err, strings.TrimSpace(stderr))
	}
	if truncated {
		return fmt.Errorf("send to %s: adapter output exceeded %d bytes", target, adapterOutputLimit)
	}
	return nil
}

func (source ShellDirectSessionSource) Discover(ctx context.Context, _ []Metadata) ([]DirectSession, error) {
	stdout, stderr, truncated, err := runBounded(
		ctx,
		source.Path,
		[]string{"list"},
		adapterEnvironment(source.Home, source.ExtraEnv),
	)
	if err != nil {
		return nil, fmt.Errorf("discover direct Codex sessions: %w: %s", err, strings.TrimSpace(stderr))
	}
	if truncated {
		return nil, fmt.Errorf("discover direct Codex sessions: adapter output exceeded %d bytes", adapterOutputLimit)
	}
	return ParseDirectSessions(stdout)
}

func (source ShellDirectSessionSource) Read(ctx context.Context, target string) ([]string, error) {
	if !directTargetPattern.MatchString(target) {
		return nil, fmt.Errorf("invalid direct-session target %q", target)
	}
	lines := source.Lines
	if lines <= 0 {
		lines = 40
	}
	stdout, stderr, truncated, err := runBounded(
		ctx,
		source.Path,
		[]string{"peek", target, strconv.Itoa(lines)},
		adapterEnvironment(source.Home, source.ExtraEnv),
	)
	if err != nil {
		return nil, fmt.Errorf("read direct Codex session %s: %w: %s", target, err, strings.TrimSpace(stderr))
	}
	rawLines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(rawLines) == 1 && rawLines[0] == "" {
		rawLines = nil
	}
	if len(rawLines) > lines {
		rawLines = rawLines[len(rawLines)-lines:]
		truncated = true
	}
	if truncated && len(rawLines) > 0 {
		rawLines[0] = "[earlier live output omitted] " + rawLines[0]
	}
	return rawLines, nil
}

// Create starts one private session through the direct-session adapter.
func (source ShellDirectSessionSource) Create(ctx context.Context, label, workdir string) (DirectSession, error) {
	stdout, stderr, truncated, err := runBounded(
		ctx,
		source.Path,
		[]string{"create", label, workdir},
		adapterEnvironment(source.Home, source.ExtraEnv),
	)
	if err != nil {
		return DirectSession{}, fmt.Errorf("create private Codex session: %w: %s", err, strings.TrimSpace(stderr))
	}
	if truncated {
		return DirectSession{}, fmt.Errorf("create private Codex session: adapter output exceeded %d bytes", adapterOutputLimit)
	}
	sessions, err := ParseDirectSessions(stdout)
	if err != nil {
		return DirectSession{}, err
	}
	if len(sessions) != 1 {
		return DirectSession{}, fmt.Errorf("private-session adapter returned %d created sessions, want one", len(sessions))
	}
	return sessions[0], nil
}

func (reader ShellLiveReader) Read(ctx context.Context, taskID string) ([]string, error) {
	lines := reader.Lines
	if lines <= 0 {
		lines = 40
	}
	stdout, stderr, truncated, err := runBounded(
		ctx,
		reader.Path,
		[]string{taskID, strconv.Itoa(lines)},
		adapterEnvironment(reader.Home, reader.ExtraEnv),
	)
	if err != nil {
		return nil, fmt.Errorf("read live output for %s: %w: %s", taskID, err, strings.TrimSpace(stderr))
	}
	rawLines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(rawLines) == 1 && rawLines[0] == "" {
		rawLines = nil
	}
	if len(rawLines) > lines {
		rawLines = rawLines[len(rawLines)-lines:]
		truncated = true
	}
	if truncated && len(rawLines) > 0 {
		rawLines[0] = "[earlier live output omitted] " + rawLines[0]
	}
	return rawLines, nil
}

func adapterEnvironment(home Home, extra []string) []string {
	environment := make([]string, 0, len(os.Environ())+len(extra)+2)
	for _, entry := range append(os.Environ(), extra...) {
		if strings.HasPrefix(entry, "FM_HOME=") ||
			strings.HasPrefix(entry, "FM_STATE_OVERRIDE=") ||
			strings.HasPrefix(entry, "FM_GUARD_READ_ONLY=") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(
		environment,
		"FM_HOME="+home.Root,
		"FM_STATE_OVERRIDE="+home.StateDir,
		"FM_GUARD_READ_ONLY=1",
	)
}

func runBounded(ctx context.Context, path string, args, environment []string) (string, string, bool, error) {
	command := exec.CommandContext(ctx, path, args...)
	command.Env = environment
	var stdout, stderr cappedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), stdout.Truncated() || stderr.Truncated(), err
}

type cappedBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (buffer *cappedBuffer) Write(content []byte) (int, error) {
	originalLength := len(content)
	remaining := adapterOutputLimit - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return originalLength, nil
	}
	if len(content) > remaining {
		content = content[:remaining]
		buffer.truncated = true
	}
	_, _ = buffer.buffer.Write(content)
	return originalLength, nil
}

func (buffer *cappedBuffer) String() string {
	return buffer.buffer.String()
}

func (buffer *cappedBuffer) Truncated() bool {
	return buffer.truncated
}
