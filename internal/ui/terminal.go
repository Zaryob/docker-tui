// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"

	"github.com/zaryob/docker-tui/internal/docker"
)

// terminalScrollback is the number of lines kept above the screen.
const terminalScrollback = 5000

var termGen int

// termSession is a connection to a process in a container whose output is
// shown in a terminal view.
type termSession struct {
	conn io.ReadWriteCloser
	// resize changes the size of the TTY; nil without a TTY.
	resize func(ctx context.Context, w, h int) error
	// ended describes how the session ended once the stream closed.
	ended func(ctx context.Context) (text string, failed bool)
	// input reports whether keys are sent to the process.
	input bool
	// crlf converts line feeds to CR LF, for output not written to a TTY.
	crlf bool
	// note is shown in the header, e.g. when the session is read-only.
	note string
}

// termConnect opens a session for a terminal of w x h cells.
type termConnect func(ctx context.Context, w, h int) (*termSession, error)

// terminalView shows an interactive session with a container inside the
// TUI, like the Exec tab of Docker Desktop: a new shell (exec) or the main
// process of the container (attach). The output is interpreted by a
// terminal emulator and drawn in the pane; keys are sent to the process.
type terminalView struct {
	gen     int
	kind    string
	name    string
	connect termConnect
	sess    *termSession
	width   int
	height  int

	// mu guards the emulator state the emulator does not lock itself, such
	// as the scrollback, against the goroutine writing the output.
	mu  sync.Mutex
	emu *vt.SafeEmulator
	// scroll is how many lines the view is scrolled up into the scrollback;
	// sbLen is the scrollback length when it was last drawn.
	scroll int
	sbLen  int

	// startSize is the width and height the session was opened with.
	startSize [2]int
	// output signals that the emulator received output to draw.
	output chan struct{}
	cancel context.CancelFunc

	connected bool
	ended     bool
	endText   string
	endFailed bool
	err       error
}

type termStartedMsg struct {
	gen  int
	sess *termSession
	err  error
}

type termOutputMsg struct{ gen int }

type termEndedMsg struct {
	gen    int
	text   string
	failed bool
}

// newTerminal opens a terminal view on the selected running container.
func (a *App) newTerminal(kind string, connect func(id string) termConnect) tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	if c := it.data.(docker.Container); c.State != "running" {
		return a.setStatus(fmt.Sprintf("%s is not running", it.name), true)
	}
	termGen++
	v := &terminalView{gen: termGen, kind: kind, name: it.name, connect: connect(it.id), output: make(chan struct{}, 1)}
	w, h := a.width, a.bodyHeight()-1
	v.emu = vt.NewSafeEmulator(max(w, 1), max(h, 1))
	v.emu.SetScrollbackSize(terminalScrollback)
	v.setSize(a.width, a.bodyHeight())
	a.pane = v
	return v.start()
}

// openTerminal opens a new shell in the selected container.
func (a *App) openTerminal() tea.Cmd {
	client := a.client
	return a.newTerminal("Terminal", func(id string) termConnect { return execConnect(client, id) })
}

// attachTerminal attaches to the main process of the selected container.
func (a *App) attachTerminal() tea.Cmd {
	client := a.client
	return a.newTerminal("Attached", func(id string) termConnect { return attachConnect(client, id) })
}

// execConnect starts a shell in a container with a TTY.
func execConnect(client *docker.Client, container string) termConnect {
	return func(ctx context.Context, w, h int) (*termSession, error) {
		size := [2]uint{uint(h), uint(w)}
		id, err := client.CreateExec(ctx, container, docker.ExecConfig{
			Cmd:          []string{"sh", "-c", shellScript},
			Env:          []string{"TERM=xterm-256color"},
			Tty:          true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			ConsoleSize:  &size,
		})
		if err != nil {
			return nil, err
		}
		conn, err := client.StartExec(ctx, id, true)
		if err != nil {
			return nil, err
		}
		return &termSession{
			conn:  conn,
			input: true,
			resize: func(ctx context.Context, w, h int) error {
				return client.ResizeExec(ctx, id, w, h)
			},
			ended: func(ctx context.Context) (string, bool) {
				info, err := client.InspectExec(ctx, id)
				switch {
				case err != nil:
					return "session ended", false
				case info.ExitCode != 0:
					return fmt.Sprintf("session ended with exit code %d", info.ExitCode), true
				}
				return "session ended", false
			},
		}, nil
	}
}

