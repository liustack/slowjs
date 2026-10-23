// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build libc.memgrind

package libquickjs

import (
	"modernc.org/libc"
	"testing"
)

func TestMemgrind2YoungGC(t *testing.T) {
	tls := libc.NewTLS()
	tls.Close()
	libc.MemAuditStart()
	t.Run("lists", TestYoungGCLists)
	t.Run("zero", TestYoungGCLeavesNoZeroCount)
	t.Run("ephemeron", TestYoungGCEphemeron)
	t.Run("graphs", TestYoungGCMixedGraph)
	t.Run("observers", TestYoungGCWeakObservers)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
