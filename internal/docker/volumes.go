// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"net/url"
)

// Volume describes a Docker volume.
type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	CreatedAt  string            `json:"CreatedAt"`
	Scope      string            `json:"Scope"`
	Labels     map[string]string `json:"Labels"`
}

// ListVolumes returns the volumes known to the daemon.
func (c *Client) ListVolumes(ctx context.Context) ([]Volume, error) {
	var out struct {
		Volumes []Volume `json:"Volumes"`
	}
	err := c.getJSON(ctx, "/volumes", nil, &out)
	return out.Volumes, err
}

// RemoveVolume removes a volume.
func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	return c.delete(ctx, "/volumes/"+url.PathEscape(name), q)
}
