// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestClient starts an HTTP server with handler and returns a client
// connected to it over TCP.
func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestNewClientHosts(t *testing.T) {
	tests := []struct {
		host    string
		wantErr bool
	}{
		{"unix:///var/run/docker.sock", false},
		{"tcp://127.0.0.1:2375", false},
		{"unix://", true},
		{"tcp://", true},
		{"npipe:////./pipe/docker_engine", true},
		{"ssh://user@host", true},
	}
	for _, tt := range tests {
		_, err := NewClient(tt.host)
		if (err != nil) != tt.wantErr {
			t.Errorf("NewClient(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
		}
	}
}

func TestNewClientUsesDockerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://10.0.0.1:2375")
	c, err := NewClient("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Host() != "tcp://10.0.0.1:2375" {
		t.Errorf("Host() = %q", c.Host())
	}
}

func TestUnixSocket(t *testing.T) {
	// Socket paths are limited to ~104 bytes on macOS, so avoid t.TempDir.
	dir, err := os.MkdirTemp("", "dtui")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "docker.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_ping" {
			io.WriteString(w, "OK")
			return
		}
		http.NotFound(w, r)
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })

	c, err := NewClient("unix://" + sock)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping over unix socket: %v", err)
	}
}

func TestListContainers(t *testing.T) {
	var gotAll string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/json" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		gotAll = r.URL.Query().Get("all")
		writeJSON(w, []map[string]any{
			{"Id": "abc123", "Names": []string{"/web"}, "Image": "nginx", "State": "running"},
		})
	}))

	list, err := c.ListContainers(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if gotAll != "1" {
		t.Errorf("all = %q, want 1", gotAll)
	}
	if len(list) != 1 || list[0].Name() != "web" || list[0].State != "running" {
		t.Errorf("unexpected containers: %+v", list)
	}

	if _, err := c.ListContainers(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if gotAll != "" {
		t.Errorf("all = %q, want empty", gotAll)
	}
}

func TestAPIError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]string{"message": "No such container: nope"})
	}))
	err := c.StartContainer(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "No such container: nope" {
		t.Errorf("error = %q", err)
	}
	if !IsNotFound(err) {
		t.Error("IsNotFound = false, want true")
	}
}

func TestContainerActions(t *testing.T) {
	var got []string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		w.WriteHeader(http.StatusNoContent)
	}))
	ctx := context.Background()
	steps := []func() error{
		func() error { return c.StartContainer(ctx, "x") },
		func() error { return c.StopContainer(ctx, "x") },
		func() error { return c.RestartContainer(ctx, "x") },
		func() error { return c.KillContainer(ctx, "x") },
		func() error { return c.PauseContainer(ctx, "x") },
		func() error { return c.UnpauseContainer(ctx, "x") },
		func() error { return c.RemoveContainer(ctx, "x", true) },
		func() error { return c.RemoveImage(ctx, "sha256:1", false) },
		func() error { return c.RemoveVolume(ctx, "data", false) },
		func() error { return c.RemoveNetwork(ctx, "n1") },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"POST /containers/x/start?",
		"POST /containers/x/stop?",
		"POST /containers/x/restart?",
		"POST /containers/x/kill?",
		"POST /containers/x/pause?",
		"POST /containers/x/unpause?",
		"DELETE /containers/x?force=1",
		"DELETE /images/sha256:1?",
		"DELETE /volumes/data?",
		"DELETE /networks/n1?",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLogsDemultiplexes(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/x/json":
			writeJSON(w, map[string]any{"Config": map[string]any{"Tty": false}})
		case "/containers/x/logs":
			if r.URL.Query().Get("tail") != "10" {
				t.Errorf("tail = %q", r.URL.Query().Get("tail"))
			}
			w.Write(frame(StreamStdout, "out\n"))
			w.Write(frame(StreamStderr, "err\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	rc, err := c.Logs(context.Background(), "x", LogOptions{Tail: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "out\nerr\n" {
		t.Errorf("logs = %q", data)
	}
}

func TestLogsTTY(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/x/json":
			writeJSON(w, map[string]any{"Config": map[string]any{"Tty": true}})
		case "/containers/x/logs":
			io.WriteString(w, "raw output\n")
		}
	}))
	rc, err := c.Logs(context.Background(), "x", LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "raw output\n" {
		t.Errorf("logs = %q", data)
	}
}

func TestListVolumesAndNetworks(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/volumes":
			writeJSON(w, map[string]any{"Volumes": []map[string]any{{"Name": "data", "Driver": "local"}}})
		case "/networks":
			writeJSON(w, []map[string]any{{
				"Id": "n1", "Name": "bridge", "Driver": "bridge",
				"IPAM": map[string]any{"Config": []map[string]any{{"Subnet": "172.17.0.0/16"}}},
			}})
		}
	}))
	vols, err := c.ListVolumes(context.Background())
	if err != nil || len(vols) != 1 || vols[0].Name != "data" {
		t.Errorf("ListVolumes = %+v, %v", vols, err)
	}
	nets, err := c.ListNetworks(context.Background())
	if err != nil || len(nets) != 1 {
		t.Fatalf("ListNetworks = %+v, %v", nets, err)
	}
	if nets[0].Subnet() != "172.17.0.0/16" || !nets[0].IsPredefined() {
		t.Errorf("unexpected network %+v", nets[0])
	}
}

func TestTop(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/containers/x/top" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		writeJSON(w, map[string]any{
			"Titles":    []string{"UID", "PID", "CMD"},
			"Processes": [][]string{{"root", "1", "nginx: master process"}},
		})
	}))
	p, err := c.Top(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Titles) != 3 || len(p.Processes) != 1 || p.Processes[0][2] != "nginx: master process" {
		t.Errorf("unexpected processes %+v", p)
	}
}

func TestEvents(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("filters"); got != `{"type":["container","image"]}` {
			t.Errorf("filters = %q", got)
		}
		writeJSON(w, map[string]any{"Type": "container", "Action": "start", "Actor": map[string]any{"ID": "abc"}})
		writeJSON(w, map[string]any{"Type": "image", "Action": "delete", "Actor": map[string]any{"ID": "sha256:1"}})
	}))
	s, err := c.Events(context.Background(), "container", "image")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var got []string
	for {
		ev, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, ev.Type+" "+ev.Action+" "+ev.Actor.ID)
	}
	if want := "container start abc,image delete sha256:1"; strings.Join(got, ",") != want {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestPrune(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/images/prune" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, map[string]any{
			"ImagesDeleted":  []map[string]string{{"Deleted": "sha256:1"}, {"Untagged": "foo:latest"}},
			"SpaceReclaimed": 4096,
		})
	}))
	r, err := c.PruneImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Deleted != 2 || r.SpaceReclaimed != 4096 {
		t.Errorf("report = %+v", r)
	}
}
