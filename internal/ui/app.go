// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ui contains the Bubble Tea models that make up docker-tui.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
)

// Options configures the application.
type Options struct {
	// RefreshInterval is how often the visible list is reloaded. Zero or
	// less disables automatic refreshing.
	RefreshInterval time.Duration
}

// App is the root Bubble Tea model.
type App struct {
	opts    Options
	client  *docker.Client
	version docker.Version

	tabs   []*tab
	active int
	// onlyRunning hides stopped containers, like "docker ps" without -a.
	onlyRunning bool

	confirm *confirmDialog
	// filtering is true while the filter input has focus.
	filtering bool
	filter    textinput.Model
	// pane, when set, replaces the resource table (logs, inspect, ...).
	pane pane

	help      help.Model
	spinner   spinner.Model
	busy      int
	status    string
	statusErr bool
	statusSeq int

	width  int
	height int
}

// New creates the root model.
func New(client *docker.Client, opts Options) *App {
	fi := textinput.New()
	fi.Prompt = "/"
	fi.Placeholder = "filter"
	fi.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	return &App{
		opts:   opts,
		filter: fi,
		client: client,
		tabs: []*tab{
			newContainersTab(),
			newImagesTab(),
			newVolumesTab(),
			newNetworksTab(),
		},
		help: help.New(),
		spinner: spinner.New(
			spinner.WithSpinner(spinner.MiniDot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(colorAccent)),
		),
	}
}

const statusTimeout = 4 * time.Second

type refreshTickMsg struct{}

// refreshTick schedules the next periodic refresh, if enabled.
func (a *App) refreshTick() tea.Cmd {
	if a.opts.RefreshInterval <= 0 {
		return nil
	}
	return tea.Tick(a.opts.RefreshInterval, func(time.Time) tea.Msg { return refreshTickMsg{} })
}

type clearStatusMsg struct{ seq int }

// setStatus shows a transient message in the status line.
func (a *App) setStatus(text string, isErr bool) tea.Cmd {
	a.statusSeq++
	a.status, a.statusErr = text, isErr
	seq := a.statusSeq
	return tea.Tick(statusTimeout, func(time.Time) tea.Msg { return clearStatusMsg{seq: seq} })
}

type versionMsg struct {
	version docker.Version
	err     error
}

func (a *App) loadVersion() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := a.client.ServerVersion(ctx)
	return versionMsg{version: v, err: err}
}

type actionMsg struct {
	kind   tabKind
	status string
	err    error
}

type actionFunc func(ctx context.Context, id string) error

// objectAction runs fn against the selected object of the active tab and
// reports the outcome as an actionMsg.
func (a *App) objectAction(verb string, fn actionFunc) tea.Cmd {
	t := a.current()
	it, ok := t.selected()
	if !ok {
		return nil
	}
	a.busy++
	kind := t.kind
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := fn(ctx, it.id); err != nil {
			return actionMsg{kind: kind, err: fmt.Errorf("%s %s: %w", verb, it.name, err)}
		}
		return actionMsg{kind: kind, status: fmt.Sprintf("%s %s", verb, it.name)}
	}
}

// removeDialog builds the confirmation dialog for removing the selected
// object of the active tab.
func (a *App) removeDialog() *confirmDialog {
	t := a.current()
	it, ok := t.selected()
	if !ok {
		return nil
	}
	remove := func(force bool) func() tea.Cmd {
		return func() tea.Cmd {
			return a.objectAction("removed", func(ctx context.Context, id string) error {
				switch t.kind {
				case tabContainers:
					return a.client.RemoveContainer(ctx, id, force)
				case tabImages:
					return a.client.RemoveImage(ctx, id, force)
				case tabVolumes:
					return a.client.RemoveVolume(ctx, id, force)
				case tabNetworks:
					return a.client.RemoveNetwork(ctx, id)
				}
				return nil
			})
		}
	}
	d := &confirmDialog{
		title:   "Remove " + strings.TrimSuffix(t.noun, "s") + "?",
		message: it.name,
		onYes:   remove(false),
	}
	if t.kind != tabNetworks {
		d.onForce = remove(true)
	}
	return d
}

func (a *App) current() *tab { return a.tabs[a.active] }

