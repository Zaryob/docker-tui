// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

var pushGen int

// pushView asks for an image reference and shows the progress of the push, layer by layer.
type pushView struct {
	gen    int
	client *docker.Client
	input  textinput.Model
	width  int
	height int

	ref      string
	layers   []string
	progress map[string]*layerProgress
	messages []string

	ch       chan pullEvent
	cancel   context.CancelFunc
	done     bool
	canceled bool
	err      error
}

type pushMsg struct {
	gen    int
	events []docker.PullProgress
	done   bool
	err    error
}

// showPush opens the running push, or asks for an image to push.
func (a *App) showPush() tea.Cmd {
	if a.push != nil {
		a.pane = a.push
		a.layout()
		return nil
	}
	defaultRef := ""
	if it, ok := a.selectedObject(); ok {
		if img, ok := it.data.(docker.Image); ok && img.Tag() != "<none>:<none>" {
			defaultRef = img.Tag()
		} else if it.name != "" && it.name != "<none>:<none>" {
			defaultRef = it.name
		}
	}
	in := textinput.New()
	in.Prompt = "Image  "
	in.PromptStyle = statsLabelStyle
	in.Placeholder = "e.g. repo/image:tag or myregistry:5000/app:1.0"
	if defaultRef != "" {
		in.SetValue(defaultRef)
	}
	pushGen++
	v := &pushView{gen: pushGen, client: a.client, input: in, progress: map[string]*layerProgress{}}
	v.setSize(a.width, a.bodyHeight())
	a.pane = v
	a.layout()
	return v.input.Focus()
}

func (v *pushView) startPush(ref string) tea.Cmd {
	v.ref = ref
	v.input.Blur()
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	ch := make(chan pullEvent, 64)
	v.ch = ch
	client := v.client
	go func() {
		defer close(ch)
		s, err := client.PushImage(ctx, ref)
		if err != nil {
			ch <- pullEvent{err: err}
			return
		}
		defer s.Close()
		for {
			p, err := s.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
					ch <- pullEvent{err: err}
				}
				return
			}
			ch <- pullEvent{p: p}
		}
	}()
	return v.wait()
}

func (v *pushView) wait() tea.Cmd {
	ch, gen := v.ch, v.gen
	return func() tea.Msg {
		var events []docker.PullProgress
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return pushMsg{gen: gen, events: events, done: true}
				}
				if ev.err != nil {
					return pushMsg{gen: gen, events: events, err: ev.err}
				}
				events = append(events, ev.p)
				if len(events) >= 16 {
					return pushMsg{gen: gen, events: events}
				}
			default:
				if len(events) > 0 {
					return pushMsg{gen: gen, events: events}
				}
				ev, ok := <-ch
				if !ok {
					return pushMsg{gen: gen, done: true}
				}
				if ev.err != nil {
					return pushMsg{gen: gen, err: ev.err}
				}
				return pushMsg{gen: gen, events: []docker.PullProgress{ev.p}}
			}
		}
	}
}

func (v *pushView) running() bool {
	return v.ref != "" && !v.done && !v.canceled && v.err == nil
}

func (v *pushView) succeeded() bool {
	return v.done && v.err == nil && !v.canceled
}

func (v *pushView) stop() {
	if v.cancel != nil {
		v.cancel()
	}
}

func (v *pushView) capturesInput() bool { return v.ref == "" }

func (v *pushView) setSize(w, h int) {
	v.width, v.height = w, h
	v.input.Width = min(60, max(20, w-14))
}

func (v *pushView) close() {}

func (v *pushView) layerDone(id string) bool {
	l := v.progress[id]
	if l == nil {
		return false
	}
	switch l.status {
	case "Layer already exists", "Pushed", "Push complete":
		return true
	}
	return l.total > 0 && l.current >= l.total
}

func (v *pushView) layersDone() int {
	n := 0
	for _, id := range v.layers {
		if v.layerDone(id) {
			n++
		}
	}
	return n
}

func (v *pushView) apply(events []docker.PullProgress) {
	for _, p := range events {
		if p.ID == "" {
			if p.Status != "" {
				v.messages = append(v.messages, p.Status)
			}
			continue
		}
		l, ok := v.progress[p.ID]
		if !ok {
			l = &layerProgress{}
			v.progress[p.ID] = l
			v.layers = append(v.layers, p.ID)
		}
		if p.Status != "" {
			l.status = p.Status
		}
		if p.Detail.Total > 0 {
			l.total = p.Detail.Total
		}
		if p.Detail.Current > 0 {
			l.current = p.Detail.Current
		}
	}
}

