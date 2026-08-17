// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"net/url"
)

// Network describes a Docker network.
type Network struct {
	ID       string            `json:"Id"`
	Name     string            `json:"Name"`
	Driver   string            `json:"Driver"`
	Scope    string            `json:"Scope"`
	Created  string            `json:"Created"`
	Internal bool              `json:"Internal"`
	IPAM     NetworkIPAM       `json:"IPAM"`
	Labels   map[string]string `json:"Labels"`
}

// NetworkIPAM holds the IP address management settings of a network.
type NetworkIPAM struct {
	Driver string `json:"Driver"`
	Config []struct {
		Subnet  string `json:"Subnet"`
		Gateway string `json:"Gateway"`
	} `json:"Config"`
}

// Subnet returns the first configured subnet of the network, if any.
func (n Network) Subnet() string {
	for _, c := range n.IPAM.Config {
		if c.Subnet != "" {
			return c.Subnet
		}
	}
	return ""
}

// IsPredefined reports whether the network is one of the networks created by
// the daemon itself, which cannot be removed.
func (n Network) IsPredefined() bool {
	switch n.Name {
	case "bridge", "host", "none":
		return true
	}
	return false
}

// ListNetworks returns the networks known to the daemon.
func (c *Client) ListNetworks(ctx context.Context) ([]Network, error) {
	var out []Network
	err := c.getJSON(ctx, "/networks", nil, &out)
	return out, err
}

// RemoveNetwork removes a network.
func (c *Client) RemoveNetwork(ctx context.Context, id string) error {
	return c.delete(ctx, "/networks/"+url.PathEscape(id), nil)
}
