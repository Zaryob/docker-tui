// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"encoding/json"
	"net/http"
)

// PruneReport summarizes the result of a prune operation.
type PruneReport struct {
	// Deleted is the number of removed objects.
	Deleted int
	// SpaceReclaimed is the disk space freed, in bytes.
	SpaceReclaimed uint64
}

func (c *Client) prune(ctx context.Context, path string) (PruneReport, error) {
	resp, err := c.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return PruneReport{}, err
	}
	defer resp.Body.Close()
	var out struct {
		ContainersDeleted []string         `json:"ContainersDeleted"`
		ImagesDeleted     []map[string]any `json:"ImagesDeleted"`
		VolumesDeleted    []string         `json:"VolumesDeleted"`
		NetworksDeleted   []string         `json:"NetworksDeleted"`
		SpaceReclaimed    uint64           `json:"SpaceReclaimed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return PruneReport{}, err
	}
	return PruneReport{
		Deleted: len(out.ContainersDeleted) + len(out.ImagesDeleted) +
			len(out.VolumesDeleted) + len(out.NetworksDeleted),
		SpaceReclaimed: out.SpaceReclaimed,
	}, nil
}

// PruneContainers removes all stopped containers.
func (c *Client) PruneContainers(ctx context.Context) (PruneReport, error) {
	return c.prune(ctx, "/containers/prune")
}

// PruneImages removes dangling images.
func (c *Client) PruneImages(ctx context.Context) (PruneReport, error) {
	return c.prune(ctx, "/images/prune")
}

// PruneVolumes removes anonymous volumes not used by any container.
func (c *Client) PruneVolumes(ctx context.Context) (PruneReport, error) {
	return c.prune(ctx, "/volumes/prune")
}

// PruneNetworks removes networks not used by any container.
func (c *Client) PruneNetworks(ctx context.Context) (PruneReport, error) {
	return c.prune(ctx, "/networks/prune")
}
