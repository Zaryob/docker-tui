// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
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
	v := &terminalView{name: "web"}
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
