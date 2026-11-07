// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import "testing"

// TestReviewFindingARM64StackCompareUnchecked shows that the post-link check
// of arm64 accepts a function whose stack compare (the word after the load of
// the stack guard) is replaced by a no-op. The check scans for the b.ls and
// never looks at the words between. It fails while the check has that gap.
func TestReviewFindingARM64StackCompareUnchecked(t *testing.T) {
	k, m, w := a64CheckCase()
	if err := k.function(m, 0x1000, a64Bytes(w)); err != nil {
		t.Fatalf("the function as made: %v", err)
	}
	w[1] = a64Nop // was cmp x16, sp
	err := k.function(m, 0x1000, a64Bytes(w))
	if err == nil {
		t.Errorf("a stack check whose compare is a no-op passed the check")
	}
}
