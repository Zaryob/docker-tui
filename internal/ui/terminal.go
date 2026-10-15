// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"fmt"
	"io"
	"strings"
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
const terminalScrollback = 1000

var termGen int

// terminalView runs an interactive shell in a container inside the TUI,
// like the Exec tab of Docker Desktop. The output is interpreted by a
// terminal emulator and drawn in the pane; keys are sent to the shell.
type terminalView struct {
	gen    int
	name   string
	client *docker.Client
	emu    *vt.SafeEmulator
	width  int
	height int

	execID string
	conn   io.ReadWriteCloser
	// startSize is the width and height the session was created with.
	startSize [2]int
	// output signals that the emulator received output to draw.
	output chan struct{}
	cancel context.CancelFunc

	connected bool
	ended     bool
	exitCode  int
	err       error
}

type termStartedMsg struct {
	gen    int
	execID string
	conn   io.ReadWriteCloser
	err    error
}

type termOutputMsg struct{ gen int }

type termEndedMsg struct {
	gen      int
	exitCode int
	err      error
}

// openTerminal opens a terminal in the selected container.
func (a *App) openTerminal() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	if c := it.data.(docker.Container); c.State != "running" {
		return a.setStatus(fmt.Sprintf("%s is not running", it.name), true)
	}
	termGen++
	v := &terminalView{gen: termGen, name: it.name, client: a.client, output: make(chan struct{}, 1)}
	w, h := a.width, a.bodyHeight()-1
	v.emu = vt.NewSafeEmulator(max(w, 1), max(h, 1))
	v.emu.SetScrollbackSize(terminalScrollback)
	v.setSize(a.width, a.bodyHeight())
	a.pane = v
	return v.start(it.id)
}

// start creates the exec instance and attaches to it.
func (v *terminalView) start(container string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	client, gen := v.client, v.gen
	v.startSize = [2]int{v.emu.Width(), v.emu.Height()}
	size := [2]uint{uint(v.emu.Height()), uint(v.emu.Width())}
	return func() tea.Msg {
		sctx, scancel := context.WithTimeout(ctx, 15*time.Second)
		defer scancel()
		id, err := client.CreateExec(sctx, container, docker.ExecConfig{
			Cmd:          []string{"sh", "-c", shellScript},
			Env:          []string{"TERM=xterm-256color"},
			Tty:          true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			ConsoleSize:  &size,
		})
		if err != nil {
			return termStartedMsg{gen: gen, err: err}
		}
		conn, err := client.StartExec(sctx, id, true)
		return termStartedMsg{gen: gen, execID: id, conn: conn, err: err}
	}
}

// pump copies the shell output into the emulator and the emulator replies,
// such as cursor position reports, back to the shell. It returns when the
// shell exits.
func (v *terminalView) pump() tea.Cmd {
	conn, emu, output, gen := v.conn, v.emu, v.output, v.gen
	go func() {
		_, _ = io.Copy(conn, emu)
	}()
	return func() tea.Msg {
		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				_, _ = emu.Write(buf[:n])
				select {
				case output <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return termEndedMsg{gen: gen}
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
	v.emu.Resize(w, h)
	if v.execID != "" && !v.ended {
		client, id := v.client, v.execID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = client.ResizeExec(ctx, id, w, h)
		}()
	}
}

func (v *terminalView) close() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.conn != nil {
		v.conn.Close()
	}
	// End the copy of the emulator replies. Closing the input pipe rather
	// than the emulator keeps clear of the emulator's unsynchronized state.
	if c, ok := v.emu.InputPipe().(io.Closer); ok {
		_ = c.Close()
	}
}

// capturesInput sends every key to the shell while it runs; afterwards the
// usual keys close the view.
func (v *terminalView) capturesInput() bool { return !v.ended && v.err == nil }

// rawInput also forwards ctrl+c, which must interrupt the shell's command
// rather than quit docker-tui.
func (v *terminalView) rawInput() bool { return v.capturesInput() }

func (v *terminalView) keyMap() help.KeyMap { return terminalKeys{v} }

