// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ExecConfig describes a command to run in a container.
type ExecConfig struct {
	Cmd []string `json:"Cmd"`
	Env []string `json:"Env,omitempty"`
	// Tty allocates a pseudo terminal, which interactive shells need.
	Tty          bool `json:"Tty"`
	AttachStdin  bool `json:"AttachStdin"`
	AttachStdout bool `json:"AttachStdout"`
	AttachStderr bool `json:"AttachStderr"`
	// ConsoleSize is the initial height and width of the TTY.
	ConsoleSize *[2]uint `json:"ConsoleSize,omitempty"`
}

// CreateExec prepares a command to run in a running container and returns
// the ID of the exec instance.
func (c *Client) CreateExec(ctx context.Context, container string, cfg ExecConfig) (string, error) {
	body, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/containers/"+url.PathEscape(container)+"/exec", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", decodeError(resp)
	}
	var out struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// hijackedConn is a connection taken over from HTTP. Reads first drain what
// the HTTP response reader buffered.
type hijackedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *hijackedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// CloseWrite closes the sending side, telling the process that its input
// ended, when the transport supports it.
func (c *hijackedConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// StartExec starts an exec instance and returns a connection attached to
// it: writes go to the standard input of the process and reads return its
// output. With a TTY the output is the raw terminal stream. The caller must
// close the connection.
func (c *Client) StartExec(ctx context.Context, execID string, tty bool) (io.ReadWriteCloser, error) {
	body, err := json.Marshal(map[string]bool{"Detach": false, "Tty": tty})
	if err != nil {
		return nil, err
	}
	return c.hijack(ctx, "/exec/"+url.PathEscape(execID)+"/start", nil, body)
}

// hijack sends a POST request that the daemon answers by handing the
// connection over to a raw stream, and returns that connection.
func (c *Client) hijack(ctx context.Context, path string, query url.Values, body []byte) (*hijackedConn, error) {
	if c.dial == nil {
		return nil, fmt.Errorf("interactive sessions are not supported for %s", c.host)
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	u := "http://docker" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		conn.Close()
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		conn.Close()
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusSwitchingProtocols, http.StatusOK:
	default:
		err := decodeError(resp)
		resp.Body.Close()
		conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return &hijackedConn{Conn: conn, r: br}, nil
}

// AttachInfo describes the main process of a container for attaching.
type AttachInfo struct {
	// Tty reports whether the container has a terminal; without one, the
	// output is plain lines.
	Tty bool
	// OpenStdin reports whether the process reads standard input.
	OpenStdin bool
	Running   bool
	ExitCode  int
}

// ContainerAttachInfo returns what attaching to a container offers.
func (c *Client) ContainerAttachInfo(ctx context.Context, id string) (AttachInfo, error) {
	var info struct {
		Config struct {
			Tty       bool `json:"Tty"`
			OpenStdin bool `json:"OpenStdin"`
		} `json:"Config"`
		State struct {
			Running  bool `json:"Running"`
			ExitCode int  `json:"ExitCode"`
		} `json:"State"`
	}
	err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/json", nil, &info)
	return AttachInfo{
		Tty: info.Config.Tty, OpenStdin: info.Config.OpenStdin,
		Running: info.State.Running, ExitCode: info.State.ExitCode,
	}, err
}

// AttachContainer connects to the main process of a container, like
// "docker attach": writes go to its standard input, if it is open, and reads
// return its output. Without a TTY the daemon multiplexes stdout and stderr,
// which is undone here. Closing the connection detaches without stopping
// the container.
func (c *Client) AttachContainer(ctx context.Context, id string, tty bool) (io.ReadWriteCloser, error) {
	q := url.Values{}
	for _, k := range []string{"stream", "stdin", "stdout", "stderr"} {
		q.Set(k, "1")
	}
	conn, err := c.hijack(ctx, "/containers/"+url.PathEscape(id)+"/attach", q, nil)
	if err != nil {
		return nil, err
	}
	if tty {
		return conn, nil
	}
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(Demux(conn, pw, pw))
	}()
	return &demuxConn{Reader: pr, conn: conn}, nil
}

// demuxConn reads the demultiplexed output of a connection and writes to
// the connection itself.
type demuxConn struct {
	io.Reader
	conn net.Conn
}

func (c *demuxConn) Write(p []byte) (int, error) { return c.conn.Write(p) }
func (c *demuxConn) Close() error                { return c.conn.Close() }

// ResizeContainer changes the size of the TTY of a container.
func (c *Client) ResizeContainer(ctx context.Context, id string, width, height int) error {
	q := url.Values{}
	q.Set("w", strconv.Itoa(width))
	q.Set("h", strconv.Itoa(height))
	return c.post(ctx, "/containers/"+url.PathEscape(id)+"/resize", q)
}

// ResizeExec changes the size of the TTY of an exec instance.
func (c *Client) ResizeExec(ctx context.Context, execID string, width, height int) error {
	q := url.Values{}
	q.Set("w", strconv.Itoa(width))
	q.Set("h", strconv.Itoa(height))
	return c.post(ctx, "/exec/"+url.PathEscape(execID)+"/resize", q)
}

// ExecInfo describes the state of an exec instance.
type ExecInfo struct {
	Running  bool `json:"Running"`
	ExitCode int  `json:"ExitCode"`
}

// InspectExec returns the state of an exec instance, e.g. the exit code of
// its process once it ended.
func (c *Client) InspectExec(ctx context.Context, execID string) (ExecInfo, error) {
	var info ExecInfo
	err := c.getJSON(ctx, "/exec/"+url.PathEscape(execID)+"/json", nil, &info)
	return info, err
}
