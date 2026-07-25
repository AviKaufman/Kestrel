package firstmate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultStatusLines      = 20
	defaultStatusBytes      = 64 * 1024
	defaultReportBytes      = 64 * 1024
	defaultProbeTimeout     = 15 * time.Second
	defaultProbeConcurrency = 8
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

type managedProbeResult struct {
	task    Task
	include bool
	err     error
}

type directProbeResult struct {
	sessions []DirectSession
	err      error
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
	workerCount := min(defaultProbeConcurrency, len(metas))
	if deadline, ok := ctx.Deadline(); ok && workerCount > 0 {
		waves := (len(metas) + workerCount - 1) / workerCount
		refreshBudget := time.Until(deadline) / time.Duration(waves)
		if refreshBudget < probeTimeout {
			probeTimeout = refreshBudget
		}
	}

	directResults := make(chan directProbeResult, 1)
	if loader.Direct != nil {
		go func() {
			sessions, discoverErr := loader.Direct.Discover(ctx, metas)
			directResults <- directProbeResult{sessions: sessions, err: discoverErr}
		}()
	} else {
		directResults <- directProbeResult{}
	}

	results := make([]managedProbeResult, len(metas))
	jobs := make(chan int, len(metas))
	for index := range metas {
		jobs <- index
	}
	close(jobs)

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for index := range jobs {
				results[index] = loader.loadManagedTask(
					ctx,
					metas[index],
					probeTimeout,
					statusLines,
					statusBytes,
					reportBytes,
				)
			}
		}()
	}
	workers.Wait()

	tasks := make([]Task, 0, len(metas))
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		if result.include {
			tasks = append(tasks, result.task)
		}
	}

	direct := <-directResults
	if direct.err != nil {
		return nil, direct.err
	}
	for _, session := range direct.sessions {
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
	return tasks, nil
}

func (loader Loader) loadManagedTask(
	ctx context.Context,
	meta Metadata,
	probeTimeout time.Duration,
	statusLines int,
	statusBytes int,
	reportBytes int,
) managedProbeResult {
	taskCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	type stateResult struct {
		current CurrentState
		err     error
	}
	type agentResult struct {
		state string
		err   error
	}
	stateResults := make(chan stateResult, 1)
	agentResults := make(chan agentResult, 1)
	go func() {
		current, stateErr := loader.States.Resolve(taskCtx, meta.ID)
		stateResults <- stateResult{current: current, err: stateErr}
	}()
	go func() {
		state, agentErr := loader.Agents.ResolveAgentState(taskCtx, meta.ID)
		agentResults <- agentResult{state: state, err: agentErr}
	}()

	resolvedState := <-stateResults
	resolvedAgent := <-agentResults
	current := resolvedState.current
	if resolvedState.err != nil {
		current = CurrentState{
			State:  "unknown",
			Source: "adapter-error",
			Detail: resolvedState.err.Error(),
		}
	}
	if resolvedAgent.err != nil || resolvedAgent.state != "alive" || isTerminalState(current.State) {
		return managedProbeResult{}
	}
	events, err := ReadStatusEvents(
		filepath.Join(loader.Home.StateDir, meta.ID+".status"),
		statusLines,
		statusBytes,
	)
	if err != nil {
		return managedProbeResult{err: err}
	}
	report, err := ReadReport(
		filepath.Join(loader.Home.DataDir, meta.ID, "report.md"),
		reportBytes,
	)
	if err != nil {
		return managedProbeResult{err: err}
	}
	return managedProbeResult{
		include: true,
		task: Task{
			Metadata:  meta,
			Current:   current,
			Events:    events,
			Report:    report,
			Ownership: FirstmateManaged,
			Target:    meta.ID,
		},
	}
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
