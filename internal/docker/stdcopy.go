// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Stream identifiers used in multiplexed attach/logs streams.
const (
	StreamStdin  = 0
	StreamStdout = 1
	StreamStderr = 2
	// StreamSystem carries an error message from the daemon (API >= 1.42).
	StreamSystem = 3
)

const stdHeaderLen = 8

// Demux splits a multiplexed stream, as returned by the logs and attach
// endpoints for containers without a TTY, into stdout and stderr.
//
// Each frame starts with an 8 byte header: one byte stream id, three bytes of
// padding and a big-endian uint32 payload length.
func Demux(r io.Reader, stdout, stderr io.Writer) error {
	var header [stdHeaderLen]byte
	buf := make([]byte, 32*1024)
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		size := int(binary.BigEndian.Uint32(header[4:]))
		var w io.Writer
		switch header[0] {
		case StreamStdin, StreamStdout:
			w = stdout
		case StreamStderr:
			w = stderr
		case StreamSystem:
			msg := make([]byte, size)
			if _, err := io.ReadFull(r, msg); err != nil {
				return err
			}
			return fmt.Errorf("docker: %s", msg)
		default:
			return fmt.Errorf("docker: unknown stream id %d in multiplexed stream", header[0])
		}
		for size > 0 {
			n := size
			if n > len(buf) {
				n = len(buf)
			}
			if _, err := io.ReadFull(r, buf[:n]); err != nil {
				return err
			}
			if _, err := w.Write(buf[:n]); err != nil {
				return err
			}
			size -= n
		}
	}
}
