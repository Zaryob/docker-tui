// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

var pullGen int

// pullBarWidth is the width of the progress bar of a layer.
const pullBarWidth = 24

// layerProgress is the state of one layer of a pull.
type layerProgress struct {
	status         string
	current, total int64
}

// pullView asks for an image reference and then shows the progress of the
// pull, layer by layer. Like a build, the pull keeps running when the view
// is closed.
type pullView struct {
	gen    int
	client *docker.Client
	input  textinput.Model
	width  int
	height int

	// ref is the image being pulled; empty while the reference is asked.
	ref      string
	layers   []string
	progress map[string]*layerProgress
	// messages are the progress messages about the whole pull.
	messages []string

	ch       chan pullEvent
	cancel   context.CancelFunc
	done     bool
	canceled bool
	err      error
}

type pullEvent struct {
	p   docker.PullProgress
	err error
}

type pullMsg struct {
	gen    int
	events []docker.PullProgress
	done   bool
	err    error
}

// showPull opens the running pull, or asks for an image to pull.
func (a *App) showPull() tea.Cmd {
	if a.pull != nil {
		a.pane = a.pull
		a.layout()
		return nil
	}
	in := textinput.New()
	in.Prompt = "Image  "
	in.PromptStyle = statsLabelStyle
	in.Placeholder = "e.g. nginx, alpine:3.20, ghcr.io/owner/app:tag"
	pullGen++
	v := &pullView{gen: pullGen, client: a.client, input: in, progress: map[string]*layerProgress{}}
	v.setSize(a.width, a.bodyHeight())
	a.pane = v
	a.layout()
	return v.input.Focus()
}

