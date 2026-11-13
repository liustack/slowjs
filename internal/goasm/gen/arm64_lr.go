// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// LR in the arm64 lowering. llc keeps x30 like a callee-saved register: a
// function that calls saves it in its frame and loads it back before each
// ret. The lowering turns each ret into a branch to the end of the function,
// where the Go assembler puts its epilogue, and the epilogue of a function
// that calls loads LR from 0(RSP), where Go's prologue saved it
// (cmd/internal/obj/arm64/obj7.go, the ARET case of preprocess: MOVD.P
// off(RSP), R30, or LDP -8(RSP), (R29, R30) for a frame past 0xF0). So llc's
// own save and load of x30 are dead, and the lowering drops them. The bytes
// of the slot stay in the frame: the frame keeps its size, so the call depth
// and the shadow stack do not change.
//
// a64ProveLR holds the result to what makes the drop safe, on the lowered
// text and not on llc's: no instruction left names x30, and no instruction
// left reads, writes or takes the address of any byte of a dropped slot.
// Then on every path the slot is written only from x30 and read only into
// x30, both of them gone. Go's runtime never reads llc's slot either: the
// unwinder, which a morestack restart, a signal's traceback and a callback
// into Go all go through, finds a frame's return address at 0(RSP) of that
// frame (runtime/traceback.go, unwinder.next on an LR machine), and morestack
// takes the caller's LR from R3 and the function's own frame is not built
// yet when it runs (runtime/asm_arm64.s).
package gen

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// a64LR is one instruction of llc's that names x30, which must save it in the
// frame or load it back: a str, ldr, stp or ldp of x30 at [sp, #k], a str
// that allocates the frame ([sp, #-N]!) or an ldr that frees it ([sp], #N).
type a64LR struct {
	load      bool
	alloc     int    // bytes allocated before the access
	free      int    // bytes freed after it
	lr        int    // where x30 is, from SP at the access
	other     string // the other register of a pair, or ""
	otherAt   int    // where it is
	otherSave string // the single str or ldr of the other register, at [sp, #otherAt]
}

// a64ParseLR reads an instruction that names x30. Any other use of LR (as a
// value, in a w register, in a pair of two LR) is refused.
func a64ParseLR(l string) (a64LR, error) {
	bad := func() (a64LR, error) { return a64LR{}, fmt.Errorf("LR used outside a stack save: %q", l) }
	op, ops := a64Operands(l)
	var a a64LR
	var regs []string
	switch op {
	case "str", "ldr":
		regs = ops[:min(1, len(ops))]
	case "stp", "ldp":
		regs = ops[:min(2, len(ops))]
	default:
		return bad()
	}
	a.load = op[0] == 'l'
	mem := ops[len(regs):]
	if len(regs) == 0 || len(mem) == 0 || len(mem) > 2 {
		return bad()
	}
	lrs := 0
	for i, r := range regs {
		if r == "x30" {
			lrs++
			a.lr = 8 * i
			continue
		}
		if !strings.HasPrefix(r, "x") || a64RegNumber(r) < 0 || reA64LR.MatchString(r) {
			return bad()
		}
		a.other, a.otherAt = r, 8*i
	}
	if lrs != 1 {
		return bad()
	}
	base := 0
	switch {
	case len(mem) == 2: // [sp], #N: a load that frees the frame after it
		n, err := strconv.Atoi(strings.TrimPrefix(mem[1], "#"))
		if mem[0] != "[sp]" || !a.load || err != nil || n <= 0 || !strings.HasPrefix(mem[1], "#") {
			return bad()
		}
		a.free = n
	case strings.HasSuffix(mem[0], "]!"): // [sp, #-N]!: a store that allocates first
		m := reA64MemSP.FindStringSubmatch(strings.TrimSuffix(mem[0], "!"))
		if m == nil || a.load || m[0] != strings.TrimSuffix(mem[0], "!") {
			return bad()
		}
		n, _ := strconv.Atoi(m[1])
		if n >= 0 {
			return bad()
		}
		a.alloc = -n
	default:
		m := reA64MemSP.FindStringSubmatch(mem[0])
		if m == nil || m[0] != mem[0] {
			return bad()
		}
		base, _ = strconv.Atoi(m[1])
		if base < 0 {
			return bad()
		}
	}
	a.lr += base
	a.otherAt += base
	if a.other != "" {
		a.otherSave = "str"
		if a.load {
			a.otherSave = "ldr"
		}
	}
	return a, nil
}

