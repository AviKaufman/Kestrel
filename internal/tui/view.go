package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kunchenguid/firstmate/internal/firstmate"
)

var (
	colorInk    = lipgloss.Color("#e0def4")
	colorMuted  = lipgloss.Color("#6e6a86")
	colorSubtle = lipgloss.Color("#908caa")
	colorFoam   = lipgloss.Color("#9ccfd8")
	colorIris   = lipgloss.Color("#c4a7e7")
	colorGold   = lipgloss.Color("#f6c177")
	colorLove   = lipgloss.Color("#eb6f92")

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorInk)
	labelStyle  = lipgloss.NewStyle().Foreground(colorSubtle)
	focusStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorFoam)
	tabStyle    = lipgloss.NewStyle().Foreground(colorSubtle)
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(colorFoam)
	errorStyle  = lipgloss.NewStyle().Foreground(colorLove)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorInk).
			Background(lipgloss.Color("#26233a"))
)

func (model Model) render() string {
	width := max(40, model.width)
	height := max(16, model.height)
	header := renderHeader(model, width)
	bodyHeight := max(10, height-lipgloss.Height(header))
	var body string
	if width < 76 {
		listHeight := min(5, max(3, bodyHeight/4))
		list := renderDestinationList(model, width, listHeight)
		inspector := renderInspector(model, width, max(6, bodyHeight-listHeight))
		body = lipgloss.JoinVertical(lipgloss.Left, list, inspector)
	} else {
		leftWidth := max(24, width/4)
		rightWidth := max(40, width-leftWidth)
		list := renderDestinationList(model, leftWidth, bodyHeight)
		inspector := renderInspector(model, rightWidth, bodyHeight)
		body = lipgloss.JoinHorizontal(lipgloss.Top, list, inspector)
	}
	if model.err != nil {
		body = fitLines(errorStyle.Render("read error: "+model.err.Error())+"\n"+body, width)
	}
	frame := fitFrame(header+"\n"+body, width, height)
	if model.creatingPrivate {
		frame = overlayModal(frame, renderPrivateCreate(model, width, height), width, height)
	}
	if model.helpVisible {
		frame = overlayModal(frame, renderHelp(width, height), width, height)
	}
	return frame
}

func renderHeader(model Model, width int) string {
	managed := countOwnership(model.tasks, firstmate.FirstmateManaged)
	private := countOwnership(model.tasks, firstmate.CaptainPrivate)
	tab := func(destination Destination, label string) string {
		if model.destination == destination {
			return "[" + label + "]"
		}
		return label
	}
	content := fmt.Sprintf(
		" firstmate | 1 %s  2 %s %d  3 %s %d | workers %d | n new | home %s | ? help ",
		tab(HubDestination, "HUB"),
		tab(ManagedDestination, "MANAGED"),
		managed,
		tab(PrivateDestination, "PRIVATE"),
		private,
		len(model.tasks),
		model.home,
	)
	return headerStyle.Width(width).Render(ansi.Truncate(content, width, "…"))
}

func renderDestinationList(model Model, width, height int) string {
	innerWidth := max(1, width-2)
	if model.destination == HubDestination {
		lines := []string{
			titleStyle.Render("FIRSTMATE"),
			focusStyle.Render("> Firstmate hub"),
			labelStyle.Render("  primary supervisor"),
		}
		return panelStyle(width, height).Render(fitBlock(strings.Join(lines, "\n"), innerWidth, max(1, height-2)))
	}
	tasks := model.destinationTasks()
	title := "MANAGED WORKERS"
	empty := "No active Firstmate-managed workers."
	if model.destination == PrivateDestination {
		title = "PRIVATE CODEX"
		empty = "No active private Codex threads."
	}
	lines := []string{titleStyle.Render(title)}
	if len(tasks) == 0 {
		lines = append(lines, labelStyle.Render(empty))
		if model.destination == PrivateDestination {
			lines = append(lines, labelStyle.Render("n creates a private Codex thread."))
		}
	} else {
		for index, task := range tasks {
			prefix := "  "
			style := lipgloss.NewStyle().Foreground(colorInk)
			if index == model.selected {
				prefix = "> "
				style = focusStyle
			}
			state := valueOrDash(task.Current.State)
			row := fmt.Sprintf("%s%s", prefix, task.Metadata.ID)
			detail := fmt.Sprintf("  %s | %s | %s", valueOrDash(task.Metadata.Kind), state, valueOrDash(task.Current.Source))
			lines = append(lines, style.Render(ansi.Truncate(row, innerWidth, "…")))
			lines = append(lines, labelStyle.Render(ansi.Truncate(detail, innerWidth, "…")))
		}
	}
	return panelStyle(width, height).Render(fitBlock(strings.Join(lines, "\n"), innerWidth, max(1, height-2)))
}

