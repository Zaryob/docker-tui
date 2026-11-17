// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

type tagForm struct {
	image  docker.Image
	input  textinput.Model
	width  int
	height int
	err    string
}

func (a *App) openTag() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	img, ok := it.data.(docker.Image)
	if !ok {
		return nil
	}
	in := textinput.New()
	in.Prompt = "Target  "
	in.PromptStyle = statsLabelStyle
	in.Placeholder = "e.g. repo/image:tag or myregistry:5000/app:1.0"
	if img.Tag() != "<none>:<none>" {
		in.SetValue(img.Tag())
	}
	f := &tagForm{
		image: img,
		input: in,
	}
	f.setSize(a.width, a.bodyHeight())
	a.pane = f
	a.layout()
	return f.input.Focus()
}

func (f *tagForm) capturesInput() bool { return true }

func (f *tagForm) setSize(w, h int) {
	f.width, f.height = w, h
	f.input.Width = min(60, max(20, w-18))
}

func (f *tagForm) close() {}

func (f *tagForm) update(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			return func() tea.Msg { return closePaneMsg{} }
		case "enter":
			target := strings.TrimSpace(f.input.Value())
			if target == "" {
				f.err = "target tag cannot be empty"
				return nil
			}
			return func() tea.Msg {
				return tagSubmitMsg{imageID: f.image.ID, target: target}
			}
		}
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return cmd
}

func (f *tagForm) view() string {
	lines := []string{
		paneTitleStyle.Render("Tag image: " + format.ShortID(f.image.ID)),
		"",
		f.input.View(),
		"",
	}
	if f.err != "" {
		lines = append(lines, errorStyle.Render(f.err), "")
	}
	lines = append(lines,
		statusStyle.Render("Creates a new repository:tag reference pointing to this image. Press enter to save, esc to cancel."),
	)
	return strings.Join(lines, "\n")
}

type tagSubmitMsg struct {
	imageID string
	target  string
}
