// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zaryob/docker-tui/internal/docker"
	"github.com/zaryob/docker-tui/internal/format"
)

const (
	// usageInterval is the pause between two samples of the resource usage
	// shown in the container list.
	usageInterval = 3 * time.Second
	// usageConcurrency limits the stats requests running at once; each
	// takes about a second because the daemon samples the CPU twice.
	usageConcurrency = 8
)

// containerUsage is the CPU and memory use of a container, or the sum for a
// compose project.
type containerUsage struct {
	ok  bool
	cpu float64
	mem uint64
}

func (u containerUsage) cpuText() string {
	if !u.ok {
		return ""
	}
	return fmt.Sprintf("%.1f%%", u.cpu)
}

func (u containerUsage) memText() string {
	if !u.ok {
		return ""
	}
	return format.Size(int64(u.mem))
}

// usageMsg carries a sample of the resource usage of the running
// containers, by ID.
type usageMsg struct{ usage map[string]containerUsage }

type usageTickMsg struct{}

// loadUsage samples the resource usage of the running containers.
func (a *App) loadUsage() tea.Cmd {
	var ids []string
	for _, it := range a.tabs[tabContainers].all {
		if c, ok := it.data.(docker.Container); ok && c.State == "running" {
			ids = append(ids, it.id)
		}
	}
	client := a.client
	return func() tea.Msg {
		usage := make(map[string]containerUsage, len(ids))
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, usageConcurrency)
		for _, id := range ids {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				s, err := client.ContainerStats(ctx, id)
				if err != nil {
					return
				}
				mu.Lock()
				usage[id] = containerUsage{ok: true, cpu: s.CPUPercent(), mem: s.MemoryUsage()}
				mu.Unlock()
			}()
		}
		wg.Wait()
		return usageMsg{usage: usage}
	}
}

// applyUsage fills the CPU and memory columns of container items.
func applyUsage(items []item, usage map[string]containerUsage) {
	for i := range items {
		it := &items[i]
		if it.isGroup() || len(it.cells) <= containerStatusCol {
			continue
		}
		u := usage[it.id]
		it.usage = u
		it.cells[containerCPUCol] = u.cpuText()
		it.cells[containerMemCol] = u.memText()
		if len(it.sortKeys) <= containerMemCol {
			it.sortKeys = append(it.sortKeys, make([]string, containerMemCol+1-len(it.sortKeys))...)
		}
		it.sortKeys[containerCPUCol] = fmt.Sprintf("%020d", int64(u.cpu*1000))
		it.sortKeys[containerMemCol] = fmt.Sprintf("%020d", u.mem)
	}
}
