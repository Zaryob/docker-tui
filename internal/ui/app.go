// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ui contains the Bubble Tea models that make up docker-tui.
package ui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/buildx"
	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/preferences"
)

// Options configures the application.
type Options struct {
	// PreferencesPath enables loading and saving non-secret UI preferences.
	PreferencesPath string
	// RefreshInterval is how often the visible list is reloaded. Zero or
	// less disables automatic refreshing.
	RefreshInterval time.Duration
}

// App is the root Bubble Tea model.
type App struct {
	savedPreferences preferences.State
	opts             Options
	client           *docker.Client
	buildx           *buildx.CLI
	// pull is the running image pull, which keeps running when its view
	// is closed.
	pull *pullView
	// build is the output of the running build, which keeps running when
	// its view is closed.
	compose  *logView
	projects map[string]buildx.Project
	build    *logView
	version  docker.Version

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

	// usage is the latest resource usage sample of the running
	// containers; usagePolling is set once sampling started.
	usage        map[string]containerUsage
	usagePolling bool

	// mouseOn records whether mouse reporting is enabled.
	mouseOn bool

	// events delivers the tabs affected by daemon events.
	events chan tabKind

	width  int
	height int
}

// New creates the root model.
func New(client *docker.Client, opts Options) *App {
	fi := textinput.New()
	fi.Prompt = "/"
	fi.Placeholder = "filter"
	fi.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	a := &App{
		opts:   opts,
		filter: fi,
		client: client,
		buildx: &buildx.CLI{Host: client.Host(), Context: client.Context()},
		tabs: []*tab{
			newContainersTab(),
			newImagesTab(),
			newVolumesTab(),
			newNetworksTab(),
			newBuildsTab(),
			newLogsTab(),
			newProjectsTab(),
		},
		projects: make(map[string]buildx.Project),
		help:     help.New(),
		spinner: spinner.New(
			spinner.WithSpinner(spinner.MiniDot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(colorAccent)),
		),
	}
	a.restorePreferences()
	return a
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

// targets returns the objects an action applies to: the marked objects of
// the active tab or, when none is marked, the selected one.
func (a *App) targets() []item {
	t := a.current()
	if marked := t.markedItems(); len(marked) > 0 {
		return marked
	}
	if it, ok := t.selected(); ok {
		if it.isGroup() {
			return it.members
		}
		return []item{it}
	}
	return nil
}

// selectedObject returns the object under the cursor; group rows, which
// stand for several objects, are not returned.
func (a *App) selectedObject() (item, bool) {
	it, ok := a.current().selected()
	if !ok || it.isGroup() {
		return item{}, false
	}
	return it, true
}

// objectAction runs fn against the targets of the active tab and reports
// the outcome as an actionMsg.
func (a *App) objectAction(verb string, fn actionFunc) tea.Cmd {
	return a.itemsAction(verb, a.targets(), fn)
}

// bulkConcurrency limits how many objects a bulk action handles at once.
const bulkConcurrency = 8

// itemsAction runs fn against items, several at a time, and reports the
// outcome as an actionMsg. The marks of the active tab are cleared, since
// they have been used.
func (a *App) itemsAction(verb string, items []item, fn actionFunc) tea.Cmd {
	if len(items) == 0 {
		return nil
	}
	t := a.current()
	t.clearMarks()
	a.busy++
	kind, noun := t.kind, t.noun
	return func() tea.Msg {
		// Stopping a container can take the whole grace period, so the
		// objects are handled concurrently. Errors keep the list order.
		results := make([]error, len(items))
		sem := make(chan struct{}, bulkConcurrency)
		var wg sync.WaitGroup
		for i, it := range items {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := fn(ctx, it.id); err != nil {
					results[i] = fmt.Errorf("%s %s: %w", verb, it.name, err)
				}
			}()
		}
		wg.Wait()
		var errs []error
		for _, err := range results {
			if err != nil {
				errs = append(errs, err)
			}
		}
		return actionMsg{kind: kind, status: actionStatus(verb, noun, items, errs), err: actionError(verb, noun, len(items), errs)}
	}
}

// actionStatus describes a successful action, e.g. "stopped web" or
// "stopped 3 containers".
func actionStatus(verb, noun string, items []item, errs []error) string {
	if len(items) == 1 {
		return verb + " " + items[0].name
	}
	return verb + " " + countNoun(len(items)-len(errs), noun)
}

// actionError summarizes the failures of an action on n objects in a single
// line, or returns nil if there was none.
func actionError(verb, noun string, n int, errs []error) error {
	switch {
	case len(errs) == 0:
		return nil
	case n == 1:
		return errs[0]
	case len(errs) == 1:
		return fmt.Errorf("%s %d of %s; %w", verb, n-1, countNoun(n, noun), errs[0])
	}
	return fmt.Errorf("%s %d of %s; %w (and %d more errors)", verb, n-len(errs), countNoun(n, noun), errs[0], len(errs)-1)
}

