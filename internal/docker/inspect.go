// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"net/url"
)

// InspectContainer returns the raw JSON description of a container.
func (c *Client) InspectContainer(ctx context.Context, id string) ([]byte, error) {
	return c.getRaw(ctx, "/containers/"+url.PathEscape(id)+"/json")
}

// InspectImage returns the raw JSON description of an image.
func (c *Client) InspectImage(ctx context.Context, id string) ([]byte, error) {
	return c.getRaw(ctx, "/images/"+url.PathEscape(id)+"/json")
}

// InspectVolume returns the raw JSON description of a volume.
func (c *Client) InspectVolume(ctx context.Context, name string) ([]byte, error) {
	return c.getRaw(ctx, "/volumes/"+url.PathEscape(name))
}

// InspectNetwork returns the raw JSON description of a network.
func (c *Client) InspectNetwork(ctx context.Context, id string) ([]byte, error) {
	return c.getRaw(ctx, "/networks/"+url.PathEscape(id))
}
