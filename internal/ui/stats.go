// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

const (
	statsInterval = time.Second
	statsHistory  = 120
)

var (
	statsLabelStyle = lipgloss.NewStyle().Foreground(colorMuted).Width(10)
	barFullStyle    = lipgloss.NewStyle().Foreground(colorAccent)
	barEmptyStyle   = lipgloss.NewStyle().Foreground(colorMuted)
)

var statsGen int

// statsView shows live resource usage of a container.
type statsView struct {
	gen     int
	id      string
	name    string
	client  *docker.Client
	width   int
	height  int
	stats   docker.Stats
	cpu     []float64
	loading bool
	err     error
	ctx     context.Context
	cancel  context.CancelFunc
}

type statsMsg struct {
	gen   int
	stats docker.Stats
	err   error
}

type statsTickMsg struct{ gen int }

// openStats opens the stats view for the selected container.
func (a *App) openStats() tea.Cmd {
	it, ok := a.selectedObject()
	if !ok {
		return nil
	}
	statsGen++
	ctx, cancel := context.WithCancel(context.Background())
	v := &statsView{
		gen: statsGen, id: it.id, name: it.name, client: a.client,
		loading: true, ctx: ctx, cancel: cancel,
	}
	v.setSize(a.width, a.bodyHeight())
	a.pane = v
	return v.fetch()
}

// fetch requests the next sample. Requests are bound to the lifetime of the
// view, so closing it aborts a sample in flight.
func (v *statsView) fetch() tea.Cmd {
	ctx, gen, id, client := v.ctx, v.gen, v.id, v.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		s, err := client.ContainerStats(ctx, id)
		return statsMsg{gen: gen, stats: s, err: err}
	}
}

func (v *statsView) setSize(width, height int) { v.width, v.height = width, height }

func (v *statsView) close() { v.cancel() }

func (v *statsView) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case statsMsg:
		if msg.gen != v.gen {
			return nil
		}
		v.loading = false
		v.err = msg.err
		if msg.err == nil {
			v.stats = msg.stats
			v.cpu = append(v.cpu, msg.stats.CPUPercent())
			if len(v.cpu) > statsHistory {
				v.cpu = v.cpu[len(v.cpu)-statsHistory:]
			}
		}
		gen := v.gen
		return tea.Tick(statsInterval, func(time.Time) tea.Msg { return statsTickMsg{gen: gen} })
	case statsTickMsg:
		if msg.gen != v.gen {
			return nil
		}
		return v.fetch()
	}
	return nil
}

// bar renders a horizontal usage bar for a percentage.
func bar(percent float64, width int) string {
	if width < 1 {
		return ""
	}
	filled := int(percent / 100 * float64(width))
	filled = min(max(filled, 0), width)
	return barFullStyle.Render(strings.Repeat("█", filled)) +
		barEmptyStyle.Render(strings.Repeat("░", width-filled))
}

// sparkline renders values as a single line of block characters, scaled to
// the largest value.
func sparkline(values []float64, width int) string {
	const ticks = "▁▂▃▄▅▆▇█"
	if len(values) > width {
		values = values[len(values)-width:]
	}
	peak := 1.0
	for _, v := range values {
		peak = max(peak, v)
	}
	runes := []rune(ticks)
	var b strings.Builder
	for _, v := range values {
		i := int(v / peak * float64(len(runes)-1))
		b.WriteRune(runes[min(max(i, 0), len(runes)-1)])
	}
	return barFullStyle.Render(b.String())
}

func (v *statsView) view() string {
	header := paneTitleStyle.Render("Stats: " + v.name)
	switch {
	case v.loading:
		return header + "  " + statusStyle.Render("sampling…")
	case v.err != nil:
		return header + "  " + errorStyle.Render(v.err.Error())
	}

	s := v.stats
	barWidth := max(min(v.width-40, 60), 10)
	cpu := s.CPUPercent()
	mem := s.MemoryPercent()
	rx, tx := s.NetworkIO()
	br, bw := s.BlockIO()

	row := func(label, value string) string {
		return statsLabelStyle.Render(label) + value
	}
	lines := []string{
		header,
		"",
		row("CPU", fmt.Sprintf("%s %6.2f%%", bar(min(cpu, 100), barWidth), cpu)),
		row("", sparkline(v.cpu, barWidth)),
		"",
		row("MEMORY", fmt.Sprintf("%s %6.2f%%", bar(mem, barWidth), mem)),
		row("", fmt.Sprintf("%s / %s",
			format.Size(int64(s.MemoryUsage())), format.Size(int64(s.MemoryStats.Limit)))),
		"",
		row("NET I/O", fmt.Sprintf("%s ↓  %s ↑", format.Size(int64(rx)), format.Size(int64(tx)))),
		row("BLOCK I/O", fmt.Sprintf("%s read  %s written", format.Size(int64(br)), format.Size(int64(bw)))),
		row("PIDS", fmt.Sprintf("%d", s.PidsStats.Current)),
	}
	return strings.Join(lines, "\n")
}
