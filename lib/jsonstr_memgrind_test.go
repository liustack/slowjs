// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build libc.memgrind

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"testing"

	"modernc.org/libc"
)

// Under libc's allocation tracker (make leak), the tests of jsonQuoteRun
// and jsonStringRun, the one that runs out of memory too, leave no C memory
// behind.
func TestMemgrind2JSONStrings(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	t.Run("strings", TestJSONStrings)
	t.Run("errors", TestJSONStringErrors)
	t.Run("bytes", TestJSONStringBytes)
	t.Run("outOfMemory", TestJSONStringsOutOfMemory)
	t.Run("parseRuns", TestJSONStringRunStaysInBuffer)
	t.Run("quoteRuns", TestJSONQuoteRunStaysInBuffer)
	t.Run("runsOutOfMemory", TestJSONRunsOutOfMemory)
	t.Run("tooLong", TestJSONRunTooLong)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
