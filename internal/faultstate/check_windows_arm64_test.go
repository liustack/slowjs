// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package faultstate

import "testing"

// TestQuery: windows/arm64 answers the query, with a CONTEXT at least as large
// as one without extended state, and Check says what Decide says of it.
func TestQuery(t *testing.T) {
	mask, size, err := Query()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("extended CPU state %#x, CONTEXT with it %d bytes, with the dispatch %d of %d", mask, size, int(size)+Dispatch, StackSystem)
	if size < contextSize {
		t.Fatalf("InitializeContext2 gives %d bytes, less than a CONTEXT", size)
	}
	if got, want := Check(), Decide(mask, size, nil); (got == nil) != (want == nil) {
		t.Fatalf("Check = %v, Decide = %v", got, want)
	}
}