// a64LRSlots are the slots of x30 the lowering of one function dropped, by
// their offset in the Go frame.
type a64LRSlots struct {
	stored, loaded map[int]bool
	// extra are slots of other registers a negative build dropped
	// (DropLRPairs): the proof must refuse them.
	extra map[int]bool
}

func (s *a64LRSlots) any() bool { return len(s.stored)+len(s.loaded)+len(s.extra) > 0 }

// a64ProveLR checks the lowered text of a function from which the lowering
// dropped the slots s, and calls the number of calls it makes. See the
// comment at the top of this file.
func a64ProveLR(out []string, s *a64LRSlots, calls int, leafArgs map[int][]string) error {
	if !s.any() {
		return nil
	}
	// Go loads LR back from 0(RSP) at RET only in a function with a BL or
	// CALL, which the emit writes for each call of the record. A function
	// without one is a leaf to Go, whose epilogue leaves x30 alone: then no
	// instruction left may write x30, which holds LR from the entry to RET.
	if calls == 0 {
		for _, l := range out {
			if reA64BL.MatchString(l) || reA64BLR.MatchString(l) {
				return fmt.Errorf("a call the record does not have: %q", l)
			}
		}
	}
	for k := range s.loaded {
		if !s.stored[k] {
			return fmt.Errorf("x30 loaded from %d(RSP), where it was never stored", k)
		}
	}
	slots := map[int]bool{}
	for _, m := range []map[int]bool{s.stored, s.loaded, s.extra} {
		for k := range m {
			slots[k] = true
		}
	}
	overlaps := func(at, n int) (int, bool) {
		for k := range slots {
			if at < k+8 && k < at+n {
				return k, true
			}
		}
		return 0, false
	}
	for i, l := range out {
		if isA64Label(l) || strings.HasPrefix(l, ".") {
			continue
		}
		if reA64LR.MatchString(l) {
			return fmt.Errorf("x30 still named after its saves were dropped: %q", l)
		}
		if !reA64SPReg.MatchString(l) {
			continue // no SP, or only in a label (.Lconst.sp)
		}
		at, n := 0, 0
		if m := reA64AddrSP.FindStringSubmatch(l); m != nil {
			// The address of a tuple spill. Every byte reached through
			// it counts, up to where the register is written again.
			at, _ = strconv.Atoi(m[2])
			var err error
			if n, err = a64TupleReach(out, i+1, m[1], leafArgs); err != nil {
				return fmt.Errorf("%q: %v", l, err)
			}
		} else if m := a64PlainSPAccess(l); m != nil {
			at, _ = strconv.Atoi(m[1])
			if n = a64AccessBytes(l); n == 0 {
				return fmt.Errorf("a stack access of unknown width beside a dropped LR slot: %q", l)
			}
		} else {
			return fmt.Errorf("a stack use beside a dropped LR slot: %q", l)
		}
		if k, ok := overlaps(at, n); ok {
			return fmt.Errorf("the dropped slot %d(RSP) is also reached by %q", k, l)
		}
	}
	return nil
}

