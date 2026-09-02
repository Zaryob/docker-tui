// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"net/url"
	"time"
)

// CPUStats holds CPU usage counters of a container.
type CPUStats struct {
	CPUUsage struct {
		TotalUsage  uint64   `json:"total_usage"`
		PercpuUsage []uint64 `json:"percpu_usage"`
	} `json:"cpu_usage"`
	SystemUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs  uint32 `json:"online_cpus"`
}

// Stats is a single resource usage sample of a container.
type Stats struct {
	Read        time.Time `json:"read"`
	CPUStats    CPUStats  `json:"cpu_stats"`
	PreCPUStats CPUStats  `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IoServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PidsStats struct {
		Current uint64 `json:"current"`
	} `json:"pids_stats"`
}

// ContainerStats returns one resource usage sample of a running container.
// The daemon collects two samples to compute CPU usage, so the call takes
// about a second.
func (c *Client) ContainerStats(ctx context.Context, id string) (Stats, error) {
	q := url.Values{}
	q.Set("stream", "0")
	var s Stats
	err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/stats", q, &s)
	return s, err
}

// CPUPercent returns the CPU usage in percent, where 100% is one full CPU,
// computed the same way as "docker stats".
func (s Stats) CPUPercent() float64 {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	if cpuDelta <= 0 || sysDelta <= 0 {
		return 0
	}
	cpus := float64(s.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	return cpuDelta / sysDelta * cpus * 100
}

// MemoryUsage returns the memory used by the container, excluding the page
// cache, the same way as "docker stats".
func (s Stats) MemoryUsage() uint64 {
	usage := s.MemoryStats.Usage
	// cgroup v2 reports inactive_file, cgroup v1 total_inactive_file.
	for _, k := range []string{"inactive_file", "total_inactive_file"} {
		if v, ok := s.MemoryStats.Stats[k]; ok && v < usage {
			return usage - v
		}
	}
	return usage
}

// MemoryPercent returns the memory usage relative to the limit.
func (s Stats) MemoryPercent() float64 {
	if s.MemoryStats.Limit == 0 {
		return 0
	}
	return float64(s.MemoryUsage()) / float64(s.MemoryStats.Limit) * 100
}

// NetworkIO returns the total received and transmitted bytes.
func (s Stats) NetworkIO() (rx, tx uint64) {
	for _, n := range s.Networks {
		rx += n.RxBytes
		tx += n.TxBytes
	}
	return rx, tx
}

// BlockIO returns the total bytes read from and written to block devices.
func (s Stats) BlockIO() (read, write uint64) {
	for _, e := range s.BlkioStats.IoServiceBytesRecursive {
		switch e.Op {
		case "read", "Read":
			read += e.Value
		case "write", "Write":
			write += e.Value
		}
	}
	return read, write
}
