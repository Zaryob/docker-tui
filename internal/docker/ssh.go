// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// sshCommand creates the ssh process of a connection; tests replace it.
var sshCommand = func(args ...string) *exec.Cmd { return exec.Command("ssh", args...) }

// sshArgs returns the arguments of ssh for an "ssh://[user@]host[:port]"
// address. Like the docker CLI, the remote "docker system dial-stdio"
// relays the connection to the daemon, so ssh keys, agents and
// ~/.ssh/config apply as usual.
func sshArgs(u *url.URL) ([]string, error) {
	if u.Hostname() == "" {
		return nil, errors.New("missing host")
	}
	if u.Path != "" && u.Path != "/" {
		return nil, errors.New("paths are not supported in ssh addresses")
	}
	args := []string{"-o", "ConnectTimeout=30", "-T"}
	if user := u.User.Username(); user != "" {
		args = append(args, "-l", user)
	}
	if p := u.Port(); p != "" {
		args = append(args, "-p", p)
	}
	return append(args, "--", u.Hostname(), "docker", "system", "dial-stdio"), nil
}

// dialSSH starts ssh and returns its standard input and output as a
// connection to the daemon.
func dialSSH(ctx context.Context, args []string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := sshCommand(args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	stderr := &lastLine{}
	cmd.Stderr = stderr

	stop := context.AfterFunc(ctx, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer stop()

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("ssh: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = cmd.Process.Kill()
		stdin.Close()
		stdout.Close()
		return nil, err
	}
	return &cmdConn{cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr}, nil
}

// cmdConn is a connection over the standard streams of a process.
type cmdConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *lastLine
	once   sync.Once
}

func (c *cmdConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if errors.Is(err, io.EOF) && n == 0 {
		// The connection ended; ssh tells why, e.g. a denied key.
		if msg := c.stderr.String(); msg != "" {
			return 0, fmt.Errorf("ssh: %s", msg)
		}
	}
	return n, err
}

func (c *cmdConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

// CloseWrite ends the input of the remote side.
func (c *cmdConn) CloseWrite() error { return c.stdin.Close() }

func (c *cmdConn) Close() error {
	c.once.Do(func() {
		c.stdin.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		go func() { _ = c.cmd.Wait() }()
	})
	return nil
}

func (c *cmdConn) LocalAddr() net.Addr              { return sshAddr{} }
func (c *cmdConn) RemoteAddr() net.Addr             { return sshAddr{} }
func (c *cmdConn) SetDeadline(time.Time) error      { return nil }
func (c *cmdConn) SetReadDeadline(time.Time) error  { return nil }
func (c *cmdConn) SetWriteDeadline(time.Time) error { return nil }

type sshAddr struct{}

func (sshAddr) Network() string { return "ssh" }
func (sshAddr) String() string  { return "ssh" }

// lastLine keeps the last non-empty line written to it.
type lastLine struct {
	mu   sync.Mutex
	buf  []byte
	line string
}

func (l *lastLine) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, p...)
	for {
		i := strings.IndexByte(string(l.buf), '\n')
		if i < 0 {
			break
		}
		if s := strings.TrimSpace(string(l.buf[:i])); s != "" {
			l.line = s
		}
		l.buf = l.buf[i+1:]
	}
	return len(p), nil
}

func (l *lastLine) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if s := strings.TrimSpace(string(l.buf)); s != "" {
		return s
	}
	return l.line
}
