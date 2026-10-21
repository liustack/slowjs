// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build libc.memgrind

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"testing"

	"modernc.org/libc"
)

// Under libc's allocation tracker (make leak), the regexps of the
// transcripts and the tests of lreSkip and lreFirstSet leave no C memory
// behind. lreRun runs the backtracking machine once for each position it
// tries, and the machine grows its stack in C memory for the regexps that
// push many states.
func TestMemgrind2Regex(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	t.Run("semantics", TestRegexSemantics)
	t.Run("splitSemantics", TestRegexSplitSemantics)
	t.Run("skip", TestRegexSkip)
	t.Run("firstSet", TestRegexFirstSet)
	t.Run("interrupt", TestRegexSkipInterrupt)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
