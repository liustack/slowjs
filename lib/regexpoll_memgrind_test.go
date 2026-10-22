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

// Under libc's allocation tracker (make leak), a regexp compile the
// interrupt handler stops leaves no C memory behind: the parser fails as
// for a syntax error and frees what it built, from lre_compile, new RegExp,
// a regexp literal and eval. The steps counted by the regexp cache, the
// parser and the string buffer leave nothing behind either.
func TestMemgrind2RegexPoll(t *testing.T) {
	// Force libc environ allocation that may otherwise skew the accounting.
	tls := libc.NewTLS()
	tls.Close()

	libc.MemAuditStart()
	t.Run("compilePoll", TestRegexCompilePoll)
	t.Run("cacheCounts", TestRegexCacheCountsSteps)
	t.Run("charge", TestChargeInterrupts)
	t.Run("chargeParseAndBuild", TestChargeParseAndBuild)
	if err := libc.MemAuditReport(); err != nil {
		t.Fatal(err)
	}
}
