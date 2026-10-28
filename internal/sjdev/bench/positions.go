// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package bench builds the code-position variants of a program, times them in
// turn, and reads the result by the rules of "Code positions" in
// docs/performance.md.
package bench

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Position is where the code of the engine sits in a binary.
type Position struct {
	Shift int // the whole program moved by 0 or 32 bytes (Go puts functions on 32)
	Front int // the start of the interpreter body moved by 0, 8, 16 or 24 bytes of NOP
}

// String is the name used in file names and records: "32.16".
func (p Position) String() string { return fmt.Sprintf("%d.%d", p.Shift, p.Front) }

// Quick is the set for a quick check: 0 or 32 for the whole program, 0 or 16
// for the interpreter body.
func Quick() []Position {
	return []Position{{0, 0}, {0, 16}, {32, 0}, {32, 16}}
}

// All is the full set of 8.
func All() []Position {
	var ps []Position
	for _, s := range []int{0, 32} {
		for _, f := range []int{0, 8, 16, 24} {
			ps = append(ps, Position{s, f})
		}
	}
	return ps
}

// ParsePosition parses "W.B".
func ParsePosition(s string) (Position, error) {
	w, b, ok := strings.Cut(s, ".")
	if !ok {
		return Position{}, fmt.Errorf("position %q: want W.B, such as 32.16", s)
	}
	shift, err1 := strconv.Atoi(w)
	front, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil {
		return Position{}, fmt.Errorf("position %q: want W.B, such as 32.16", s)
	}
	if shift != 0 && shift != 32 {
		return Position{}, fmt.Errorf("position %q: the whole program moves by 0 or 32 bytes (Go puts functions on 32)", s)
	}
	if !slices.Contains([]int{0, 8, 16, 24}, front) {
		return Position{}, fmt.Errorf("position %q: the interpreter body moves by 0, 8, 16 or 24 bytes", s)
	}
	return Position{shift, front}, nil
}

// ParsePositions parses "quick", "all" or a comma list of positions.
func ParsePositions(s string) ([]Position, error) {
	switch s {
	case "", "quick":
		return Quick(), nil
	case "all":
		return All(), nil
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
// position, with front the name of the interpreter function.
func (p Position) XformOps(front string) []string {
	return []string{fmt.Sprintf("shift:%d", p.Shift), fmt.Sprintf("front:%s:%d", front, p.Front)}
}

// BinName is the file name of the binary of a side at a position.
func BinName(side string, p Position) string { return side + "@" + p.String() }
