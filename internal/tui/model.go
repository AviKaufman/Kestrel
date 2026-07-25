package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/kunchenguid/firstmate/internal/firstmate"
)

const (
	interactiveReadTimeout = 15 * time.Second
	fleetRefreshTimeout    = 2 * time.Minute
)

// OutputMode selects the dominant output region.
type OutputMode int

const (
	ReportsMode OutputMode = iota
	LiveMode
)

// Destination selects the top-level navigation context.
type Destination int

const (
	HubDestination Destination = iota
	ManagedDestination
	PrivateDestination
)

// HubState records the current primary supervisor destination or its discovery error.
type HubState struct {
	Target     firstmate.HubTarget
	History    []string
	Err        error
	HistoryErr error
}

// Source refreshes read-only Firstmate task and worker output.
type Source interface {
	Load(context.Context) ([]firstmate.Task, error)
	LoadLive(context.Context, firstmate.Task) ([]string, error)
	LoadHub(context.Context) (firstmate.HubTarget, error)
	LoadHubHistory(context.Context, firstmate.HubTarget) ([]string, error)
	Send(context.Context, firstmate.Task, string) error
	SendHub(context.Context, firstmate.HubTarget, string) error
	CreatePrivate(context.Context, string) (firstmate.DirectSession, error)
}

// Model is the keyboard-driven root Bubble Tea model.
type Model struct {
	home            string
	hub             HubState
	tasks           []firstmate.Task
	destination     Destination
	selected        int
	outputMode      OutputMode
	liveTaskID      string
	liveLines       []string
	source          Source
	width           int
	height          int
	helpVisible     bool
	err             error
	keys            keyMap
	composer        textinput.Model
	sendStatus      string
	sending         bool
	privateInput    textinput.Model
	creatingPrivate bool
	createStatus    string
	creating        bool
}

type fleetLoadedMsg struct {
	tasks         []firstmate.Task
	hub           firstmate.HubTarget
	hubErr        error
	hubHistory    []string
	hubHistoryErr error
	err           error
}

type liveLoadedMsg struct {
	taskID string
	lines  []string
	err    error
}

type sendFinishedMsg struct {
	label string
	draft string
	err   error
}

type privateCreatedMsg struct {
	session firstmate.DirectSession
	tasks   []firstmate.Task
	err     error
}