// attachConnect attaches to the main process of a container, like docker
// attach. Leaving detaches without stopping the container.
func attachConnect(client *docker.Client, container string) termConnect {
	return func(ctx context.Context, w, h int) (*termSession, error) {
		info, err := client.ContainerAttachInfo(ctx, container)
		if err != nil {
			return nil, err
		}
		if !info.Running {
			return nil, fmt.Errorf("the container is not running")
		}
		conn, err := client.AttachContainer(ctx, container, info.Tty)
		if err != nil {
			return nil, err
		}
		s := &termSession{
			conn:  conn,
			input: info.OpenStdin,
			crlf:  !info.Tty,
			ended: func(ctx context.Context) (string, bool) {
				info, err := client.ContainerAttachInfo(ctx, container)
				switch {
				case err != nil || info.Running:
					return "detached", false
				case info.ExitCode != 0:
					return fmt.Sprintf("container exited with code %d", info.ExitCode), true
				}
				return "container exited", false
			},
		}
		if !info.OpenStdin {
			s.note = "output only: the container does not read input"
		}
		if info.Tty {
			s.resize = func(ctx context.Context, w, h int) error {
				return client.ResizeContainer(ctx, container, w, h)
			}
			// Setting the size makes most programs redraw their screen,
			// which would otherwise stay empty until they print again.
			_ = s.resize(ctx, w, h)
		}
		return s, nil
	}
}

// start opens the session in the background.
func (v *terminalView) start() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	connect, gen := v.connect, v.gen
	w, h := v.emu.Width(), v.emu.Height()
	v.startSize = [2]int{w, h}
	return func() tea.Msg {
		sctx, scancel := context.WithTimeout(ctx, 15*time.Second)
		defer scancel()
		sess, err := connect(sctx, w, h)
		return termStartedMsg{gen: gen, sess: sess, err: err}
	}
}

// pump copies the output of the session into the emulator, and the replies
// of the emulator, such as cursor position reports, back to the session. It
// returns when the session ends.
func (v *terminalView) pump() tea.Cmd {
	sess, emu, output, gen := v.sess, v.emu, v.output, v.gen
	go func() {
		if sess.input {
			_, _ = io.Copy(sess.conn, emu)
		} else {
			_, _ = io.Copy(io.Discard, emu)
		}
	}()
	return func() tea.Msg {
		buf := make([]byte, 32*1024)
		for {
			n, err := sess.conn.Read(buf)
			if n > 0 {
				out := buf[:n]
				if sess.crlf {
					out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
				}
				v.mu.Lock()
				_, _ = emu.Write(out)
				v.mu.Unlock()
				select {
				case output <- struct{}{}:
				default:
				}
			}
			if err != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				text, failed := sess.ended(ctx)
				return termEndedMsg{gen: gen, text: text, failed: failed}
			}
		}
	}
}

// waitOutput delivers a message once new output has to be drawn.
func (v *terminalView) waitOutput() tea.Cmd {
	output, gen := v.output, v.gen
	return func() tea.Msg {
		if _, ok := <-output; !ok {
			return nil
		}
		return termOutputMsg{gen: gen}
	}
}

func (v *terminalView) setSize(width, height int) {
	v.width, v.height = width, height
	w, h := max(width, 1), max(height-1, 1)
	if v.emu == nil || (v.emu.Width() == w && v.emu.Height() == h) {
		return
	}
	v.mu.Lock()
	v.emu.Resize(w, h)
	v.mu.Unlock()
	if v.sess != nil && v.sess.resize != nil && !v.ended {
		resize := v.sess.resize
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = resize(ctx, w, h)
		}()
	}
}

func (v *terminalView) close() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.sess != nil {
		v.sess.conn.Close()
	}
	// End the copy of the emulator replies. Closing the input pipe rather
	// than the emulator keeps clear of the emulator's unsynchronized state.
	if c, ok := v.emu.InputPipe().(io.Closer); ok {
		_ = c.Close()
	}
}

// capturesInput sends every key to the session while it runs; afterwards
// the usual keys close the view.
func (v *terminalView) capturesInput() bool { return !v.ended && v.err == nil }

// rawInput also forwards ctrl+c, which must interrupt the command in the
// container rather than quit docker-tui.
func (v *terminalView) rawInput() bool { return v.capturesInput() }

func (v *terminalView) keyMap() help.KeyMap { return terminalKeys{v} }

