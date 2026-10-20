// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/zaryob/docker-tui/internal/docker"
)

const (
	logTail     = 1000
	logMaxLines = 10000
	logBatch    = 500
)

var (
	paneTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	linkStyle      = lipgloss.NewStyle().Foreground(colorAccent)
	matchStyle     = lipgloss.NewStyle().Background(colorWarn).Foreground(lipgloss.Color("#000000")).Bold(true)
	colHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	timeStyle      = lipgloss.NewStyle().Foreground(colorMuted)
	dividerStyle   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#D0D0D0", Dark: "#2C313A"})
	searchBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#B0B0B0", Dark: "#3A3F4B"}).
			Padding(0, 1)
)

// logGen identifies a log view so that messages from a stream belonging to a
// previously closed view are ignored.
var logGen int

// logSource is a container whose output is shown in a log view.
type logSource struct {
	id   string
	name string
	// open, when set, replaces the container logs as the source of the
	// lines, e.g. for the output of a build.
	open func(ctx context.Context, timestamps bool) (io.ReadCloser, error)
}

// read opens the output of the source.
func (src logSource) read(ctx context.Context, client *docker.Client, tail int, timestamps bool) (io.ReadCloser, error) {
	if src.open != nil {
		return src.open(ctx, timestamps)
	}
	return client.Logs(ctx, src.id, docker.LogOptions{Follow: true, Tail: tail, Timestamps: timestamps})
}

// logEntry is a structured log line.
type logEntry struct {
	raw     string
	time    string
	object  string
	isBuild bool
	message string
	color   lipgloss.AdaptiveColor
}

// parseLogLine parses raw text into a structured log entry.
func parseLogLine(raw, defaultObj string, isBuild bool, defaultTime time.Time, colIndex int) logEntry {
	entry := logEntry{
		raw:     raw,
		time:    defaultTime.Format("2006-01-02 15:04:05"),
		object:  defaultObj,
		isBuild: isBuild,
		color:   logPrefixColors[colIndex%len(logPrefixColors)],
	}
	s := strings.TrimRight(raw, "\r")

	// Multi-container prefix like "name | ..."
	if idx := strings.Index(s, " | "); idx > 0 && idx < 30 {
		entry.object = strings.TrimSpace(ansi.Strip(s[:idx]))
		s = s[idx+3:]
	}

	plain := ansi.Strip(s)

	// Check for timestamp in RFC3339Nano, RFC3339, or standard format
	if len(plain) >= 19 && plain[4] == '-' && plain[7] == '-' {
		if plain[10] == 'T' {
			spaceIdx := strings.IndexByte(plain, ' ')
			if spaceIdx > 19 {
				timeStr := plain[:spaceIdx]
				if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
					entry.time = t.Local().Format("2006-01-02 15:04:05")
					s = strings.TrimPrefix(s, timeStr)
					s = strings.TrimLeft(s, " ")
					plain = strings.TrimPrefix(plain, timeStr)
					plain = strings.TrimLeft(plain, " ")
				}
			}
		} else if (plain[10] == ' ' || plain[10] == '_') && len(plain) >= 19 {
			timeCandidate := plain[:19]
			if t, err := time.Parse("2006-01-02 15:04:05", timeCandidate); err == nil {
				entry.time = t.Format("2006-01-02 15:04:05")
				s = strings.TrimPrefix(s, timeCandidate)
				s = strings.TrimLeft(s, " ")
				plain = strings.TrimPrefix(plain, timeCandidate)
				plain = strings.TrimLeft(plain, " ")
			}
		}
	}

	// Clean build step numbers like "#1 "
	if entry.isBuild || strings.Contains(entry.object, "build") {
		entry.isBuild = true
		if strings.HasPrefix(plain, "#") {
			if spaceIdx := strings.IndexByte(plain, ' '); spaceIdx > 1 && spaceIdx < 6 {
				s = s[spaceIdx+1:]
			}
		}
	}

	entry.message = s
	return entry
}

