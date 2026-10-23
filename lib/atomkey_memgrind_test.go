// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build libc.memgrind

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"testing"

	"modernc.org/libc"
)

// Under libc's allocation tracker (make leak), the tests of atomKey leave
// no C memory behind.
func TestMemgrind2AtomKey(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	t.Run("counts", TestAtomKeyCounts)
	t.Run("leaves", TestAtomKeyLeaves)
	t.Run("madeAgain", TestAtomKeyMadeAgain)
	t.Run("symbolText", TestAtomKeySymbolText)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