func renderInspector(model Model, width, height int) string {
	innerWidth := max(1, width-2)
	if model.destination == HubDestination {
		return renderHubInspector(model, width, height)
	}
	tasks := model.destinationTasks()
	if len(tasks) == 0 {
		empty := "No active Firstmate-managed workers."
		if model.destination == PrivateDestination {
			empty = "No active private Codex threads."
		}
		return panelStyle(width, height).Render(labelStyle.Render(empty))
	}
	task := tasks[model.selected]
	meta := task.Metadata
	var header []string
	if width < 76 {
		header = []string{
			titleStyle.Render("TASK / "+meta.ID) + "  " + ownershipStyle(task.Ownership).Render("["+string(task.Ownership)+"]"),
			fmt.Sprintf("%s %s  %s %s",
				labelStyle.Render("STATE"),
				stateStyle(task.Current.State).Render(valueOrDash(task.Current.State)),
				labelStyle.Render("SOURCE"),
				valueOrDash(task.Current.Source),
			),
			fmt.Sprintf("%s %s  %s %s  %s %s",
				labelStyle.Render("PROJECT"), valueOrDash(meta.Project),
				labelStyle.Render("KIND"), valueOrDash(meta.Kind),
				labelStyle.Render("MODE"), valueOrDash(meta.Mode),
			),
			renderModeSwitch(model.outputMode),
		}
	} else {
		header = []string{
			titleStyle.Render("TASK / " + meta.ID),
			fmt.Sprintf("%s %s", labelStyle.Render("OWNERSHIP"), ownershipStyle(task.Ownership).Render(string(task.Ownership))),
			fmt.Sprintf("%s %s  %s %s",
				labelStyle.Render("CURRENT STATE"),
				stateStyle(task.Current.State).Render(valueOrDash(task.Current.State)),
				labelStyle.Render("SOURCE"),
				valueOrDash(task.Current.Source),
			),
			fmt.Sprintf("%s %s", labelStyle.Render("DETAIL"), valueOrDash(task.Current.Detail)),
			fmt.Sprintf("%s %s  %s %s  %s %s",
				labelStyle.Render("PROJECT"), valueOrDash(meta.Project),
				labelStyle.Render("KIND"), valueOrDash(meta.Kind),
				labelStyle.Render("MODE"), valueOrDash(meta.Mode),
			),
			fmt.Sprintf("%s %s  %s %s  %s %s  %s %s",
				labelStyle.Render("YOLO"), valueOrDash(meta.Yolo),
				labelStyle.Render("HARNESS"), valueOrDash(meta.Harness),
				labelStyle.Render("MODEL"), valueOrDash(meta.Model),
				labelStyle.Render("EFFORT"), valueOrDash(meta.Effort),
			),
			fmt.Sprintf("%s %s", labelStyle.Render("WORKTREE"), valueOrDash(meta.Worktree)),
			fmt.Sprintf("%s %s", labelStyle.Render("WINDOW"), valueOrDash(meta.Window)),
			renderModeSwitch(model.outputMode),
		}
	}

	var output string
	if model.outputMode == ReportsMode {
		output = renderReport(task)
	} else {
		live := model.liveLines
		if model.liveTaskID != meta.ID {
			live = nil
		}
		output = renderLive(task, live)
	}
	headerContent := ansi.Hardwrap(strings.Join(header, "\n"), innerWidth, false)
	composer := renderComposer(model, task, innerWidth)
	availableOutputHeight := max(3, height-2-lipgloss.Height(headerContent)-lipgloss.Height(composer)-2)
	output = fitBlock(ansi.Hardwrap(output, innerWidth, false), innerWidth, availableOutputHeight)
	content := headerContent + "\n\n" + output + "\n" + composer
	return panelStyle(width, height).Render(fitBlock(content, innerWidth, max(1, height-2)))
}

