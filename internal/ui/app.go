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
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
)

// App is the root Bubble Tea model.
type App struct {
	client  *docker.Client
	version docker.Version

	tabs   []*tab
	active int

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
func New(client *docker.Client) *App {
	return &App{
		client: client,
		tabs: []*tab{
			newContainersTab(),
			newImagesTab(),
		},
		help: help.New(),
		spinner: spinner.New(
			spinner.WithSpinner(spinner.MiniDot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(colorAccent)),
		),
	}
}

const statusTimeout = 4 * time.Second

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

func (a *App) current() *tab { return a.tabs[a.active] }

// load returns a command that reloads the given tab.
func (a *App) load(kind tabKind) tea.Cmd {
	switch kind {
	case tabContainers:
		return a.loadContainers
	case tabImages:
		return a.loadImages
	}
	return nil
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return tea.Batch(a.loadVersion, a.load(tabContainers), a.spinner.Tick)
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
	case clearStatusMsg:
		if msg.seq == a.statusSeq {
			a.status = ""
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		return a, cmd
	case tea.KeyMsg:
		return a, a.handleKey(msg)
	}
	return a, nil
}

func (a *App) handleKey(msg tea.KeyMsg) tea.Cmd {
	t := a.current()
	switch {
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
	default:
		if t.kind == tabContainers {
			return a.containerKeys(msg)
		}
	}
	return nil
}

// layout recomputes the size of the tables from the window size.
func (a *App) layout() {
	helpHeight := lipgloss.Height(a.help.View(keys))
	// header + status line + help
	h := a.height - 2 - helpHeight
	for _, t := range a.tabs {
		t.table.SetSize(a.width, h)
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
	b.WriteString(a.current().table.View())
	b.WriteString("\n")
	b.WriteString(a.statusLine())
	b.WriteString("\n")
	b.WriteString(a.help.View(keys))
	return b.String()
}

func (a *App) statusLine() string {
	t := a.current()
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
		return prefix + statusStyle.Render(fmt.Sprintf("%d %s", len(t.items), t.noun))
	}
}
