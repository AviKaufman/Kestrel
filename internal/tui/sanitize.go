package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/kunchenguid/firstmate/internal/firstmate"
)

func safeScalar(value string) string {
	return strings.Map(func(character rune) rune {
		switch character {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, ansi.Strip(value))
}

func safeMultiline(value string) string {
	value = strings.ReplaceAll(ansi.Strip(value), "\t", "    ")
	return strings.Map(func(character rune) rune {
		if character == '\n' {
			return character
		}
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, value)
}

func safeMultilineLines(lines []string) []string {
	sanitized := make([]string, len(lines))
	for index, line := range lines {
		sanitized[index] = safeMultiline(line)
	}
	return sanitized
}

func safeHub(hub HubState) HubState {
	hub.Target.Backend = safeScalar(hub.Target.Backend)
	hub.Target.Target = safeScalar(hub.Target.Target)
	hub.History = safeMultilineLines(hub.History)
	return hub
}

func safeTasks(tasks []firstmate.Task) []firstmate.Task {
	sanitized := make([]firstmate.Task, len(tasks))
	for index, task := range tasks {
		task.Metadata.ID = safeScalar(task.Metadata.ID)
		task.Metadata.Project = safeScalar(task.Metadata.Project)
		task.Metadata.Kind = safeScalar(task.Metadata.Kind)
		task.Metadata.Mode = safeScalar(task.Metadata.Mode)
		task.Metadata.Yolo = safeScalar(task.Metadata.Yolo)
		task.Metadata.Harness = safeScalar(task.Metadata.Harness)
		task.Metadata.Model = safeScalar(task.Metadata.Model)
		task.Metadata.Effort = safeScalar(task.Metadata.Effort)
		task.Metadata.Worktree = safeScalar(task.Metadata.Worktree)
		task.Metadata.Window = safeScalar(task.Metadata.Window)
		task.Current.State = safeScalar(task.Current.State)
		task.Current.Source = safeScalar(task.Current.Source)
		task.Current.Detail = safeScalar(task.Current.Detail)
		task.Current.Raw = safeScalar(task.Current.Raw)
		task.Events = append([]firstmate.StatusEvent(nil), task.Events...)
		for eventIndex := range task.Events {
			task.Events[eventIndex].Verb = safeScalar(task.Events[eventIndex].Verb)
			task.Events[eventIndex].Note = safeMultiline(task.Events[eventIndex].Note)
			task.Events[eventIndex].Raw = safeMultiline(task.Events[eventIndex].Raw)
		}
		task.Report.Path = safeScalar(task.Report.Path)
		task.Report.Content = safeMultiline(task.Report.Content)
		task.Target = safeScalar(task.Target)
		sanitized[index] = task
	}
	return sanitized
}