// logView streams the output of one or more containers in a structured table.
type logView struct {
	gen     int
	name    string
	sources []logSource
	client  *docker.Client
	// timestamps prefixes every line with the time it was logged.
	timestamps bool
	vp         viewport.Model
	lines      []string
	entries    []logEntry
	ch         chan logEvent
	cancel     context.CancelFunc
	done       bool
	err        error
	// kind titles the view ("Logs" by default); running and ended describe
	// the state of the stream in the header.
	kind       string
	running    string
	ended      string
	cancelable bool
	canceled   bool

	cursor        int
	marked        map[int]bool
	follow        bool
	caseSensitive bool

	searching bool
	search    textinput.Model
	query     string
	matches   []int
	match     int

	inspecting bool
	status     string
}

type logEvent struct {
	line string
	err  error
}

type logChunkMsg struct {
	gen   int
	lines []string
	done  bool
	err   error
}

func newLogView(name string, width, height int) *logView {
	logGen++
	si := textinput.New()
	si.Prompt = "🔍 "
	si.PromptStyle = lipgloss.NewStyle().Foreground(colorAccent)
	si.Placeholder = "Search logs (supports regex like \"/error|warn/\")"
	l := &logView{
		gen:        logGen,
		name:       name,
		vp:         viewport.New(0, 0),
		search:     si,
		marked:     make(map[int]bool),
		follow:     true,
		timestamps: false,
	}
	l.setSize(width, height)
	return l
}

func (l *logView) setSize(width, height int) {
	l.vp.Width = width
	// Header (1) + search box (3) + column header (1) + divider (1) = 6 lines
	l.vp.Height = max(height-6, 1)
}

// openLogs opens the log view for the marked containers or, when none is marked.
func (a *App) openLogs() tea.Cmd {
	items := a.targets()
	if len(items) == 0 {
		return nil
	}
	sources := make([]logSource, len(items))
	for i, it := range items {
		sources[i] = logSource{id: it.id, name: it.name}
	}
	name := items[0].name
	if len(items) > 1 {
		name = describeItems(items, "containers")
	}
	l := newLogView(name, a.width, a.bodyHeight())
	l.client, l.sources = a.client, sources
	a.pane = l
	return l.start()
}

// logPrefixColors are used for container and object names.
var logPrefixColors = []lipgloss.AdaptiveColor{
	{Light: "#0B5CAD", Dark: "#2496ED"},
	{Light: "#2E7D32", Dark: "#5FD787"},
	{Light: "#B26A00", Dark: "#FFAF5F"},
	{Light: "#8E24AA", Dark: "#D787FF"},
	{Light: "#00838F", Dark: "#5FD7D7"},
	{Light: "#C62828", Dark: "#FF8787"},
}

func logPrefixes(sources []logSource) []string {
	if len(sources) < 2 {
		return nil
	}
	width := 0
	for _, src := range sources {
		width = max(width, ansi.StringWidth(src.name))
	}
	prefixes := make([]string, len(sources))
	for i, src := range sources {
		style := lipgloss.NewStyle().Foreground(logPrefixColors[i%len(logPrefixColors)])
		prefixes[i] = style.Render(src.name+strings.Repeat(" ", width-ansi.StringWidth(src.name))+" |") + " "
	}
	return prefixes
}

