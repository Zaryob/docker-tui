// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func configured(t *testing.T, data string) *Client {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_AUTH_CONFIG", "")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return &Client{ConfigDir: dir}
}

func TestFileCredentialsAreRegistryScoped(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("alice:p:a:ss"))
	c := configured(t, `{"auths":{"https://index.docker.io/v1/":{"auth":"`+encoded+`"},"https://private.example/v1/":{"identitytoken":"private-token"}}}`)
	for _, ref := range []string{"alpine", "library/alpine:3.21", "docker.io/library/alpine", "index.docker.io/library/alpine"} {
		auth, err := c.Credentials(context.Background(), ref)
		if err != nil || auth.Username != "alice" || auth.Password != "p:a:ss" || auth.ServerAddress != hubKey {
			t.Fatalf("%s: credentials mismatch, error %v", ref, err)
		}
	}
	auth, err := c.Credentials(context.Background(), "private.example/app:tag")
	if err != nil || auth.IdentityToken != "private-token" || auth.Password != "" {
		t.Fatalf("private credentials mismatch: %v", err)
	}
	auth, err = c.Credentials(context.Background(), "unrelated.example/app")
	if err != nil || auth.Username != "" || auth.IdentityToken != "" {
		t.Fatal("credentials crossed registries")
	}
}

func TestHelpersOverrideFileCredentialsAndStore(t *testing.T) {
	c := configured(t, `{"credsStore":"global","credHelpers":{"private.example":"specific"},"auths":{"private.example":{"username":"stale","password":"stale"}}}`)
	var name string
	var args []string
	c.Command = func(ctx context.Context, executable string, argv ...string) *exec.Cmd {
		name, args = executable, argv
		return exec.CommandContext(ctx, "sh", "-c", `read -r server; test "$server" = private.example || exit 1; echo '{"Username":"<token>","Secret":"helper-token"}'`)
	}
	auth, err := c.Credentials(context.Background(), "private.example/app")
	if err != nil || name != "docker-credential-specific" || !slices.Equal(args, []string{"get"}) || auth.IdentityToken != "helper-token" || auth.Password != "" {
		t.Fatalf("helper selection/result mismatch: %s %q, %v", name, args, err)
	}
	c.Command = func(ctx context.Context, executable string, _ ...string) *exec.Cmd {
		name = executable
		return exec.CommandContext(ctx, "sh", "-c", `echo 'credentials not found in native keychain'; exit 1`)
	}
	auth, err = c.Credentials(context.Background(), "other.example/app")
	if err != nil || name != "docker-credential-global" || auth.Password != "" {
		t.Fatalf("missing credential should allow public pull: %v", err)
	}
	c.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `echo 'sensitive-helper-output'; exit 1`)
	}
	_, err = c.Credentials(context.Background(), "private.example/app")
	if err == nil || strings.Contains(err.Error(), "sensitive-helper-output") {
		t.Fatal("helper error output leaked")
	}
}

func TestHubHelperKeyAndEnvironmentOverride(t *testing.T) {
	c := configured(t, `{"credsStore":"native"}`)
	c.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `read -r server; test "$server" = 'https://index.docker.io/v1/' || exit 1; echo '{"Username":"alice","Secret":"secret"}'`)
	}
	if auth, err := c.Credentials(context.Background(), "alpine"); err != nil || auth.Password != "secret" {
		t.Fatalf("Hub helper: %v", err)
	}
	t.Setenv("DOCKER_AUTH_CONFIG", `{"auths":{"private.example":{"auth":"`+base64.StdEncoding.EncodeToString([]byte("env:token"))+`"}}}`)
	c.Command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("environment credential should override helper")
		return nil
	}
	if auth, err := c.Credentials(context.Background(), "private.example/app"); err != nil || auth.Username != "env" || auth.Password != "token" {
		t.Fatalf("environment override: %v", err)
	}
}

