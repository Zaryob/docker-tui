// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// LogOptions controls which logs are returned.
type LogOptions struct {
	// Follow keeps the stream open and returns new output as it arrives.
	Follow bool
	// Tail limits the output to the last n lines. Zero or less means all.
	Tail int
	// Timestamps prefixes every line with an RFC3339Nano timestamp.
	Timestamps bool
}

func (c *Client) containerTTY(ctx context.Context, id string) (bool, error) {
	var info struct {
		Config struct {
			Tty bool `json:"Tty"`
		} `json:"Config"`
	}
	err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/json", nil, &info)
	return info.Config.Tty, err
}

type logReader struct {
	io.Reader
	closers []io.Closer
}

func (r *logReader) Close() error {
	var first error
	for _, c := range r.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Logs returns the combined stdout and stderr output of a container as plain
// text. The caller must close the returned reader.
func (c *Client) Logs(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error) {
	tty, err := c.containerTTY(ctx, id)
	if err != nil {
		return nil, err
	}

	q := url.Values{}
	q.Set("stdout", "1")
	q.Set("stderr", "1")
	if opts.Follow {
		q.Set("follow", "1")
	}
	if opts.Tail > 0 {
		q.Set("tail", strconv.Itoa(opts.Tail))
	}
	if opts.Timestamps {
		q.Set("timestamps", "1")
	}
	resp, err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/logs", q, nil)
	if err != nil {
		return nil, err
	}
	if tty {
		return resp.Body, nil
	}

	pr, pw := io.Pipe()
	go func() {
		err := Demux(resp.Body, pw, pw)
		resp.Body.Close()
		pw.CloseWithError(err)
	}()
	return &logReader{Reader: pr, closers: []io.Closer{pr, resp.Body}}, nil
}