func (l *logView) start() tea.Cmd {
	client, timestamps := l.client, l.timestamps
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	ch := make(chan logEvent, logBatch)
	l.ch = ch
	prefixes := logPrefixes(l.sources)
	tail := max(logTail/max(len(l.sources), 1), 100)
	var wg sync.WaitGroup
	for i, src := range l.sources {
		prefix := ""
		if prefixes != nil {
			prefix = prefixes[i]
		}
		wg.Add(1)
		go func(s logSource, pfx string) {
			defer wg.Done()
			fail := func(err error) {
				if len(l.sources) > 1 {
					err = fmt.Errorf("%s: %w", s.name, err)
				}
				select {
				case ch <- logEvent{err: err}:
				case <-ctx.Done():
				}
			}
			rc, err := s.read(ctx, client, tail, timestamps)
			if err != nil {
				fail(err)
				return
			}
			defer rc.Close()
			sc := bufio.NewScanner(rc)
			sc.Buffer(make([]byte, 64*1024), 1024*1024)
			for sc.Scan() {
				select {
				case ch <- logEvent{line: pfx + cleanLogLine(sc.Text())}:
				case <-ctx.Done():
					return
				}
			}
			if err := sc.Err(); err != nil && ctx.Err() == nil {
				fail(err)
			}
		}(src, prefix)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	return l.wait()
}

func (l *logView) wait() tea.Cmd {
	ch, gen := l.ch, l.gen
	return func() tea.Msg {
		msg := logChunkMsg{gen: gen}
		ev, ok := <-ch
		for {
			if !ok {
				msg.done = true
				return msg
			}
			if ev.err != nil {
				msg.err = ev.err
			} else {
				msg.lines = append(msg.lines, ev.line)
			}
			if len(msg.lines) >= logBatch {
				return msg
			}
			select {
			case ev, ok = <-ch:
			default:
				return msg
			}
		}
	}
}

func (l *logView) close() {
	if l.cancel != nil {
		l.cancel()
	}
}

func cleanLogLine(s string) string {
	s = strings.TrimRight(s, "\r")
	return strings.ReplaceAll(s, "\t", "    ")
}

func (l *logView) capturesInput() bool { return l.searching || l.inspecting }

func (l *logView) ensureEntries() {
	if len(l.entries) != len(l.lines) {
		l.entries = make([]logEntry, len(l.lines))
		isBuild := l.kind == "Build" || l.kind == "Build logs" || strings.Contains(l.name, "build")
		for i, line := range l.lines {
			l.entries[i] = parseLogLine(line, l.name, isBuild, time.Now(), i)
		}
	}
}

func (l *logView) render() {
	l.matches = l.matches[:0]
	l.ensureEntries()

	if len(l.entries) == 0 {
		l.vp.SetContent("")
		return
	}

	rendered := make([]string, len(l.entries))
	for i, entry := range l.entries {
		hlMsg, matched := highlightLogMatches(entry.message, l.query, l.caseSensitive)
		if matched {
			l.matches = append(l.matches, i)
		} else if l.query == "" {
			hlMsg = entry.message
		}
		rendered[i] = l.formatRow(i, entry, hlMsg)
	}
	l.vp.SetContent(strings.Join(rendered, "\n"))
}

func (l *logView) formatRow(idx int, entry logEntry, hlMsg string) string {
	var cb string
	if l.marked[idx] {
		cb = markStyle.Render("[✓]")
	} else {
		cb = statusStyle.Render("[ ]")
	}

	timeStr := timeStyle.Render(entry.time)

	icon := "📦 "
	if entry.isBuild {
		icon = "🔧 "
	}
	objStyle := lipgloss.NewStyle().Foreground(entry.color)
	objStr := objStyle.Render(runewidth.FillRight(runewidth.Truncate(icon+entry.object, 16, "…"), 16))

	isSelected := (idx == l.cursor)
	endIcon := "⌄"
	if isSelected {
		endIcon = "📋"
	}
	endIconStr := lipgloss.NewStyle().Foreground(colorAccent).Render(endIcon)

	fixedWidth := 3 + 2 + 19 + 2 + 16 + 2 + 2 + 2
	msgWidth := max(l.vp.Width-fixedWidth-1, 10)
	msgFormatted := runewidth.FillRight(runewidth.Truncate(hlMsg, msgWidth, "…"), msgWidth)

	line := fmt.Sprintf("%s  %s  %s  %s  %s", cb, timeStr, objStr, msgFormatted, endIconStr)
	targetW := max(l.vp.Width, 10)
	line = ansi.Truncate(line, targetW, "")
	if pad := targetW - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	if isSelected {
		line = selectedRowStyle.Render(line)
	}
	return line
}

// highlightMatches returns line with every case-insensitive occurrence of query highlighted.
func highlightMatches(line, query string) (string, bool) {
	return highlightLogMatches(line, query, false)
}

func highlightLogMatches(line, query string, caseSensitive bool) (string, bool) {
	if query == "" {
		return line, false
	}
	plain := ansi.Strip(line)

	// Regex search: /pattern/
	if strings.HasPrefix(query, "/") && strings.HasSuffix(query, "/") && len(query) > 2 {
		pattern := query[1 : len(query)-1]
		if !caseSensitive {
			pattern = "(?i)" + pattern
		}
		if re, err := regexp.Compile(pattern); err == nil {
			locs := re.FindAllStringIndex(plain, -1)
			if len(locs) == 0 {
				return line, false
			}
			var b strings.Builder
			last := 0
			for _, loc := range locs {
				b.WriteString(plain[last:loc[0]])
				b.WriteString(matchStyle.Render(plain[loc[0]:loc[1]]))
				last = loc[1]
			}
			b.WriteString(plain[last:])
			return b.String(), true
		}
	}

	if caseSensitive {
		i := strings.Index(plain, query)
		if i < 0 {
			return line, false
		}
		var b strings.Builder
		for i >= 0 {
			b.WriteString(plain[:i])
			b.WriteString(matchStyle.Render(plain[i : i+len(query)]))
			plain = plain[i+len(query):]
			i = strings.Index(plain, query)
		}
		b.WriteString(plain)
		return b.String(), true
	}

	i := indexFold(plain, query)
	if i < 0 {
		return line, false
	}
	var b strings.Builder
	for i >= 0 {
		b.WriteString(plain[:i])
		b.WriteString(matchStyle.Render(plain[i : i+len(query)]))
		plain = plain[i+len(query):]
		i = indexFold(plain, query)
	}
	b.WriteString(plain)
	return b.String(), true
}

func indexFold(s, substr string) int {
	n := len(substr)
	for i := range s {
		if len(s)-i < n {
			break
		}
		if strings.EqualFold(s[i:i+n], substr) {
			return i
		}
	}
	return -1
}

func (l *logView) setQuery(q string) {
	l.query = q
	l.render()
	l.match = len(l.matches) - 1
	l.showMatch()
}

func (l *logView) nextMatch(delta int) {
	if len(l.matches) == 0 {
		return
	}
	l.match = (l.match + delta + len(l.matches)) % len(l.matches)
	l.showMatch()
}

func (l *logView) showMatch() {
	if l.match < 0 || l.match >= len(l.matches) {
		return
	}
	target := l.matches[l.match]
	l.cursor = target
	l.vp.SetYOffset(target - l.vp.Height/2)
	l.render()
}

func (l *logView) handleSearchKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		l.searching = false
		l.search.Blur()
		l.setQuery("")
		return nil
	case "enter":
		l.searching = false
		l.search.Blur()
		return nil
	}
	var cmd tea.Cmd
	l.search, cmd = l.search.Update(msg)
	l.setQuery(l.search.Value())
	return cmd
}

