// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/buildx"
	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

const buildStatusCol = 2

func newBuildsTab() *tab {
	t := newTab(tabBuilds, "Builds", "builds",
		Column{Title: "BUILD ID", Width: 12},
		Column{Title: "NAME", Grow: 2},
		Column{Title: "STATUS", Width: 10},
		Column{Title: "STEPS", Width: 7},
		Column{Title: "CACHED", Width: 6},
		Column{Title: "CREATED", Width: 18},
		Column{Title: "DURATION", Width: 9},
	)
	t.table.SetCellStyle(func(col int, value string) (lipgloss.Style, bool) {
		if col != buildStatusCol {
			return lipgloss.Style{}, false
		}
		switch strings.ToLower(value) {
		case "completed":
			return okStyle, true
		case "running":
			return warnStyle, true
		case "error", "failed":
			return errorStyle, true
		}
		return statusStyle, true
	})
	return t
}

func (a *App) loadBuilds() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	records, err := a.buildx.History(ctx)
	return listMsg{kind: tabBuilds, items: buildItems(records, time.Now()), err: err}
}

func buildItems(records []buildx.Record, now time.Time) []item {
	items := make([]item, len(records))
	for i, r := range records {
		cached := "-"
		if r.TotalSteps > 0 {
			cached = fmt.Sprintf("%d%%", r.CachedSteps*100/r.TotalSteps)
		}
		name := r.Name
		if name == "" {
			name = r.ID()
		}
		d := r.Duration(now)
		items[i] = item{
			id:   r.ID(),
			name: name,
			cells: []string{
				format.ShortID(r.ID()),
				name,
				r.Status,
				fmt.Sprintf("%d/%d", r.CompletedSteps, r.TotalSteps),
				cached,
				format.Ago(r.CreatedAt, now),
				formatDuration(d),
			},
			sortKeys: []string{
				5: fmt.Sprintf("%020d", r.CreatedAt.UnixNano()),
				6: fmt.Sprintf("%020d", d),
			},
			data: r,
		}
	}
	return items
}

// formatDuration renders a build duration compactly, e.g. "850ms", "21.7s"
// or "3m05s".
func formatDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return "-"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

// openBuildLogs shows the progress output of the selected build.
func (a *App) openBuildLogs() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	cli, id := a.buildx, it.id
	l := newLogView(it.name, a.width, a.bodyHeight())
	l.kind, l.ended = "Build logs", "complete"
	l.running = "● loading"
	l.client = a.client
	l.sources = []logSource{{id: id, name: it.name, open: func(ctx context.Context, _ bool) (io.ReadCloser, error) {
		return cli.Logs(ctx, id)
	}}}
	a.pane = l
	return l.start()
}

// startBuildMsg asks the app to run a build from the build form.
type startBuildMsg struct{ opts buildx.BuildOptions }

// startBuild replaces the build form with the output of the build. The
// build keeps running when its output is closed; b shows it again.
func (a *App) startBuild(opts buildx.BuildOptions) tea.Cmd {
	cli := a.buildx
	name := filepath.Base(opts.Context)
	if len(opts.Tags) > 0 {
		name = strings.Join(opts.Tags, ", ")
	}
	l := newLogView(name, a.width, a.bodyHeight())
	l.kind, l.running, l.ended = "Build", "● building", "✓ build finished"
	l.cancelable = true
	l.client = a.client
	l.sources = []logSource{{name: name, open: func(ctx context.Context, _ bool) (io.ReadCloser, error) {
		return cli.Build(ctx, opts)
	}}}
	a.build = l
	a.pane = l
	a.layout()
	return l.start()
}

// showBuild opens the build form, or the output of the running build.
func (a *App) showBuild() tea.Cmd {
	if a.build != nil {
		a.build.setSize(a.width, a.bodyHeight())
		a.pane = a.build
		a.layout()
		return nil
	}
	return a.openBuildForm()
}

// buildFinished reports the end of the running build when its output is
// not shown, and refreshes the lists it changed.
func (a *App) buildFinished() tea.Cmd {
	l := a.build
	a.build = nil
	cmds := []tea.Cmd{}
	for _, k := range []tabKind{tabImages, tabBuilds} {
		if a.tabs[k].loaded {
			cmds = append(cmds, a.load(k))
		}
	}
	if a.pane != asPane(l) {
		switch {
		case l.canceled:
			cmds = append(cmds, a.setStatus("build of "+l.name+" canceled", false))
		case l.err != nil:
			cmds = append(cmds, a.setStatus("build of "+l.name+" failed: "+l.err.Error(), true))
		default:
			cmds = append(cmds, a.setStatus("build of "+l.name+" finished", false))
		}
	}
	return tea.Batch(cmds...)
}

// asPane converts a log view to a pane, keeping nil as a nil interface.
func asPane(l *logView) pane {
	if l == nil {
		return nil
	}
	return l
}

