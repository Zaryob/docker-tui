// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// Event is a state change reported by the daemon, e.g. a container being
// started or an image being removed.
type Event struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
	TimeNano int64 `json:"timeNano"`
}

// EventStream is an open stream of daemon events.
type EventStream struct {
	body io.ReadCloser
	dec  *json.Decoder
}

// Events subscribes to the daemon events of the given object types
// ("container", "image", ...). All types are reported when none is given.
// The stream stays open until ctx is canceled or Close is called.
func (c *Client) Events(ctx context.Context, types ...string) (*EventStream, error) {
	q := url.Values{}
	if len(types) > 0 {
		filters, err := json.Marshal(map[string][]string{"type": types})
		if err != nil {
			return nil, err
		}
		q.Set("filters", string(filters))
	}
	resp, err := c.do(ctx, http.MethodGet, "/events", q)
	if err != nil {
		return nil, err
	}
	return &EventStream{body: resp.Body, dec: json.NewDecoder(resp.Body)}, nil
}

// Next blocks until the next event arrives. It returns io.EOF when the
// daemon closes the stream.
func (s *EventStream) Next() (Event, error) {
	var ev Event
	err := s.dec.Decode(&ev)
	return ev, err
}

// Close ends the stream.
func (s *EventStream) Close() error { return s.body.Close() }