func (l *logView) copySelected() string {
	l.ensureEntries()
	if len(l.entries) == 0 {
		return ""
	}
	var texts []string
	if len(l.marked) > 0 {
		for i := 0; i < len(l.entries); i++ {
			if l.marked[i] {
				texts = append(texts, l.entries[i].message)
			}
		}
	}
	if len(texts) == 0 && l.cursor >= 0 && l.cursor < len(l.entries) {
		texts = append(texts, l.entries[l.cursor].message)
	}
	if len(texts) == 0 {
		return ""
	}
	if err := copyToClipboard(strings.Join(texts, "\n")); err != nil {
		return fmt.Sprintf("sent %s to terminal clipboard (OSC 52)", countNoun(len(texts), "log lines"))
	}
	return fmt.Sprintf("copied %s to clipboard", countNoun(len(texts), "log lines"))
}

func (l *logView) update(msg tea.Msg) tea.Cmd {
	if km, ok := msg.(tea.KeyMsg); ok && l.searching {
		return l.handleSearchKey(km)
	}

	if km, ok := msg.(tea.KeyMsg); ok && l.inspecting {
		switch km.String() {
		case "esc", "q", "enter":
			l.inspecting = false
			return nil
		case "y", "c":
			l.status = l.copySelected()
			return nil
		}
		return nil
	}

	switch msg := msg.(type) {
	case logChunkMsg:
		if msg.gen != l.gen {
			return nil
		}
		if msg.err != nil {
			l.err = msg.err
		}
		if len(msg.lines) > 0 {
			l.ensureEntries()
			atBottom := l.vp.AtBottom() || len(l.lines) == 0
			isBuild := l.kind == "Build" || l.kind == "Build logs" || strings.Contains(l.name, "build")
			for _, line := range msg.lines {
				entry := parseLogLine(line, l.name, isBuild, time.Now(), len(l.entries))
				l.entries = append(l.entries, entry)
			}
			l.lines = append(l.lines, msg.lines...)
			dropped := 0
			if over := len(l.lines) - logMaxLines; over > 0 {
				l.lines = l.lines[over:]
				l.entries = l.entries[over:]
				dropped = over
				marked := make(map[int]bool, len(l.marked))
				for idx, selected := range l.marked {
					if selected && idx >= over {
						marked[idx-over] = true
					}
				}
				l.marked = marked
				if l.cursor < over {
					l.inspecting = false
				}
				l.cursor = max(0, l.cursor-over)
			}
			offset := max(0, l.vp.YOffset-dropped)

			var cur int
			hasCur := l.match >= 0 && l.match < len(l.matches)
			if hasCur {
				cur = l.matches[l.match] - dropped
			}
			l.render()
			l.vp.SetYOffset(offset)
			if hasCur {
				l.match = min(sort.SearchInts(l.matches, cur), max(len(l.matches)-1, 0))
			} else {
				l.match = len(l.matches) - 1
			}
			if atBottom && l.follow {
				l.cursor = max(0, len(l.entries)-1)
				l.vp.GotoBottom()
				l.render()
			}
		}
		if msg.done {
			l.done = true
			return nil
		}
		return l.wait()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Top):
			l.cursor = 0
			l.vp.GotoTop()
			l.render()
			return nil
		case key.Matches(msg, keys.Bottom):
			l.cursor = max(0, len(l.entries)-1)
			l.vp.GotoBottom()
			l.render()
			return nil
		case key.Matches(msg, keys.Up):
			if l.cursor > 0 {
				l.cursor--
				if l.cursor < l.vp.YOffset {
					l.vp.SetYOffset(l.cursor)
				}
				l.render()
			}
			return nil
		case key.Matches(msg, keys.Down):
			if l.cursor < len(l.entries)-1 {
				l.cursor++
				if l.cursor >= l.vp.YOffset+l.vp.Height {
					l.vp.SetYOffset(l.cursor - l.vp.Height + 1)
				}
				l.render()
			}
			return nil
		case key.Matches(msg, keys.PageUp):
			l.cursor = max(0, l.cursor-l.vp.Height)
			l.vp.SetYOffset(max(0, l.vp.YOffset-l.vp.Height))
			l.render()
			return nil
		case key.Matches(msg, keys.PageDown):
			l.cursor = max(0, min(len(l.entries)-1, l.cursor+l.vp.Height))
			l.vp.SetYOffset(min(max(0, len(l.entries)-l.vp.Height), l.vp.YOffset+l.vp.Height))
			l.render()
			return nil
		case msg.String() == " ":
			l.ensureEntries()
			if l.cursor >= 0 && l.cursor < len(l.entries) {
				if l.marked[l.cursor] {
					delete(l.marked, l.cursor)
				} else {
					l.marked[l.cursor] = true
				}
				l.render()
			}
			return nil
		case msg.String() == "ctrl+a":
			l.ensureEntries()
			allMarked := len(l.marked) == len(l.entries)
			l.marked = make(map[int]bool)
			if !allMarked {
				for i := range l.entries {
					l.marked[i] = true
				}
			}
			l.render()
			return nil
		case msg.String() == "y" || msg.String() == "c":
			l.status = l.copySelected()
			return nil
		case msg.String() == "enter":
			l.ensureEntries()
			if l.cursor >= 0 && l.cursor < len(l.entries) {
				l.inspecting = true
			}
			return nil
		case msg.String() == "a":
			l.caseSensitive = !l.caseSensitive
			l.setQuery(l.query)
			return nil
		case msg.String() == "f":
			l.follow = !l.follow
			if l.follow {
				l.cursor = max(0, len(l.entries)-1)
				l.vp.GotoBottom()
				l.render()
			}
			return nil
		case key.Matches(msg, logKeys.Cancel) && l.cancelable && !l.done:
			l.canceled = true
			l.ended = strings.ToLower(l.kind) + " canceled"
			l.close()
			return nil
		case key.Matches(msg, logKeys.Search):
			l.searching = true
			l.search.SetValue(l.query)
			l.search.CursorEnd()
			return l.search.Focus()
		case key.Matches(msg, logKeys.Timestamps):
			return l.toggleTimestamps()
		case key.Matches(msg, logKeys.NextMatch):
			l.nextMatch(1)
			return nil
		case key.Matches(msg, logKeys.PrevMatch):
			l.nextMatch(-1)
			return nil
		}
	}
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	return cmd
}