func NewModel(home string, hub HubState, tasks []firstmate.Task, live []string, source Source) Model {
	composer := textinput.New()
	composer.Prompt = "> "
	composer.Placeholder = "Write a short message to the selected worker"
	composer.CharLimit = firstmate.MaxMessageBytes
	composer.SetWidth(60)
	privateInput := textinput.New()
	privateInput.Prompt = "label: "
	privateInput.Placeholder = "notes"
	privateInput.CharLimit = 32
	privateInput.SetWidth(36)

	model := Model{
		home:         home,
		hub:          hub,
		tasks:        tasks,
		liveLines:    live,
		source:       source,
		width:        110,
		height:       34,
		keys:         defaultKeys(),
		outputMode:   ReportsMode,
		composer:     composer,
		privateInput: privateInput,
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
		model.composer.SetWidth(max(16, model.width-model.width/4-8))
		return model, nil
	case fleetLoadedMsg:
		if message.err != nil {
			model.err = message.err
			return model, nil
		}
		selectedID := model.selectedID()
		model.tasks = message.tasks
		model.hub = HubState{
			Target:     message.hub,
			History:    message.hubHistory,
			Err:        message.hubErr,
			HistoryErr: message.hubHistoryErr,
		}
		model.selected = indexOfTask(model.destinationTasks(), selectedID)
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
	case sendFinishedMsg:
		model.sending = false
		if message.err != nil {
			model.sendStatus = "Send to " + message.label + " failed: " + message.err.Error()
			return model, nil
		}
		if model.composer.Value() == message.draft {
			model.composer.Reset()
		}
		model.sendStatus = "Sent to " + message.label
		return model, nil
	case privateCreatedMsg:
		model.creating = false
		if message.err != nil {
			model.createStatus = "Create failed: " + message.err.Error()
			return model, nil
		}
		selected := indexOfPrivateTarget(message.tasks, message.session.Target)
		if selected < 0 {
			model.createStatus = "Create failed: created session was not rediscovered"
			return model, nil
		}
		model.tasks = message.tasks
		model.destination = PrivateDestination
		model.selected = selected
		model.creatingPrivate = false
		model.privateInput.Blur()
		model.privateInput.Reset()
		model.createStatus = "Created " + message.session.Target
		model.outputMode = ReportsMode
		model.liveLines = nil
		model.liveTaskID = ""
		return model, nil
	case tea.KeyPressMsg:
		if model.creatingPrivate {
			switch message.String() {
			case "ctrl+c":
				return model, tea.Quit
			case "esc":
				model.creatingPrivate = false
				model.creating = false
				model.privateInput.Blur()
				model.privateInput.Reset()
				model.createStatus = ""
				return model, nil
			case "enter":
				return model, model.createPrivate()
			default:
				var command tea.Cmd
				model.privateInput, command = model.privateInput.Update(message)
				return model, command
			}
		}
		if model.composer.Focused() {
			switch message.String() {
			case "ctrl+c":
				return model, tea.Quit
			case "esc":
				model.composer.Blur()
				model.sendStatus = ""
				return model, nil
			case "enter":
				return model, model.sendDraft()
			default:
				var command tea.Cmd
				model.composer, command = model.composer.Update(message)
				return model, command
			}
		}
		if key.Matches(message, model.keys.Quit) {
			return model, tea.Quit
		}
		if key.Matches(message, model.keys.Help) {
			model.helpVisible = !model.helpVisible
			return model, nil
		}
		if key.Matches(message, model.keys.Back) {
			if model.helpVisible {
				model.helpVisible = false
			} else {
				model.outputMode = ReportsMode
			}
			return model, nil
		}
		if model.helpVisible {
			return model, nil
		}
		if message.String() == "n" {
			model.creatingPrivate = true
			model.createStatus = ""
			return model, model.privateInput.Focus()
		}
		switch message.String() {
		case "tab":
			return model.switchDestination((model.destination + 1) % 3)
		case "shift+tab":
			return model.switchDestination((model.destination + 2) % 3)
		case "1":
			return model.switchDestination(HubDestination)
		case "2":
			return model.switchDestination(ManagedDestination)
		case "3":
			return model.switchDestination(PrivateDestination)
		}
		destinationTasks := model.destinationTasks()
		switch {
		case key.Matches(message, model.keys.Up):
			if model.selected > 0 {
				model.selected--
				return model, model.loadSelectedLiveIfVisible()
			}
		case key.Matches(message, model.keys.Down):
			if model.selected+1 < len(destinationTasks) {
				model.selected++
				return model, model.loadSelectedLiveIfVisible()
			}
		case key.Matches(message, model.keys.Top):
			if len(destinationTasks) > 0 {
				model.selected = 0
				return model, model.loadSelectedLiveIfVisible()
			}
		case key.Matches(message, model.keys.Bottom):
			if len(destinationTasks) > 0 {
				model.selected = len(destinationTasks) - 1
				return model, model.loadSelectedLiveIfVisible()
			}
		case key.Matches(message, model.keys.Reports):
			if model.destination == HubDestination {
				return model, nil
			}
			model.outputMode = ReportsMode
		case key.Matches(message, model.keys.Live):
			if model.destination == HubDestination {
				return model, nil
			}
			model.outputMode = LiveMode
			return model, model.loadSelectedLive()
		case key.Matches(message, model.keys.Toggle):
			if model.destination == HubDestination {
				return model, nil
			}
			if model.outputMode == ReportsMode {
				model.outputMode = LiveMode
				return model, model.loadSelectedLive()
			}
			model.outputMode = ReportsMode
		case key.Matches(message, model.keys.Refresh):
			return model, model.refresh()
		case key.Matches(message, model.keys.Compose):
			if model.destination != HubDestination && len(destinationTasks) == 0 {
				model.sendStatus = "No active worker selected."
				return model, nil
			}
			model.sendStatus = ""
			return model, model.composer.Focus()
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

func (model Model) ComposerFocused() bool {
	return model.composer.Focused()
}

func (model Model) Draft() string {
	return model.composer.Value()
}

func (model Model) SendStatus() string {
	return model.sendStatus
}

func (model Model) CreatingPrivate() bool {
	return model.creatingPrivate
}

func (model Model) PrivateDraft() string {
	return model.privateInput.Value()
}

func (model Model) CreateStatus() string {
	return model.createStatus
}

// Destination returns the active top-level navigation context.
func (model Model) Destination() Destination {
	return model.destination
}

func (model Model) selectedID() string {
	tasks := model.destinationTasks()
	if model.selected < 0 || model.selected >= len(tasks) {
		return ""
	}
	return tasks[model.selected].Metadata.ID
}

func (model Model) refresh() tea.Cmd {
	if model.source == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fleetRefreshTimeout)
		defer cancel()
		tasks, err := model.source.Load(ctx)
		if err != nil {
			return fleetLoadedMsg{err: err}
		}
		hub, hubErr := model.source.LoadHub(ctx)
		var history []string
		var historyErr error
		if hubErr == nil {
			history, historyErr = model.source.LoadHubHistory(ctx, hub)
		}
		return fleetLoadedMsg{
			tasks:         tasks,
			hub:           hub,
			hubErr:        hubErr,
			hubHistory:    history,
			hubHistoryErr: historyErr,
		}
	}
}

func (model Model) loadSelectedLive() tea.Cmd {
	task, found := model.selectedTask()
	if model.source == nil || !found {
		return nil
	}
	taskID := task.Metadata.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), interactiveReadTimeout)
		defer cancel()
		lines, err := model.source.LoadLive(ctx, task)
		return liveLoadedMsg{taskID: taskID, lines: lines, err: err}
	}
}

