// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package main

import (
	"bytes"
	"os"
	"testing"
)

// TestTableInTree checks the formulas of dtoa.c and that engine/dtoa-pow10.h
// is what the command writes.
func TestTableInTree(t *testing.T) {
	var b bytes.Buffer
	if err := write(&b); err != nil {
		t.Fatal(err)
	}
	have, err := os.ReadFile("../../engine/dtoa-pow10.h")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(have, b.Bytes()) {
		t.Fatal("engine/dtoa-pow10.h differs from what go run ./internal/dtoatable writes: write it again")
	}
}

// TestFloorLog checks the exact floor of a logarithm, and that the check of
// the formulas can fail: a wrong constant is found.
func TestFloorLog(t *testing.T) {
	for _, c := range []struct {
		base int64
		e    int
	}{{10, 0}, {10, 1}, {10, -1}, {2, 0}, {2, 127}, {2, -1074}} {
		x := pow(c.base, c.e)
		if got := floorLog(c.base, x); got != c.e {
			t.Errorf("floorLog(%d, %d^%d) = %d", c.base, c.base, c.e, got)
		}
		x.Sub(x, pow(c.base, c.e-60))
		if got := floorLog(c.base, x); got != c.e-1 {
			t.Errorf("floorLog(%d, %d^%d - a little) = %d", c.base, c.base, c.e, got)
		}
	}
	off := func(f func(int) int, at int) func(int) int {
		return func(e int) int {
			if e == at {
				return f(e) + 1
			}
			return f(e)
		}
	}
	if checkFormulasOf(off(floorLog10Pow2, -1074), floorLog10ThreeQuartersPow2, floorLog2Pow10) == nil ||
		checkFormulasOf(floorLog10Pow2, off(floorLog10ThreeQuartersPow2, 971), floorLog2Pow10) == nil ||
		checkFormulasOf(floorLog10Pow2, floorLog10ThreeQuartersPow2, off(floorLog2Pow10, 0)) == nil {
		t.Error("a formula one off at one exponent passes the check")
	}
}
