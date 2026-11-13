// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"testing"

	"github.com/zaryob/docker-tui/internal/docker"
)

func sampleContainers() []item {
	return []item{
		{
			id:   "c1a2b3c4d5e6",
			name: "web-prod",
			cells: []string{
				"c1a2b3c4",
				"web-prod",
				"myproject",
				"nginx:alpine",
				"running",
				"1.2%",
				"45MB",
				"Up 2 hours (healthy)",
				"80->8080/tcp",
			},
			data: docker.Container{
				ID:     "c1a2b3c4d5e6",
				Names:  []string{"/web-prod"},
				Image:  "nginx:alpine",
				State:  "running",
				Status: "Up 2 hours (healthy)",
				Ports: []docker.Port{
					{PrivatePort: 8080, PublicPort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					"env":                        "prod",
					"app":                        "frontend",
					"com.docker.compose.project": "myproject",
				},
			},
		},
		{
			id:   "c2b3c4d5e6f7",
			name: "db-prod",
			cells: []string{
				"c2b3c4d5",
				"db-prod",
				"myproject",
				"postgres:16",
				"running",
				"0.5%",
				"120MB",
				"Up 2 hours (healthy)",
				"5432/tcp",
			},
			data: docker.Container{
				ID:     "c2b3c4d5e6f7",
				Names:  []string{"/db-prod"},
				Image:  "postgres:16",
				State:  "running",
				Status: "Up 2 hours (healthy)",
				Ports: []docker.Port{
					{PrivatePort: 5432, Type: "tcp"},
				},
				Labels: map[string]string{
					"env":                        "prod",
					"app":                        "database",
					"com.docker.compose.project": "myproject",
				},
			},
		},
		{
			id:   "c3c4d5e6f7a8",
			name: "worker-dev",
			cells: []string{
				"c3c4d5e6",
				"worker-dev",
				"",
				"python:3.12",
				"exited",
				"0.0%",
				"0B",
				"Exited (1) 10 minutes ago",
				"",
			},
			data: docker.Container{
				ID:     "c3c4d5e6f7a8",
				Names:  []string{"/worker-dev"},
				Image:  "python:3.12",
				State:  "exited",
				Status: "Exited (1) 10 minutes ago",
				Labels: map[string]string{
					"env": "dev",
					"app": "worker",
				},
			},
		},
		{
			id:   "c4d5e6f7a8b9",
			name: "cache-unhealthy",
			cells: []string{
				"c4d5e6f7",
				"cache-unhealthy",
				"",
				"redis:7",
				"running",
				"0.1%",
				"15MB",
				"Up 10 minutes (unhealthy)",
				"6379/tcp",
			},
			data: docker.Container{
				ID:     "c4d5e6f7a8b9",
				Names:  []string{"/cache-unhealthy"},
				Image:  "redis:7",
				State:  "running",
				Status: "Up 10 minutes (unhealthy)",
				Ports: []docker.Port{
					{PrivatePort: 6379, Type: "tcp"},
				},
				Labels: map[string]string{
					"env": "staging",
					"app": "cache",
				},
			},
		},
	}
}

func filterMatching(items []item, filter string) []string {
	q := parseFilter(filter)
	var matched []string
	for _, it := range items {
		if q.matches(it) {
			matched = append(matched, it.name)
		}
	}
	return matched
}

func TestFilterPlainText(t *testing.T) {
	items := sampleContainers()

	// Single term
	got := filterMatching(items, "prod")
	if len(got) != 2 || got[0] != "web-prod" || got[1] != "db-prod" {
		t.Fatalf("filter 'prod' = %v, want [web-prod db-prod]", got)
	}

	// Unrecognized prefix treated as plain text (e.g. postgres:16)
	got = filterMatching(items, "postgres:16")
	if len(got) != 1 || got[0] != "db-prod" {
		t.Fatalf("filter 'postgres:16' = %v, want [db-prod]", got)
	}

	// Empty query matches all
	got = filterMatching(items, "")
	if len(got) != 4 {
		t.Fatalf("filter '' = %v, want 4 items", got)
	}
}

func TestFilterMultiTermConjunction(t *testing.T) {
	items := sampleContainers()

	// Multi-term (AND): both web and nginx must match
	got := filterMatching(items, "web nginx")
	if len(got) != 1 || got[0] != "web-prod" {
		t.Fatalf("filter 'web nginx' = %v, want [web-prod]", got)
	}

	// No match if second term does not match
	got = filterMatching(items, "web postgres")
	if len(got) != 0 {
		t.Fatalf("filter 'web postgres' = %v, want empty", got)
	}
}

func TestFilterNegation(t *testing.T) {
	items := sampleContainers()

	// Negation with '!'
	got := filterMatching(items, "prod !db")
	if len(got) != 1 || got[0] != "web-prod" {
		t.Fatalf("filter 'prod !db' = %v, want [web-prod]", got)
	}

	// Negation with '-'
	got = filterMatching(items, "prod -db")
	if len(got) != 1 || got[0] != "web-prod" {
		t.Fatalf("filter 'prod -db' = %v, want [web-prod]", got)
	}
}

func TestFilterStatusAndState(t *testing.T) {
	items := sampleContainers()

	// status:running
	got := filterMatching(items, "status:running")
	if len(got) != 3 {
		t.Fatalf("filter 'status:running' = %v, want 3 items", got)
	}

	// state:exited
	got = filterMatching(items, "state:exited")
	if len(got) != 1 || got[0] != "worker-dev" {
		t.Fatalf("filter 'state:exited' = %v, want [worker-dev]", got)
	}

	// !status:exited
	got = filterMatching(items, "!status:exited")
	if len(got) != 3 {
		t.Fatalf("filter '!status:exited' = %v, want 3 items", got)
	}
}

func TestFilterIsPredicate(t *testing.T) {
	items := sampleContainers()

	// is:running
	got := filterMatching(items, "is:running")
	if len(got) != 3 {
		t.Fatalf("filter 'is:running' = %v, want 3 items", got)
	}

	// is:stopped / is:exited
	got = filterMatching(items, "is:stopped")
	if len(got) != 1 || got[0] != "worker-dev" {
		t.Fatalf("filter 'is:stopped' = %v, want [worker-dev]", got)
	}

	// is:healthy
	got = filterMatching(items, "is:healthy")
	if len(got) != 2 || got[0] != "web-prod" || got[1] != "db-prod" {
		t.Fatalf("filter 'is:healthy' = %v, want [web-prod db-prod]", got)
	}

	// is:unhealthy
	got = filterMatching(items, "is:unhealthy")
	if len(got) != 1 || got[0] != "cache-unhealthy" {
		t.Fatalf("filter 'is:unhealthy' = %v, want [cache-unhealthy]", got)
	}
}

func TestFilterLabels(t *testing.T) {
	items := sampleContainers()

	// label:key=value
	got := filterMatching(items, "label:env=prod")
	if len(got) != 2 || got[0] != "web-prod" || got[1] != "db-prod" {
		t.Fatalf("filter 'label:env=prod' = %v, want [web-prod db-prod]", got)
	}

	// label:key only
	got = filterMatching(items, "label:com.docker.compose.project")
	if len(got) != 2 {
		t.Fatalf("filter 'label:com.docker.compose.project' = %v, want 2 items", got)
	}

	// Combined label and name
	got = filterMatching(items, "label:env=prod name:web")
	if len(got) != 1 || got[0] != "web-prod" {
		t.Fatalf("filter 'label:env=prod name:web' = %v, want [web-prod]", got)
	}
}

func TestFilterImageAndPort(t *testing.T) {
	items := sampleContainers()

	// image:
	got := filterMatching(items, "image:postgres")
	if len(got) != 1 || got[0] != "db-prod" {
		t.Fatalf("filter 'image:postgres' = %v, want [db-prod]", got)
	}

	// port:
	got = filterMatching(items, "port:8080")
	if len(got) != 1 || got[0] != "web-prod" {
		t.Fatalf("filter 'port:8080' = %v, want [web-prod]", got)
	}
}

func TestFilterQuotedPhrases(t *testing.T) {
	items := sampleContainers()

	got := filterMatching(items, `"Up 2 hours"`)
	if len(got) != 2 {
		t.Fatalf("filter '\"Up 2 hours\"' = %v, want 2 items", got)
	}
}
