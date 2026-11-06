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
	// 32 may move nothing, and the protocol uses multiples of 64 (see
	// docs/measuring.md).
	Shift int
	// Front is the bytes of NOP at the start of the interpreter body, a multiple
	// of 8 from 0 to 56: the place of the body within its 64-byte block.
	Front int
	// Shift2 moves the code after the regexp matcher by this many bytes more, a
	// multiple of 32 below MaxShift: NOPs after the last RET of the matcher
	// (Matcher). The functions between the matcher and the interpreter, the
	// allocator and object creation among them, move by Shift plus Shift2
	// exactly. Go starts a function on 32 bytes, so their place in a 64-byte
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

// protocolShifts are the shifts of the second half of Protocol, one per front.
// They are multiples of 64, so that the interpreter moves by exactly the shift
// whatever the padding in front of it. They sit in 8 different 64-byte blocks
// of a 4 KiB page, none further than 576 bytes from the next one, and do not
// grow with the front, so that the place in the page and the place of the
// interpreter in its block change independently.
var protocolShifts = []int{64, 1664, 3264, 576, 2176, 3840, 1088, 2752}

// protocolShifts2 are the second shifts of the layouts of Protocol, in the
// order of its layouts: the 8 of shift 0 first, then the 8 of protocolShifts.
// They are multiples of 32, since Go starts a function on 32 bytes and the
// code between the regexp matcher and the interpreter moves by exactly the
// sum of the two shifts. In each half, four of them are 32 bytes into a
// 64-byte block (so those layouts put that code half a block later) and four
// are not, two of the four with each bit of the front, so the place of that
// code in its block does not follow the place of the interpreter body. The
// sums of the two shifts, modulo 4 KiB, are 16 different places of that code
// in the page, about 256 bytes apart (480 at most): the formal layout at 0,
// then 480, 704, 896, 1120, 1312, 1696, 1856, 2272, 2464, 2720, 2816, 3072,
// 3520, 3648 and 3936.
var protocolShifts2 = []int{
	0, 2464, 1120, 3520, 1696, 2816, 704, 3936,
	2208, 3328, 3904, 4000, 1472, 1568, 1632, 3200,
}

// Protocol is the set of 16 layouts of docs/measuring.md: every front from 0
// to 56 at shift 0, which includes the formal layout, and every front again at
// one of protocolShifts, each with a second shift of protocolShifts2.
func Protocol() []Position {
	var ps []Position
	for i := range 8 {
		ps = append(ps, Position{0, 8 * i, protocolShifts2[i]})
	}
	for i, s := range protocolShifts {
		ps = append(ps, Position{s, 8 * i, protocolShifts2[8+i]})
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
