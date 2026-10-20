// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLogTabRowsHaveStableUniqueIDs(t *testing.T) {
	lines := []string{
		"2026-10-15T12:00:00Z first",
		"2026-10-15T12:00:01Z second",
		"2026-10-15T12:00:02Z repeated",
		"2026-10-15T12:00:02Z repeated",
	}
	rows, err := logTabItems(strings.NewReader(strings.Join(lines, "\n")), "source", "web", false, time.Time{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, row := range rows {
		if seen[row.id] {
			t.Fatalf("duplicate row ID %q", row.id)
		}
		seen[row.id] = true
		if entry := row.data.(logTabEntry); entry.sourceID != "source" || entry.isBuildSource {
			t.Fatalf("row source = %+v", entry)
		}
	}
	updated, err := logTabItems(strings.NewReader(strings.Join(lines[1:], "\n")), "source", "web", false, time.Now(), 0)
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range updated {
		if row.id != rows[i+1].id {
			t.Errorf("row %d changed ID after an older message left the tail", i)
		}
	}
	tab := newLogsTab()
	tab.setItems(rows)
	tab.table.SetCursor(1)
	tab.toggleMark()
	if tab.table.Cursor() != 1 || len(tab.markedItems()) != 1 {
		t.Fatalf("marking one row: cursor %d, marked %d", tab.table.Cursor(), len(tab.markedItems()))
	}
	tab.setItems(updated)
	if selected, ok := tab.selected(); !ok || selected.id != rows[1].id || len(tab.markedItems()) != 1 {
		t.Fatalf("refresh lost the selected or marked message: selected %+v, marked %d", selected, len(tab.markedItems()))
	}
}

func TestOpenSelectedLogUsesSourceID(t *testing.T) {
	for _, isBuild := range []bool{false, true} {
		t.Run(fmt.Sprintf("build=%v", isBuild), func(t *testing.T) {
			requests := make(chan string, 4)
			a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path
				if strings.HasSuffix(r.URL.Path, "/json") {
					io.WriteString(w, `{"Config":{"Tty":true}}`)
					return
				}
				io.WriteString(w, "2026-10-15T12:00:00Z message\n")
			}))
			commands := make(chan string, 1)
			a.buildx.Command = func(ctx context.Context, args ...string) (*exec.Cmd, error) {
				commands <- strings.Join(args, " ")
				return exec.CommandContext(ctx, "sh", "-c", "printf 'message\\n'"), nil
			}
			// A container name may contain "build" without being a BuildKit
			// history source. Routing must use the source type, not display data.
			rows, err := logTabItems(strings.NewReader("message\n"), "actual-source", "build-worker", isBuild, time.Time{}, 0)
			if err != nil {
				t.Fatal(err)
			}
			a.active = int(tabLogs)
			a.current().setItems(rows)
			cmd := a.openSelectedLog()
			l, ok := a.pane.(*logView)
			if !ok || l.sources[0].id != "actual-source" {
				t.Fatalf("opened source = %+v", a.pane)
			}
			defer l.close()
			for cmd != nil {
				cmd = l.update(cmd())
			}
			if l.err != nil || len(l.entries) != 1 {
				t.Fatalf("source read: entries %d, err %v", len(l.entries), l.err)
			}
			if isBuild {
				if got := <-commands; got != "buildx history logs --progress plain actual-source" {
					t.Errorf("command = %q", got)
				}
				if !l.entries[0].isBuild {
					t.Error("build entry was rendered as a container")
				}
			} else {
				close(requests)
				for path := range requests {
					if !strings.Contains(path, "/containers/actual-source/") {
						t.Errorf("request used a message ID: %s", path)
					}
				}
			}
		})
	}
}

func TestLoadLogsPreservesPartialResultsAndReportsErrors(t *testing.T) {
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			io.WriteString(w, `[{"Id":"a1","Names":["/web"]},{"Id":"b2","Names":["/denied"]}]`)
		case strings.HasSuffix(r.URL.Path, "/json"):
			io.WriteString(w, `{"Config":{"Tty":true}}`)
		case strings.Contains(r.URL.Path, "/a1/logs"):
			io.WriteString(w, "2026-10-15T12:00:00Z available\n")
		default:
			http.Error(w, "logs unavailable", http.StatusInternalServerError)
		}
	}))
	a.buildx.Command = func(context.Context, ...string) (*exec.Cmd, error) {
		return nil, errors.New("buildx unavailable")
	}
	msg := a.loadLogs().(listMsg)
	if msg.err == nil || !msg.partial || len(msg.items) != 1 {
		t.Fatalf("loadLogs = %+v", msg)
	}
	for _, text := range []string{"container denied", "logs unavailable", "build history", "buildx unavailable"} {
		if !strings.Contains(msg.err.Error(), text) {
			t.Errorf("error %q missing %q", msg.err, text)
		}
	}
	a.active = int(tabLogs)
	a.Update(msg)
	if len(a.current().all) != 1 || a.current().loadErr == nil || !a.current().loaded {
		t.Fatal("partial rows and their load error were not applied together")
	}
	if !strings.Contains(a.statusLine(), "logs unavailable") {
		t.Errorf("load error is not visible: %s", a.statusLine())
	}
}

func TestLoadLogsTotalFailureKeepsPreviousRows(t *testing.T) {
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "daemon unavailable", http.StatusInternalServerError)
	}))
	a.buildx.Command = func(context.Context, ...string) (*exec.Cmd, error) {
		return nil, errors.New("buildx unavailable")
	}
	a.active = int(tabLogs)
	a.current().setItems([]item{{id: "previous", cells: []string{"[ ]", "", "web", "previous message"}}})
	msg := a.loadLogs().(listMsg)
	if msg.err == nil || msg.partial || len(msg.items) != 0 {
		t.Fatalf("total failure = %+v", msg)
	}
	a.Update(msg)
	if len(a.current().all) != 1 || a.current().all[0].id != "previous" || a.current().loadErr == nil {
		t.Fatal("total failure cleared previous rows or hid its error")
	}
}

func TestLoadLogsReportsScannerErrorsWithPartialRows(t *testing.T) {
	a := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			io.WriteString(w, `[{"Id":"a1","Names":["/web"]}]`)
		case strings.HasSuffix(r.URL.Path, "/json"):
			io.WriteString(w, `{"Config":{"Tty":true}}`)
		default:
			// A short HTTP body makes the reader fail after one valid line.
			w.Header().Set("Content-Length", "1000")
			io.WriteString(w, "2026-10-15T12:00:00Z available\n")
		}
	}))
	a.buildx.Command = func(ctx context.Context, _ ...string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, "sh", "-c", "exit 0"), nil
	}
	msg := a.loadLogs().(listMsg)
	if msg.err == nil || !msg.partial || len(msg.items) != 1 || !strings.Contains(msg.err.Error(), "unexpected EOF") {
		t.Fatalf("scanner failure = %+v", msg)
	}
}

func TestLogTabReportsOversizedLines(t *testing.T) {
	rows, err := logTabItems(strings.NewReader("first\n"+strings.Repeat("x", 1024*1024)), "source", "web", false, time.Time{}, 0)
	if err == nil || len(rows) != 1 {
		t.Fatalf("oversized line: rows %d, err %v", len(rows), err)
	}
}
