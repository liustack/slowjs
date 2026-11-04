// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions without Go's stack check. Every unified entry starts with the
// stack check of the Go assembler, three instructions that compare the stack
// pointer with the guard in g and call morestack below it, unless its TEXT
// says NOSPLIT. The generator chooses no function to go without it. The
// functions without one are those that cannot have one:
//
//   - the register-entry leaves (memcpy, memmove, memset), which are NOSPLIT
//     by their form: they take their arguments in registers that morestack
//     would not keep,
//   - the leaves the Go assembler marks NOSPLIT by itself: a function that
//     calls nothing and whose frame with the saved base pointer is under 128
//     bytes (abi.StackSmall, cmd/internal/obj/x86/obj6.go and
//     cmd/internal/obj/arm64/obj7.go).
//
// The generator names both in their TEXT, so that the list is written down
// and the post-link check holds every entry to it: a function on it must have
// no stack check, every other one must have one. A Go assembler that marks
// other leaves by itself, or no longer marks these, fails that check.
//
// An automatic leaf calls nothing. A register-entry leaf calls nothing but
// other register-entry leaves, whose arguments stay in registers (the
// engine's three call nothing), and MarkNoSplit fails when a function on the
// list makes another call, when register-entry leaves call each other in a
// cycle, or when one takes 128 bytes or more below its entry with the leaves
// it calls. A leaf is entered from a function that passed its stack check,
// which leaves at least 800 bytes above the bottom of the goroutine's stack
// once its frame is made (internal/abi.StackNosplitBase with the 128 bytes of
// abi.StackSmall in the guard). A fault in the leaf (a wild pointer of the C)
// becomes a call of runtime.sigpanic that the signal handler pushes where the
// fault is, and sigpanic's stack check may call morestack: that takes 16
// bytes on amd64 and 24 on arm64, far below what the leaf leaves of the 800.
// The linker still adds up every chain of NOSPLIT calls on each build
// (cmd/link/internal/ld/stackcheck.go), the ABI wrapper of a Go func value
// that calls a leaf included.
package gen

import (
	"fmt"
	"strings"
)

// autoNoSplitFrame is the frame, with the saved base pointer, under which
// the Go assembler marks a function that calls nothing NOSPLIT
// (abi.StackSmall, cmd/internal/obj/x86/obj6.go).
const autoNoSplitFrame = 128

// StackRules is how the Go assembler of an architecture lays out a frame.
type StackRules struct {
	Arch string
	// Local is the bytes a frame of the TEXT size frame takes below the
	// entry, for a function that calls (or calls nothing).
	Local func(frame int, calls bool) int
}

// AMD64Stack: a frame takes the saved base pointer on top, and a leaf with
// no frame takes nothing.
var AMD64Stack = StackRules{Arch: "amd64",
	Local: func(frame int, calls bool) int {
		if frame == 0 && !calls {
			return 0
		}
		return frame + 8
	}}

// ARM64Stack: a frame is the TEXT size with the saved LR rounded up as
// cmd/internal/obj/arm64/obj7.go does: 8 more bytes when that is 8 past a
// multiple of 16, else 16, for the frame pointer. A leaf with no frame gets
// no frame at all (NOFRAME).
var ARM64Stack = StackRules{Arch: "arm64",
	Local: func(frame int, calls bool) int {
		if frame == 0 && !calls {
			return 0
		}
		s := frame + 8
		if s%16 == 8 {
			return s + 8
		}
		return s + 16
	}}

// NoSplitList is the functions whose TEXT says NOSPLIT.
type NoSplitList struct {
	// Set holds every one of them, Auto the leaves the assembler marks
	// NOSPLIT by itself and Register the register-entry leaves.
	Set            map[string]bool
	Auto, Register []string
}

// autoNoSplit says whether the Go assembler marks a unified function with
// this frame NOSPLIT by itself (or, for an arm64 leaf with no frame, gives
// it no stack check at all).
func autoNoSplit(r StackRules, frame int, calls bool) bool {
	return !calls && r.Local(frame, false) < autoNoSplitFrame
}

// MarkNoSplit sets Meta.NoSplit, and NoSplit of its record, on the functions
// without a stack check: the register-entry leaves and the unified leaves
// the Go assembler marks NOSPLIT by itself. It fails when a register-entry
// leaf calls anything but another one, when they call each other in a cycle,
// or when one takes autoNoSplitFrame bytes or more below its entry with the
// leaves it calls (8 bytes for each call, on arm64 too).
func MarkNoSplit(r StackRules, metas []Meta) (*NoSplitList, error) {
	l := &NoSplitList{Set: map[string]bool{}}
	by := map[string]*Meta{}
	for i := range metas {
		by[metas[i].Name] = &metas[i]
	}
	const busy = -1
	height := map[string]int{}
	var leafHeight func(m *Meta) (int, error)
	leafHeight = func(m *Meta) (int, error) {
		switch h, ok := height[m.Name]; {
		case h == busy:
			return 0, fmt.Errorf("nosplit: the register-entry leaf %s calls itself through other leaves", m.Name)
		case ok:
			return h, nil
		}
		height[m.Name] = busy
		own := r.Local(m.Frame, len(m.Rec.Calls) > 0)
		h := own
		for _, c := range m.Rec.Calls {
			callee := by[c.Target]
			if c.Kind != CallLeaf || callee == nil || callee.Kind != Register {
				return 0, fmt.Errorf("nosplit: the register-entry leaf %s calls %s, which is not a register-entry leaf", m.Name, c.Target)
			}
			ch, err := leafHeight(callee)
			if err != nil {
				return 0, err
			}
			h = max(h, own+8+ch)
		}
		height[m.Name] = h
		return h, nil
	}
	for i := range metas {
		m := &metas[i]
		calls := len(m.Rec.Calls) > 0
		switch {
		case m.Kind == Register:
			h, err := leafHeight(m)
			if err != nil {
				return nil, err
			}
			if h >= autoNoSplitFrame {
				return nil, fmt.Errorf("nosplit: the register-entry leaf %s takes %d bytes below its entry with the leaves it calls, %d at most", m.Name, h, autoNoSplitFrame-1)
			}
			l.Register = append(l.Register, m.Name)
		case m.Kind == Unified && autoNoSplit(r, m.Frame, calls):
			l.Auto = append(l.Auto, m.Name)
		default:
			m.NoSplit, m.Rec.NoSplit = false, false
			continue
		}
		if l.Set[m.Name] {
			return nil, fmt.Errorf("nosplit: %s twice", m.Name)
		}
		l.Set[m.Name] = true
		m.NoSplit, m.Rec.NoSplit = true, true
	}
	return l, nil
}

// NoSplitReport is the list as the generator writes it down: one line per
// function, its kind (auto or register), frame and arguments.
func NoSplitReport(l *NoSplitList, metas []Meta) string {
	kind := map[string]string{}
	for _, n := range l.Auto {
		kind[n] = "auto"
	}
	for _, n := range l.Register {
		kind[n] = "register"
	}
	var b strings.Builder
	for _, m := range metas {
		if l.Set[m.Name] {
			fmt.Fprintf(&b, "%s %s frame %d args %d\n", m.Name, kind[m.Name], m.Frame, m.ArgBytes)
		}
	}
	return b.String()
}
