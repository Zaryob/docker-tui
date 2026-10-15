// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package buildx

import (
	"os/exec"
	"syscall"
	"time"
)

// setProcessGroup runs cmd in its own process group and makes canceling it
// stop the whole group. The docker CLI runs buildx as a plugin process,
// which would otherwise keep running, and the output open, after the CLI
// is killed. SIGKILL is used because SIGTERM may be ignored when inherited
// as ignored; BuildKit cancels the build once the client is gone.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pgid := cmd.Process.Pid
		err := syscall.Kill(-pgid, syscall.SIGKILL)
		// A process being forked while the group is signaled can miss the
		// signal, so the group is signaled once more shortly after.
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}()
		return err
	}
}
