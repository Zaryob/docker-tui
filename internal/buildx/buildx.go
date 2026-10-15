// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

// Package buildx runs image builds and reads the build history through the
// docker CLI. BuildKit builds use a gRPC session that the Engine HTTP API
// alone does not provide, so the CLI is used instead of the API.
package buildx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"
)

// CLI runs docker CLI commands against one daemon.
type CLI struct {
	// Host is the daemon address passed to the CLI as DOCKER_HOST.
	Host string
	// Context, when set, is selected instead of passing Host, so that the
	// CLI also uses its TLS material.
	Context string
	// Command creates the command to run; it defaults to exec.CommandContext
	// with the docker binary found in PATH. Tests replace it.
	Command func(ctx context.Context, args ...string) (*exec.Cmd, error)
}

// ErrNoCLI is returned when the docker CLI is not installed.
var ErrNoCLI = errors.New("docker CLI not found in PATH; it is required for builds")

func (c *CLI) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if c.Command != nil {
		var err error
		if cmd, err = c.Command(ctx, args...); err != nil {
			return nil, err
		}
	} else {
		bin, err := exec.LookPath("docker")
		if err != nil {
			return nil, ErrNoCLI
		}
		cmd = exec.CommandContext(ctx, bin, args...)
	}
	cmd.Env = EnvFor(os.Environ(), c.Host, c.Context)
	setProcessGroup(cmd)
	// Do not wait for leftover processes holding the output open.
	cmd.WaitDelay = 5 * time.Second
	return cmd, nil
}

// Env returns environ with DOCKER_HOST set to host, so that the CLI talks to
// the same daemon as docker-tui. A context selection, which could point
// elsewhere, is dropped.
func Env(environ []string, host string) []string { return EnvFor(environ, host, "") }

// EnvFor is like Env, but selects the CLI context ctxName instead of the
// host when it is set.
func EnvFor(environ []string, host, ctxName string) []string {
	env := slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
		return strings.HasPrefix(kv, "DOCKER_CONTEXT=") || strings.HasPrefix(kv, "DOCKER_HOST=")
	})
	switch {
	case ctxName != "":
		env = append(env, "DOCKER_CONTEXT="+ctxName)
	case host != "":
		env = append(env, "DOCKER_HOST="+host)
	}
	return env
}

// output runs a command and returns its standard output. On failure the
// error carries the last line written to standard error.
func (c *CLI) output(ctx context.Context, args ...string) ([]byte, error) {
	cmd, err := c.command(ctx, args...)
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := lastLine(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return out, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Record is an entry of the build history.
type Record struct {
	// Ref is the full reference "builder/node/id" of the build.
	Ref            string    `json:"ref"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	CompletedAt    time.Time `json:"completed_at"`
	TotalSteps     int       `json:"total_steps"`
	CompletedSteps int       `json:"completed_steps"`
	CachedSteps    int       `json:"cached_steps"`
}

// ID returns the build ID, the last element of the reference, which the
// history commands accept.
func (r Record) ID() string { return path.Base(r.Ref) }

// Duration returns how long the build took, or has been running at now.
func (r Record) Duration(now time.Time) time.Duration {
	if r.CompletedAt.IsZero() {
		return now.Sub(r.CreatedAt)
	}
	return r.CompletedAt.Sub(r.CreatedAt)
}

// History returns the build records, newest first.
func (c *CLI) History(ctx context.Context) ([]Record, error) {
	out, err := c.output(ctx, "buildx", "history", "ls", "--no-trunc", "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseHistory(out)
}

// parseHistory decodes the output of "history ls --format json", which is
// one JSON object per line.
func parseHistory(data []byte) ([]Record, error) {
	var records []Record
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, fmt.Errorf("build history: %w", err)
		}
		records = append(records, r)
	}
	return records, sc.Err()
}

// Inspect returns the description of a build as JSON.
func (c *CLI) Inspect(ctx context.Context, id string) ([]byte, error) {
	return c.output(ctx, "buildx", "history", "inspect", "--format", "json", id)
}

// Remove deletes a build record.
func (c *CLI) Remove(ctx context.Context, id string) error {
	_, err := c.output(ctx, "buildx", "history", "rm", id)
	return err
}

// RemoveAll deletes every build record.
func (c *CLI) RemoveAll(ctx context.Context) error {
	_, err := c.output(ctx, "buildx", "history", "rm", "--all")
	return err
}

// Logs returns the plain progress output of a finished build.
func (c *CLI) Logs(ctx context.Context, id string) (io.ReadCloser, error) {
	return c.stream(ctx, "buildx", "history", "logs", "--progress", "plain", id)
}

// BuildOptions describes an image build.
type BuildOptions struct {
	// Context is the build context directory.
	Context string
	// Dockerfile is the path of the Dockerfile; empty means
	// Context/Dockerfile.
	Dockerfile string
	// Tags name the resulting image.
	Tags []string
}

// Args returns the CLI arguments of the build.
func (o BuildOptions) Args() []string {
	args := []string{"build", "--progress", "plain"}
	for _, t := range o.Tags {
		args = append(args, "--tag", t)
	}
	if o.Dockerfile != "" {
		args = append(args, "--file", o.Dockerfile)
	}
	return append(args, o.Context)
}

// Build starts an image build and returns its combined output. Reading
// returns the error of the build, if any, once the output ends. Closing the
// reader or canceling ctx aborts the build.
func (c *CLI) Build(ctx context.Context, opts BuildOptions) (io.ReadCloser, error) {
	return c.stream(ctx, opts.Args()...)
}

// stream runs a command and returns its combined standard output and error.
func (c *CLI) stream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd, err := c.command(ctx, args...)
	if err != nil {
		cancel()
		return nil, err
	}
	pr, pw := io.Pipe()
	// Keep the last line of output for the error message. Both streams use
	// the same writer, so exec serializes the writes.
	tail := &tailWriter{}
	cmd.Stdout = io.MultiWriter(pw, tail)
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	go func() {
		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			if msg := tail.last(); msg != "" {
				err = fmt.Errorf("%s (%w)", msg, err)
			}
		}
		pw.CloseWithError(err)
	}()
	return &streamReader{PipeReader: pr, cancel: cancel}, nil
}

type streamReader struct {
	*io.PipeReader
	cancel context.CancelFunc
}

func (r *streamReader) Close() error {
	r.cancel()
	return r.PipeReader.Close()
}

// tailWriter remembers the last non-empty line written to it.
type tailWriter struct {
	buf  []byte
	line string
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		if l := strings.TrimSpace(string(w.buf[:i])); l != "" {
			w.line = l
		}
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

func (w *tailWriter) last() string {
	if l := strings.TrimSpace(string(w.buf)); l != "" {
		return l
	}
	return w.line
}
