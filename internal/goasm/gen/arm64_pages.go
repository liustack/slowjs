// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"regexp"
	"strings"
)

// a64Pages says, per register, which data symbol's page the register holds
// right now: 0 for none, else the number of the symbol plus one.
type a64Pages [32]int16

// a64OwnPages says which registers hold the page of an adrp of the function's
// own data (constant pools, jump tables) before each line of its body. The
// lowering reads such data relative to the PC, which is the same bytes only
// where the register holds exactly that page, so a use needs to know it, and a
// read of the register as anything else is refused.
type a64OwnPages struct {
	syms   []string
	before []a64Pages // before[i]: the registers before line i of the body runs
}

// At is the symbol whose page register r holds before line i, or "".
func (p *a64OwnPages) At(i, r int) string {
	if r < 0 || r >= len(p.before[i]) || p.before[i][r] == 0 {
		return ""
	}
	return p.syms[p.before[i][r]-1]
}

var reA64Label = regexp.MustCompile(`\.L[\w$.]+`)

// a64IndirectLabels are the labels the code can reach in a way the branches
// of the body do not show: the ones its data names (jump tables, the table of
// the computed goto) and the ones an adr takes.
func a64IndirectLabels(body []string, data string) map[string]bool {
	out := map[string]bool{}
	for _, l := range reA64Label.FindAllString(data, -1) {
		out[l] = true
	}
	for _, l := range body {
		if m := reA64Adr.FindStringSubmatch(l); m != nil {
			out[m[2]] = true
		}
	}
	return out
}