func (v *terminalView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case termStartedMsg:
		if msg.gen != v.gen {
			if msg.conn != nil {
				msg.conn.Close()
			}
			return nil
		}
		if msg.err != nil {
			v.err = msg.err
			return nil
		}
		v.execID, v.conn, v.connected = msg.execID, msg.conn, true
		// Apply the current size if it changed while connecting. Resizing
		// needlessly makes some shells redraw and drop early input.
		w, h := v.emu.Width(), v.emu.Height()
		if v.startSize == [2]int{w, h} {
			return tea.Batch(v.pump(), v.waitOutput())
		}
		client, id := v.client, v.execID
		resize := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = client.ResizeExec(ctx, id, w, h)
			return nil
		}
		return tea.Batch(v.pump(), v.waitOutput(), resize)
	case termOutputMsg:
		if msg.gen != v.gen || v.ended {
			return nil
		}
		return v.waitOutput()
	case termEndedMsg:
		if msg.gen != v.gen {
			return nil
		}
		if v.execID != "" && msg.exitCode == 0 && msg.err == nil {
			// Look up the exit code of the shell.
			client, id, gen := v.client, v.execID, v.gen
			v.execID = ""
			return func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				info, err := client.InspectExec(ctx, id)
				if err != nil {
					return termEndedMsg{gen: gen, exitCode: -1}
				}
				return termEndedMsg{gen: gen, exitCode: info.ExitCode}
			}
		}
		v.ended, v.exitCode = true, msg.exitCode
		return nil
	case tea.KeyMsg:
		if !v.capturesInput() {
			return nil
		}
		if key.Matches(msg, terminalLeaveKey) {
			return func() tea.Msg { return closePaneMsg{} }
		}
		v.sendKey(msg)
	}
	return nil
}

// sendKey encodes a key press for the shell. The emulator knows the modes
// the shell has set, e.g. application cursor keys.
func (v *terminalView) sendKey(msg tea.KeyMsg) {
	if !v.connected {
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
	// shell receives as they are.
	if msg.Type >= 0 && msg.Type < 32 || msg.Type == tea.KeyBackspace {
		v.emu.SendText(alt + string(rune(msg.Type)))
	}
}

// termKeys maps the special keys of Bubble Tea to the key events of the
// emulator.
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
	tea.KeyShiftUp:   {Code: uv.KeyUp, Mod: uv.ModShift},
	tea.KeyShiftDown: {Code: uv.KeyDown, Mod: uv.ModShift},
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

// terminalLeaveKey closes the terminal, like the escape key of telnet; esc
// itself belongs to the shell and editors such as vi.
var terminalLeaveKey = key.NewBinding(key.WithKeys("ctrl+]"), key.WithHelp("ctrl+]", "leave terminal"))

type terminalKeys struct{ v *terminalView }

func (k terminalKeys) ShortHelp() []key.Binding {
	if k.v.capturesInput() {
		return []key.Binding{terminalLeaveKey}
	}
	return []key.Binding{keys.Back}
}

func (k terminalKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var cursorStyle = "\x1b[7m"

// screen renders the emulator screen with the cursor, exactly width x
// height cells.
func (v *terminalView) screen() string {
	w, h := v.emu.Width(), v.emu.Height()
	lines := strings.Split(strings.ReplaceAll(v.emu.Render(), "\r\n", "\n"), "\n")
	out := make([]string, h)
	for y := range out {
		line := ""
		if y < len(lines) {
			line = lines[y]
		}
		line = ansi.Truncate(line, w, "")
		if pad := w - ansi.StringWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out[y] = line + ansi.ResetStyle
	}
	if v.connected && !v.ended && !v.emu.CursorHidden() {
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
	title := paneTitleStyle.Render("Terminal: " + v.name)
	var info string
	switch {
	case v.err != nil:
		return title + "  " + errorStyle.Render(v.err.Error())
	case v.ended && v.exitCode == 0:
		info = statusStyle.Render("session ended · esc to close")
	case v.ended:
		info = errorStyle.Render(fmt.Sprintf("session ended with exit code %d", v.exitCode)) + statusStyle.Render(" · esc to close")
	case !v.connected:
		info = statusStyle.Render("connecting…")
	default:
		info = okStyle.Render("● connected") + statusStyle.Render(" · ctrl+] to leave")
	}
	return title + "  " + info + "\n" + v.screen()
}
