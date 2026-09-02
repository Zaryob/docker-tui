// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"encoding/json"
	"math"
	"testing"
)

const sampleStats = `{
  "cpu_stats": {"cpu_usage": {"total_usage": 400000000}, "system_cpu_usage": 20000000000, "online_cpus": 4},
  "precpu_stats": {"cpu_usage": {"total_usage": 200000000}, "system_cpu_usage": 16000000000, "online_cpus": 4},
  "memory_stats": {"usage": 104857600, "limit": 1048576000, "stats": {"inactive_file": 4857600}},
  "networks": {"eth0": {"rx_bytes": 1000, "tx_bytes": 200}, "eth1": {"rx_bytes": 24, "tx_bytes": 6}},
  "blkio_stats": {"io_service_bytes_recursive": [
    {"op": "read", "value": 4096}, {"op": "write", "value": 8192}, {"op": "Read", "value": 4}
  ]},
  "pids_stats": {"current": 7}
}`

func TestStatsCalculations(t *testing.T) {
	var s Stats
	if err := json.Unmarshal([]byte(sampleStats), &s); err != nil {
		t.Fatal(err)
	}
	// 0.2s of CPU over 4s of system time on 4 CPUs => 20%.
	if got := s.CPUPercent(); math.Abs(got-20) > 1e-9 {
		t.Errorf("CPUPercent = %v, want 20", got)
	}
	if got := s.MemoryUsage(); got != 100_000_000 {
		t.Errorf("MemoryUsage = %d, want 100000000", got)
	}
	if got := s.MemoryPercent(); math.Abs(got-9.5367431640625) > 1e-9 {
		t.Errorf("MemoryPercent = %v", got)
	}
	if rx, tx := s.NetworkIO(); rx != 1024 || tx != 206 {
		t.Errorf("NetworkIO = %d, %d", rx, tx)
	}
	if r, w := s.BlockIO(); r != 4100 || w != 8192 {
		t.Errorf("BlockIO = %d, %d", r, w)
	}
}

func TestCPUPercentWithoutPreviousSample(t *testing.T) {
	var s Stats
	s.CPUStats.CPUUsage.TotalUsage = 100
	s.CPUStats.SystemUsage = 1000
	if got := s.CPUPercent(); got != 0 {
		t.Errorf("CPUPercent = %v, want 0", got)
	}
}
