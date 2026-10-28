// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"regexp"
	"strconv"
	"strings"
)

// a64Pages says, per register, whether it holds the page an adrp made: 0 for
// a value that is no page, a64MaybePage when some ways to this point leave a
// page in it and others do not, else the number of the page (see a64PageKey)
// plus one.
type a64Pages [32]int16

const a64MaybePage = -1

type a64PageKey struct {
	sym string
	add int64
}

// a64OwnPages says which registers hold the page of an adrp (of the function's
// own data, of other data or of a function) before each line of its body. The
// lowering puts the whole address in the register at the adrp, so a register
// that holds a page is no plain value: a :lo12: use of the function's own data
// reads it relative to the PC, which is the same bytes only where the
// register holds exactly the page of the address of the use, and any other
// read of a register that holds a page, or may, is refused.
type a64OwnPages struct {
	keys   []a64PageKey
	before []a64Pages // before[i]: the registers before line i of the body runs
}

// Page says what register r holds before line i: def is true for the page of
// sym+add on every way here, held is true if it holds a page on some way
// (def or not).
func (p *a64OwnPages) Page(i, r int) (sym string, add int64, def, held bool) {
	if r < 0 || r >= len(p.before[i]) || p.before[i][r] == 0 {
		return "", 0, false, false
	}
	v := p.before[i][r]
	if v == a64MaybePage {
		return "", 0, false, true
	}
	k := p.keys[v-1]
	return k.sym, k.add, true, true
}

// joinPage is what a register holds where two ways meet.
func joinPage(a, b int16) int16 {
	if a == b {
		return a
	}
	return a64MaybePage
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

var reA64TableEntry = regexp.MustCompile(`(\.L[\w$.]+)-([\w$.]+)`)

// a64LabelTables reads the tables of labels in a function's data: an entry is
// a label minus the base of the table, which is a label of the code for a jump
// table and a symbol of the data for the table of the computed goto. The
// result maps each base to the labels of the entries.
func a64LabelTables(data string) map[string][]string {
	out := map[string][]string{}
	for _, m := range reA64TableEntry.FindAllStringSubmatch(data, -1) {
		out[m[2]] = append(out[m[2]], m[1])
	}
	return out
}

// a64BrTargets says where the indirect jump at body[at] can go, from the
// tables of the function: a jump table (the register is the base from an adr
// plus an entry, and the table has that base) goes to the labels of that
// table, the computed goto (the register is a base plus an offset loaded from
// the base) goes to the labels of the tables that have no adr for a base.
// Any other shape, or a table not known, goes everywhere (nil).
func a64BrTargets(body []string, start, at int, tables map[string][]string, adrBase map[string]bool) []string {
	_, ops := a64Operands(body[at])
	if len(ops) != 1 {
		return nil
	}
	// def finds the last line of the block before line i that writes register
	// reg as its first operand with one of the mnemonics, or -1.
	def := func(i int, reg string, mnemonics ...string) (int, string, []string) {
		for k := i - 1; k >= start; k-- {
			op, o := a64Operands(body[k])
			if len(o) == 0 || isA64Label(body[k]) || a64RegNumber(o[0]) != a64RegNumber(reg) || (o[0][0] != 'x' && o[0][0] != 'w') {
				continue
			}
			for _, m := range mnemonics {
				if op == m {
					return k, op, o
				}
			}
			return -1, "", nil
		}
		return -1, "", nil
	}
	d1, _, o1 := def(at, ops[0], "add")
	if d1 < 0 || len(o1) < 3 {
		return nil
	}
	if d2, _, o2 := def(d1, o1[1], "adr"); d2 >= 0 && len(o2) == 2 && adrBase[o2[1]] {
		return tables[o2[1]]
	}
	// The computed goto: the offset added was loaded from the base.
	if d2, _, o2 := def(d1, o1[2], "ldrsw", "ldr"); d2 >= 0 && len(o2) == 2 && strings.HasPrefix(o2[1], "["+o1[1]+",") {
		var out []string
		for base, ls := range tables {
			if !adrBase[base] {
				out = append(out, ls...)
			}
		}
		return out
	}
	return nil
}

// a64FollowPages runs the data flow over body (the lines the lowering walks:
// instructions, labels, indirect-call markers). Every adrp starts a page in its
// register. A call ends the pages of the registers it may change. Where two
// ways meet, a register holds a page only if both ways agree on it, and holds
// the maybe of a64MaybePage if just one has a page or they have two. An
// indirect jump (br) goes where a64BrTargets says, or to every label the code
// can reach that way.
func a64FollowPages(body []string, indirect map[string]bool, tables map[string][]string) *a64OwnPages {
	adrBase := map[string]bool{}
	for _, l := range body {
		if m := reA64Adr.FindStringSubmatch(l); m != nil {
			adrBase[m[2]] = true
		}
	}
	res := &a64OwnPages{before: make([]a64Pages, len(body))}
	id := map[a64PageKey]int16{}
	pageID := func(k a64PageKey) int16 {
		if v, ok := id[k]; ok {
			return v
		}
		res.keys = append(res.keys, k)
		id[k] = int16(len(res.keys))
		return id[k]
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
			add := int64(0)
			if m[3] != "" {
				add, _ = strconv.ParseInt(m[3], 10, 64)
			}
			st[a64RegNumber(m[1])] = pageID(a64PageKey{m[2], add})
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
		// A :lo12: use keeps every page except the register it writes.
		if m := reA64Lo12.FindStringSubmatch(l); m != nil {
			if dest, _ := a64Lo12Regs(l); dest >= 0 {
				st[dest] = 0
			}
			return
		}
		// Anything else that mentions a page register reads the page (the
		// lowering refuses that) or writes over it with a plain value.
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
	var indirectBlocks []int
	for i, b := range blocks {
		if isA64Label(body[b.start]) && indirect[strings.TrimSuffix(body[b.start], ":")] {
			indirectBlocks = append(indirectBlocks, i)
		}
	}
	work := []int{0}
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
			if strings.Fields(body[last])[0] == "br" {
				if ls := a64BrTargets(body, b.start, last, tables, adrBase); ls != nil {
					for _, l := range ls {
						if i, ok := labelAt[l]; ok {
							succs = append(succs, blockOf[i])
						}
					}
				} else {
					succs = append(succs, indirectBlocks...)
				}
			}
		}
		if !stops && bi+1 < len(blocks) {
			succs = append(succs, bi+1)
		}
		for _, s := range succs {
			sb := blocks[s]
			changed := false
			if !sb.known {
				sb.in, sb.known, changed = st, true, true
			} else {
				for r := range sb.in {
					if j := joinPage(sb.in[r], st[r]); j != sb.in[r] {
						sb.in[r] = j
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