func (l *logView) toggleTimestamps() tea.Cmd {
	// Custom sources can run operations, including image builds. Reopening
	// them would cancel and repeat the operation instead of changing display.
	for _, source := range l.sources {
		if source.open != nil {
			l.status = "timestamps are only available for container logs"
			return nil
		}
	}
	l.close()
	l.timestamps = !l.timestamps
	logGen++
	l.gen = logGen
	l.lines, l.entries, l.done, l.err = nil, nil, false, nil
	l.marked = make(map[int]bool)
	l.cursor, l.inspecting, l.status = 0, false, ""
	l.render()
	l.match = -1
	return l.start()
}

type logKeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Search     key.Binding
	NextMatch  key.Binding
	PrevMatch  key.Binding
	Mark       key.Binding
	Copy       key.Binding
	Inspect    key.Binding
	Case       key.Binding
	Follow     key.Binding
	Timestamps key.Binding
	Cancel     key.Binding
	Back       key.Binding
	showCancel bool
}

var logKeys = logKeyMap{
	Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	NextMatch:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n/N", "next/prev match")),
	PrevMatch:  key.NewBinding(key.WithKeys("N")),
	Mark:       key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "mark")),
	Copy:       key.NewBinding(key.WithKeys("y", "c"), key.WithHelp("y", "copy")),
	Inspect:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "inspect")),
	Case:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "toggle case")),
	Follow:     key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "toggle follow")),
	Timestamps: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "timestamps")),
	Cancel:     key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "cancel operation")),
	Back:       key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
}