func (v *terminalView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case termStartedMsg:
		if msg.gen != v.gen {
			if msg.sess != nil {
				msg.sess.conn.Close()
			}
			return nil
		}
		if msg.err != nil {
			v.err = msg.err
			return nil
		}
		v.sess, v.connected = msg.sess, true
		// Apply the current size if it changed while connecting. Resizing
		// needlessly makes some shells redraw and drop early input.
		w, h := v.emu.Width(), v.emu.Height()
		if v.startSize == [2]int{w, h} || v.sess.resize == nil {
			return tea.Batch(v.pump(), v.waitOutput())
		}
		resize := v.sess.resize
		return tea.Batch(v.pump(), v.waitOutput(), func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = resize(ctx, w, h)
			return nil
		})
	case termOutputMsg:
		if msg.gen != v.gen || v.ended {
			return nil
		}
		return v.waitOutput()
	case termEndedMsg:
		if msg.gen != v.gen {
			return nil
		}
		v.ended, v.endText, v.endFailed = true, msg.text, msg.failed
		return nil
	case tea.KeyMsg:
		if v.scrollKey(msg) {
			return nil
		}
		if !v.capturesInput() {
			return nil
		}
		if key.Matches(msg, terminalLeaveKey) {
			return func() tea.Msg { return closePaneMsg{} }
		}
		// Typing returns to the bottom, like in a terminal emulator; esc
		// only returns.
		if v.scroll > 0 {
			v.scroll = 0
			if msg.Type == tea.KeyEsc {
				return nil
			}
		}
		v.sendKey(msg)
	}
	return nil
}

// scrollKey scrolls through the output for the scroll keys and reports
// whether msg was one. Once the session ended, the usual movement keys
// scroll too.
func (v *terminalView) scrollKey(msg tea.KeyMsg) bool {
	page := max(v.emu.Height()-1, 1)
	ended := !v.capturesInput()
	switch {
	case key.Matches(msg, terminalScrollUp), ended && key.Matches(msg, keys.Up):
		v.scrollBy(1)
	case key.Matches(msg, terminalScrollDown), ended && key.Matches(msg, keys.Down):
		v.scrollBy(-1)
	case key.Matches(msg, terminalPageUp), ended && key.Matches(msg, keys.PageUp):
		v.scrollBy(page)
	case key.Matches(msg, terminalPageDown), ended && key.Matches(msg, keys.PageDown):
		v.scrollBy(-page)
	case ended && key.Matches(msg, keys.Top):
		v.scrollBy(terminalScrollback)
	case ended && key.Matches(msg, keys.Bottom):
		v.scroll = 0
	default:
		return false
	}
	return true
}

// scrollBy moves the view up (delta > 0) or down into the scrollback.
// Full-screen programs such as editors use the alternate screen, which has
// no scrollback.
func (v *terminalView) scrollBy(delta int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.emu.IsAltScreen() {
		v.scroll = 0
		return
	}
	n := v.emu.Scrollback().Len()
	v.sbLen = n
	v.scroll = min(max(v.scroll+delta, 0), n)
}

// sendKey encodes a key press for the session. The emulator knows the modes
// the program has set, e.g. application cursor keys.
func (v *terminalView) sendKey(msg tea.KeyMsg) {
	if !v.connected || !v.sess.input {
		return
	}
	if msg.Paste {
		v.emu.Paste(string(msg.Runes))
		return
	}
	alt := ""
	if msg.Alt {
		alt = "\x1b"
	}
	switch msg.Type {
	case tea.KeyRunes:
		v.emu.SendText(alt + string(msg.Runes))
		return
	case tea.KeySpace:
		v.emu.SendText(alt + " ")
		return
	}
	if k, ok := termKeys[msg.Type]; ok {
		if msg.Alt {
			k.Mod |= uv.ModAlt
		}
		v.emu.SendKey(k)
		return
	}
	// Remaining keys are control characters (ctrl+a is 1, ...), which the
	// program receives as they are.
	if msg.Type >= 0 && msg.Type < 32 || msg.Type == tea.KeyBackspace {
		v.emu.SendText(alt + string(rune(msg.Type)))
	}
}

// termKeys maps the special keys of Bubble Tea to the key events of the
// emulator. shift+up and shift+down scroll the view instead.
var termKeys = map[tea.KeyType]uv.KeyPressEvent{
	tea.KeyEnter:     {Code: uv.KeyEnter},
	tea.KeyTab:       {Code: uv.KeyTab},
	tea.KeyShiftTab:  {Code: uv.KeyTab, Mod: uv.ModShift},
	tea.KeyBackspace: {Code: uv.KeyBackspace},
	tea.KeyEsc:       {Code: uv.KeyEscape},
	tea.KeyUp:        {Code: uv.KeyUp},
	tea.KeyDown:      {Code: uv.KeyDown},
	tea.KeyRight:     {Code: uv.KeyRight},
	tea.KeyLeft:      {Code: uv.KeyLeft},
	tea.KeyHome:      {Code: uv.KeyHome},
	tea.KeyEnd:       {Code: uv.KeyEnd},
	tea.KeyPgUp:      {Code: uv.KeyPgUp},
	tea.KeyPgDown:    {Code: uv.KeyPgDown},
	tea.KeyDelete:    {Code: uv.KeyDelete},
	tea.KeyInsert:    {Code: uv.KeyInsert},
	tea.KeyCtrlUp:    {Code: uv.KeyUp, Mod: uv.ModCtrl},
	tea.KeyCtrlDown:  {Code: uv.KeyDown, Mod: uv.ModCtrl},
	tea.KeyCtrlRight: {Code: uv.KeyRight, Mod: uv.ModCtrl},
	tea.KeyCtrlLeft:  {Code: uv.KeyLeft, Mod: uv.ModCtrl},
	tea.KeyF1:        {Code: uv.KeyF1},
	tea.KeyF2:        {Code: uv.KeyF2},
	tea.KeyF3:        {Code: uv.KeyF3},
	tea.KeyF4:        {Code: uv.KeyF4},
	tea.KeyF5:        {Code: uv.KeyF5},
	tea.KeyF6:        {Code: uv.KeyF6},
	tea.KeyF7:        {Code: uv.KeyF7},
	tea.KeyF8:        {Code: uv.KeyF8},
	tea.KeyF9:        {Code: uv.KeyF9},
	tea.KeyF10:       {Code: uv.KeyF10},
	tea.KeyF11:       {Code: uv.KeyF11},
	tea.KeyF12:       {Code: uv.KeyF12},
}

