// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package docker implements a small client for the Docker Engine HTTP API.
//
// Only the endpoints needed by docker-tui are implemented. The client talks
// to the daemon directly over its unix socket, so no Docker SDK is required.
package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultHost is the address used when neither an explicit host nor the
// DOCKER_HOST environment variable is set.
const DefaultHost = "unix:///var/run/docker.sock"

// Client is a minimal Docker Engine API client.
type Client struct {
	http *http.Client
	base string
	host string
}

// NewClient returns a client for the daemon at host, which is either a
// "unix://" socket path or a "tcp://" address. An empty host falls back to
// the DOCKER_HOST environment variable and then to DefaultHost.
func NewClient(host string) (*Client, error) {
	if host == "" {
		host = os.Getenv("DOCKER_HOST")
	}
	if host == "" {
		host = defaultHost()
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("invalid docker host %q: %w", host, err)
	}

	transport := &http.Transport{}
	base := "http://docker"
	switch u.Scheme {
	case "unix":
		socket := u.Path
		if socket == "" {
			return nil, fmt.Errorf("invalid docker host %q: missing socket path", host)
		}
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		}
	case "tcp", "http":
		if u.Host == "" {
			return nil, fmt.Errorf("invalid docker host %q: missing address", host)
		}
		base = "http://" + u.Host
	default:
		return nil, fmt.Errorf("unsupported docker host %q: only unix:// and tcp:// are supported", host)
	}
	return &Client{
		http: &http.Client{Transport: transport},
		base: base,
		host: host,
	}, nil
}

// defaultHost returns DefaultHost, unless its socket does not exist and one
// of the well-known per-user sockets does (Docker Desktop, Colima, rootless
// Docker).
func defaultHost() string {
	if _, err := os.Stat(strings.TrimPrefix(DefaultHost, "unix://")); err == nil {
		return DefaultHost
	}
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".docker", "run", "docker.sock"),
			filepath.Join(home, ".colima", "default", "docker.sock"),
		)
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "docker.sock"))
	}
	for _, path := range candidates {
		if fi, err := os.Stat(path); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return "unix://" + path
		}
	}
	return DefaultHost
}

// Host returns the daemon address the client connects to.
func (c *Client) Host() string { return c.host }

// APIError is returned when the daemon answers with a non-2xx status.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("docker: unexpected status %d", e.StatusCode)
	}
	return e.Message
}

// IsNotFound reports whether err is a 404 returned by the daemon.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Response, error) {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, decodeError(resp)
	}
	return resp, nil
}

func decodeError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var payload struct {
		Message string `json:"message"`
	}
	msg := strings.TrimSpace(string(data))
	if json.Unmarshal(data, &payload) == nil && payload.Message != "" {
		msg = payload.Message
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg}
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	resp, err := c.do(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) getRaw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, path, query, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) post(ctx context.Context, path string, query url.Values) error {
	resp, err := c.do(ctx, http.MethodPost, path, query, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) delete(ctx context.Context, path string, query url.Values) error {
	resp, err := c.do(ctx, http.MethodDelete, path, query, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Ping checks that the daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.getRaw(ctx, "/_ping", nil)
	return err
}

// Version describes the daemon version.
type Version struct {
	Version       string `json:"Version"`
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	Os            string `json:"Os"`
	Arch          string `json:"Arch"`
	KernelVersion string `json:"KernelVersion"`
}

// ServerVersion returns version information about the daemon.
func (c *Client) ServerVersion(ctx context.Context) (Version, error) {
	var v Version
	err := c.getJSON(ctx, "/version", nil, &v)
	return v, err
}