// buildIndicator describes the build running in the background.
func (a *App) buildIndicator() string {
	if a.build == nil || a.pane == asPane(a.build) {
		return ""
	}
	return warnStyle.Render("⚙ building " + a.build.name)
}

// buildForm asks for the context, Dockerfile and tag of a new build.
type buildForm struct {
	inputs []textinput.Model
	focus  int
	err    string
	width  int
}

const (
	formContext = iota
	formDockerfile
	formTag
)

func newBuildForm(cwd string) *buildForm {
	labels := []struct{ prompt, placeholder, value string }{
		{"Context     ", "directory", cwd},
		{"Dockerfile  ", "<context>/Dockerfile", ""},
		{"Tag         ", "name:tag, optional, comma separated", ""},
	}
	f := &buildForm{}
	for _, l := range labels {
		in := textinput.New()
		in.Prompt = l.prompt
		in.PromptStyle = statsLabelStyle
		in.Placeholder = l.placeholder
		in.SetValue(l.value)
		f.inputs = append(f.inputs, in)
	}
	f.inputs[0].Focus()
	return f
}

// openBuildForm opens the form for a new build, starting in the current
// directory.
func (a *App) openBuildForm() tea.Cmd {
	cwd, _ := os.Getwd()
	f := newBuildForm(cwd)
	f.setSize(a.width, a.bodyHeight())
	a.pane = f
	a.layout()
	return textinput.Blink
}

func (f *buildForm) setSize(width, _ int) {
	f.width = width
	for i := range f.inputs {
		f.inputs[i].Width = max(width-20, 10)
	}
}

func (f *buildForm) close() {}

// capturesInput keeps every key, so that "q" can be typed.
func (f *buildForm) capturesInput() bool { return true }

func (f *buildForm) keyMap() help.KeyMap { return buildFormKeys{} }

// options validates the form and returns the build options.
func (f *buildForm) options() (buildx.BuildOptions, error) {
	dir := expandHome(strings.TrimSpace(f.inputs[formContext].Value()))
	if dir == "" {
		return buildx.BuildOptions{}, fmt.Errorf("the build context is required")
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return buildx.BuildOptions{}, fmt.Errorf("%s is not a directory", dir)
	}
	opts := buildx.BuildOptions{Context: dir}
	if df := expandHome(strings.TrimSpace(f.inputs[formDockerfile].Value())); df != "" {
		if !filepath.IsAbs(df) {
			df = filepath.Join(dir, df)
		}
		opts.Dockerfile = df
	}
	if _, err := os.Stat(cmpOr(opts.Dockerfile, filepath.Join(dir, "Dockerfile"))); err != nil {
		return buildx.BuildOptions{}, fmt.Errorf("no Dockerfile: %s", cmpOr(opts.Dockerfile, filepath.Join(dir, "Dockerfile")))
	}
	for _, t := range strings.Split(f.inputs[formTag].Value(), ",") {
		if t = strings.TrimSpace(t); t != "" {
			opts.Tags = append(opts.Tags, t)
		}
	}
	return opts, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// closePaneMsg asks the app to close the current pane.
type closePaneMsg struct{}

func (f *buildForm) update(msg tea.Msg) tea.Cmd {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
		return cmd
	}
	switch km.String() {
	case "esc":
		return func() tea.Msg { return closePaneMsg{} }
	case "tab", "down":
		return f.move(1)
	case "shift+tab", "up":
		return f.move(-1)
	case "enter":
		opts, err := f.options()
		if err != nil {
			f.err = err.Error()
			return nil
		}
		return func() tea.Msg { return startBuildMsg{opts: opts} }
	}
	f.err = ""
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(km)
	return cmd
}

func (f *buildForm) move(delta int) tea.Cmd {
	f.inputs[f.focus].Blur()
	f.focus = (f.focus + delta + len(f.inputs)) % len(f.inputs)
	return f.inputs[f.focus].Focus()
}

func (f *buildForm) view() string {
	lines := []string{paneTitleStyle.Render("Build an image"), ""}
	for _, in := range f.inputs {
		lines = append(lines, in.View())
	}
	lines = append(lines, "")
	if f.err != "" {
		lines = append(lines, errorStyle.Render(f.err))
	} else {
		lines = append(lines, statusStyle.Render("Runs docker build with BuildKit; esc on the output keeps it running in the background."))
	}
	return strings.Join(lines, "\n")
}

type buildFormKeys struct{}

var (
	formNextKey   = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field"))
	formSubmitKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "build"))
	formCancelKey = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
)

func (buildFormKeys) ShortHelp() []key.Binding {
	return []key.Binding{formNextKey, formSubmitKey, formCancelKey}
}

func (k buildFormKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// pruneBuilds removes the whole build history.
func (a *App) pruneBuilds(ctx context.Context) (docker.PruneReport, error) {
	n := len(a.tabs[tabBuilds].all)
	if err := a.buildx.RemoveAll(ctx); err != nil {
		return docker.PruneReport{}, err
	}
	return docker.PruneReport{Deleted: n}, nil
}
