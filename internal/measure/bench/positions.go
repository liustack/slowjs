// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package bench builds the code-position variants of a program, times them in
// turn, and reads the result by the protocol of docs/measuring.md.
package bench

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Position is where the code of the engine sits in a binary: a layout.
type Position struct {
	// Shift moves the whole program after the first function of the engine by
	// this many bytes, a multiple of 32 below MaxShift. The first function after
	// it that is aligned to 64 bytes (the regexp matcher) takes up what is left
	// over of a shift that is not a multiple of 64, and everything after that
	// function, the interpreter too, moves by a multiple of 64. So a shift of
	// 32 may move nothing. The functions before the matcher move by exactly
	// the shift (see docs/measuring.md).
	Shift int
	// Front is the bytes of NOP at the start of the interpreter body, a multiple
	// of 8 from 0 to 56: the place of the body within its 64-byte block.
	Front int
	// Shift2 moves the code after the regexp matcher by this many bytes more, a
	// multiple of 32 below MaxShift: NOPs after the last RET of the matcher
	// (Matcher). The functions between the matcher and the interpreter, the
	// allocator and object creation among them, move by what the
	// matcher moved plus Shift2 exactly. Go starts a function on 32 bytes, so their place in a 64-byte
	// block takes two values, the formal one and 32 bytes later. The
	// interpreter is aligned to 64 bytes and moves by a multiple of 64 within 32
	// of the sum, so the code after it does not follow the half block.
	Shift2 int
}

// MaxShift is the limit of Shift.
const MaxShift = 8192

// Formal is the layout of the binary as it is built, with nothing moved.
var Formal = Position{}

// String is the name used in file names and records: "32.16", and "32.16.2208"
// with a second shift. A position without one keeps the two-number name it had
// before the second shift existed, so old records and binaries still match.
func (p Position) String() string {
	if p.Shift2 != 0 {
		return fmt.Sprintf("%d.%d.%d", p.Shift, p.Front, p.Shift2)
	}
	return fmt.Sprintf("%d.%d", p.Shift, p.Front)
}

// Matcher is the regexp matcher, the engine function that a second shift goes
// after. Its body is aligned to 64 bytes like the interpreter's, and it is the
// first of the two in the binary.
const Matcher = "lre_exec_backtrack"

// Quick is the set for a quick check: 0 or 32 for the whole program, 0 or 16
// for the interpreter body.
func Quick() []Position {
	return []Position{{0, 0, 0}, {0, 16, 0}, {32, 0, 0}, {32, 16, 0}}
}

// All is the older set of 8: shift 0 or 32 and front 0 to 24. It covers only
// half of the 64-byte block of the interpreter.
func All() []Position {
	var ps []Position
	for _, s := range []int{0, 32} {
		for _, f := range []int{0, 8, 16, 24} {
			ps = append(ps, Position{s, f, 0})
		}
	}
	return ps
}

// protocolShifts and protocolShifts2 are the shifts and second shifts of the 16
// layouts of Protocol, in its order: the 8 fronts of the first half, then the 8
// of the second. Formal is first, with neither.
//
// The engine has three sections, each with its own place modulo 64, and each
// can only be one of two since Go starts a function on 32 bytes:
//   - from the first engine function to the regexp matcher (the heap, libc,
//     the regexp compiler) moves by exactly the shift. The matcher takes up the
//     rest of a shift that is not a multiple of 64, so an odd multiple of 32
//     puts this section 32 bytes into the block and the matcher on a multiple
//     of 64.
//   - from the matcher to the interpreter (the allocator, object creation)
//     moves by what the matcher moved plus the second shift. The matcher moved
//     by a multiple of 64, so its place modulo 64 is that of the second shift.
//   - the interpreter and the code after it move by a multiple of 64 and then
//     take their place modulo 64 from the front.
//
// In each half, the shift and the second shift are each 32 bytes into a block
// in 4 layouts, and with the 3 bits of the front the 5 choices are independent
// of each other (any two both set in 2 layouts): the shift follows x0^x1 and
// the second shift x0^x2 of the number x of the front in the first half, x1^x2
// and x0^x1^x2 in the second. The shifts sit in 16 different places of the
// page, one in each 256 bytes, and so do the sums of the two shifts (the
// matcher may take 32 bytes of that off or add 32 to it). Only the formal
// layout has no shift, and no shift is 32, which the matcher may take up
// entirely.
var protocolShifts = []int{
	0, 1376, 2784, 896, 3584, 1952, 3168, 448,
	2176, 3328, 736, 2912, 1760, 3936, 1024, 2432,
}

