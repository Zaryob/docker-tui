// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"net/url"
)

// Image is the summary returned by the image list endpoint.
type Image struct {
	ID          string            `json:"Id"`
	ParentID    string            `json:"ParentId"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	Containers  int64             `json:"Containers"`
	Labels      map[string]string `json:"Labels"`
}

// Tag returns the first repository tag of the image, or "<none>:<none>" for
// dangling images.
func (i Image) Tag() string {
	for _, t := range i.RepoTags {
		if t != "<none>:<none>" {
			return t
		}
	}
	return "<none>:<none>"
}

// ListImages returns the top-level images known to the daemon.
func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	var out []Image
	err := c.getJSON(ctx, "/images/json", nil, &out)
	return out, err
}

// RemoveImage removes an image. With force, the image is removed even if it
// is referenced by stopped containers or has multiple tags.
func (c *Client) RemoveImage(ctx context.Context, id string, force bool) error {
	q := url.Values{}
	if force {
		q.Set("force", "1")
	}
	return c.delete(ctx, "/images/"+url.PathEscape(id), q)
}