// load returns a command that reloads the given tab.
func (a *App) load(kind tabKind) tea.Cmd {
	switch kind {
	case tabContainers:
		return a.loadContainers
	case tabImages:
		return a.loadImages
	case tabVolumes:
		return a.loadVolumes
	case tabNetworks:
		return a.loadNetworks
	}
	return nil
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return tea.Batch(a.loadVersion, a.load(tabContainers), a.spinner.Tick, a.refreshTick())
}

func (a *App) handleFilterKey(msg tea.KeyMsg) tea.Cmd {
	t := a.current()
	switch msg.String() {
	case "esc":
		a.filtering = false
		a.filter.Blur()
		t.setFilter("")
		return nil
	case "enter":
		a.filtering = false
		a.filter.Blur()
		return nil
	case "up", "down":
		if msg.String() == "up" {
			t.table.Move(-1)
		} else {
			t.table.Move(1)
		}
		return nil
	}
	var cmd tea.Cmd
	a.filter, cmd = a.filter.Update(msg)
	t.setFilter(a.filter.Value())
	return cmd
}

func (a *App) switchTab(i int) tea.Cmd {
	if i < 0 || i >= len(a.tabs) || i == a.active {
		return nil
	}
	a.active = i
	if !a.current().loaded {
		return a.load(a.current().kind)
	}
	return nil
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.help.Width = msg.Width
		a.layout()
	case versionMsg:
		a.version = msg.version
		if msg.err != nil {
			return a, a.setStatus(msg.err.Error(), true)
		}
	case listMsg:
		t := a.tabs[msg.kind]
		t.loadErr = msg.err
		if msg.err == nil {
			t.setItems(msg.items)
		}
	case actionMsg:
		a.busy--
		if msg.err != nil {
			return a, a.setStatus(msg.err.Error(), true)
		}
		return a, tea.Batch(a.setStatus(msg.status, false), a.load(msg.kind))
	case execDoneMsg:
		if msg.err != nil {
			return a, tea.Batch(a.setStatus(fmt.Sprintf("exec %s: %v", msg.name, msg.err), true), a.load(tabContainers))
		}
		return a, a.load(tabContainers)
	case refreshTickMsg:
		// Only refresh the list that is visible; panes and dialogs keep
		// their own state.
		if a.pane == nil && a.confirm == nil {
			return a, tea.Batch(a.load(a.current().kind), a.refreshTick())
		}
		return a, a.refreshTick()
	case clearStatusMsg:
		if msg.seq == a.statusSeq {
			a.status = ""
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	case logChunkMsg, inspectMsg, statsMsg, statsTickMsg:
		if a.pane != nil {
			return a, a.pane.update(msg)
		}
	case tea.KeyMsg:
		return a, a.handleKey(msg)
	}
	return a, nil
}

func (a *App) handleKey(msg tea.KeyMsg) tea.Cmd {
	if a.confirm != nil {
		cmd, done := a.confirm.handleKey(msg)
		if done {
			a.confirm = nil
		}
		return cmd
	}
	if a.filtering {
		return a.handleFilterKey(msg)
	}
	if a.pane != nil {
		switch {
		case key.Matches(msg, keys.Back):
			a.pane.close()
			a.pane = nil
			a.layout()
			return nil
		case msg.String() == "ctrl+c":
			a.pane.close()
			return tea.Quit
		}
		return a.pane.update(msg)
	}

	t := a.current()
	switch {
	case key.Matches(msg, keys.ClearFilter) && t.filter != "":
		t.setFilter("")
	case key.Matches(msg, keys.Filter):
		a.filtering = true
		a.filter.SetValue(t.filter)
		a.filter.CursorEnd()
		return a.filter.Focus()
	case key.Matches(msg, keys.Quit):
		return tea.Quit
	case key.Matches(msg, keys.Help):
		a.help.ShowAll = !a.help.ShowAll
		a.layout()
	case key.Matches(msg, keys.NextTab):
		return a.switchTab((a.active + 1) % len(a.tabs))
	case key.Matches(msg, keys.PrevTab):
		return a.switchTab((a.active + len(a.tabs) - 1) % len(a.tabs))
	case key.Matches(msg, keys.JumpTab):
		return a.switchTab(int(msg.Runes[0] - '1'))
	case key.Matches(msg, keys.Refresh):
		return a.load(t.kind)
	case key.Matches(msg, keys.Up):
		t.table.Move(-1)
	case key.Matches(msg, keys.Down):
		t.table.Move(1)
	case key.Matches(msg, keys.PageUp):
		t.table.Move(-t.table.PageSize())
	case key.Matches(msg, keys.PageDown):
		t.table.Move(t.table.PageSize())
	case key.Matches(msg, keys.Top):
		t.table.Top()
	case key.Matches(msg, keys.Bottom):
		t.table.Bottom()
	case key.Matches(msg, keys.Remove):
		a.confirm = a.removeDialog()
	case key.Matches(msg, keys.Sort):
		t.cycleSort()
	case key.Matches(msg, keys.ReverseSort):
		t.reverseSort()
	case key.Matches(msg, keys.Prune):
		a.confirm = a.pruneDialog()
	case key.Matches(msg, keys.Inspect):
		return a.openInspect()
	default:
		if t.kind == tabContainers {
			return a.containerKeys(msg)
		}
		// Objects other than containers have no logs; enter inspects them.
		if key.Matches(msg, keys.Logs) {
			return a.openInspect()
		}
	}
	return nil
}

