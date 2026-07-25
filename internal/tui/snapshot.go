package tui

import (
	"fmt"
	"strings"

	"github.com/kunchenguid/firstmate/internal/firstmate"
)

// RenderSnapshot emits an ANSI-free, stable review and test surface.
func RenderSnapshot(home string, tasks []firstmate.Task, live []string) string {
	var output strings.Builder
	fmt.Fprintln(&output, "FIRSTMATE TUI SNAPSHOT")
	fmt.Fprintf(&output, "home: %s\n", home)
	fmt.Fprintf(&output, "tasks: %d\n", len(tasks))
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "TASKS")
	if len(tasks) == 0 {
		fmt.Fprintln(&output, "No task metadata found.")
		return output.String()
	}
	var previous firstmate.Ownership
	for index, task := range tasks {
		if index == 0 || task.Ownership != previous {
			fmt.Fprintf(&output, "[%s]\n", task.Ownership)
			previous = task.Ownership
		}
		prefix := "  "
		if index == 0 {
			prefix = "> "
		}
		fmt.Fprintf(
			&output,
			"%s%s | %s | %s | %s\n",
			prefix,
			task.Metadata.ID,
			valueOrDash(task.Metadata.Kind),
			valueOrDash(task.Current.State),
			valueOrDash(task.Current.Source),
		)
	}

	task := tasks[0]
	meta := task.Metadata
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "INSPECTOR")
	fmt.Fprintf(&output, "task id: %s\n", meta.ID)
	fmt.Fprintf(&output, "ownership: %s\n", task.Ownership)
	fmt.Fprintf(&output, "current state: %s\n", valueOrDash(task.Current.State))
	fmt.Fprintf(&output, "current source: %s\n", valueOrDash(task.Current.Source))
	fmt.Fprintf(&output, "current detail: %s\n", valueOrDash(task.Current.Detail))
	fmt.Fprintf(&output, "project: %s\n", valueOrDash(meta.Project))
	fmt.Fprintf(&output, "kind: %s\n", valueOrDash(meta.Kind))
	fmt.Fprintf(&output, "mode: %s\n", valueOrDash(meta.Mode))
	fmt.Fprintf(&output, "yolo: %s\n", valueOrDash(meta.Yolo))
	fmt.Fprintf(&output, "harness: %s\n", valueOrDash(meta.Harness))
	fmt.Fprintf(&output, "model: %s\n", valueOrDash(meta.Model))
	fmt.Fprintf(&output, "effort: %s\n", valueOrDash(meta.Effort))
	fmt.Fprintf(&output, "worktree: %s\n", valueOrDash(meta.Worktree))
	fmt.Fprintf(&output, "window: %s\n", valueOrDash(meta.Window))

	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "REPORTS")
	if task.Report.Present {
		fmt.Fprintf(&output, "path: %s\n", task.Report.Path)
		fmt.Fprintln(&output, strings.TrimSpace(task.Report.Content))
		if task.Report.Truncated {
			fmt.Fprintln(&output, "[report truncated at configured byte bound]")
		}
	} else {
		fmt.Fprintf(&output, "No durable report present at %s\n", task.Report.Path)
	}

	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "STATUS EVENT HISTORY (bounded; not current-state truth)")
	if len(task.Events) == 0 {
		fmt.Fprintln(&output, "No status events present.")
	} else {
		for _, event := range task.Events {
			if event.Truncated {
				fmt.Fprint(&output, "[earlier events omitted] ")
			}
			fmt.Fprintln(&output, event.Raw)
		}
	}

	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "WORKER CAPTURE (bounded; read-only)")
	if len(live) == 0 {
		fmt.Fprintln(&output, "No worker capture available.")
	} else {
		for _, line := range live {
			fmt.Fprintln(&output, line)
		}
	}
	return output.String()
}
