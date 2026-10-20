// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullUsesMatchingCredentialsAndRedactsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	t.Setenv("DOCKER_AUTH_CONFIG", "")
	encoded := base64.StdEncoding.EncodeToString([]byte("alice:private-secret"))
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"auths":{"private.example":{"auth":"`+encoded+`"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := base64.URLEncoding.DecodeString(r.Header.Get("X-Registry-Auth"))
		if err != nil {
			t.Error("invalid registry auth header")
			return
		}
		var auth struct{ Username, Password, ServerAddress string }
		if err := json.Unmarshal(data, &auth); err != nil {
			t.Error(err)
			return
		}
		if r.URL.Query().Get("fromImage") == "private.example/app" {
			if auth.Username != "alice" || auth.Password != "private-secret" || auth.ServerAddress != "private.example" {
				t.Error("missing target registry credentials")
			}
			writeJSON(w, map[string]string{"status": "working private-secret"})
			writeJSON(w, map[string]string{"error": "unauthorized private-secret " + encoded})
		} else {
			if auth.Username != "" || auth.Password != "" {
				t.Error("credentials sent for a different registry")
			}
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]string{"message": "authentication required"})
		}
	}))
	stream, err := c.PullImage(context.Background(), "private.example/app:latest")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	progress, err := stream.Next()
	if err != nil || strings.Contains(progress.Status, "private-secret") {
		t.Fatal("progress leaked secret")
	}
	progress, err = stream.Next()
	if err == nil || !strings.Contains(err.Error(), "ctrl+l") || strings.Contains(err.Error(), "private-secret") || strings.Contains(err.Error(), encoded) || strings.Contains(progress.Error, "private-secret") {
		t.Fatalf("unsafe or unhelpful auth failure: %v", err)
	}
	_, err = c.PullImage(context.Background(), "other.example/app")
	if err == nil || !strings.Contains(err.Error(), "ctrl+l") {
		t.Fatal("missing authentication recovery guidance")
	}
}
