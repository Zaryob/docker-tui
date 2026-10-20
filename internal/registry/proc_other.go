// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package registry

import "os/exec"

func setProcessGroup(*exec.Cmd) {}