// startPull starts pulling ref in the background.
func (v *pullView) startPull(ref string) tea.Cmd {
	v.ref = ref
	v.input.Blur()
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	ch := make(chan pullEvent, 64)
	v.ch = ch
	client := v.client
	go func() {
		defer close(ch)
		s, err := client.PullImage(ctx, ref)
		if err != nil {
			ch <- pullEvent{err: err}
			return
		}
		defer s.Close()
		for {
			p, err := s.Next()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				if ctx.Err() == nil {
					ch <- pullEvent{err: err}
				}
				return
			}
			select {
			case ch <- pullEvent{p: p}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return v.wait()
}

// wait delivers the progress received since the previous message.
func (v *pullView) wait() tea.Cmd {
	ch, gen := v.ch, v.gen
	return func() tea.Msg {
		msg := pullMsg{gen: gen}
		ev, ok := <-ch
		for {
			if !ok {
				msg.done = true
				return msg
			}
			if ev.err != nil {
				msg.err = ev.err
			} else {
				msg.events = append(msg.events, ev.p)
			}
			select {
			case ev, ok = <-ch:
			default:
				return msg
			}
		}
	}
}

// running reports whether a pull is in progress.
func (v *pullView) running() bool { return v.ref != "" && !v.done }

func (v *pullView) setSize(width, height int) {
	v.width, v.height = width, height
	v.input.Width = max(width-12, 10)
}

func (v *pullView) close() {}

// stop cancels the pull.
func (v *pullView) stop() {
	if v.cancel != nil {
		v.cancel()
	}
}

// capturesInput keeps every key while the reference is typed, so that q
// can be typed.
func (v *pullView) capturesInput() bool { return v.ref == "" }

func (v *pullView) keyMap() help.KeyMap { return pullKeys{v} }

func (v *pullView) apply(p docker.PullProgress) {
	if p.ID == "" || p.Detail.Total == 0 && !isLayerStatus(p.Status) {
		// Messages about the whole pull, e.g. "Digest: sha256:..." or
		// "Pulling from library/alpine" (whose ID is the tag).
		if p.Status != "" && (len(v.messages) == 0 || v.messages[len(v.messages)-1] != p.Status) {
			v.messages = append(v.messages, p.Status)
		}
		return
	}
	l, ok := v.progress[p.ID]
	if !ok {
		l = &layerProgress{}
		v.progress[p.ID] = l
		v.layers = append(v.layers, p.ID)
	}
	l.status = p.Status
	if p.Detail.Total > 0 {
		l.current, l.total = p.Detail.Current, p.Detail.Total
	}
}

// isLayerStatus reports whether status is one that the daemon reports for
// layers.
func isLayerStatus(status string) bool {
	switch status {
	case "Pulling fs layer", "Waiting", "Downloading", "Verifying Checksum", "Download complete",
		"Extracting", "Pull complete", "Already exists", "Retrying":
		return true
	}
	return strings.HasPrefix(status, "Retrying")
}

// layerDone reports whether a layer is complete. Once the pull succeeded
// every layer is, whatever its last status; the containerd image store of
// Docker Desktop leaves some at "Download complete".
func (v *pullView) layerDone(id string) bool {
	if v.succeeded() {
		return true
	}
	switch v.progress[id].status {
	case "Pull complete", "Already exists":
		return true
	}
	return false
}

// succeeded reports whether the pull finished without error.
func (v *pullView) succeeded() bool { return v.done && v.err == nil && !v.canceled }

// layersDone counts the layers that are complete.
func (v *pullView) layersDone() int {
	n := 0
	for _, id := range v.layers {
		if v.layerDone(id) {
			n++
		}
	}
	return n
}

func (v *pullView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case pullMsg:
		if msg.gen != v.gen {
			return nil
		}
		for _, p := range msg.events {
			v.apply(p)
		}
		if msg.err != nil {
			v.err = msg.err
		}
		if msg.done {
			v.done = true
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
				return v.startPull(ref)
			}
			var cmd tea.Cmd
			v.input, cmd = v.input.Update(msg)
			return cmd
		}
		if key.Matches(msg, pullCancelKey) && v.running() {
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

// pullBar renders a progress bar of width cells for current of total.
func pullBar(current, total int64, width int) string {
	if total <= 0 {
		return strings.Repeat("·", width)
	}
	n := int(min(current, total) * int64(width) / total)
	return okStyle.Render(strings.Repeat("█", n)) + statusStyle.Render(strings.Repeat("░", width-n))
}

func (v *pullView) view() string {
	if v.ref == "" {
		return strings.Join([]string{
			paneTitleStyle.Render("Pull an image"), "",
			v.input.View(), "",
			statusStyle.Render("Pulls from Docker Hub or the registry named in the reference; esc on the progress keeps it running."),
		}, "\n")
	}
	var info string
	switch {
	case v.canceled:
		info = statusStyle.Render("pull canceled")
	case v.err != nil:
		info = errorStyle.Render(v.err.Error())
	case v.done:
		info = okStyle.Render("✓ pull finished")
	default:
		info = warnStyle.Render("● pulling")
	}
	if len(v.layers) > 0 {
		info += statusStyle.Render(fmt.Sprintf(" · %d/%d layers", v.layersDone(), len(v.layers)))
	}
	lines := []string{paneTitleStyle.Render("Pull: "+v.ref) + "  " + info, ""}
	for _, id := range v.layers {
		l := v.progress[id]
		size, status := "", l.status
		if l.total > 0 {
			size = format.Size(l.current) + " / " + format.Size(l.total)
		}
		b := pullBar(l.current, l.total, pullBarWidth)
		if v.layerDone(id) {
			b = pullBar(1, 1, pullBarWidth)
			if v.succeeded() && status == "Download complete" {
				status = "Pull complete"
			}
		}
		lines = append(lines, fmt.Sprintf("%-12s  %-18s  %s  %s", format.ShortID(id), status, b, size))
	}
	if len(v.messages) > 0 {
		lines = append(lines, "")
		for _, m := range v.messages {
			lines = append(lines, statusStyle.Render(m))
		}
	}
	// Keep the end visible when there are more lines than room.
	if over := len(lines) - max(v.height, 1); over > 0 {
		lines = append(lines[:1], lines[1+over:]...)
	}
	return strings.Join(lines, "\n")
}

var pullCancelKey = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "cancel pull"))

type pullKeys struct{ v *pullView }

func (k pullKeys) ShortHelp() []key.Binding {
	switch {
	case k.v.ref == "":
		return []key.Binding{formSubmitKeyPull, formCancelKey}
	case k.v.running():
		return []key.Binding{pullCancelKey, keys.Back}
	}
	return []key.Binding{keys.Back}
}

func (k pullKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var formSubmitKeyPull = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "pull"))

// pullFinished reports the end of the pull when its view is not shown and
// refreshes the image list.
func (a *App) pullFinished() tea.Cmd {
	v := a.pull
	a.pull = nil
	cmds := []tea.Cmd{}
	if a.tabs[tabImages].loaded {
		cmds = append(cmds, a.load(tabImages))
	}
	if a.pane != pane(v) {
		switch {
		case v.canceled:
			cmds = append(cmds, a.setStatus("pull of "+v.ref+" canceled", false))
		case v.err != nil:
			cmds = append(cmds, a.setStatus("pull of "+v.ref+" failed: "+v.err.Error(), true))
		default:
			cmds = append(cmds, a.setStatus("pulled "+v.ref, false))
		}
	}
	return tea.Batch(cmds...)
}

// pullIndicator describes the pull running in the background.
func (a *App) pullIndicator() string {
	if a.pull == nil || a.pane == pane(a.pull) {
		return ""
	}
	text := "⇣ pulling " + a.pull.ref
	if n := len(a.pull.layers); n > 0 {
		text += fmt.Sprintf(" %d/%d", a.pull.layersDone(), n)
	}
	return warnStyle.Render(text)
}
