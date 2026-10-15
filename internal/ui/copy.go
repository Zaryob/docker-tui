// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/archive"
	"github.com/zaryob/docker-tui/internal/docker"
)

// copyTimeout bounds a copy, which can move large directories.
const copyTimeout = 10 * time.Minute

// copyForm asks what to copy between a container and the local machine,
// like "docker cp": the container side is written as "name:/path".
type copyForm struct {
	id, name string
	inputs   []textinput.Model
	focus    int
	err      string
}

// copyMsg asks the app to run a copy from the form.
type copyMsg struct{ id, name, from, to string }

// openCopy opens the copy form for the selected container. Copying also
// works with stopped containers.
func (a *App) openCopy() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	cwd, _ := os.Getwd()
	f := &copyForm{id: it.id, name: it.name}
	for _, l := range []struct{ prompt, value string }{
		{"From  ", it.name + ":/"},
		{"To    ", cwd},
	} {
		in := textinput.New()
		in.Prompt = l.prompt
		in.PromptStyle = statsLabelStyle
		in.SetValue(l.value)
		f.inputs = append(f.inputs, in)
	}
	f.inputs[0].Focus()
	f.inputs[0].CursorEnd()
	f.setSize(a.width, a.bodyHeight())
	a.pane = f
	a.layout()
	return textinput.Blink
}

func (f *copyForm) setSize(width, _ int) {
	for i := range f.inputs {
		f.inputs[i].Width = max(width-10, 10)
	}
}

func (f *copyForm) close() {}

func (f *copyForm) capturesInput() bool { return true }

func (f *copyForm) keyMap() help.KeyMap { return copyFormKeys{} }

func (f *copyForm) update(msg tea.Msg) tea.Cmd {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
		return cmd
	}
	switch km.String() {
	case "esc":
		return func() tea.Msg { return closePaneMsg{} }
	case "tab", "down", "shift+tab", "up":
		f.inputs[f.focus].Blur()
		f.focus = 1 - f.focus
		return f.inputs[f.focus].Focus()
	case "ctrl+s":
		// Swap the direction of the copy.
		from, to := f.inputs[0].Value(), f.inputs[1].Value()
		f.inputs[0].SetValue(to)
		f.inputs[1].SetValue(from)
		return nil
	case "enter":
		from, to := strings.TrimSpace(f.inputs[0].Value()), strings.TrimSpace(f.inputs[1].Value())
		_, fromC := containerPath(from, f.name)
		_, toC := containerPath(to, f.name)
		if fromC == toC {
			f.err = fmt.Sprintf("one side must be in the container, written as %s:/path", f.name)
			return nil
		}
		id, name := f.id, f.name
		return func() tea.Msg { return copyMsg{id: id, name: name, from: from, to: to} }
	}
	f.err = ""
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(km)
	return cmd
}

func (f *copyForm) view() string {
	lines := []string{paneTitleStyle.Render("Copy files: " + f.name), ""}
	for _, in := range f.inputs {
		lines = append(lines, in.View())
	}
	lines = append(lines, "")
	if f.err != "" {
		lines = append(lines, errorStyle.Render(f.err))
	} else {
		lines = append(lines, statusStyle.Render(fmt.Sprintf("Write the container side as %s:/path, like docker cp; ctrl+s swaps the direction.", f.name)))
	}
	return strings.Join(lines, "\n")
}

type copyFormKeys struct{}

var (
	copySubmitKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "copy"))
	copySwapKey   = key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "swap direction"))
)

func (copyFormKeys) ShortHelp() []key.Binding {
	return []key.Binding{formNextKey, copySwapKey, copySubmitKey, formCancelKey}
}

func (k copyFormKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// containerPath returns the path of s if it names a path in the container
// called name, as in "web:/etc/nginx".
func containerPath(s, name string) (string, bool) {
	if p, ok := strings.CutPrefix(s, name+":"); ok {
		if p == "" {
			p = "/"
		}
		return p, true
	}
	return "", false
}

// localPath makes a local path absolute, expanding ~.
func localPath(p string) (string, error) {
	p = expandHome(p)
	if p == "" {
		return "", errors.New("no local path given")
	}
	return filepath.Abs(p)
}

// runCopy performs the copy of a copyMsg and reports it as an actionMsg.
func (a *App) runCopy(msg copyMsg) tea.Cmd {
	a.busy++
	client := a.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), copyTimeout)
		defer cancel()
		status, err := copyFiles(ctx, client, msg)
		if err != nil {
			err = fmt.Errorf("copy: %w", err)
		}
		return actionMsg{kind: tabContainers, status: status, err: err}
	}
}

// copyFiles copies with the semantics of docker cp: copying into an
// existing directory puts the source inside it, otherwise the source is
// copied under the destination name.
func copyFiles(ctx context.Context, client *docker.Client, msg copyMsg) (string, error) {
	if src, ok := containerPath(msg.from, msg.name); ok {
		dst, err := localPath(msg.to)
		if err != nil {
			return "", err
		}
		rc, err := client.CopyFromContainer(ctx, msg.id, src)
		if err != nil {
			return "", err
		}
		defer rc.Close()
		dir, rename := dst, ""
		if fi, err := os.Stat(dst); err != nil || !fi.IsDir() {
			dir, rename = filepath.Dir(dst), filepath.Base(dst)
		}
		n, err := archive.Untar(rc, dir, rename)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("copied %s to %s (%s)", msg.from, dst, countNoun(n, "files")), nil
	}

	dst, _ := containerPath(msg.to, msg.name)
	src, err := localPath(msg.from)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s does not exist", src)
	}
	dir, root := dst, filepath.Base(src)
	st, err := client.StatPath(ctx, msg.id, dst)
	switch {
	case err == nil && st.Mode.IsDir():
	case err == nil || docker.IsNotFound(err):
		dir, root = path.Dir(dst), path.Base(dst)
	default:
		return "", err
	}
	tr := archive.Tar(src, root)
	defer tr.Close()
	if err := client.CopyToContainer(ctx, msg.id, dir, tr); err != nil {
		return "", err
	}
	return fmt.Sprintf("copied %s to %s:%s", src, msg.name, dst), nil
}
