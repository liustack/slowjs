// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"math"
	"strings"
	"testing"
	"unsafe"
)

// TestValuesFit: the engine counts at most math.MaxInt32 values, as a call,
// an array or the snapshot of the results of a Go function.
func TestValuesFit(t *testing.T) {
	for _, c := range []struct {
		have, n int
		ok      bool
	}{
		{0, 0, true},
		{0, math.MaxInt32, true},
		{0, math.MaxInt32 + 1, false},
		{math.MaxInt32 - 5, 5, true},
		{math.MaxInt32 - 5, 6, false},
		{math.MaxInt32, 0, true},
		{math.MaxInt32, 1, false},
		{0, math.MaxInt, false},
	} {
		err := valuesFit(c.have, c.n)
		if (err == nil) != c.ok {
			t.Errorf("valuesFit(%d, %d) = %v", c.have, c.n, err)
		}
		if err != nil && !strings.Contains(err.Error(), "the engine takes at most 2147483647") {
			t.Errorf("valuesFit(%d, %d): %v", c.have, c.n, err)
		}
	}
}

// TestSnapshotTooManyValues: a walk whose nodes or kids would pass what an
// int32 numbers fails with the error of valuesFit before an index is cut to
// 32 bits. The lengths are made to look that long over one element, which
// nothing reads: a snapshot that long would take tens of GiB.
func TestSnapshotTooManyValues(t *testing.T) {
	var kid snapKid
	w := refWalk{s: &snapshot{kids: unsafe.Slice(&kid, math.MaxInt32-2)}}
	if start := w.reserve(3); start != -1 || w.err == nil {
		t.Fatalf("reserve past MaxInt32 kids: %d, %v", start, w.err)
	}
	if !strings.Contains(w.err.Error(), "3 values, the engine takes at most") {
		t.Errorf("reserve: %v", w.err)
	}

	var nd snapNode
	w = refWalk{s: &snapshot{nodes: unsafe.Slice(&nd, math.MaxInt32)}}
	if i := w.add(snapNode{kind: snapNull}); i != 0 || w.err == nil {
		t.Fatalf("add past MaxInt32 nodes: %d, %v", i, w.err)
	}
	if len(w.s.nodes) != math.MaxInt32 {
		t.Errorf("add appended past the limit: %d nodes", len(w.s.nodes))
	}
}
