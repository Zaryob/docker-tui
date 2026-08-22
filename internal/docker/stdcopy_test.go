// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package docker

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func frame(stream byte, payload string) []byte {
	h := make([]byte, stdHeaderLen, stdHeaderLen+len(payload))
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}

func TestDemux(t *testing.T) {
	var in bytes.Buffer
	in.Write(frame(StreamStdout, "hello\n"))
	in.Write(frame(StreamStderr, "oops\n"))
	in.Write(frame(StreamStdout, strings.Repeat("x", 40000)))

	var stdout, stderr bytes.Buffer
	if err := Demux(&in, &stdout, &stderr); err != nil {
		t.Fatalf("Demux: %v", err)
	}
	if want := "hello\n" + strings.Repeat("x", 40000); stdout.String() != want {
		t.Errorf("stdout has %d bytes, want %d", stdout.Len(), len(want))
	}
	if stderr.String() != "oops\n" {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestDemuxSystemError(t *testing.T) {
	in := bytes.NewReader(frame(StreamSystem, "container gone"))
	err := Demux(in, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "container gone") {
		t.Fatalf("Demux error = %v, want system error", err)
	}
}

func TestDemuxTruncated(t *testing.T) {
	data := frame(StreamStdout, "hello")
	err := Demux(bytes.NewReader(data[:len(data)-2]), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Demux of truncated frame succeeded, want error")
	}
}