func (model Model) loadSelectedLiveIfVisible() tea.Cmd {
	if model.outputMode != LiveMode {
		return nil
	}
	return model.loadSelectedLive()
}

func (model *Model) sendDraft() tea.Cmd {
	if model.sending {
		model.sendStatus = "Send already in progress."
		return nil
	}
	draft := model.composer.Value()
	if len(strings.TrimSpace(draft)) == 0 {
		model.sendStatus = "Message must not be empty."
		return nil
	}
	if len([]byte(draft)) > firstmate.MaxMessageBytes {
		model.sendStatus = fmt.Sprintf("Message exceeds %d-byte limit.", firstmate.MaxMessageBytes)
		return nil
	}
	if model.source == nil {
		model.sendStatus = "Send source is unavailable."
		return nil
	}
	model.sending = true
	model.sendStatus = "Sending..."
	if model.destination == HubDestination {
		target := model.hub.Target
		if model.hub.Err != nil {
			model.sending = false
			model.sendStatus = "Firstmate hub unavailable: " + model.hub.Err.Error()
			return nil
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), interactiveReadTimeout)
			defer cancel()
			err := model.source.SendHub(ctx, target, draft)
			return sendFinishedMsg{label: "Firstmate hub", draft: draft, err: err}
		}
	}
	task, found := model.selectedTask()
	if !found {
		model.sending = false
		model.sendStatus = "No active worker selected."
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), interactiveReadTimeout)
		defer cancel()
		err := model.source.Send(ctx, task, draft)
		return sendFinishedMsg{label: task.Metadata.ID, draft: draft, err: err}
	}
}

func (model *Model) createPrivate() tea.Cmd {
	if model.creating {
		model.createStatus = "Create already in progress."
		return nil
	}
	label := model.privateInput.Value()
	if len(strings.TrimSpace(label)) == 0 {
		model.createStatus = "Label must not be empty."
		return nil
	}
	if model.source == nil {
		model.createStatus = "Private-session source is unavailable."
		return nil
	}
	model.creating = true
	model.createStatus = "Creating..."
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), interactiveReadTimeout)
		defer cancel()
		session, err := model.source.CreatePrivate(ctx, label)
		if err != nil {
			return privateCreatedMsg{err: err}
		}
		tasks, err := model.source.Load(ctx)
		if err != nil {
			return privateCreatedMsg{err: fmt.Errorf("rediscover created session: %w", err)}
		}
		for _, task := range tasks {
			if task.Ownership == firstmate.CaptainPrivate &&
				task.Target == session.Target &&
				task.Metadata.ID == session.Target {
				return privateCreatedMsg{session: session, tasks: tasks}
			}
		}
		return privateCreatedMsg{err: fmt.Errorf("created session %s was not rediscovered", session.Target)}
	}
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

func indexOfPrivateTarget(tasks []firstmate.Task, target string) int {
	index := 0
	for _, task := range tasks {
		if task.Ownership != firstmate.CaptainPrivate {
			continue
		}
		if task.Target == target && task.Metadata.ID == target {
			return index
		}
		index++
	}
	return -1
}

func (model Model) selectedTask() (firstmate.Task, bool) {
	tasks := model.destinationTasks()
	if model.selected < 0 || model.selected >= len(tasks) {
		return firstmate.Task{}, false
	}
	return tasks[model.selected], true
}

func (model Model) destinationTasks() []firstmate.Task {
	var ownership firstmate.Ownership
	switch model.destination {
	case ManagedDestination:
		ownership = firstmate.FirstmateManaged
	case PrivateDestination:
		ownership = firstmate.CaptainPrivate
	default:
		return nil
	}
	tasks := make([]firstmate.Task, 0, len(model.tasks))
	for _, task := range model.tasks {
		if task.Ownership == ownership {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (model Model) switchDestination(destination Destination) (tea.Model, tea.Cmd) {
	model.destination = destination
	model.selected = 0
	model.outputMode = ReportsMode
	model.sendStatus = ""
	model.composer.Blur()
	return model, model.loadSelectedLiveIfVisible()
}
