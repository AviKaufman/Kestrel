package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Top     key.Binding
	Bottom  key.Binding
	Reports key.Binding
	Live    key.Binding
	Toggle  key.Binding
	Back    key.Binding
	Refresh key.Binding
	Help    key.Binding
	Quit    key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/up", "previous"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/down", "next"),
		),
		Top: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "bottom"),
		),
		Reports: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("left", "reports"),
		),
		Live: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("right", "live"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "switch output"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "keys"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

func (keys keyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Toggle, keys.Refresh, keys.Help, keys.Quit}
}

func (keys keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keys.Up, keys.Down, keys.Top, keys.Bottom},
		{keys.Reports, keys.Live, keys.Toggle, keys.Back},
		{keys.Refresh, keys.Help, keys.Quit},
	}
}
