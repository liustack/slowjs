// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build libc.memgrind

package quickjs

import (
	"testing"

	"modernc.org/libc"
)

func TestMemgrind2YoungGC(t *testing.T) {
	tls := libc.NewTLS()
	tls.Close()
	libc.MemAuditStart()
	t.Run("reachable", TestYoungGCKeepsReachable)
	t.Run("cycles", TestYoungGCFreesCycles)
	t.Run("old", TestYoungGCFreesWhatOnlyGarbageHeld)
	t.Run("ephemeron", TestYoungGCEphemeron)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
