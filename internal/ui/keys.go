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
	RangeUp     key.Binding
	RangeDown   key.Binding
	Mark        key.Binding
	MarkAll     key.Binding
	MarkProject key.Binding
	Collapse    key.Binding
	ToggleAll   key.Binding
	Start       key.Binding
	Stop        key.Binding
	Restart     key.Binding
	Kill        key.Binding
	Pause       key.Binding
	Remove      key.Binding
	Prune       key.Binding
	Logs        key.Binding
	Inspect     key.Binding
	CopyID      key.Binding
	Stats       key.Binding
	Processes   key.Binding
	Exec        key.Binding
	ExecCLI     key.Binding
	Attach      key.Binding
	Build       key.Binding
	Login       key.Binding
	Logout      key.Binding
	Pull        key.Binding
	Copy        key.Binding
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
	JumpTab:     key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7"), key.WithHelp("1-7", "jump to tab")),
	Refresh:     key.NewBinding(key.WithKeys("r", "ctrl+r"), key.WithHelp("r", "refresh")),
	Filter:      key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Sort:        key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "sort")),
	ReverseSort: key.NewBinding(key.WithKeys("O"), key.WithHelp("O", "reverse sort")),
	ClearFilter: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear range/filter/marks")),
	RangeUp:     key.NewBinding(key.WithKeys("shift+up"), key.WithHelp("shift+↑", "extend range up")),
	RangeDown:   key.NewBinding(key.WithKeys("shift+down"), key.WithHelp("shift+↓", "extend range down")),
	Mark:        key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "mark")),
	MarkAll:     key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "mark all")),
	MarkProject: key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "mark project")),
	Collapse:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "fold project")),
	ToggleAll:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle stopped")),
	Start:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
	Stop:        key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
	Restart:     key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restart")),
	Kill:        key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "kill")),
	Pause:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause/unpause")),
	Remove:      key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "remove")),
	Prune:       key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "prune")),
	Logs:        key.NewBinding(key.WithKeys("enter", "L"), key.WithHelp("enter", "logs")),
	Inspect:     key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "inspect")),
	CopyID:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy ID")),
	Stats:       key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "stats")),
	Processes:   key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "processes")),
	Build:       key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build image")),
	Login:       key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "registry login")),
	Logout:      key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "registry logout")),
	Pull:        key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "pull image")),
	Copy:        key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "copy files")),
	Exec:        key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "terminal")),
	Attach:      key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "attach")),
	ExecCLI:     key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "full-screen shell")),
	Back:        key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
	Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// Bindings whose meaning depends on the tab they are shown in.
var (
	buildLogsKey    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "build logs"))
	openLogKey      = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open log"))
	pruneImagesKey  = key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "prune dangling"))
	pruneBuildsKey  = key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "clear history"))
	projectUpKey    = key.NewBinding(key.WithKeys("u"), key.WithHelp("u/enter", "up / progress"))
	pullShortKey    = key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "pull"))
	buildShortKey   = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build"))
	loginShortKey   = key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "login"))
	logoutShortKey  = key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "logout"))
	projectDownKey  = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "down"))
	projectBuildKey = key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "rebuild"))
	projectFileKey  = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "open Compose file"))
)

// tabKeyMap is the help of a resource list: the actions that apply to the
// objects of the tab, so that e.g. the log list does not offer "remove".
type tabKeyMap struct{ kind tabKind }

// actions returns the bindings specific to the tab, in order of use.
func (k tabKeyMap) actions() []key.Binding {
	switch k.kind {
	case tabContainers:
		return []key.Binding{keys.Logs, keys.Inspect, keys.Stats, keys.Processes, keys.Exec, keys.Attach, keys.ExecCLI, keys.Copy,
			keys.Start, keys.Stop, keys.Restart, keys.Kill, keys.Pause, keys.Remove, keys.Prune, keys.ToggleAll}
	case tabImages:
		return []key.Binding{keys.Pull, keys.Build, keys.Login, keys.Logout, keys.Inspect, keys.Remove, pruneImagesKey}
	case tabVolumes, tabNetworks:
		return []key.Binding{keys.Inspect, keys.Remove, keys.Prune}
	case tabBuilds:
		return []key.Binding{buildLogsKey, keys.Build, keys.Inspect, keys.Remove, pruneBuildsKey}
	case tabLogs:
		return []key.Binding{openLogKey}
	case tabProjects:
		return []key.Binding{projectUpKey, projectDownKey, projectBuildKey, projectFileKey}
	}
	return nil
}

// ShortHelp implements help.KeyMap with the most used actions of the tab.
func (k tabKeyMap) ShortHelp() []key.Binding {
	b := []key.Binding{keys.NextTab, keys.Filter}
	switch k.kind {
	case tabContainers:
		b = append(b, keys.Logs, keys.Inspect, keys.Stats, keys.Exec, keys.Start, keys.Stop, keys.Remove)
	case tabImages:
		b = append(b, pullShortKey, buildShortKey, loginShortKey, logoutShortKey, keys.Inspect, keys.Remove)
	case tabVolumes, tabNetworks:
		b = append(b, keys.Inspect, keys.Remove, keys.Prune, keys.CopyID, keys.Sort)
	case tabBuilds:
		b = append(b, buildLogsKey, keys.Build, keys.Inspect, keys.Remove, pruneBuildsKey)
	case tabLogs:
		b = append(b, openLogKey, keys.Mark, keys.Sort, keys.Refresh)
	case tabProjects:
		b = append(b, projectUpKey, projectDownKey, projectBuildKey, projectFileKey, keys.Refresh)
	}
	return append(b, keys.Help, keys.Quit)
}

// FullHelp implements help.KeyMap: navigation, the actions of the tab,
// tabs, and the list commands that apply to the tab.
func (k tabKeyMap) FullHelp() [][]key.Binding {
	list := []key.Binding{keys.Mark, keys.MarkAll}
	if k.kind == tabContainers {
		list = append(list, keys.MarkProject, keys.Collapse)
	}
	list = append(list, keys.Filter, keys.ClearFilter, keys.Sort, keys.ReverseSort)
	if k.kind != tabLogs && k.kind != tabProjects {
		list = append(list, keys.CopyID)
	}
	list = append(list, keys.Refresh, keys.Help, keys.Quit)
	return [][]key.Binding{
		{keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Top, keys.Bottom, keys.RangeUp, keys.RangeDown},
		k.actions(),
		{keys.NextTab, keys.PrevTab, keys.JumpTab},
		list,
	}
}

// paneKeyMap is the key map shown while a pane (logs, inspect) is open.
type paneKeyMap struct{}

func (paneKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Top, keys.Bottom, keys.Back}
}

func (k paneKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