// describeItems names the objects of a bulk action for a dialog, listing at
// most a few of them.
func describeItems(items []item, noun string) string {
	const maxNames = 5
	if len(items) == 1 {
		return items[0].name
	}
	names := make([]string, 0, maxNames)
	for _, it := range items[:min(len(items), maxNames)] {
		names = append(names, it.name)
	}
	text := strings.Join(names, ", ")
	if more := len(items) - maxNames; more > 0 {
		text += fmt.Sprintf(" and %d more", more)
	}
	return countNoun(len(items), noun) + ": " + text
}

// removeDialog builds the confirmation dialog for removing the targets of
// the active tab.
func (a *App) removeDialog() *confirmDialog {
	t := a.current()
	if t.kind == tabLogs || t.kind == tabProjects {
		return nil
	}
	items := a.targets()
	if len(items) == 0 {
		return nil
	}
	remove := func(force bool) func() tea.Cmd {
		return func() tea.Cmd {
			return a.itemsAction("removed", items, func(ctx context.Context, id string) error {
				switch t.kind {
				case tabContainers:
					return a.client.RemoveContainer(ctx, id, force)
				case tabImages:
					return a.client.RemoveImage(ctx, id, force)
				case tabVolumes:
					return a.client.RemoveVolume(ctx, id, force)
				case tabNetworks:
					return a.client.RemoveNetwork(ctx, id)
				case tabBuilds:
					return a.buildx.Remove(ctx, id)
				}
				return nil
			})
		}
	}
	title := "Remove " + strings.TrimSuffix(t.noun, "s") + "?"
	if len(items) > 1 {
		title = "Remove " + countNoun(len(items), t.noun) + "?"
	}
	d := &confirmDialog{
		title:   title,
		message: describeItems(items, t.noun),
		onYes:   remove(false),
	}
	if t.kind != tabNetworks && t.kind != tabBuilds && t.kind != tabLogs {
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
	case tabBuilds:
		return a.loadBuilds
	case tabProjects:
		return a.loadProjects
	case tabLogs:
		return a.loadLogs
	}
	return nil
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	// Daemon events keep the lists up to date as soon as something changes;
	// the periodic refresh still updates relative times such as "Up 5
	// minutes". The watcher runs until the program exits.
	a.events = make(chan tabKind, 64)
	go watchEvents(context.Background(), a.client, a.events)
	return tea.Batch(a.loadVersion, a.load(tabContainers), a.spinner.Tick, a.refreshTick(), waitEvents(a.events))
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
	before := a.preferenceSnapshot()
	m, cmd := a.update(msg)
	if a.opts.PreferencesPath != "" {
		after := a.preferenceSnapshot()
		if !reflect.DeepEqual(before, after) {
			if err := preferences.Save(a.opts.PreferencesPath, after); err != nil {
				cmd = tea.Batch(cmd, a.setStatus("save preferences: "+err.Error(), true))
			} else {
				a.savedPreferences = after
			}
		}
	}
	// A pull started from its view keeps running in the background.
	if v, ok := a.pane.(*pullView); ok && v.running() && a.pull == nil {
		a.pull = v
	}
	return m, tea.Batch(cmd, a.syncMouse())
}

// syncMouse turns mouse reporting on while a terminal is shown, for wheel
// scrolling, and off otherwise, so that text can be selected as usual.
func (a *App) syncMouse() tea.Cmd {
	_, want := a.pane.(*terminalView)
	if want == a.mouseOn {
		return nil
	}
	a.mouseOn = want
	if want {
		return tea.EnableMouseCellMotion
	}
	return tea.DisableMouse
}