func (k logKeyMap) ShortHelp() []key.Binding {
	b := []key.Binding{k.Up, k.Down, k.Search, k.Mark, k.Copy, k.Inspect, k.Case, k.Follow, k.Back}
	if k.showCancel {
		b = append([]key.Binding{k.Cancel}, b...)
	}
	return b
}

func (k logKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func (l *logView) keyMap() help.KeyMap {
	k := logKeys
	k.showCancel = l.cancelable && !l.done
	return k
}

func (l *logView) searchInfo() string {
	switch {
	case l.query == "":
		return ""
	case len(l.matches) == 0:
		return errorStyle.Render(fmt.Sprintf("no match for %q", l.query))
	default:
		return statusStyle.Render(fmt.Sprintf("match %d/%d for %q", l.match+1, len(l.matches), l.query))
	}
}

func (l *logView) view() string {
	l.ensureEntries()

	// 1. Header line: Logs  Give feedback ↗           ● following · 22 lines
	kind := l.kind
	if kind == "" {
		kind = "Logs"
	}
	title := paneTitleStyle.Render(kind)
	if l.name != "" && l.name != kind {
		title += statusStyle.Render(": ") + lipgloss.NewStyle().Foreground(colorAccent).Render(l.name)
	}
	feedback := linkStyle.Render("Give feedback ↗")
	left := title + "  " + feedback

	var status string
	switch {
	case l.err != nil:
		status = errorStyle.Render(l.err.Error())
	case l.status != "":
		status = okStyle.Render(l.status)
	case l.done:
		if l.ended != "" {
			status = okStyle.Render(l.ended) + statusStyle.Render(" · "+countNoun(len(l.entries), "lines"))
		} else {
			status = statusStyle.Render(countNoun(len(l.entries), "lines") + " · stream ended")
		}
	default:
		running := l.running
		if running == "" {
			running = "● following"
		}
		status = okStyle.Render(running) + statusStyle.Render(" · "+countNoun(len(l.entries), "lines"))
	}
	if n := len(l.marked); n > 0 {
		status += statusStyle.Render(fmt.Sprintf(" · %d marked", n))
	}
	if si := l.searchInfo(); si != "" {
		status = si + statusStyle.Render(" · ") + status
	}

	gap := max(l.vp.Width-lipgloss.Width(left)-lipgloss.Width(status), 1)
	headerLine := left + strings.Repeat(" ", gap) + status

	// 2. Search bar line: [ 🔍 Search logs ...    Aa  ▼  💾  ⇄  📋  📥  ⓘ ]
	aaStyle := statusStyle
	if l.caseSensitive {
		aaStyle = activeTabStyle
	}
	followStyle := statusStyle
	if l.follow {
		followStyle = okStyle
	}

	icons := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s",
		aaStyle.Render("Aa"),
		statusStyle.Render("▼"),
		statusStyle.Render("💾"),
		followStyle.Render("⇄"),
		statusStyle.Render("📋"),
		statusStyle.Render("📥"),
		statusStyle.Render("ⓘ"),
	)

	var searchContent string
	switch {
	case l.searching:
		searchContent = l.search.View()
	case l.query != "":
		searchContent = lipgloss.NewStyle().Foreground(colorAccent).Render("🔍 ") + l.query
	default:
		searchContent = lipgloss.NewStyle().Foreground(colorAccent).Render("🔍 ") +
			statusStyle.Render("Search logs (supports regex like \"/error|warn/\")")
	}

	innerW := max(l.vp.Width-4, 20)
	iconW := lipgloss.Width(icons)
	contentW := lipgloss.Width(searchContent)
	searchBarLine := searchBoxStyle.Width(l.vp.Width - 2).Render(
		searchContent + strings.Repeat(" ", max(innerW-contentW-iconW, 1)) + icons,
	)

	// 3. Column headers line
	colHdr := colHeaderStyle.Render(
		fmt.Sprintf(" [ ]  %-19s  %-16s  %s", "Timestamp", "Object", "Message"),
	)

	// 4. Divider line
	divLine := dividerStyle.Render(strings.Repeat("─", max(l.vp.Width, 10)))

	// 5. Body
	body := l.vp.View()
	if len(l.entries) == 0 {
		body = statusStyle.Render("no output yet")
	}

	// 6. Inspect Modal overlay
	if l.inspecting && l.cursor >= 0 && l.cursor < len(l.entries) {
		entry := l.entries[l.cursor]
		modal := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2).
			Width(max(l.vp.Width-10, 40)).
			Render(
				lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("Log Message Details") + "\n\n" +
					statusStyle.Render("Timestamp: ") + entry.time + "\n" +
					statusStyle.Render("Object:    ") + entry.object + "\n" +
					statusStyle.Render("Message:\n") + entry.message + "\n\n" +
					statusStyle.Render("Press esc / enter to close • y to copy message"),
			)
		return headerLine + "\n" + searchBarLine + "\n" + colHdr + "\n" + divLine + "\n" + modal
	}

	return headerLine + "\n" + searchBarLine + "\n" + colHdr + "\n" + divLine + "\n" + body
}