func TestMalformedCredentialsDoNotDiscloseContents(t *testing.T) {
	for _, data := range []string{`{"auths":{"private.example":{"auth":"secret-invalid-base64"}}}`, `{"auths":{"private.example":{"auth":42}}}`, `{"credsStore":"../../evil"}`} {
		c := configured(t, data)
		_, err := c.Credentials(context.Background(), "private.example/app")
		if err == nil || strings.Contains(err.Error(), "secret-invalid") {
			t.Fatalf("unsafe config error: %v", err)
		}
	}
}

func TestLoginUsesStdinAndSelectedDaemon(t *testing.T) {
	c := &Client{ConfigDir: t.TempDir(), Context: "remote", Host: "tcp://wrong:2375"}
	const secret = "token-with-$-and-'"
	var args []string
	c.Command = func(ctx context.Context, name string, argv ...string) *exec.Cmd {
		if name != "docker" {
			t.Fatal(name)
		}
		args = argv
		if strings.Contains(strings.Join(args, " "), secret) {
			t.Fatal("password in command arguments")
		}
		return exec.CommandContext(ctx, "sh", "-c", `test "$DOCKER_CONTEXT" = remote && test -z "$DOCKER_HOST" || exit 1; IFS= read -r password; test -n "$password"`)
	}
	if err := c.Login(context.Background(), "private.example", "alice", secret); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(args, []string{"--config", c.ConfigDir, "login", "--username", "alice", "--password-stdin", "private.example"}) {
		t.Fatal(args)
	}
	c.Command = func(ctx context.Context, _ string, argv ...string) *exec.Cmd {
		args = argv
		return exec.CommandContext(ctx, "sh", "-c", `cat >/dev/null; echo sensitive-cli-output; exit 1`)
	}
	if err := c.Login(context.Background(), "private.example", "alice", secret); err == nil || strings.Contains(err.Error(), "sensitive-cli-output") {
		t.Fatalf("unsafe login error: %v", err)
	}
	c.Command = func(ctx context.Context, _ string, argv ...string) *exec.Cmd {
		args = argv
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}
	if err := c.Logout(context.Background(), "private.example"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(args, []string{"--config", c.ConfigDir, "logout", "private.example"}) {
		t.Fatal(args)
	}
}

func TestCredentialHelperCancellation(t *testing.T) {
	c := configured(t, `{"credsStore":"slow"}`)
	c.Command = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "sleep 30")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Credentials(ctx, "alpine"); err == nil {
		t.Fatal("canceled helper succeeded")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("helper did not cancel promptly")
	}
}

func TestHeaderAndRedaction(t *testing.T) {
	auth := Auth{Username: "alice", Password: "secret", IdentityToken: "token"}
	data, err := base64.URLEncoding.DecodeString(auth.Header())
	if err != nil {
		t.Fatal(err)
	}
	var payload Auth
	if err := json.Unmarshal(data, &payload); err != nil || payload.Password != "secret" {
		t.Fatal("invalid Engine auth payload")
	}
	message := auth.Redact("secret token " + auth.Header() + " " + base64.StdEncoding.EncodeToString([]byte("alice:secret")))
	for _, secret := range []string{"secret", "token", auth.Header()} {
		if strings.Contains(message, secret) {
			t.Fatal("credential leaked in diagnostic")
		}
	}
	if auth.String() != "[registry credentials]" {
		t.Fatal("unsafe formatted credentials")
	}
}

func TestRegistryValidation(t *testing.T) {
	for _, server := range []string{"user:pass@host", "https://example.com", "host/path", "--config", "host name", "host\nname"} {
		if _, err := Server(server); err == nil {
			t.Fatalf("accepted invalid server %q", server)
		}
	}
	for _, server := range []string{"localhost:5000", "ghcr.io", "[::1]:5000", "docker.io"} {
		if _, err := Server(server); err != nil {
			t.Fatalf("valid server rejected: %v", err)
		}
	}
}
