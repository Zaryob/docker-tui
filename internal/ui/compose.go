// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/buildx"
)

func newProjectsTab() *tab {
	return newTab(tabProjects, "Projects", "projects",
		Column{Title: "PROJECT", Grow: 1}, Column{Title: "STATUS", Width: 20}, Column{Title: "COMPOSE FILES", Grow: 2})
}

type projectsMsg struct {
	projects []buildx.Project
	err      error
}
type openComposeMsg struct {
	form    *composeForm
	project buildx.Project
	err     error
}

func (a *App) loadProjects() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	projects, err := a.buildx.ComposeProjects(ctx)
	if err == nil {
		// Labels preserve --project-directory when it differs from the file directory.
		containers, labelErr := a.client.ListContainers(ctx, true)
		if labelErr == nil {
			for i := range projects {
				for _, c := range containers {
					if c.Labels[composeProjectLabel] == projects[i].Name {
						projects[i].WorkingDir = c.Labels["com.docker.compose.project.working_dir"]
						break
					}
				}
			}
		}
	}
	return projectsMsg{projects, err}
}

// mergeProjects retains local sources after down removes the last container.
func (a *App) mergeProjects(projects []buildx.Project, err error) {
	t := a.tabs[tabProjects]
	t.loadErr = err
	if err != nil {
		return
	}
	for name, p := range a.projects {
		p.Status = "not running"
		a.projects[name] = p
	}
	for _, p := range projects {
		if old, ok := a.projects[p.Name]; ok && (old.LocalSource || p.ConfigFiles == "") {
			p.ConfigFiles, p.WorkingDir, p.LocalSource = old.ConfigFiles, old.WorkingDir, old.LocalSource
		}
		a.projects[p.Name] = p
	}
	a.refreshProjects()
}

func (a *App) refreshProjects() {
	names := make([]string, 0, len(a.projects))
	for name := range a.projects {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]item, 0, len(names))
	for _, name := range names {
		p := a.projects[name]
		items = append(items, item{id: name, name: name, cells: []string{name, p.Status, p.ConfigFiles}, data: p})
	}
	a.tabs[tabProjects].setItems(items)
}

func (a *App) projectKeys(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "f" {
		f := &composeForm{input: textinput.New(), cli: a.buildx}
		f.input.Prompt = "Compose file  "
		f.input.Placeholder = "/path/to/compose.yaml"
		f.input.Focus()
		a.pane = f
		a.layout()
		return textinput.Blink
	}
	if a.compose != nil && (msg.String() == "u" || msg.String() == "enter" || msg.String() == "b" || msg.String() == "x") {
		a.pane = a.compose
		a.layout()
		return nil
	}
	it, ok := a.current().selected()
	if !ok {
		return nil
	}
	p, ok := it.data.(buildx.Project)
	if !ok {
		return nil
	}
	switch msg.String() {
	case "u", "enter":
		return a.startCompose(p, "up")
	case "b":
		return a.startCompose(p, "rebuild")
	case "x":
		if _, err := p.Args(); err != nil {
			return a.setStatus(err.Error(), true)
		}
		a.confirm = &confirmDialog{title: "Bring down Compose project?", message: p.Name + "\nStops and removes this project's containers and networks. Volumes are kept.", onYes: func() tea.Cmd { return a.startCompose(p, "down") }}
	}
	return nil
}

func (a *App) startCompose(p buildx.Project, action string) tea.Cmd {
	if a.compose != nil {
		return a.setStatus("a Compose operation is already running", true)
	}
	if _, err := p.Args(); err != nil {
		return a.setStatus(err.Error(), true)
	}
	cli := a.buildx
	l := newLogView(p.Name+" · "+action, a.width, a.bodyHeight())
	l.kind, l.running, l.ended = "Compose", "● running", "✓ operation finished"
	l.cancelable = true
	l.client = a.client
	l.sources = []logSource{{name: p.Name, open: func(ctx context.Context, _ bool) (io.ReadCloser, error) {
		return cli.Compose(ctx, p, action)
	}}}
	a.compose, a.pane = l, l
	a.layout()
	return l.start()
}

func (a *App) composeFinished() tea.Cmd {
	l := a.compose
	a.compose = nil
	cmds := []tea.Cmd{a.loadProjects, a.load(tabContainers)}
	if a.tabs[tabImages].loaded {
		cmds = append(cmds, a.load(tabImages))
	}
	if a.pane != asPane(l) {
		switch {
		case l.canceled:
			cmds = append(cmds, a.setStatus("Compose "+l.name+" canceled", false))
		case l.err != nil:
			cmds = append(cmds, a.setStatus("Compose "+l.name+" failed: "+l.err.Error(), true))
		default:
			cmds = append(cmds, a.setStatus("Compose "+l.name+" finished", false))
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) composeIndicator() string {
	if a.compose == nil || a.pane == asPane(a.compose) {
		return ""
	}
	return warnStyle.Render("⚙ Compose " + a.compose.name)
}

type composeForm struct {
	input   textinput.Model
	cli     *buildx.CLI
	err     string
	loading bool
	cancel  context.CancelFunc
}

func (f *composeForm) setSize(w, _ int) { f.input.Width = max(w-20, 10) }
func (f *composeForm) close() {
	if f.cancel != nil {
		f.cancel()
	}
}
func (f *composeForm) capturesInput() bool { return true }
func (f *composeForm) keyMap() help.KeyMap { return composeFormKeys{} }
func (f *composeForm) update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			f.close()
			return func() tea.Msg { return closePaneMsg{} }
		case "enter":
			if f.loading {
				return nil
			}
			file := expandHome(strings.TrimSpace(f.input.Value()))
			if file == "" {
				f.err = "enter the path to a local Compose file"
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			f.cancel, f.loading, f.err = cancel, true, ""
			return func() tea.Msg {
				defer cancel()
				p, err := f.cli.OpenCompose(ctx, file)
				// A closed form must not replace a newer screen.
				if ctx.Err() == context.Canceled {
					return nil
				}
				return openComposeMsg{f, p, err}
			}
		}
	}
	if f.loading {
		return nil
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return cmd
}
func (f *composeForm) view() string {
	note := "Open a local Compose file; no services start until you choose up."
	if f.loading {
		note = "Reading Compose configuration…"
	}
	if f.err != "" {
		note = errorStyle.Render(f.err)
	}
	return paneTitleStyle.Render("Open Compose project") + "\n\n" + f.input.View() + "\n\n" + note
}

type composeFormKeys struct{}

func (composeFormKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")), formCancelKey,
	}
}
func (k composeFormKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

type projectKeyMap struct{}

func (projectKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		keys.NextTab, keys.Filter,
		key.NewBinding(key.WithKeys("u"), key.WithHelp("u/enter", "up / show progress")),
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "down")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "rebuild")),
		key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "open Compose file")), keys.Refresh, keys.Help, keys.Quit,
	}
}
func (k projectKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp(), {keys.Up, keys.Down, keys.Sort, keys.ReverseSort, keys.ClearFilter}}
}
