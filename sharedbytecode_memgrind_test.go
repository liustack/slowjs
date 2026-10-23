// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build libc.memgrind

package slowjs

import (
	"testing"

	"modernc.org/libc"
)

// Shared bytecode is freed once, after the last VM that loaded it, and no
// VM frees the source text or line tables that point into it.
func TestMemgrind2SharedBytecode(t *testing.T) {
	tls := libc.NewTLS()
	tls.Close()
	libc.MemAuditStart()
	t.Run("sameAsCopy", TestSharedBytecodeSameAsCopy)
	t.Run("lifetime", TestSharedBytecodeLifetime)
	t.Run("truncated", TestSharedBytecodeTruncated)
	t.Run("concurrent", TestSharedBytecodeConcurrent)
	t.Run("module", TestSharedBytecodeModule)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