func ownershipStyle(ownership firstmate.Ownership) lipgloss.Style {
	if ownership == firstmate.CaptainPrivate {
		return lipgloss.NewStyle().Bold(true).Foreground(colorIris)
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colorFoam)
}

func renderModeSwitch(mode OutputMode) string {
	reports := tabStyle.Render("REPORTS")
	live := tabStyle.Render("LIVE")
	if mode == ReportsMode {
		reports = activeTab.Render("[REPORTS]")
	} else {
		live = activeTab.Render("[LIVE]")
	}
	return reports + "  " + live
}

func renderComposer(model Model, task firstmate.Task, width int) string {
	return renderComposerWithLabel(model, string(task.Ownership), width)
}

func renderComposerWithLabel(model Model, label string, width int) string {
	focus := "i to focus"
	style := labelStyle
	if model.composer.Focused() {
		focus = "focused - enter sends, esc returns"
		style = focusStyle
	}
	status := model.sendStatus
	if status == "" {
		status = focus
	}
	return strings.Join([]string{
		style.Render("MESSAGE / " + label),
		fitLines(model.composer.View(), width),
		fitLines(labelStyle.Render(status), width),
	}, "\n")
}

func renderHubInspector(model Model, width, height int) string {
	innerWidth := max(1, width-2)
	target := fmt.Sprintf("%s %s", valueOrDash(model.hub.Target.Backend), valueOrDash(model.hub.Target.Target))
	if model.hub.Err != nil {
		target = errorStyle.Render("unavailable: " + model.hub.Err.Error())
	}
	content := strings.Join([]string{
		titleStyle.Render("FIRSTMATE HUB"),
		labelStyle.Render("DESTINATION") + " primary supervisor session",
		labelStyle.Render("TARGET") + " " + target,
		"",
		"Send a message to the active Firstmate supervisor.",
		"No worker selection is required.",
		"",
		renderComposerWithLabel(model, "Firstmate hub", innerWidth),
	}, "\n")
	return panelStyle(width, height).Render(fitBlock(content, innerWidth, max(1, height-2)))
}

func renderReport(task firstmate.Task) string {
	lines := []string{labelStyle.Render("OFFICIAL OUTPUT / DURABLE REPORT")}
	if task.Ownership == firstmate.CaptainPrivate {
		lines = append(lines, "Direct Codex sessions have no durable Firstmate report.")
		return strings.Join(lines, "\n")
	}
	report := task.Report
	if !report.Present {
		lines = append(lines, "No durable report present at "+report.Path)
		return strings.Join(lines, "\n")
	}
	lines = append(lines, labelStyle.Render("path: ")+report.Path, "", strings.TrimSpace(report.Content))
	if report.Truncated {
		lines = append(lines, "", labelStyle.Render("[report truncated at configured byte bound]"))
	}
	return strings.Join(lines, "\n")
}

func renderLive(task firstmate.Task, live []string) string {
	lines := []string{
		labelStyle.Render("STATUS EVENT HISTORY (bounded; not current-state truth)"),
	}
	if len(task.Events) == 0 {
		lines = append(lines, "No status events present.")
	} else {
		for _, event := range task.Events {
			prefix := ""
			if event.Truncated {
				prefix = "[earlier events omitted] "
			}
			lines = append(lines, prefix+event.Raw)
		}
	}
	lines = append(lines, "", labelStyle.Render("WORKER CAPTURE (bounded; read-only)"))
	if len(live) == 0 {
		lines = append(lines, "No worker capture available.")
	} else {
		lines = append(lines, live...)
	}
	return strings.Join(lines, "\n")
}

