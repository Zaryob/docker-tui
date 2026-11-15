// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
		f := newComposeForm(a.buildx)
		f.setSize(a.width, a.bodyHeight())
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
	cmds := []tea.Cmd{a.load(tabProjects), a.load(tabContainers)}
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
	dir     string
	entries []fileEntry
	cursor  int
	offset  int
	width   int
	height  int
	err     string
	loading bool
	cancel  context.CancelFunc
}

func newComposeForm(cli *buildx.CLI) *composeForm {
	in := textinput.New()
	in.Prompt = "Compose file  "
	in.Placeholder = "/path/to/compose.yaml"
	in.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	in.Focus()

	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		cwd = "."
	}

	f := &composeForm{
		input: in,
		cli:   cli,
		dir:   cwd,
	}
	f.refreshEntries()
	return f
}

func (f *composeForm) setSize(w, h int) {
	f.width = w
	f.height = h
	f.input.Width = max(w-20, 10)
}

func (f *composeForm) close() {
	if f.cancel != nil {
		f.cancel()
	}
}

func (f *composeForm) capturesInput() bool { return true }

func (f *composeForm) keyMap() help.KeyMap { return composeFormKeys{} }

func (f *composeForm) refreshEntries() {
	if f.dir == "" {
		cwd, _ := os.Getwd()
		if cwd == "" {
			cwd = "."
		}
		f.dir = cwd
	}
	entries, err := readDirEntries(f.dir)
	if err != nil {
		f.err = err.Error()
		f.entries = nil
		f.cursor = 0
		f.offset = 0
		return
	}
	f.entries = entries
	if f.cursor >= len(entries) {
		f.cursor = max(0, len(entries)-1)
	}
	// Prefer pre-selecting a compose file if one is present in the directory
	for i, e := range entries {
		if e.isCompose {
			f.cursor = i
			break
		}
	}
	f.syncInput()
}

func (f *composeForm) setDir(dir string) {
	clean := filepath.Clean(expandHome(dir))
	f.dir = clean
	f.cursor = 0
	f.offset = 0
	f.err = ""
	f.refreshEntries()
}

func (f *composeForm) syncInput() {
	if f.cursor >= 0 && f.cursor < len(f.entries) {
		e := f.entries[f.cursor]
		if !e.isParent {
			f.input.SetValue(filepath.Join(f.dir, e.name))
			f.input.CursorEnd()
		} else {
			f.input.SetValue(filepath.Dir(f.dir))
			f.input.CursorEnd()
		}
	}
}

func (f *composeForm) update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			f.close()
			return func() tea.Msg { return closePaneMsg{} }

		case "up":
			if len(f.entries) > 0 {
				if f.cursor > 0 {
					f.cursor--
					f.syncInput()
				}
			}
			return nil

		case "down":
			if len(f.entries) > 0 {
				if f.cursor < len(f.entries)-1 {
					f.cursor++
					f.syncInput()
				}
			}
			return nil

		case "pgup":
			if len(f.entries) > 0 {
				f.cursor = max(0, f.cursor-5)
				f.syncInput()
			}
			return nil

		case "pgdown":
			if len(f.entries) > 0 {
				f.cursor = min(len(f.entries)-1, f.cursor+5)
				f.syncInput()
			}
			return nil

		case "tab":
			val := strings.TrimSpace(f.input.Value())
			if val != "" {
				if completed, ok := completePath(val); ok {
					f.input.SetValue(completed)
					f.input.CursorEnd()
					if st, err := os.Stat(expandHome(completed)); err == nil && st.IsDir() {
						f.setDir(expandHome(completed))
					}
					return nil
				}
			}

		case "enter":
			if f.loading {
				return nil
			}

			// If cursor points to a directory or parent, navigate into it!
			if f.cursor >= 0 && f.cursor < len(f.entries) {
				e := f.entries[f.cursor]
				if e.isParent {
					parent := filepath.Dir(f.dir)
					if parent != f.dir {
						f.setDir(parent)
						return nil
					}
				} else if e.isDir {
					f.setDir(filepath.Join(f.dir, e.name))
					return nil
				}
			}

			file := expandHome(strings.TrimSpace(f.input.Value()))
			if file == "" {
				f.err = "enter or select the path to a local Compose file"
				return nil
			}

			// If user typed a directory path, browse into it
			if st, err := os.Stat(file); err == nil && st.IsDir() {
				f.setDir(file)
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
	var b strings.Builder
	b.WriteString(paneTitleStyle.Render("Open Compose project"))
	b.WriteString("\n\n")
	b.WriteString(f.input.View())
	b.WriteString("\n\n")

	displayDir := f.dir
	if displayDir == "" {
		displayDir = "."
	}
	dirLine := "Directory: " + displayDir
	if f.width > 0 && len(dirLine) > f.width-4 {
		dirLine = "Directory: …" + dirLine[len(dirLine)-(max(10, f.width-16)):]
	}
	b.WriteString(statusStyle.Render(dirLine))
	b.WriteString("\n")

	visibleHeight := max(5, f.height-12)
	if f.height <= 0 {
		visibleHeight = 8
	}

	// Clamp offset
	if f.cursor < f.offset {
		f.offset = f.cursor
	} else if f.cursor >= f.offset+visibleHeight {
		f.offset = f.cursor - visibleHeight + 1
	}
	if f.offset < 0 {
		f.offset = 0
	}

	end := min(len(f.entries), f.offset+visibleHeight)
	maxWidth := max(f.width-6, 30)

	composeStyle := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	dirStyle := lipgloss.NewStyle().Bold(true).Foreground(colorDockerBlue)
	fileStyle := lipgloss.NewStyle().Foreground(colorMuted)

	if len(f.entries) == 0 {
		b.WriteString(statusStyle.Render("  (empty directory)"))
		b.WriteString("\n")
	} else {
		for i := f.offset; i < end; i++ {
			e := f.entries[i]
			isSel := (i == f.cursor)

			prefix := "  "
			if isSel {
				prefix = "› "
			}

			icon := "📄 "
			var nameStyled string
			switch {
			case e.isParent:
				icon = "📁 "
				nameStyled = dirStyle.Render(".. (parent directory)")
			case e.isDir:
				icon = "📁 "
				nameStyled = dirStyle.Render(e.name + "/")
			case e.isCompose:
				icon = "🐳 "
				nameStyled = composeStyle.Render(e.name)
			default:
				nameStyled = fileStyle.Render(e.name)
			}

			if isSel {
				label := prefix + icon + e.name
				if e.isParent {
					label = prefix + icon + ".. (parent directory)"
				} else if e.isDir {
					label = prefix + icon + e.name + "/"
				}
				if len(label) < maxWidth {
					label += strings.Repeat(" ", maxWidth-len(label))
				} else if len(label) > maxWidth {
					label = label[:maxWidth]
				}
				b.WriteString(selectedRowStyle.Render(label))
			} else {
				b.WriteString(prefix + icon + nameStyled)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	switch {
	case f.loading:
		b.WriteString(warnStyle.Render("Reading Compose configuration…"))
	case f.err != "":
		b.WriteString(errorStyle.Render(f.err))
	default:
		b.WriteString(statusStyle.Render("Enter: open/browse · ↑/↓: navigate · Tab: complete path · Esc: cancel"))
	}

	return b.String()
}

type composeFormKeys struct{}

func (composeFormKeys) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open / browse")),
		key.NewBinding(key.WithKeys("↑/↓"), key.WithHelp("↑/↓", "navigate")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "complete path")),
		formCancelKey,
	}
}

func (k composeFormKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }
