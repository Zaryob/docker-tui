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
	"strings"
	"time"
)

// DefaultSocket is the default location of the Docker daemon socket.
const DefaultSocket = "/var/run/docker.sock"

// Client is a minimal Docker Engine API client.
type Client struct {
	http *http.Client
	base string
}

// NewClient returns a client connected to the daemon listening on the given
// unix socket path.
func NewClient(socket string) *Client {
	if socket == "" {
		socket = DefaultSocket
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}
	return &Client{
		http: &http.Client{Transport: transport},
		base: "http://docker",
	}
}

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
