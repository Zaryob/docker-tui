// Copyright (C) 2026 Suleyman Poyraz
// SPDX-License-Identifier: GPL-3.0-or-later

package ui

import "github.com/charmbracelet/x/vt"

func newTestEmulator(w, h int) *vt.SafeEmulator { return vt.NewSafeEmulator(w, h) }
