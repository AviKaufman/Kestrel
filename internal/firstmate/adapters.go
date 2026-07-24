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
		if strings.HasPrefix(entry, "FM_HOME=") || strings.HasPrefix(entry, "FM_STATE_OVERRIDE=") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(
		environment,
		"FM_HOME="+home.Root,
		"FM_STATE_OVERRIDE="+home.StateDir,
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