var protocolShifts2 = []int{
	0, 1120, 2432, 2592, 2208, 896, 1632, 3456,
	4032, 1760, 2336, 1600, 1824, 1600, 1728, 3552,
}

// Protocol is the set of 16 layouts of docs/measuring.md: every front from 0
// to 56 twice, with the shifts and second shifts above. The first is the formal
// layout.
func Protocol() []Position {
	var ps []Position
	for i := range 16 {
		ps = append(ps, Position{protocolShifts[i], 8 * (i % 8), protocolShifts2[i]})
	}
	return ps
}

// Full is front 0 to 56 at shift 0 and at shift 32.
func Full() []Position {
	var ps []Position
	for _, s := range []int{0, 32} {
		for f := 0; f < 64; f += 8 {
			ps = append(ps, Position{s, f, 0})
		}
	}
	return ps
}

// ParsePosition parses "W.B".
func ParsePosition(s string) (Position, error) {
	f := strings.Split(s, ".")
	if len(f) != 2 && len(f) != 3 {
		return Position{}, fmt.Errorf("position %q: want W.B or W.B.S, such as 32.16 or 64.8.2208", s)
	}
	n := make([]int, len(f))
	for i, x := range f {
		var err error
		if n[i], err = strconv.Atoi(x); err != nil {
			return Position{}, fmt.Errorf("position %q: want W.B or W.B.S, such as 32.16 or 64.8.2208", s)
		}
	}
	shift, front, shift2 := n[0], n[1], 0
	if len(n) == 3 {
		if shift2 = n[2]; shift2 == 0 {
			return Position{}, fmt.Errorf("position %q: no second shift is written W.B, without the 0", s)
		}
	}
	if shift < 0 || shift >= MaxShift || shift%32 != 0 {
		return Position{}, fmt.Errorf("position %q: the whole program moves by a multiple of 32 bytes below %d (Go puts functions on 32, and a shift that is not a multiple of 64 may be taken up by the alignment)", s, MaxShift)
	}
	if front < 0 || front > 56 || front%8 != 0 {
		return Position{}, fmt.Errorf("position %q: the interpreter body moves by a multiple of 8 bytes from 0 to 56", s)
	}
	if shift2 < 0 || shift2 >= MaxShift || shift2%32 != 0 {
		return Position{}, fmt.Errorf("position %q: the code after the regexp matcher moves by a multiple of 32 bytes below %d more", s, MaxShift)
	}
	return Position{shift, front, shift2}, nil
}

// ParsePositions parses "protocol", "full", "quick", "all" or a comma list of
// positions.
func ParsePositions(s string) ([]Position, error) {
	switch s {
	case "quick":
		return Quick(), nil
	case "all":
		return All(), nil
	case "full":
		return Full(), nil
	case "", "protocol":
		return Protocol(), nil
	}
	var ps []Position
	for _, f := range strings.Split(s, ",") {
		p, err := ParsePosition(strings.TrimSpace(f))
		if err != nil {
			return nil, err
		}
		if slices.Contains(ps, p) {
			return nil, fmt.Errorf("position %s is repeated", p)
		}
		ps = append(ps, p)
	}
	return ps, nil
}

// XformOps returns the operations of internal/goasm/cmd/xform that make the
// position, with front the name of the interpreter function and pad the
// function after which the shift goes (see shift in xform).
func (p Position) XformOps(front, pad string) []string {
	shift := fmt.Sprintf("shift:%d", p.Shift)
	if pad != "" {
		shift += ":" + pad
	}
	ops := []string{shift}
	if p.Shift2 > 0 {
		ops = append(ops, fmt.Sprintf("shift:%d:%s", p.Shift2, Matcher))
	}
	return append(ops, fmt.Sprintf("front:%s:%d", front, p.Front))
}

// BinName is the file name of the binary of a side at a position.
func BinName(side string, p Position) string { return side + "@" + p.String() }
