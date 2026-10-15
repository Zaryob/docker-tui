// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// inspectView shows the JSON description of a Docker object.
type inspectView struct {
	gen     int
	title   string
	vp      viewport.Model
	loading bool
	err     error
}

type inspectMsg struct {
	gen  int
	data []byte
	err  error
}

var inspectGen int

// openInspect opens the inspect view for the selected object.
func (a *App) openInspect() tea.Cmd {
	t := a.current()
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	var fetch func(context.Context, string) ([]byte, error)
	switch t.kind {
	case tabContainers:
		fetch = a.client.InspectContainer
	case tabImages:
		fetch = a.client.InspectImage
	case tabVolumes:
		fetch = a.client.InspectVolume
	case tabNetworks:
		fetch = a.client.InspectNetwork
	default:
		return nil
	}

	inspectGen++
	v := &inspectView{gen: inspectGen, title: "Inspect: " + it.name, loading: true, vp: viewport.New(0, 0)}
	v.setSize(a.width, a.bodyHeight())
	a.pane = v

	gen, id := v.gen, it.id
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		data, err := fetch(ctx, id)
		return inspectMsg{gen: gen, data: data, err: err}
	}
}

func (v *inspectView) setSize(width, height int) {
	v.vp.Width = width
	v.vp.Height = max(height-1, 1)
}

func (v *inspectView) close() {}

func (v *inspectView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case inspectMsg:
		if msg.gen != v.gen {
			return nil
		}
		v.loading = false
		v.err = msg.err
		if msg.err == nil {
			var buf bytes.Buffer
			if err := json.Indent(&buf, msg.data, "", "  "); err != nil {
				buf.Reset()
				buf.Write(msg.data)
			}
			v.vp.SetContent(highlightJSON(buf.String()))
		}
		return nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Top):
			v.vp.GotoTop()
			return nil
		case key.Matches(msg, keys.Bottom):
			v.vp.GotoBottom()
			return nil
		}
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return cmd
}

func (v *inspectView) view() string {
	header := paneTitleStyle.Render(v.title)
	switch {
	case v.loading:
		return header + "  " + statusStyle.Render("loading…")
	case v.err != nil:
		return header + "  " + errorStyle.Render(v.err.Error())
	}
	return header + "\n" + v.vp.View()
}