var (
	// terminalLeaveKey closes the terminal, like the escape key of telnet;
	// esc itself belongs to the program, e.g. vi.
	terminalLeaveKey   = key.NewBinding(key.WithKeys("ctrl+]"), key.WithHelp("ctrl+]", "leave"))
	terminalScrollUp   = key.NewBinding(key.WithKeys("shift+up"), key.WithHelp("shift+↑/↓", "scroll"))
	terminalScrollDown = key.NewBinding(key.WithKeys("shift+down"))
	terminalPageUp     = key.NewBinding(key.WithKeys("ctrl+pgup"), key.WithHelp("ctrl+pgup/pgdn", "page"))
	terminalPageDown   = key.NewBinding(key.WithKeys("ctrl+pgdown"))
)

type terminalKeys struct{ v *terminalView }

func (k terminalKeys) ShortHelp() []key.Binding {
	if k.v.capturesInput() {
		return []key.Binding{terminalLeaveKey, terminalScrollUp, terminalPageUp}
	}
	return []key.Binding{keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Back}
}

func (k terminalKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var cursorStyle = "\x1b[7m"

// screen renders the emulator screen, or the part of the output scrolled
// to, exactly width x height cells.
func (v *terminalView) screen() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	w, h := v.emu.Width(), v.emu.Height()
	lines := strings.Split(strings.ReplaceAll(v.emu.Render(), "\r\n", "\n"), "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}

	sb := v.emu.Scrollback()
	n := sb.Len()
	if v.emu.IsAltScreen() {
		v.scroll = 0
	}
	if v.scroll > 0 {
		// Keep the lines in view while new output pushes lines into the
		// scrollback; once it is full, the oldest lines drop out.
		if n > v.sbLen {
			v.scroll += n - v.sbLen
		}
		v.scroll = min(v.scroll, n)
	}
	v.sbLen = n

	out := make([]string, h)
	for y := range out {
		var line string
		if idx := n - v.scroll + y; idx < n {
			line = sb.Line(idx).Render()
		} else {
			line = lines[idx-n]
		}
		line = ansi.Truncate(line, w, "")
		if pad := w - ansi.StringWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out[y] = line + ansi.ResetStyle
	}
	if v.scroll == 0 && v.connected && !v.ended && !v.emu.CursorHidden() {
		pos := v.emu.CursorPosition()
		if pos.Y >= 0 && pos.Y < h && pos.X >= 0 && pos.X < w {
			line := out[pos.Y]
			cell := ansi.Strip(ansi.Cut(line, pos.X, pos.X+1))
			if cell == "" {
				cell = " "
			}
			out[pos.Y] = ansi.Cut(line, 0, pos.X) + cursorStyle + cell + ansi.ResetStyle + ansi.Cut(line, pos.X+1, w)
		}
	}
	return strings.Join(out, "\n")
}

func (v *terminalView) view() string {
	title := paneTitleStyle.Render(v.kind + ": " + v.name)
	var info string
	switch {
	case v.err != nil:
		return title + "  " + errorStyle.Render(v.err.Error())
	case v.ended && v.endFailed:
		info = errorStyle.Render(v.endText) + statusStyle.Render(" · esc to close")
	case v.ended:
		info = statusStyle.Render(v.endText + " · esc to close")
	case !v.connected:
		info = statusStyle.Render("connecting…")
	default:
		info = okStyle.Render("● connected") + statusStyle.Render(" · ctrl+] to leave")
		if v.sess.note != "" {
			info += statusStyle.Render(" · " + v.sess.note)
		}
	}
	screen := v.screen()
	if v.scroll > 0 {
		info += warnStyle.Render(" · ↑ " + countNoun(v.scroll, "lines") + " up")
	}
	return title + "  " + info + "\n" + screen
}