// pane is a full-size view shown instead of the resource table.
type pane interface {
	update(tea.Msg) tea.Cmd
	view() string
	setSize(width, height int)
	close()
}

// helpKeys returns the key map for the current screen.
func (a *App) helpKeys() help.KeyMap {
	if a.pane != nil {
		return paneKeyMap{}
	}
	return keys
}

// bodyHeight returns the height available between the header and the
// status line.
func (a *App) bodyHeight() int {
	helpHeight := lipgloss.Height(a.help.View(a.helpKeys()))
	// header + status line + help
	return max(a.height-2-helpHeight, 1)
}

// layout recomputes the size of the panes from the window size.
func (a *App) layout() {
	h := a.bodyHeight()
	for _, t := range a.tabs {
		t.table.SetSize(a.width, h)
	}
	if a.pane != nil {
		a.pane.setSize(a.width, h)
	}
}

func (a *App) header() string {
	parts := []string{titleStyle.Render("docker-tui")}
	for i, t := range a.tabs {
		label := fmt.Sprintf("%d %s", i+1, t.title)
		if i == a.active {
			parts = append(parts, activeTabStyle.Render(label))
		} else {
			parts = append(parts, tabStyle.Render(label))
		}
	}
	left := lipgloss.JoinHorizontal(lipgloss.Top, parts...)

	info := "connecting…"
	if a.version.Version != "" {
		info = fmt.Sprintf("Docker %s · API %s", a.version.Version, a.version.APIVersion)
	}
	right := headerInfoStyle.Render(info)
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// View implements tea.Model.
func (a *App) View() string {
	var b strings.Builder
	b.WriteString(a.header())
	b.WriteString("\n")
	switch {
	case a.confirm != nil:
		b.WriteString(a.confirm.view(a.width, a.bodyHeight()))
	case a.pane != nil:
		b.WriteString(a.pane.view())
	default:
		b.WriteString(a.current().table.View())
	}
	b.WriteString("\n")
	b.WriteString(a.statusLine())
	b.WriteString("\n")
	b.WriteString(a.help.View(a.helpKeys()))
	return b.String()
}

func (a *App) statusLine() string {
	t := a.current()
	if a.filtering {
		return a.filter.View()
	}
	prefix := ""
	if a.busy > 0 {
		prefix = a.spinner.View() + " "
	}
	switch {
	case a.status != "" && a.statusErr:
		return prefix + errorStyle.Render(a.status)
	case a.status != "":
		return prefix + okStyle.Render(a.status)
	case t.loadErr != nil:
		return prefix + errorStyle.Render("error: "+t.loadErr.Error())
	case !t.loaded:
		return prefix + statusStyle.Render("loading "+t.noun+"…")
	default:
		text := countNoun(len(t.all), t.noun)
		if t.filter != "" {
			text = fmt.Sprintf("%d of %s matching %q", len(t.items), text, t.filter)
		}
		return prefix + statusStyle.Render(text)
	}
}

// countNoun formats a count with a plural noun, singularizing it for one.
func countNoun(n int, plural string) string {
	if n == 1 {
		return "1 " + strings.TrimSuffix(plural, "s")
	}
	return fmt.Sprintf("%d %s", n, plural)
}
