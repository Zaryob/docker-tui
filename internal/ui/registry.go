// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/registry"
)

type registryResultMsg struct {
	form   *registryForm
	logout bool
	err    error
}

// registryForm never stores credentials outside its masked input and the active
// command. Results, status messages and saved preferences contain no secrets.
type registryForm struct {
	inputs []textinput.Model
	focus  int
	logout bool
	busy   bool
	err    string
	cancel context.CancelFunc
	client *registry.Client
}

func (a *App) openRegistry(logout bool) tea.Cmd {
	f := newRegistryForm(logout, &registry.Client{Host: a.client.Host(), Context: a.client.Context()})
	a.pane = f
	a.layout()
	return textinput.Blink
}

func newRegistryForm(logout bool, client *registry.Client) *registryForm {
	f := &registryForm{logout: logout, client: client}
	prompts := []string{"Registry  ", "Username  ", "Password  "}
	if logout {
		prompts = prompts[:1]
	}
	for i, prompt := range prompts {
		in := textinput.New()
		in.Prompt = prompt
		in.PromptStyle = statsLabelStyle
		if i == 0 {
			in.SetValue("docker.io")
		}
		if i == 2 {
			in.EchoMode = textinput.EchoPassword
			in.EchoCharacter = '•'
			in.Placeholder = "password or access token"
		}
		f.inputs = append(f.inputs, in)
	}
	f.inputs[0].Focus()
	return f
}

func (f *registryForm) capturesInput() bool { return true }
func (f *registryForm) close() {
	if f.cancel != nil {
		f.cancel()
	}
	for i := range f.inputs {
		f.inputs[i].Reset()
	}
}
func (f *registryForm) setSize(width, _ int) {
	for i := range f.inputs {
		f.inputs[i].Width = max(width-30, 10)
	}
}
func (f *registryForm) keyMap() help.KeyMap { return registryFormKeys{logout: f.logout} }
func (f *registryForm) update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			f.close()
			return func() tea.Msg { return closePaneMsg{} }
		case "tab", "down":
			return f.move(1)
		case "shift+tab", "up":
			return f.move(-1)
		case "enter":
			if f.busy {
				return nil
			}
			server, err := registry.Server(f.inputs[0].Value())
			if err != nil {
				f.err = err.Error()
				return nil
			}
			if !f.logout && (strings.TrimSpace(f.inputs[1].Value()) == "" || f.inputs[2].Value() == "") {
				f.err = "username and password or access token are required"
				return nil
			}
			username, password := "", ""
			if !f.logout {
				username, password = strings.TrimSpace(f.inputs[1].Value()), f.inputs[2].Value()
				f.inputs[2].Reset()
			}
			f.busy, f.err = true, ""
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			f.cancel = cancel
			return func() tea.Msg {
				defer cancel()
				var err error
				if f.logout {
					err = f.client.Logout(ctx, server)
				} else {
					err = f.client.Login(ctx, server, username, password)
				}
				return registryResultMsg{form: f, logout: f.logout, err: err}
			}
		}
	}
	if f.busy {
		return nil
	}
	f.err = ""
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	return cmd
}
func (f *registryForm) move(delta int) tea.Cmd {
	if f.busy {
		return nil
	}
	f.inputs[f.focus].Blur()
	f.focus = (f.focus + delta + len(f.inputs)) % len(f.inputs)
	return f.inputs[f.focus].Focus()
}
func (f *registryForm) view() string {
	title, note := "Registry login", "Uses Docker's configured credential store; password is passed through stdin."
	if f.logout {
		title, note = "Registry logout", "Removes the selected registry's saved Docker credentials when you submit."
	}
	if f.busy {
		note = "Working… esc cancels"
	}
	if f.err != "" {
		note = errorStyle.Render(f.err)
	}
	lines := []string{paneTitleStyle.Render(title), ""}
	for _, in := range f.inputs {
		lines = append(lines, in.View())
	}
	return strings.Join(append(lines, "", note), "\n")
}

type registryFormKeys struct{ logout bool }

func (k registryFormKeys) ShortHelp() []key.Binding {
	action := "log in"
	if k.logout {
		action = "log out"
	}
	return []key.Binding{formNextKey, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", action)), formCancelKey}
}
func (k registryFormKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

// imageKeyMap exposes registry actions where they are available.
type imageKeyMap struct{ keyMap }

func (k imageKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.NextTab, k.Filter, k.Pull, k.Build, k.Login, k.Logout, k.Inspect, k.Remove, k.Help, k.Quit}
}
