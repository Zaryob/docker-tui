// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up          key.Binding
	Down        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
	Top         key.Binding
	Bottom      key.Binding
	NextTab     key.Binding
	PrevTab     key.Binding
	JumpTab     key.Binding
	Refresh     key.Binding
	Filter      key.Binding
	Sort        key.Binding
	ReverseSort key.Binding
	ClearFilter key.Binding
	ToggleAll   key.Binding
	Start       key.Binding
	Stop        key.Binding
	Restart     key.Binding
	Pause       key.Binding
	Remove      key.Binding
	Prune       key.Binding
	Logs        key.Binding
	Inspect     key.Binding
	Stats       key.Binding
	Exec        key.Binding
	Back        key.Binding
	Help        key.Binding
	Quit        key.Binding
}

var keys = keyMap{
	Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PageUp:      key.NewBinding(key.WithKeys("pgup", "ctrl+b"), key.WithHelp("pgup", "page up")),
	PageDown:    key.NewBinding(key.WithKeys("pgdown", "ctrl+f"), key.WithHelp("pgdn", "page down")),
	Top:         key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	Bottom:      key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
	NextTab:     key.NewBinding(key.WithKeys("tab", "right", "l"), key.WithHelp("tab", "next tab")),
	PrevTab:     key.NewBinding(key.WithKeys("shift+tab", "left", "h"), key.WithHelp("shift+tab", "prev tab")),
	JumpTab:     key.NewBinding(key.WithKeys("1", "2", "3", "4"), key.WithHelp("1-4", "jump to tab")),
	Refresh:     key.NewBinding(key.WithKeys("r", "ctrl+r"), key.WithHelp("r", "refresh")),
	Filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Sort:        key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "sort")),
	ReverseSort: key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "reverse sort")),
	ClearFilter: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
	ToggleAll:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle stopped")),
	Start:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
	Stop:        key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
	Restart:     key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restart")),
	Pause:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause/unpause")),
	Remove:      key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "remove")),
	Prune:       key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "prune")),
	Logs:        key.NewBinding(key.WithKeys("enter", "L"), key.WithHelp("enter", "logs")),
	Inspect:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "inspect")),
	Stats:       key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "stats")),
	Exec:        key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "shell")),
	Back:        key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
	Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.NextTab, k.Filter, k.Logs, k.Inspect, k.Start, k.Stop, k.Restart, k.Remove, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom},
		{k.ToggleAll, k.Logs, k.Inspect, k.Stats, k.Exec, k.Start, k.Stop, k.Restart, k.Pause, k.Remove, k.Prune},
		{k.NextTab, k.PrevTab, k.JumpTab},
		{k.Filter, k.ClearFilter, k.Sort, k.ReverseSort, k.Refresh, k.Help, k.Quit},
	}
}

// paneKeyMap is the key map shown while a pane (logs, inspect) is open.
type paneKeyMap struct{}

func (paneKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Top, keys.Bottom, keys.Back}
}

func (k paneKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
