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

// Under libc's allocation tracker (make leak), the tests of objlitDefine,
// the one that runs out of memory too, leave no C memory behind.
func TestMemgrind2Objlit(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	t.Run("semantics", TestObjlitSemantics)
	t.Run("fastPath", TestObjlitFastPath)
	t.Run("refuses", TestObjlitDefineRefuses)
	t.Run("outOfMemory", TestObjlitDefineOutOfMemory)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
