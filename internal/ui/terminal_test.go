// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// fakeShell serves an exec session that prints a prompt, reports the input
// it receives on received and ends when it reads "exit".
func fakeShell(t *testing.T, received chan<- string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/exec") && strings.HasPrefix(r.URL.Path, "/containers/"):
			var cfg map[string]any
			_ = json.NewDecoder(r.Body).Decode(&cfg)
			if cfg["Tty"] != true {
				t.Errorf("exec config = %v", cfg)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "e1"})
		case r.URL.Path == "/exec/e1/start":
			_, _ = io.ReadAll(r.Body)
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			buf.WriteString("HTTP/1.1 101 UPGRADED\r\nUpgrade: tcp\r\n\r\nwelcome\r\n$ ")
			buf.Flush()
			p := make([]byte, 64)
			var all string
			for {
				n, err := buf.Read(p)
				if err != nil {
					return
				}
				received <- string(p[:n])
				// Like a shell, end the session on "exit".
				if all += string(p[:n]); strings.HasSuffix(all, "exit\r") {
					return
				}
			}
		case r.URL.Path == "/exec/e1/resize":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/exec/e1/json":
			_ = json.NewEncoder(w).Encode(map[string]any{"ExitCode": 0})
		default:
			http.NotFound(w, r)
		}
	})
}

// cmdRunner runs commands in the background like Bubble Tea does and feeds
// their messages back into the app.
type cmdRunner struct {
	a    *App
	msgs chan tea.Msg
}

func newCmdRunner(a *App) *cmdRunner { return &cmdRunner{a: a, msgs: make(chan tea.Msg, 64)} }

func (r *cmdRunner) start(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() { r.msgs <- cmd() }()
}

// run starts cmd and processes messages for d, or until done reports true.
func (r *cmdRunner) run(cmd tea.Cmd, d time.Duration, done func() bool) {
	r.start(cmd)
	deadline := time.After(d)
	for done == nil || !done() {
		select {
		case msg := <-r.msgs:
			switch msg := msg.(type) {
			case nil:
			case tea.BatchMsg:
				for _, c := range msg {
					r.start(c)
				}
			default:
				_, next := r.a.Update(msg)
				r.start(next)
			}
		case <-deadline:
			return
		}
	}
}

func TestTerminal(t *testing.T) {
	received := make(chan string, 16)
	a := newTestApp(t, fakeShell(t, received))
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	selectID(t, t1, "a1")
	r := newCmdRunner(a)
	r.run(a.handleKey(keyRunes("e")), 2*time.Second, func() bool {
		v, ok := a.pane.(*terminalView)
		return ok && strings.Contains(v.view(), "welcome")
	})
	v, ok := a.pane.(*terminalView)
	if !ok {
		t.Fatalf("pane = %T", a.pane)
	}
	screen := ansi.Strip(v.view())
	if !strings.Contains(screen, "welcome") || !strings.Contains(screen, "● connected") {
		t.Fatalf("screen:\n%s", screen)
	}

	// Keys, ctrl+c included, go to the shell instead of docker-tui.
	for _, k := range []tea.KeyMsg{
		keyRunes("ls"),
		{Type: tea.KeyEnter},
		{Type: tea.KeyCtrlC},
		{Type: tea.KeyUp},
		{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true},
	} {
		if cmd := a.handleKey(k); cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatalf("%v quit docker-tui", k)
			}
		}
	}
	var got []string
	timeout := time.After(2 * time.Second)
	for strings.Join(got, "") != "ls\r\x03\x1b[A\x1bx" {
		select {
		case s := <-received:
			got = append(got, s)
		case <-timeout:
			t.Fatalf("shell received %q", strings.Join(got, ""))
		}
	}

	// ctrl+p followed by another key reaches the shell as it is.
	a.handleKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	a.handleKey(keyRunes("z"))
	got = nil
	timeout = time.After(2 * time.Second)
	for strings.Join(got, "") != "\x10z" {
		select {
		case s := <-received:
			got = append(got, s)
		case <-timeout:
			t.Fatalf("after ctrl+p z the shell received %q", strings.Join(got, ""))
		}
	}

	// ctrl+p ctrl+q leaves, like in docker.
	a.handleKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	if cmd := a.handleKey(tea.KeyMsg{Type: tea.KeyCtrlQ}); cmd == nil {
		t.Fatal("ctrl+p ctrl+q did not leave")
	} else if _, ok := cmd().(closePaneMsg); !ok {
		t.Fatal("ctrl+p ctrl+q did not close the terminal")
	}

	// ctrl+] leaves the terminal.
	cmd := a.handleKey(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	a.Update(cmd())
	if a.pane != nil {
		t.Errorf("pane = %T after ctrl+]", a.pane)
	}
}

