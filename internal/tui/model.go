package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kunchenguid/firstmate/internal/firstmate"
)

const interactiveReadTimeout = 15 * time.Second

// OutputMode selects the dominant output region.
type OutputMode int

const (
	ReportsMode OutputMode = iota
	LiveMode
)

// Source refreshes read-only Firstmate task and worker output.
type Source interface {
	Load(context.Context) ([]firstmate.Task, error)
	LoadLive(context.Context, string) ([]string, error)
}

// Model is the keyboard-driven root Bubble Tea model.
type Model struct {
	home        string
	tasks       []firstmate.Task
	selected    int
	outputMode  OutputMode
	liveTaskID  string
	liveLines   []string
	source      Source
	width       int
	height      int
	helpVisible bool
	err         error
	keys        keyMap
	help        help.Model
}

type fleetLoadedMsg struct {
	tasks []firstmate.Task
	err   error
}

type liveLoadedMsg struct {
	taskID string
	lines  []string
	err    error
}

func NewModel(home string, tasks []firstmate.Task, live []string, source Source) Model {
	helpModel := help.New()
	helpModel.Styles.ShortKey = lipgloss.NewStyle().Foreground(colorGold)
	helpModel.Styles.ShortDesc = lipgloss.NewStyle().Foreground(colorSubtle)
	helpModel.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(colorMuted)
	helpModel.Styles.FullKey = helpModel.Styles.ShortKey
	helpModel.Styles.FullDesc = helpModel.Styles.ShortDesc
	helpModel.Styles.FullSeparator = helpModel.Styles.ShortSeparator

	model := Model{
		home:       home,
		tasks:      tasks,
		liveLines:  live,
		source:     source,
		width:      110,
		height:     34,
		keys:       defaultKeys(),
		help:       helpModel,
		outputMode: ReportsMode,
	}
	if len(tasks) > 0 {
		model.liveTaskID = tasks[0].Metadata.ID
	}
	return model
}

func (model Model) Init() tea.Cmd {
	return nil
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		model.width = max(40, message.Width)
		model.height = max(16, message.Height)
		model.help.SetWidth(model.width - 2)
		return model, nil
	case fleetLoadedMsg:
		if message.err != nil {
			model.err = message.err
			return model, nil
		}
		selectedID := model.selectedID()
		model.tasks = message.tasks
		model.selected = indexOfTask(model.tasks, selectedID)
		if model.selected < 0 {
			model.selected = 0
		}
		model.err = nil
		return model, model.loadSelectedLiveIfVisible()
	case liveLoadedMsg:
		if message.taskID != model.selectedID() {
			return model, nil
		}
		model.liveTaskID = message.taskID
		model.liveLines = message.lines
		model.err = message.err
		return model, nil
	case tea.KeyPressMsg:
		if key.Matches(message, model.keys.Quit) {
			return model, tea.Quit
		}
		if key.Matches(message, model.keys.Help) {
			model.helpVisible = !model.helpVisible
			model.help.ShowAll = model.helpVisible
			return model, nil
		}
		if key.Matches(message, model.keys.Back) {
			if model.helpVisible {
				model.helpVisible = false
				model.help.ShowAll = false
			} else {
				model.outputMode = ReportsMode
			}
			return model, nil
		}
		if model.helpVisible {
			return model, nil
		}
		switch {
		case key.Matches(message, model.keys.Up):
			if model.selected > 0 {
				model.selected--
				return model, model.loadSelectedLiveIfVisible()
			}
		case key.Matches(message, model.keys.Down):
			if model.selected+1 < len(model.tasks) {
				model.selected++
				return model, model.loadSelectedLiveIfVisible()
			}
		case key.Matches(message, model.keys.Top):
			if len(model.tasks) > 0 {
				model.selected = 0
				return model, model.loadSelectedLiveIfVisible()
			}
		case key.Matches(message, model.keys.Bottom):
			if len(model.tasks) > 0 {
				model.selected = len(model.tasks) - 1
				return model, model.loadSelectedLiveIfVisible()
			}
		case key.Matches(message, model.keys.Reports):
			model.outputMode = ReportsMode
		case key.Matches(message, model.keys.Live):
			model.outputMode = LiveMode
			return model, model.loadSelectedLive()
		case key.Matches(message, model.keys.Toggle):
			if model.outputMode == ReportsMode {
				model.outputMode = LiveMode
				return model, model.loadSelectedLive()
			}
			model.outputMode = ReportsMode
		case key.Matches(message, model.keys.Refresh):
			return model, model.refresh()
		}
	}
	return model, nil
}

func (model Model) View() tea.View {
	view := tea.NewView(model.render())
	view.AltScreen = true
	view.WindowTitle = "Firstmate TUI"
	return view
}

func (model Model) Selected() int {
	return model.selected
}

func (model Model) OutputMode() OutputMode {
	return model.outputMode
}

func (model Model) HelpVisible() bool {
	return model.helpVisible
}

func (model Model) selectedID() string {
	if model.selected < 0 || model.selected >= len(model.tasks) {
		return ""
	}
	return model.tasks[model.selected].Metadata.ID
}

func (model Model) refresh() tea.Cmd {
	if model.source == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), interactiveReadTimeout)
		defer cancel()
		tasks, err := model.source.Load(ctx)
		return fleetLoadedMsg{tasks: tasks, err: err}
	}
}

func (model Model) loadSelectedLive() tea.Cmd {
	if model.source == nil || model.selectedID() == "" {
		return nil
	}
	taskID := model.selectedID()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), interactiveReadTimeout)
		defer cancel()
		lines, err := model.source.LoadLive(ctx, taskID)
		return liveLoadedMsg{taskID: taskID, lines: lines, err: err}
	}
}

func (model Model) loadSelectedLiveIfVisible() tea.Cmd {
	if model.outputMode != LiveMode {
		return nil
	}
	return model.loadSelectedLive()
}

func indexOfTask(tasks []firstmate.Task, taskID string) int {
	for index, task := range tasks {
		if task.Metadata.ID == taskID {
			return index
		}
	}
	if len(tasks) == 0 {
		return -1
	}
	return 0
}