// a64FollowOwnPages runs the data flow over body (the lines the lowering
// walks: instructions, labels, indirect-call markers). A label with an
// unknown way in and the entry start with no page, and a call ends the pages
// of the registers it may change. At a join a register keeps a page only if
// every way in agrees.
func a64FollowOwnPages(body []string, own func(sym string) bool, indirect map[string]bool) *a64OwnPages {
	res := &a64OwnPages{before: make([]a64Pages, len(body))}
	id := map[string]int16{}
	symID := func(s string) int16 {
		if v, ok := id[s]; ok {
			return v
		}
		res.syms = append(res.syms, s)
		id[s] = int16(len(res.syms))
		return id[s]
	}
	labelAt := map[string]int{}
	for i, l := range body {
		if isA64Label(l) {
			labelAt[strings.TrimSuffix(l, ":")] = i
		}
	}
	// target gives the line a branch goes to, or -1.
	target := func(l string) int {
		_, ops := a64Operands(l)
		if len(ops) == 0 {
			return -1
		}
		if i, ok := labelAt[ops[len(ops)-1]]; ok {
			return i
		}
		return -1
	}
	// stops: the line never falls through. branches: it may jump.
	kind := func(l string) (stops, branches bool) {
		op := strings.Fields(l)[0]
		switch {
		case op == "b":
			return true, true
		case op == "br" || op == "ret":
			return true, false
		case strings.HasPrefix(op, "b.") || op == "cbz" || op == "cbnz" || op == "tbz" || op == "tbnz":
			return false, true
		}
		return false, false
	}
	instr := func(l string) bool {
		return !isA64Label(l) && !strings.HasPrefix(l, "@") && !strings.HasPrefix(l, ".p2align")
	}
	transfer := func(l string, st *a64Pages) {
		if !instr(l) {
			return
		}
		if m := reA64Adrp.FindStringSubmatch(l); m != nil {
			r := a64RegNumber(m[1])
			st[r] = 0
			if own(m[2]) {
				st[r] = symID(m[2])
			}
			return
		}
		op := strings.Fields(l)[0]
		if op == "bl" || op == "blr" {
			// A call may change any register, except that a register-entry
			// leaf keeps the callee-saved ones (x19 to x28) of AAPCS.
			keep := false
			if m := reA64BL.FindStringSubmatch(l); m != nil {
				keep = calleeKinds[m[1]] == Register
			}
			for r := range st {
				if !keep || r < 19 || r > 28 {
					st[r] = 0
				}
			}
			return
		}
		// A :lo12: use of the function's own data keeps every page except
		// the register it writes.
		if m := reA64Lo12.FindStringSubmatch(l); m != nil && own(m[1]) {
			if dest, _ := a64Lo12Regs(l); dest >= 0 {
				st[dest] = 0
			}
			return
		}
		// Anything else that mentions a page register reads the page (the
		// lowering refuses that) or writes over it.
		for r := range st {
			if st[r] != 0 {
				if reads, writes := a64Reads(l, r); reads || writes {
					st[r] = 0
				}
			}
		}
	}
	// Blocks: a leader is the first line, a label, or the line after a
	// jump.
	n := len(body)
	leader := make([]bool, n+1)
	leader[0], leader[n] = true, true
	for i, l := range body {
		if isA64Label(l) {
			leader[i] = true
		}
		if instr(l) {
			if stops, branches := kind(l); stops || branches {
				leader[i+1] = true
			}
		}
	}
	type block struct {
		start, end int
		in         a64Pages
		known      bool
	}
	var blocks []*block
	blockOf := make([]int, n)
	for i := 0; i < n; {
		j := i + 1
		for !leader[j] {
			j++
		}
		for k := i; k < j; k++ {
			blockOf[k] = len(blocks)
		}
		blocks = append(blocks, &block{start: i, end: j})
		i = j
	}
	if len(blocks) == 0 {
		return res
	}
	blocks[0].known = true
	for _, b := range blocks {
		if isA64Label(body[b.start]) && indirect[strings.TrimSuffix(body[b.start], ":")] {
			b.known = true
		}
	}
	work := []int{}
	for i, b := range blocks {
		if b.known {
			work = append(work, i)
		}
	}
	for len(work) > 0 {
		bi := work[len(work)-1]
		work = work[:len(work)-1]
		b := blocks[bi]
		st := b.in
		for k := b.start; k < b.end; k++ {
			transfer(body[k], &st)
		}
		var succs []int
		stops, branches := false, false
		if last := b.end - 1; instr(body[last]) {
			stops, branches = kind(body[last])
			if branches {
				if t := target(body[last]); t >= 0 {
					succs = append(succs, blockOf[t])
				}
			}
		}
		if !stops && bi+1 < len(blocks) {
			succs = append(succs, bi+1)
		}
		for _, s := range succs {
			sb := blocks[s]
			// Blocks with an unknown way in stay empty.
			if sb.known && isA64Label(body[sb.start]) && indirect[strings.TrimSuffix(body[sb.start], ":")] {
				continue
			}
			changed := false
			if !sb.known {
				sb.in, sb.known, changed = st, true, true
			} else {
				for r := range sb.in {
					if sb.in[r] != 0 && sb.in[r] != st[r] {
						sb.in[r] = 0
						changed = true
					}
				}
			}
			if changed {
				work = append(work, s)
			}
		}
	}
	for _, b := range blocks {
		st := b.in // an unreached block starts empty
		for k := b.start; k < b.end; k++ {
			res.before[k] = st
			transfer(body[k], &st)
		}
	}
	return res
}

// a64Lo12Regs gives the register an :lo12: add or load writes (-1 when it
// writes none of the general registers) and the base it reads.
func a64Lo12Regs(l string) (dest, base int) {
	dest, base = -1, -1
	var d, b string
	if a := reA64AddLo.FindStringSubmatch(l); a != nil {
		d, b = a[1], a[2]
	} else if a := reA64LdLo.FindStringSubmatch(l); a != nil {
		d, b = a[2], a[3]
	} else {
		return
	}
	base = a64RegNumber(b)
	if d[0] == 'x' || d[0] == 'w' {
		dest = a64RegNumber(d)
	}
	return
}
