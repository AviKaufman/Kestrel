package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/kunchenguid/firstmate/internal/firstmate"
)

func safeText(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' {
			return character
		}
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, ansi.Strip(value))
}

func safeLines(lines []string) []string {
	sanitized := make([]string, len(lines))
	for index, line := range lines {
		sanitized[index] = safeText(line)
	}
	return sanitized
}

func safeHub(hub HubState) HubState {
	hub.Target.Backend = safeText(hub.Target.Backend)
	hub.Target.Target = safeText(hub.Target.Target)
	hub.History = safeLines(hub.History)
	return hub
}

func safeTasks(tasks []firstmate.Task) []firstmate.Task {
	sanitized := make([]firstmate.Task, len(tasks))
	for index, task := range tasks {
		task.Metadata.ID = safeText(task.Metadata.ID)
		task.Metadata.Project = safeText(task.Metadata.Project)
		task.Metadata.Kind = safeText(task.Metadata.Kind)
		task.Metadata.Mode = safeText(task.Metadata.Mode)
		task.Metadata.Yolo = safeText(task.Metadata.Yolo)
		task.Metadata.Harness = safeText(task.Metadata.Harness)
		task.Metadata.Model = safeText(task.Metadata.Model)
		task.Metadata.Effort = safeText(task.Metadata.Effort)
		task.Metadata.Worktree = safeText(task.Metadata.Worktree)
		task.Metadata.Window = safeText(task.Metadata.Window)
		task.Current.State = safeText(task.Current.State)
		task.Current.Source = safeText(task.Current.Source)
		task.Current.Detail = safeText(task.Current.Detail)
		task.Current.Raw = safeText(task.Current.Raw)
		for eventIndex := range task.Events {
			task.Events[eventIndex].Verb = safeText(task.Events[eventIndex].Verb)
			task.Events[eventIndex].Note = safeText(task.Events[eventIndex].Note)
			task.Events[eventIndex].Raw = safeText(task.Events[eventIndex].Raw)
		}
		task.Report.Path = safeText(task.Report.Path)
		task.Report.Content = safeText(task.Report.Content)
		task.Target = safeText(task.Target)
		sanitized[index] = task
	}
	return sanitized
}
