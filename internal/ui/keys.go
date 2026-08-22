// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Top      key.Binding
	Bottom   key.Binding
	NextTab  key.Binding
	PrevTab  key.Binding
	JumpTab  key.Binding
	Refresh  key.Binding
	Start    key.Binding
	Stop     key.Binding
	Restart  key.Binding
	Remove   key.Binding
	Logs     key.Binding
	Back     key.Binding
	Help     key.Binding
	Quit     key.Binding
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PageUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+b"), key.WithHelp("pgup", "page up")),
	PageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+f"), key.WithHelp("pgdn", "page down")),
	Top:      key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	Bottom:   key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
	NextTab:  key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("tab", "next tab")),
	PrevTab:  key.NewBinding(key.WithKeys("shift+tab", "left", "h"), key.WithHelp("shift+tab", "prev tab")),
	JumpTab:  key.NewBinding(key.WithKeys("1", "2", "3", "4"), key.WithHelp("1-4", "jump to tab")),
	Refresh:  key.NewBinding(key.WithKeys("r", "ctrl+r"), key.WithHelp("r", "refresh")),
	Start:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
	Stop:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
	Restart:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restart")),
	Remove:   key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "remove")),
	Logs:     key.NewBinding(key.WithKeys("enter", "L"), key.WithHelp("enter", "logs")),
	Back:     key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.NextTab, k.Up, k.Down, k.Logs, k.Start, k.Stop, k.Restart, k.Remove, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom},
		{k.Logs, k.Start, k.Stop, k.Restart, k.Remove},
		{k.NextTab, k.PrevTab, k.JumpTab},
		{k.Refresh, k.Help, k.Quit},
	}
}

// logKeys is the key map shown while viewing logs.
type logKeyMap struct{}

func (logKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Top, keys.Bottom, keys.Back}
}

func (k logKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
