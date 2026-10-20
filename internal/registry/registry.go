// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package registry reuses Docker's credential configuration and helper protocol.
package registry

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

const hubKey = "https://index.docker.io/v1/"

// Client reads Docker credentials and delegates login/logout storage to its CLI.
type Client struct {
	// Host and Context select the same daemon for CLI authentication as the TUI.
	Host    string
	Context string
	// ConfigDir overrides DOCKER_CONFIG, primarily for isolated integrations.
	ConfigDir string
	// Command overrides executable creation for tests.
	Command func(context.Context, string, ...string) *exec.Cmd
}

// Auth is the Engine API registry authentication payload. Never log its fields.
type Auth struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Auth          string `json:"auth,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
	RegistryToken string `json:"registrytoken,omitempty"`
	ServerAddress string `json:"serveraddress,omitempty"`
}

// String prevents accidental credential disclosure in formatted diagnostics.
func (Auth) String() string { return "[registry credentials]" }

// Header returns the URL-safe base64 JSON expected by X-Registry-Auth.
func (a Auth) Header() string {
	data, _ := json.Marshal(a)
	return base64.URLEncoding.EncodeToString(data)
}

// Redact removes known credential representations from daemon diagnostics.
func (a Auth) Redact(value string) string {
	secrets := []string{a.Password, a.IdentityToken, a.RegistryToken, a.Auth, a.Header()}
	if a.Username != "" && a.Password != "" {
		secrets = append(secrets, base64.StdEncoding.EncodeToString([]byte(a.Username+":"+a.Password)))
	}
	// Replace longer representations first, including the full encoded header.
	slices.SortFunc(secrets, func(a, b string) int { return len(b) - len(a) })
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	return value
}

// Server validates a login target and normalizes Docker Hub aliases.
func Server(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "docker.io", nil
	}
	if value == hubKey || value == strings.TrimSuffix(hubKey, "/") {
		return "docker.io", nil
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.Contains(value, "@") {
		return "", errors.New("enter a registry hostname with an optional port")
	}
	u, err := url.Parse("https://" + value)
	if err != nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.HasPrefix(value, "-") {
		return "", errors.New("enter a registry hostname with an optional port, without a URL scheme or path")
	}
	host := strings.ToLower(u.Host)
	if host == "index.docker.io" || host == "registry-1.docker.io" {
		host = "docker.io"
	}
	return host, nil
}

func imageServer(ref string) string {
	first, _, hasSlash := strings.Cut(ref, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost" || first != strings.ToLower(first)) {
		if server, err := Server(first); err == nil {
			return server
		}
	}
	return "docker.io"
}

func credentialKey(server string) string {
	if server == "docker.io" {
		return hubKey
	}
	return server
}
func keyServer(key string) string {
	if key == "" {
		return ""
	}
	if strings.Contains(key, "://") {
		if u, err := url.Parse(key); err == nil {
			key = u.Host
		}
	}
	server, _ := Server(strings.TrimSuffix(key, "/"))
	return server
}

func (c *Client) configDir() (string, error) {
	if c.ConfigDir != "" {
		return c.ConfigDir, nil
	}
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("cannot locate Docker credential configuration")
	}
	return filepath.Join(home, ".docker"), nil
}

func (c *Client) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	var cmd *exec.Cmd
	if c.Command != nil {
		cmd = c.Command(ctx, name, args...)
	} else {
		cmd = exec.CommandContext(ctx, name, args...)
	}
	if c.Host != "" || c.Context != "" {
		cmd.Env = slices.DeleteFunc(os.Environ(), func(kv string) bool {
			return strings.HasPrefix(kv, "DOCKER_HOST=") || strings.HasPrefix(kv, "DOCKER_CONTEXT=")
		})
		if c.Context != "" {
			cmd.Env = append(cmd.Env, "DOCKER_CONTEXT="+c.Context)
		} else {
			cmd.Env = append(cmd.Env, "DOCKER_HOST="+c.Host)
		}
	}
	cmd.WaitDelay = time.Second
	setProcessGroup(cmd)
	return cmd
}

type config struct {
	Auths   map[string]Auth   `json:"auths"`
	Store   string            `json:"credsStore"`
	Helpers map[string]string `json:"credHelpers"`
}

// Credentials resolves only the target registry. Registry-specific helpers take
// precedence over credsStore, which takes precedence over file credentials.
func (c *Client) Credentials(ctx context.Context, ref string) (Auth, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	dir, err := c.configDir()
	if err != nil {
		return Auth{}, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Auth{}, errors.New("cannot read Docker credential configuration")
	}
	var cfg config
	if len(data) > 0 && json.Unmarshal(data, &cfg) != nil {
		return Auth{}, errors.New("invalid Docker credential configuration")
	}
	server := imageServer(ref)
	key := credentialKey(server)
	// Recent Docker CLIs also accept an in-memory credential configuration.
	if env := os.Getenv("DOCKER_AUTH_CONFIG"); env != "" {
		var override config
		if json.Unmarshal([]byte(env), &override) != nil {
			return Auth{}, errors.New("invalid DOCKER_AUTH_CONFIG")
		}
		if auth, ok := findAuth(override.Auths, server); ok {
			return decodeAuth(auth, key)
		}
	}
	helper := cfg.Store
	if specific, ok := cfg.Helpers[key]; ok {
		helper = specific
	} else if specific, ok := cfg.Helpers[server]; ok {
		helper = specific
	}
	if helper != "" {
		// A configured suffix names an executable in PATH, never a path or option.
		if strings.ContainsAny(helper, "/\\") || strings.TrimSpace(helper) != helper {
			return Auth{}, errors.New("invalid Docker credential helper name")
		}
		cmd := c.command(ctx, "docker-credential-"+helper, "get")
		cmd.Stdin = strings.NewReader(key)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return Auth{}, ctx.Err()
			}
			if strings.TrimSpace(output.String()) == "credentials not found in native keychain" {
				return Auth{}, nil
			}
			return Auth{}, errors.New("cannot read Docker credential helper; check the configured helper or log in again")
		}
		var credential struct {
			Username string
			Secret   string
		}
		if json.Unmarshal(output.Bytes(), &credential) != nil {
			return Auth{}, errors.New("invalid response from Docker credential helper")
		}
		auth := Auth{ServerAddress: key}
		if credential.Username == "<token>" {
			auth.IdentityToken = credential.Secret
		} else {
			auth.Username, auth.Password = credential.Username, credential.Secret
		}
		return auth, nil
	}
	auth, _ := findAuth(cfg.Auths, server)
	return decodeAuth(auth, key)
}

func findAuth(auths map[string]Auth, server string) (Auth, bool) {
	if auth, ok := auths[credentialKey(server)]; ok {
		return auth, true
	}
	if auth, ok := auths[server]; ok {
		return auth, true
	}
	// Legacy Docker configs use URL-form keys for private registries too.
	keys := make([]string, 0, len(auths))
	for key := range auths {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if keyServer(key) == server {
			return auths[key], true
		}
	}
	return Auth{}, false
}

func decodeAuth(auth Auth, key string) (Auth, error) {
	if auth.Auth != "" {
		data, err := base64.StdEncoding.DecodeString(auth.Auth)
		if err != nil {
			return Auth{}, errors.New("invalid encoded Docker registry credentials")
		}
		username, password, ok := strings.Cut(string(data), ":")
		if !ok || username == "" {
			return Auth{}, errors.New("invalid encoded Docker registry credentials")
		}
		auth.Username, auth.Password = username, strings.Trim(password, "\x00")
		auth.Auth = ""
	}
	if auth.Username != "" || auth.Password != "" || auth.IdentityToken != "" || auth.RegistryToken != "" {
		auth.ServerAddress = key
	}
	return auth, nil
}

// Login authenticates and stores credentials using Docker's configured store.
// Passwords travel only through stdin; CLI output is never exposed to the UI.
func (c *Client) Login(ctx context.Context, server, username, password string) error {
	server, err := Server(server)
	if err != nil {
		return err
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return errors.New("username and password or access token are required")
	}
	return c.change(ctx, "login", server, username, password)
}

// Logout removes the target registry credentials through Docker's CLI.
func (c *Client) Logout(ctx context.Context, server string) error {
	server, err := Server(server)
	if err != nil {
		return err
	}
	return c.change(ctx, "logout", server, "", "")
}

func (c *Client) change(ctx context.Context, action, server, username, password string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	args := []string{}
	if c.ConfigDir != "" {
		args = append(args, "--config", c.ConfigDir)
	}
	args = append(args, action)
	if action == "login" {
		args = append(args, "--username", username, "--password-stdin")
	}
	args = append(args, server)
	cmd := c.command(ctx, "docker", args...)
	if action == "login" {
		cmd.Stdin = strings.NewReader(password + "\n")
	}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("docker CLI is required for registry login/logout")
		}
		return fmt.Errorf("registry %s failed; check credentials, registry connectivity and Docker credential-store settings", action)
	}
	return nil
}
