// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
)

// PathStat describes a path inside a container.
type PathStat struct {
	Name string      `json:"name"`
	Size int64       `json:"size"`
	Mode fs.FileMode `json:"mode"`
}

// StatPath returns information about a path in a container. The error
// satisfies IsNotFound when the path does not exist.
func (c *Client) StatPath(ctx context.Context, id, path string) (PathStat, error) {
	q := url.Values{}
	q.Set("path", path)
	resp, err := c.do(ctx, http.MethodHead, "/containers/"+url.PathEscape(id)+"/archive", q)
	if err != nil {
		return PathStat{}, err
	}
	resp.Body.Close()
	var st PathStat
	data, err := base64.StdEncoding.DecodeString(resp.Header.Get("X-Docker-Container-Path-Stat"))
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

// CopyFromContainer returns a tar stream of a file or directory in a
// container, like "docker cp container:path ...".
func (c *Client) CopyFromContainer(ctx context.Context, id, path string) (io.ReadCloser, error) {
	q := url.Values{}
	q.Set("path", path)
	resp, err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/archive", q)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// CopyToContainer unpacks a tar stream into dir, an existing directory in a
// container.
func (c *Client) CopyToContainer(ctx context.Context, id, dir string, content io.Reader) error {
	q := url.Values{}
	q.Set("path", dir)
	resp, err := c.doBody(ctx, http.MethodPut, "/containers/"+url.PathEscape(id)+"/archive", q, content, "application/x-tar")
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
