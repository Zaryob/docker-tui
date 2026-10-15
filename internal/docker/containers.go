// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"net/url"
	"strings"
)

// Port is a port mapping of a container.
type Port struct {
	IP          string `json:"IP"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort"`
	Type        string `json:"Type"`
}

// Container is the summary returned by the container list endpoint.
type Container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	Command string            `json:"Command"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Ports   []Port            `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
}

// Name returns the primary name of the container without the leading slash.
func (c Container) Name() string {
	if len(c.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// ListContainers returns the containers known to the daemon. When all is
// false only running containers are returned.
func (c *Client) ListContainers(ctx context.Context, all bool) ([]Container, error) {
	q := url.Values{}
	if all {
		q.Set("all", "1")
	}
	var out []Container
	err := c.getJSON(ctx, "/containers/json", q, &out)
	return out, err
}

func (c *Client) containerAction(ctx context.Context, id, action string) error {
	return c.post(ctx, "/containers/"+url.PathEscape(id)+"/"+action, nil)
}

// StartContainer starts a stopped container.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	return c.containerAction(ctx, id, "start")
}

// StopContainer stops a running container, killing it after the daemon's
// default grace period.
func (c *Client) StopContainer(ctx context.Context, id string) error {
	return c.containerAction(ctx, id, "stop")
}

// KillContainer sends SIGKILL to the main process of a running container.
func (c *Client) KillContainer(ctx context.Context, id string) error {
	return c.containerAction(ctx, id, "kill")
}

// RestartContainer restarts a container.
func (c *Client) RestartContainer(ctx context.Context, id string) error {
	return c.containerAction(ctx, id, "restart")
}

// RemoveContainer removes a container. With force, a running container is
// killed first.
func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	return c.delete(ctx, "/containers/"+url.PathEscape(id), q)
}

// PauseContainer suspends all processes in a container.
func (c *Client) PauseContainer(ctx context.Context, id string) error {
	return c.containerAction(ctx, id, "pause")
}

// UnpauseContainer resumes a paused container.
func (c *Client) UnpauseContainer(ctx context.Context, id string) error {
	return c.containerAction(ctx, id, "unpause")
}
