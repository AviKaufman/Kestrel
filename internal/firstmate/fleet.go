package firstmate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultStatusLines  = 20
	defaultStatusBytes  = 64 * 1024
	defaultReportBytes  = 64 * 1024
	defaultProbeTimeout = 15 * time.Second
)

// StateResolver owns current-state resolution for one task.
type StateResolver interface {
	Resolve(context.Context, string) (CurrentState, error)
}

// LiveReader provides a bounded, read-only worker capture.
type LiveReader interface {
	Read(context.Context, string) ([]string, error)
}

// Report is durable official output for a task when data/<id>/report.md exists.
type Report struct {
	Path      string
	Present   bool
	Content   string
	Truncated bool
}

// Task is the read-only view model assembled from Firstmate's existing owners.
type Task struct {
	Metadata  Metadata
	Current   CurrentState
	Events    []StatusEvent
	Report    Report
	Ownership Ownership
	Target    string
}

// Loader joins task metadata, current state, bounded event history, and reports.
type Loader struct {
	Home           Home
	States         StateResolver
	Agents         AgentStateResolver
	Live           LiveReader
	Direct         DirectSessionSource
	DirectCreate   PrivateSessionCreator
	PrivateWorkdir string
	ManagedSend    MessageSender
	DirectSend     MessageSender
	Hub            HubAdapter
	StatusMaxLines int
	StatusMaxBytes int
	ReportMaxBytes int
	ProbeTimeout   time.Duration
}

func (loader Loader) Load(ctx context.Context) ([]Task, error) {
	if loader.States == nil {
		return nil, fmt.Errorf("current-state resolver is required")
	}
	if loader.Agents == nil {
		return nil, fmt.Errorf("agent-state resolver is required")
	}
	metas, err := LoadTaskMetadata(loader.Home.StateDir)
	if err != nil {
		return nil, err
	}

	statusLines := loader.StatusMaxLines
	if statusLines <= 0 {
		statusLines = defaultStatusLines
	}
	statusBytes := loader.StatusMaxBytes
	if statusBytes <= 0 {
		statusBytes = defaultStatusBytes
	}
	reportBytes := loader.ReportMaxBytes
	if reportBytes <= 0 {
		reportBytes = defaultReportBytes
	}
	probeTimeout := loader.ProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = defaultProbeTimeout
	}

	tasks := make([]Task, 0, len(metas))
	for _, meta := range metas {
		stateCtx, cancelState := context.WithTimeout(ctx, probeTimeout)
		current, stateErr := loader.States.Resolve(stateCtx, meta.ID)
		cancelState()
		if stateErr != nil {
			current = CurrentState{
				State:  "unknown",
				Source: "adapter-error",
				Detail: stateErr.Error(),
			}
		}
		agentCtx, cancelAgent := context.WithTimeout(ctx, probeTimeout)
		agentState, agentErr := loader.Agents.ResolveAgentState(agentCtx, meta.ID)
		cancelAgent()
		if agentErr != nil || agentState != "alive" || isTerminalState(current.State) {
			continue
		}
		events, err := ReadStatusEvents(
			filepath.Join(loader.Home.StateDir, meta.ID+".status"),
			statusLines,
			statusBytes,
		)
		if err != nil {
			return nil, err
		}
		report, err := ReadReport(
			filepath.Join(loader.Home.DataDir, meta.ID, "report.md"),
			reportBytes,
		)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, Task{
			Metadata:  meta,
			Current:   current,
			Events:    events,
			Report:    report,
			Ownership: FirstmateManaged,
			Target:    meta.ID,
		})
	}
	if loader.Direct != nil {
		sessions, err := loader.Direct.Discover(ctx, metas)
		if err != nil {
			return nil, err
		}
		for _, session := range sessions {
			tasks = append(tasks, Task{
				Metadata: Metadata{
					ID:       session.Target,
					Project:  session.Project,
					Kind:     "direct-session",
					Mode:     "private",
					Harness:  "codex",
					Worktree: session.Project,
					Window:   session.Target,
				},
				Current: CurrentState{
					State:  "working",
					Source: "tmux",
					Detail: "live Direct Codex session",
				},
				Ownership: CaptainPrivate,
				Target:    session.Target,
			})
		}
	}
	return tasks, nil
}

func (loader Loader) LoadLive(ctx context.Context, task Task) ([]string, error) {
	switch task.Ownership {
	case FirstmateManaged:
		if loader.Live == nil {
			return nil, fmt.Errorf("live reader is required")
		}
		return loader.Live.Read(ctx, task.Metadata.ID)
	case CaptainPrivate:
		if loader.Direct == nil {
			return nil, fmt.Errorf("direct-session reader is required")
		}
		return loader.Direct.Read(ctx, task.Target)
	default:
		return nil, fmt.Errorf("unsupported worker ownership %q", task.Ownership)
	}
}

// ReadReport reads at most maxBytes from durable official output.
func ReadReport(path string, maxBytes int) (Report, error) {
	report := Report{Path: path}
	if maxBytes <= 0 {
		return report, fmt.Errorf("report byte bound must be positive")
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("open report %q: %w", path, err)
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return report, fmt.Errorf("read report %q: %w", path, err)
	}
	report.Present = true
	if len(content) > maxBytes {
		content = content[:maxBytes]
		report.Truncated = true
	}
	report.Content = string(content)
	return report, nil
}
