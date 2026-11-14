// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// TestValuesFit: the engine counts at most math.MaxInt32 values, as a call,
// an array or the snapshot of the results of a Go function.
func TestValuesFit(t *testing.T) {
	if maxValues != math.MaxInt32 {
		t.Fatalf("maxValues is %d", maxValues)
	}
	for _, c := range []struct {
		have, n int
		ok      bool
	}{
		{0, 0, true},
		{0, maxValues, true},
		{0, maxValues + 1, false},
		{maxValues - 5, 5, true},
		{maxValues - 5, 6, false},
		{maxValues, 0, true},
		{maxValues, 1, false},
		{0, math.MaxInt, false},
	} {
		err := valuesFit(c.have, c.n, maxValues)
		if (err == nil) != c.ok {
			t.Errorf("valuesFit(%d, %d) = %v", c.have, c.n, err)
		}
		if err != nil && !strings.Contains(err.Error(), "the engine takes at most 2147483647") {
			t.Errorf("valuesFit(%d, %d): %v", c.have, c.n, err)
		}
	}
}

// TestSnapshotTooManyValues: a walk of more nodes or kids than its max
// fails with the error of valuesFit before an index is cut to 32 bits, and
// still takes every reference the results hand over, for the snapshot to
// free. The walk is given a small max: a snapshot of math.MaxInt32 values
// would take tens of GiB.
func TestSnapshotTooManyValues(t *testing.T) {
	m := newTestVM(t)
	vals := func(n int) []Value {
		a := make([]Value, n)
		for i := range a {
			a[i] = mustString(t, m, "v")
		}
		return a
	}
	asAny := func(a []Value) []any {
		r := make([]any, len(a))
		for i, v := range a {
			r[i] = v
		}
		return r
	}
	asMap := func(a []Value) map[string]any {
		r := make(map[string]any, len(a))
		for i, v := range a {
			r[string(rune('a'+i))] = v
		}
		return r
	}
	asIntMap := func(a []Value) map[int]Value {
		r := make(map[int]Value, len(a))
		for i, v := range a {
			r[i] = v
		}
		return r
	}
	for _, c := range []struct {
		name string
		max  int
		refs int
		v    func(a []Value) any
	}{
		// 5 kids past a max of 4
		{"kids of a slice", 4, 5, func(a []Value) any { return a }},
		{"kids of a []any", 4, 5, func(a []Value) any { return asAny(a) }},
		{"kids of a map[string]any", 4, 5, func(a []Value) any { return asMap(a) }},
		{"kids of a map[int]Value", 4, 5, func(a []Value) any { return asIntMap(a) }},
		// 3 kids, and 3 nodes of values and one of the array past a max of 3
		{"nodes of a slice", 3, 3, func(a []Value) any { return a }},
		{"nodes of a []any", 3, 3, func(a []Value) any { return asAny(a) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := m.takeSnapshot()
			w := refWalk{m: m, s: s, max: c.max}
			w.node(reflect.ValueOf(c.v(vals(c.refs))), false)
			if w.err == nil || !strings.Contains(w.err.Error(), "values, the engine takes at most "+string(rune('0'+c.max))) {
				t.Errorf("walk past %d: %v", c.max, w.err)
			}
			if len(s.nodes) > c.max || len(s.kids) > c.max {
				t.Errorf("walk past %d: %d nodes, %d kids", c.max, len(s.nodes), len(s.kids))
			}
			if len(s.held) != c.refs {
				t.Errorf("walk past %d: %d references taken of %d", c.max, len(s.held), c.refs)
			}
			m.releaseSnapshot(s)
		})
	}
}