func renderHelp(width, height int) string {
	lines := []string{
		focusStyle.Render("FOCUSED MODAL / KEYBOARD HELP"),
		"",
		"j / down       next task",
		"k / up         previous task",
		"g / G          first / last task",
		"tab / shift-tab next / previous destination",
		"1 / 2 / 3      Hub / Managed / Private",
		"left / right   Reports / Live",
		"enter          switch Reports / Live",
		"esc            close help or return to Reports",
		"r              refresh current Firstmate reads",
		"i              focus destination message composer",
		"n              create a private Codex thread",
		"enter          send while composer is focused",
		"?              toggle this help",
		"q              quit",
		"",
		labelStyle.Render("Hub messages route to the current primary supervisor."),
	}
	modalWidth := min(60, max(32, width-4))
	modalHeight := min(18, max(10, height-4))
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorGold).
		Width(max(1, modalWidth-2)).
		Height(max(1, modalHeight-2)).
		Render(fitBlock(strings.Join(lines, "\n"), max(1, modalWidth-2), max(1, modalHeight-2)))
}

func renderPrivateCreate(model Model, width, height int) string {
	status := model.createStatus
	if status == "" {
		status = "enter creates, esc cancels"
	}
	lines := []string{
		focusStyle.Render("FOCUSED MODAL / NEW PRIVATE CODEX"),
		"",
		"Create a captain-private Codex thread.",
		labelStyle.Render("Working directory is fixed by --private-cwd."),
		"",
		model.privateInput.View(),
		labelStyle.Render(status),
	}
	modalWidth := min(58, max(34, width-4))
	modalHeight := min(12, max(9, height-4))
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorIris).
		Width(max(1, modalWidth-2)).
		Height(max(1, modalHeight-2)).
		Render(fitBlock(strings.Join(lines, "\n"), max(1, modalWidth-2), max(1, modalHeight-2)))
}

func overlayModal(base, modal string, width, height int) string {
	baseLines := strings.Split(fitFrame(base, width, height), "\n")
	frameHeight := len(baseLines)
	modalLines := strings.Split(modal, "\n")
	modalWidth := 0
	for _, line := range modalLines {
		modalWidth = max(modalWidth, lipgloss.Width(line))
	}
	startX := max(0, (width-modalWidth)/2)
	startY := max(0, (frameHeight-len(modalLines))/2)
	for index, modalLine := range modalLines {
		row := startY + index
		if row >= frameHeight {
			break
		}
		baseLine := padLine(baseLines[row], width)
		left := ansi.Cut(baseLine, 0, startX)
		right := ansi.Cut(baseLine, startX+modalWidth, width)
		baseLines[row] = left + modalLine + right
	}
	return strings.Join(baseLines, "\n")
}

func padLine(line string, width int) string {
	if current := lipgloss.Width(line); current < width {
		return line + strings.Repeat(" ", width-current)
	}
	return ansi.Truncate(line, width, "")
}

func panelStyle(width, height int) lipgloss.Style {
	contentWidth := max(1, width-2)
	contentHeight := max(1, height-2)
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorMuted).
		Width(contentWidth).
		Height(contentHeight)
}

func stateStyle(state string) lipgloss.Style {
	color := colorSubtle
	switch state {
	case "working":
		color = colorFoam
	case "done":
		color = colorGold
	case "blocked", "failed":
		color = colorLove
	case "parked", "paused":
		color = colorIris
	}
	return lipgloss.NewStyle().Foreground(color)
}

func fitLines(content string, width int) string {
	if width <= 0 {
		return ""
	}
	wrapped := ansi.Hardwrap(content, width, false)
	lines := strings.Split(wrapped, "\n")
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}

func fitFrame(content string, width, height int) string {
	content = fitLines(content, width)
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func fitBlock(content string, width, height int) string {
	content = fitLines(content, width)
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func countOwnership(tasks []firstmate.Task, ownership firstmate.Ownership) int {
	count := 0
	for _, task := range tasks {
		if task.Ownership == ownership {
			count++
		}
	}
	return count
}