func TestTerminalSessionEnds(t *testing.T) {
	received := make(chan string, 16)
	a := newTestApp(t, fakeShell(t, received))
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	selectID(t, t1, "a1")
	r := newCmdRunner(a)
	r.run(a.handleKey(keyRunes("e")), 2*time.Second, func() bool {
		v, ok := a.pane.(*terminalView)
		return ok && v.connected
	})
	v := a.pane.(*terminalView)
	a.handleKey(keyRunes("exit"))
	a.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	r.run(nil, 3*time.Second, func() bool { return v.ended })
	if !v.ended || !strings.Contains(ansi.Strip(v.view()), "session ended") {
		t.Fatalf("ended = %v, header %q", v.ended, strings.SplitN(ansi.Strip(v.view()), "\n", 2)[0])
	}
	// Once the shell exited, esc closes the view again.
	a.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if a.pane != nil {
		t.Error("esc did not close the ended terminal")
	}
}

func TestTerminalScreenSize(t *testing.T) {
	v := &terminalView{name: "web", kind: "Terminal"}
	v.emu = newTestEmulator(20, 3)
	v.connected = true
	_, _ = v.emu.Write([]byte("a\x1b[31mred\x1b[0m"))
	lines := strings.Split(v.screen(), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("line %d width %d", i, w)
		}
	}
	if !strings.Contains(lines[0], cursorStyle+" ") {
		t.Errorf("cursor not drawn after the text: %q", lines[0])
	}
	if ansi.Strip(lines[0])[:4] != "ared" {
		t.Errorf("line 0 = %q", ansi.Strip(lines[0]))
	}
}

func TestTerminalScrollback(t *testing.T) {
	v := &terminalView{name: "web", kind: "Terminal", connected: true, sess: &termSession{}}
	v.emu = newTestEmulator(20, 4)
	v.emu.SetScrollbackSize(100)
	for i := range 10 {
		_, _ = fmt.Fprintf(v.emu, "line %d\r\n", i)
	}
	top := func() string { return strings.TrimSpace(ansi.Strip(strings.SplitN(v.screen(), "\n", 2)[0])) }
	if got := top(); got != "line 7" {
		t.Fatalf("top line = %q, want line 7", got)
	}
	v.update(tea.KeyMsg{Type: tea.KeyShiftUp})
	if got := top(); got != "line 6" {
		t.Errorf("after shift+up: %q, want line 6", got)
	}
	if !strings.Contains(ansi.Strip(v.view()), "↑ 1 line up") {
		t.Errorf("header = %q", strings.SplitN(ansi.Strip(v.view()), "\n", 2)[0])
	}
	// New output does not move what is in view.
	_, _ = v.emu.Write([]byte("line 10\r\nline 11\r\n"))
	if got := top(); got != "line 6" {
		t.Errorf("after new output: %q, want line 6", got)
	}
	v.update(tea.KeyMsg{Type: tea.KeyCtrlPgUp})
	if got := top(); got != "line 3" {
		t.Errorf("after ctrl+pgup: %q, want line 3", got)
	}
	v.scrollBy(1000)
	if got := top(); got != "line 0" {
		t.Errorf("scrolled to the top: %q, want line 0", got)
	}
	// Typing returns to the bottom.
	v.update(keyRunes("a"))
	if v.scroll != 0 || top() != "line 9" {
		t.Errorf("after typing: scroll %d, top %q", v.scroll, top())
	}
}

// muxFrame encodes p as a frame of the multiplexed stream of a container
// without TTY.
func muxFrame(stream byte, p string) []byte {
	n := len(p)
	return append([]byte{stream, 0, 0, 0, byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}, p...)
}

func TestAttachTerminal(t *testing.T) {
	var inspections atomic.Int32
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/a1/json":
			running := inspections.Add(1) == 1
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Config": map[string]any{"Tty": false, "OpenStdin": true},
				"State":  map[string]any{"Running": running, "ExitCode": 2},
			})
		case "/containers/a1/attach":
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			buf.WriteString("HTTP/1.1 101 UPGRADED\r\nUpgrade: tcp\r\n\r\n")
			buf.Write(muxFrame(1, "ready\n"))
			buf.Flush()
			line, _ := buf.ReadString('\r')
			buf.Write(muxFrame(2, "got "+strings.TrimSpace(line)+"\n"))
			buf.Flush()
		default:
			http.NotFound(w, r)
		}
	}))
	t1 := a.current()
	t1.setItems(containerItems(containersFixture()))
	selectID(t, t1, "a1")
	r := newCmdRunner(a)
	r.run(a.handleKey(keyRunes("A")), 2*time.Second, func() bool {
		v, ok := a.pane.(*terminalView)
		return ok && v.connected && strings.Contains(ansi.Strip(v.screen()), "ready")
	})
	v, ok := a.pane.(*terminalView)
	if !ok || !v.connected {
		t.Fatalf("pane = %T, err = %v", a.pane, v.err)
	}
	a.handleKey(keyRunes("hi"))
	a.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	r.run(nil, 3*time.Second, func() bool { return v.ended })
	lines := strings.Split(ansi.Strip(v.screen()), "\n")
	if strings.TrimSpace(lines[0]) != "ready" || strings.TrimSpace(lines[1]) != "got hi" {
		t.Errorf("screen lines = %q", lines[:3])
	}
	if header := strings.SplitN(ansi.Strip(v.view()), "\n", 2)[0]; !strings.Contains(header, "Attached: web") || !strings.Contains(header, "container exited with code 2") {
		t.Errorf("header = %q", header)
	}
}
