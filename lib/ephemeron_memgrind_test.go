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

func TestMemgrind2Ephemeron(t *testing.T) {
	tls := libc.NewTLS()
	tls.Close()
	libc.MemAuditStart()
	t.Run("graph", TestEphemeron)
	t.Run("observers", TestEphemeronWeakObservers)
	t.Run("symbols", TestEphemeronLiveSymbols)
	t.Run("mutation", TestEphemeronMutationAfterGC)
	t.Run("unregister", TestEphemeronRegistryUnregister)
	t.Run("graphs", TestEphemeronMixedGraph)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