// a64TupleReach is how many bytes from the address in reg the lines of out
// from index from reach, on every path the code can take, up to where reg is
// written without being read: only st1 or ld1 of a tuple through [reg] may
// use it. The walk follows the code as the processor does: through a label,
// into the target of a branch and, for a conditional branch, also on to the
// next line. A path ends where reg is written without being read, at a call
// or at a return. It is refused where reg is read in any other way, at an
// indirect branch (br), whose targets the text does not name, at a branch to
// a label the function does not have, at a call while reg is one of the
// registers a call keeps, at a directive other than .p2align, and at the end
// of the function. Each line is walked once: what reg holds there is the
// address on every path that comes from from, so one visit sees all it can
// reach.
//
// A call ends the address in a register that no convention keeps across it:
// llc reads such a register after a call only once it has written it again.
// The AAPCS registers a register-entry leaf keeps, x19 to x28, are not taken.
//
// The call to a unified function reads its arguments from the stores the
// lowering wrote before it, which the walk sees. A register-entry leaf reads
// them from registers no line names, and leafArgs gives those registers for
// the call at each index: the address must not be one of them.
//
// llc puts the address of a tuple's spill slot in a register its scavenger
// finds free and that is dead after the st1 or ld1, so a path seldom goes
// far: with a profile, llc places a reload at the end of a block whose
// branch comes before any write of the register (found in
// JS_GetOwnPropertyNamesInternal under clang PGO), which a walk that stopped
// at the first branch refused.
func a64TupleReach(out []string, from int, reg string, leafArgs map[int][]string) (int, error) {
	r, n := a64RegNumber(reg), 0
	labels := map[string]int{}
	for i, l := range out {
		if isA64Label(l) {
			labels[strings.TrimSuffix(l, ":")] = i
		}
	}
	seen := map[int]bool{}
	work := []int{from}
	for len(work) > 0 {
		i := work[len(work)-1]
		work = work[:len(work)-1]
	path:
		for ; ; i++ {
			if i >= len(out) {
				return 0, fmt.Errorf("the stack address in %s lives to the end of the function", reg)
			}
			if seen[i] {
				break
			}
			seen[i] = true
			l := out[i]
			if isA64Label(l) || strings.HasPrefix(l, ".p2align") {
				continue
			}
			if strings.HasPrefix(l, ".") {
				return 0, fmt.Errorf("the stack address in %s may live past %q", reg, l)
			}
			if m := reA64Tuple.FindStringSubmatch(l); m != nil && m[3] == reg {
				n = max(n, 16*strings.Count(m[2], "v"))
				continue
			}
			op, ops := a64Operands(l)
			switch {
			case strings.HasPrefix(l, "b .Lgoret_"):
				break path
			case op == "bl" || op == "blr":
				if r >= 19 && r <= 28 {
					return 0, fmt.Errorf("the stack address in %s may live past %q", reg, l)
				}
				if reads, _ := a64Reads(l, r); reads {
					return 0, fmt.Errorf("the stack address in %s is read by %q", reg, l)
				}
				for _, a := range leafArgs[i] {
					if a64RegNumber(a) == r {
						return 0, fmt.Errorf("the stack address in %s is an argument of %q", reg, l)
					}
				}
				break path
			case op == "br":
				return 0, fmt.Errorf("the stack address in %s may live past %q", reg, l)
			case op == "b" || strings.HasPrefix(op, "b.") || op == "cbz" || op == "cbnz" || op == "tbz" || op == "tbnz":
				if reads, _ := a64Reads(l, r); reads {
					return 0, fmt.Errorf("the stack address in %s is read by %q", reg, l)
				}
				if len(ops) == 0 {
					return 0, fmt.Errorf("the stack address in %s may live past %q", reg, l)
				}
				t, ok := labels[ops[len(ops)-1]]
				if !ok {
					return 0, fmt.Errorf("the stack address in %s may live past %q, to a label outside the function", reg, l)
				}
				work = append(work, t)
				if op == "b" {
					break path
				}
				continue
			}
			reads, writes := a64Reads(l, r)
			if reads {
				return 0, fmt.Errorf("the stack address in %s is read by %q", reg, l)
			}
			if writes {
				break path
			}
		}
	}
	return n, nil
}

// a64PlainSPAccess matches a load or store whose last operand is [sp] or
// [sp, #k] whole, with SP in no other operand: no writeback, written as ]!
// or ] !, and no post-index offset after it.
func a64PlainSPAccess(l string) []string {
	_, ops := a64Operands(l)
	if len(ops) < 2 || len(reA64SPReg.FindAllString(l, -1)) != 1 {
		return nil
	}
	return reA64MemSPWhole.FindStringSubmatch(ops[len(ops)-1])
}

var reA64MemSPWhole = regexp.MustCompile(`^\[sp(?:, #(-?\d+))?\]$`)

// reA64SPReg finds SP as an operand, not as part of a label.
var reA64SPReg = regexp.MustCompile(`(?:^|[ ,\[])sp(?:[,\]]|$)`)

// a64LeafArgs are the general registers a call to the register-entry leaf t
// reads: those llc's facts give for the call and those the AAPCS gives its
// signature, or x0 to x8 when neither is known.
func a64LeafArgs(t string, self FuncSpec, facts *CallFacts) []string {
	var regs []string
	sig, ok := callSig(t, self, calleeSigs)
	if ok {
		if r, err := a64ArgRegs(sig, false); err == nil {
			regs = append(regs, r...)
		} else {
			ok = false
		}
	}
	if facts != nil {
		regs = append(regs, facts.Args...)
	}
	if !ok && facts == nil {
		regs = []string{"x0", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8"}
	}
	var out []string
	for _, r := range regs {
		if (r[0] == 'x' || r[0] == 'w') && a64RegNumber(r) >= 0 {
			out = append(out, r)
		}
	}
	return out
}
