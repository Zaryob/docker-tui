// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/zaryob/docker-tui/internal/registry"
)

// PullProgress is a progress message of an image pull. Messages with an ID
// describe one layer; the others describe the whole pull.
type PullProgress struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Progress string `json:"progress"`
	Detail   struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail"`
	Error string `json:"error"`
}

// PullStream is the progress of a running pull.
type PullStream struct {
	auth registry.Auth
	body io.ReadCloser
	dec  *json.Decoder
}

// SplitReference splits an image reference into the image and the tag or
// digest the pull API expects. A missing tag means "latest".
func SplitReference(ref string) (image, tag string) {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, "@"); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	// A colon after the last slash separates the tag; one before it
	// belongs to a registry port, as in localhost:5000/app.
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[:i], ref[i+1:]
	}
	return ref, "latest"
}

// PullImage starts pulling an image, e.g. "alpine" or
// "ghcr.io/owner/app:1.2". Canceling ctx or closing the stream stops it.
func (c *Client) PullImage(ctx context.Context, ref string) (*PullStream, error) {
	image, tag := SplitReference(ref)
	if image == "" {
		return nil, errors.New("no image given")
	}
	q := url.Values{}
	q.Set("fromImage", image)
	q.Set("tag", tag)
	auth, err := (&registry.Client{}).Credentials(ctx, ref)
	if err != nil {
		return nil, err
	}
	resp, err := c.doHeaders(ctx, http.MethodPost, "/images/create", q, nil, "", http.Header{"X-Registry-Auth": []string{auth.Header()}})
	if err != nil {
		return nil, pullError(err, auth)
	}
	return &PullStream{auth: auth, body: resp.Body, dec: json.NewDecoder(resp.Body)}, nil
}

// Next returns the next progress message. It returns io.EOF when the pull
// completed, and the error reported by the daemon when it failed.
func (s *PullStream) Next() (PullProgress, error) {
	var p PullProgress
	if err := s.dec.Decode(&p); err != nil {
		return p, err
	}
	p.ID = s.auth.Redact(p.ID)
	p.Status = s.auth.Redact(p.Status)
	p.Progress = s.auth.Redact(p.Progress)
	p.Error = s.auth.Redact(p.Error)
	if p.Error != "" {
		return p, pullError(errors.New(p.Error), s.auth)
	}
	return p, nil
}

// Close stops reading the progress, which cancels the pull.
func (s *PullStream) Close() error { return s.body.Close() }

func pullError(err error, auth registry.Auth) error {
	message := auth.Redact(err.Error())
	lower := strings.ToLower(message)
	var apiErr *APIError
	if (errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden)) || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "access denied") || strings.Contains(lower, "authentication required") || strings.Contains(lower, "no basic auth credentials") {
		return fmt.Errorf("%s; use ctrl+l in Images to log in, then retry the pull", message)
	}
	return errors.New(message)
}