func (a *App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if v, ok := a.pane.(*terminalView); ok {
			v.mouse(msg)
		}
		return a, nil
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
		if msg.err == nil || msg.partial {
			if msg.kind == tabContainers {
				applyUsage(msg.items, a.usage)
			}
			t.setItems(msg.items)
		}
		// Start sampling the resource usage once the containers are known.
		if msg.kind == tabContainers && msg.err == nil && !a.usagePolling {
			a.usagePolling = true
			return a, a.loadUsage()
		}
	case projectsMsg:
		a.mergeProjects(msg.projects, msg.err)
	case openComposeMsg:
		f, ok := a.pane.(*composeForm)
		if !ok || f != msg.form {
			return a, nil
		}
		f.loading = false
		if msg.err != nil {
			f.err = msg.err.Error()
			return a, nil
		}
		f.close()
		a.projects[msg.project.Name] = msg.project
		a.refreshProjects()
		a.pane = nil
		a.active = int(tabProjects)
		a.layout()
		return a, a.setStatus("opened Compose project "+msg.project.Name, false)
	case usageMsg:
		a.usage = msg.usage
		t := a.tabs[tabContainers]
		applyUsage(t.all, a.usage)
		t.refresh()
		return a, tea.Tick(usageInterval, func(time.Time) tea.Msg { return usageTickMsg{} })
	case usageTickMsg:
		return a, a.loadUsage()
	case actionMsg:
		a.busy--
		if msg.err != nil {
			return a, a.setStatus(msg.err.Error(), true)
		}
		return a, tea.Batch(a.setStatus(msg.status, false), a.load(msg.kind))
	case startBuildMsg:
		if a.pane != nil {
			a.pane.close()
		}
		return a, a.startBuild(msg.opts)
	case copyMsg:
		if a.pane != nil {
			a.pane.close()
			a.pane = nil
			a.layout()
		}
		return a, a.runCopy(msg)
	case closePaneMsg:
		if a.pane != nil {
			a.pane.close()
			a.pane = nil
			a.layout()
		}
		return a, nil
	case execDoneMsg:
		if msg.err != nil {
			return a, tea.Batch(a.setStatus(fmt.Sprintf("exec %s: %v", msg.name, msg.err), true), a.load(tabContainers))
		}
		return a, a.load(tabContainers)
	case eventsMsg:
		cmds := []tea.Cmd{waitEvents(a.events)}
		for kind := range msg.kinds {
			if a.tabs[kind].loaded {
				cmds = append(cmds, a.load(kind))
			}
		}
		return a, tea.Batch(cmds...)
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
	case pullMsg:
		if a.pull != nil && msg.gen == a.pull.gen {
			cmd := a.pull.update(msg)
			if a.pull.done {
				return a, tea.Batch(cmd, a.pullFinished())
			}
			return a, cmd
		}
	case logChunkMsg:
		if a.compose != nil && msg.gen == a.compose.gen {
			cmd := a.compose.update(msg)
			if a.compose.done {
				return a, tea.Batch(cmd, a.composeFinished())
			}
			return a, cmd
		}
		if a.build != nil && msg.gen == a.build.gen {
			cmd := a.build.update(msg)
			if a.build.done {
				return a, tea.Batch(cmd, a.buildFinished())
			}
			return a, cmd
		}
		if a.pane != nil {
			return a, a.pane.update(msg)
		}
	case inspectMsg, statsMsg, statsTickMsg, topMsg, topTickMsg:
		if a.pane != nil {
			return a, a.pane.update(msg)
		}
	case termStartedMsg:
		// A terminal closed before it connected leaves its session behind.
		if a.pane == nil && msg.sess != nil {
			msg.sess.conn.Close()
		}
		if a.pane != nil {
			return a, a.pane.update(msg)
		}
	case termOutputMsg, termEndedMsg, termSettleMsg:
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
		if r, ok := a.pane.(rawInputPane); ok && r.rawInput() {
			return a.pane.update(msg)
		}
		if c, ok := a.pane.(inputCapturer); ok && c.capturesInput() && msg.String() != "ctrl+c" {
			return a.pane.update(msg)
		}
		switch {
		case key.Matches(msg, keys.Back) && a.pull != nil && a.pane == pane(a.pull) && a.pull.running():
			a.pane = nil
			a.layout()
			return a.setStatus("pull continues in the background; u shows it again", false)
		case key.Matches(msg, keys.Back) && a.pane == asPane(a.compose):
			a.pane = nil
			a.layout()
			return a.setStatus("Compose continues in the background; 7 then u shows progress", false)
		case key.Matches(msg, keys.Back) && a.pane == asPane(a.build):
			// Closing the output of a build leaves the build running.
			a.pane = nil
			a.layout()
			return a.setStatus("build continues in the background; b shows it again", false)
		case key.Matches(msg, keys.Back):
			a.pane.close()
			a.pane = nil
			a.layout()
			return nil
		case msg.String() == "ctrl+c":
			a.pane.close()
			a.closeOperations()
			return tea.Quit
		}
		return a.pane.update(msg)
	}

	t := a.current()
	switch {
	case key.Matches(msg, keys.ClearFilter) && t.filter != "":
		t.setFilter("")
	case key.Matches(msg, keys.ClearFilter) && len(t.marked) > 0:
		t.clearMarks()
	case key.Matches(msg, keys.Mark):
		t.toggleMark()
		t.table.Move(1)
	case key.Matches(msg, keys.MarkAll):
		t.markAll()
	case key.Matches(msg, keys.Filter):
		a.filtering = true
		a.filter.SetValue(t.filter)
		a.filter.CursorEnd()
		return a.filter.Focus()
	case key.Matches(msg, keys.Quit):
		a.closeOperations()
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
	case key.Matches(msg, keys.CopyID):
		return a.copyID()
	default:
		if t.kind == tabProjects {
			return a.projectKeys(msg)
		}
		if t.kind == tabContainers {
			return a.containerKeys(msg)
		}
		if t.kind == tabImages && key.Matches(msg, keys.Pull) {
			return a.showPull()
		}
		if (t.kind == tabImages || t.kind == tabBuilds) && key.Matches(msg, keys.Build) {
			return a.showBuild()
		}
		if t.kind == tabBuilds && key.Matches(msg, keys.Logs) {
			return a.openBuildLogs()
		}
		if t.kind == tabLogs && key.Matches(msg, keys.Logs) {
			return a.openSelectedLog()
		}
		// Other objects have no logs; enter inspects them.
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

// inputCapturer is implemented by panes that temporarily need every key,
// e.g. while a text input has focus.
type inputCapturer interface {
	capturesInput() bool
}

// rawInputPane is implemented by panes that need every key, ctrl+c
// included, such as the terminal.
type rawInputPane interface {
	rawInput() bool
}

// keyMapper is implemented by panes with their own key bindings.
type keyMapper interface {
	keyMap() help.KeyMap
}

// helpKeys returns the key map for the current screen.
func (a *App) helpKeys() help.KeyMap {
	if a.pane != nil {
		if m, ok := a.pane.(keyMapper); ok {
			return m.keyMap()
		}
		return paneKeyMap{}
	}
	if a.current().kind == tabProjects {
		return projectKeyMap{}
	}
	return keys
}

// bodyHeight returns the height available between the header and the
// status line.
func (a *App) bodyHeight() int {
	headerHeight := lipgloss.Height(a.header())
	helpHeight := lipgloss.Height(a.help.View(a.helpKeys()))
	// header + status line + help
	return max(a.height-headerHeight-1-helpHeight, 1)
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
	styleContainer := lipgloss.NewStyle().Foreground(colorDockerBlue)
	styleBody := lipgloss.NewStyle().Foreground(colorDockerPale)

	// Fixed 12-column geometric logo with whale tail
	logoLine1 := "    " + styleContainer.Render("▪ ▪ ▪") + "   "
	logoLine2 := "  " + styleContainer.Render("▪ ▪ ▪ ▪") + "  " + styleBody.Render("╮")
	logoLine3 := "   " + styleBody.Render("╰───────╯")

	var tabRendered []string
	for i, t := range a.tabs {
		label := fmt.Sprintf("%d %s", i+1, t.title)
		if i == a.active {
			label = strings.ToUpper(label)
			pill := lipgloss.NewStyle().Foreground(colorActiveTabBg).Render("▌") +
				activeTabStyle.Render(" "+label+" ") +
				lipgloss.NewStyle().Foreground(colorActiveTabBg).Render("▐")
			tabRendered = append(tabRendered, pill)
		} else {
			tabRendered = append(tabRendered, tabStyle.Render(label))
		}
	}
	tabSep := " " + tabSepStyle.Render("│") + " "
	tabBar := strings.Join(tabRendered, tabSep)

	info := "connecting…"
	if a.version.Version != "" {
		info = fmt.Sprintf("Docker %s", a.version.Version)
	}
	right := headerInfoStyle.Render(info)
	if bi := a.buildIndicator(); bi != "" {
		right = bi + "  " + right
	}

	const logoW = 12
	const logoGap = 3
	availWidth := a.width - logoW - logoGap
	if availWidth < lipgloss.Width(tabBar)+lipgloss.Width(right) {
		right = ""
	}
	gap := max(availWidth-lipgloss.Width(tabBar)-lipgloss.Width(right), 1)
	row2Right := tabBar
	if right != "" {
		row2Right = tabBar + strings.Repeat(" ", gap) + right
	}

	row1 := logoLine1
	row2 := logoLine2 + strings.Repeat(" ", logoGap) + row2Right
	row3 := logoLine3
	divider := tabSepStyle.Render(strings.Repeat("─", max(a.width, 10)))

	return row1 + "\n" + row2 + "\n" + row3 + "\n" + divider
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
	// A build running in the background stays visible whatever the width
	// of the header.
	if bi := a.buildIndicator(); bi != "" {
		prefix += bi + statusStyle.Render(" · ")
	}
	if ci := a.composeIndicator(); ci != "" {
		prefix += ci + statusStyle.Render(" · ")
	}
	if pi := a.pullIndicator(); pi != "" {
		prefix += pi + statusStyle.Render(" · ")
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
			text = fmt.Sprintf("%d of %s matching %q", len(t.visible), text, t.filter)
		}
		if n := len(t.marked); n > 0 {
			text += fmt.Sprintf(" · %d marked", n)
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

func (a *App) closeOperations() {
	if a.compose != nil {
		a.compose.close()
	}
	if a.build != nil {
		a.build.close()
	}
	if a.pull != nil {
		a.pull.close()
	}
}