func (v *pushView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case pushMsg:
		if msg.gen != v.gen {
			return nil
		}
		v.apply(msg.events)
		if msg.err != nil {
			v.err = msg.err
		}
		if msg.done {
			v.done = true
			return nil
		}
		if v.err != nil {
			return nil
		}
		return v.wait()
	case tea.KeyMsg:
		if v.ref == "" {
			switch msg.String() {
			case "esc":
				return func() tea.Msg { return closePaneMsg{} }
			case "enter":
				ref := strings.TrimSpace(v.input.Value())
				if ref == "" {
					return nil
				}
				return v.startPush(ref)
			}
			var cmd tea.Cmd
			v.input, cmd = v.input.Update(msg)
			return cmd
		}
		if key.Matches(msg, pushCancelKey) && v.running() {
			v.canceled = true
			v.stop()
		}
		return nil
	}
	if v.ref == "" {
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(msg)
		return cmd
	}
	return nil
}

func (v *pushView) view() string {
	if v.ref == "" {
		return strings.Join([]string{
			paneTitleStyle.Render("Push an image"), "",
			v.input.View(), "",
			statusStyle.Render("Pushes to Docker Hub or the registry named in the reference; esc on the progress keeps it running."),
		}, "\n")
	}
	var info string
	switch {
	case v.canceled:
		info = statusStyle.Render("push canceled")
	case v.err != nil:
		info = errorStyle.Render(v.err.Error())
	case v.done:
		info = okStyle.Render("✓ push finished")
	default:
		info = warnStyle.Render("● pushing")
	}
	if len(v.layers) > 0 {
		info += statusStyle.Render(fmt.Sprintf(" · %d/%d layers", v.layersDone(), len(v.layers)))
	}
	lines := []string{paneTitleStyle.Render("Push: "+v.ref) + "  " + info, ""}
	for _, id := range v.layers {
		l := v.progress[id]
		size, status := "", l.status
		if l.total > 0 {
			size = format.Size(l.current) + " / " + format.Size(l.total)
		}
		b := pullBar(l.current, l.total, pullBarWidth)
		if v.layerDone(id) {
			b = pullBar(1, 1, pullBarWidth)
		}
		lines = append(lines, fmt.Sprintf("%-12s  %-18s  %s  %s", format.ShortID(id), status, b, size))
	}
	if len(v.messages) > 0 {
		lines = append(lines, "")
		for _, m := range v.messages {
			lines = append(lines, statusStyle.Render(m))
		}
	}
	if over := len(lines) - max(v.height, 1); over > 0 {
		lines = append(lines[:1], lines[1+over:]...)
	}
	return strings.Join(lines, "\n")
}

var pushCancelKey = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "cancel push"))

var formSubmitKeyPush = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "push"))

type pushKeys struct{ v *pushView }

func (k pushKeys) ShortHelp() []key.Binding {
	switch {
	case k.v.ref == "":
		return []key.Binding{formSubmitKeyPush, formCancelKey}
	case k.v.running():
		return []key.Binding{pushCancelKey, keys.Back}
	}
	return []key.Binding{keys.Back}
}

func (k pushKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func (a *App) pushFinished() tea.Cmd {
	v := a.push
	a.push = nil
	cmds := []tea.Cmd{}
	if a.tabs[tabImages].loaded {
		cmds = append(cmds, a.load(tabImages))
	}
	if a.pane != pane(v) {
		switch {
		case v.canceled:
			cmds = append(cmds, a.setStatus("push of "+v.ref+" canceled", false))
		case v.err != nil:
			cmds = append(cmds, a.setStatus("push of "+v.ref+" failed: "+v.err.Error(), true))
		default:
			cmds = append(cmds, a.setStatus("pushed "+v.ref, false))
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) pushIndicator() string {
	if a.push == nil || a.pane == pane(a.push) {
		return ""
	}
	text := "⇡ pushing " + a.push.ref
	if n := len(a.push.layers); n > 0 {
		text += fmt.Sprintf(" %d/%d", a.push.layersDone(), n)
	}
	return warnStyle.Render(text)
}
