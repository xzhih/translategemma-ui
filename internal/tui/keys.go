package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit      key.Binding
	QuitAlt   key.Binding
	Back      key.Binding
	Confirm   key.Binding
	MoveUp    key.Binding
	MoveDown  key.Binding
	FocusNext key.Binding
	FocusPrev key.Binding
	Run       key.Binding
	Swap      key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
	Top       key.Binding
	Bottom    key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		QuitAlt: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		MoveUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		MoveDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		FocusNext: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next"),
		),
		FocusPrev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev"),
		),
		Run: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "translate"),
		),
		Swap: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "swap"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		Top: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "bottom"),
		),
	}
}

func (m model) shortHelpBindings() []key.Binding {
	switch m.screen {
	case modelScreen:
		return []key.Binding{m.keys.MoveUp, m.keys.Confirm, m.keys.QuitAlt, m.keys.Quit}
	case provisionScreen:
		return []key.Binding{m.keys.QuitAlt, m.keys.Quit}
	case translateScreen:
		switch m.focus {
		case sourceFocus, targetFocus:
			return []key.Binding{m.keys.FocusNext, m.keys.MoveUp, m.keys.Swap, m.keys.Back, m.keys.Quit}
		case outputFocus:
			return []key.Binding{m.keys.FocusNext, m.keys.MoveUp, m.keys.PageDown, m.keys.Top, m.keys.Back, m.keys.Quit}
		default:
			return []key.Binding{m.keys.FocusNext, m.keys.Run, m.keys.Swap, m.keys.Back, m.keys.Quit}
		}
	}
	return []key.Binding{m.keys.Quit}
}
