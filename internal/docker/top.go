// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"net/url"
)

// Processes is the process list of a container, as reported by ps inside
// the container's PID namespace.
type Processes struct {
	Titles    []string   `json:"Titles"`
	Processes [][]string `json:"Processes"`
}

// Top returns the processes running in a container.
func (c *Client) Top(ctx context.Context, id string) (Processes, error) {
	var p Processes
	err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/top", nil, &p)
	return p, err
}
