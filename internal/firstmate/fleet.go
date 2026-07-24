package firstmate

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultStatusLines = 20
	defaultStatusBytes = 64 * 1024
	defaultReportBytes = 64 * 1024
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
	Metadata Metadata
	Current  CurrentState
	Events   []StatusEvent
	Report   Report
}

// Loader joins task metadata, current state, bounded event history, and reports.
type Loader struct {
	Home           Home
	States         StateResolver
	Live           LiveReader
	StatusMaxLines int
	StatusMaxBytes int
	ReportMaxBytes int
}

func (loader Loader) Load(ctx context.Context) ([]Task, error) {
	if loader.States == nil {
		return nil, fmt.Errorf("current-state resolver is required")
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

	tasks := make([]Task, 0, len(metas))
	for _, meta := range metas {
		current, stateErr := loader.States.Resolve(ctx, meta.ID)
		if stateErr != nil {
			current = CurrentState{
				State:  "unknown",
				Source: "adapter-error",
				Detail: stateErr.Error(),
			}
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
			Metadata: meta,
			Current:  current,
			Events:   events,
			Report:   report,
		})
	}
	return tasks, nil
}

func (loader Loader) LoadLive(ctx context.Context, taskID string) ([]string, error) {
	if loader.Live == nil {
		return nil, fmt.Errorf("live reader is required")
	}
	return loader.Live.Read(ctx, taskID)
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
