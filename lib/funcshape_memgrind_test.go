// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build libc.memgrind

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"testing"

	"modernc.org/libc"
)

// Under libc's allocation tracker (make leak), the tests of funcShape,
// with the shapes held for contexts, leave no C memory behind.
func TestMemgrind2FuncShape(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	t.Run("semantics", TestFuncShapeSemantics)
	t.Run("remembers", TestFuncShapeRemembers)
	t.Run("refuses", TestFuncShapeRefuses)
	t.Run("free", TestFuncShapesFree)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
