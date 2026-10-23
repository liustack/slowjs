// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// The bytecode lreSkip reads, as lre_compile writes it. A regexp that is not
// sticky starts with a loop that tries every position in turn, then saves
// the start of the match:
//
//	split_goto_first +6   try the match here first
//	any                   else step over one character
//	goto -11              and try again
//	save_start 0
//
// lreSkipLoop is the length of the loop, lreSkipMatch the offset of the
// match at one position (save_start 0) from the start of the bytecode, and
// lreSkipBody the offset of the first op of the regexp itself.
const (
	lreSkipLoop  = 5 + 1 + 5
	lreSkipMatch = MRE_HEADER_LEN + lreSkipLoop
	lreSkipBody  = lreSkipMatch + 2
)

// lreSkipChunk is how many units of the string lreSkip looks at between two
// checks for an interrupt. The backtracking machine checks every
// INTERRUPT_COUNTER_INIT steps, about as many positions of the loop, so a
// regexp in a loop over a long string stops when the eval timeout fires or
// Interrupt is called. lreSkip looks at a unit in about a nanosecond, and
// checks less often than the machine so that the check costs nothing.
const lreSkipChunk = 1 << 16

// lreFirstBudget is how many ops lreFirstSet reads at most before it gives
// up. A regexp of 148 alternatives, each starting with a character, takes
// about 300.
const lreFirstBudget = 1024

// How lreRun runs the bytecode from where lreSkip says.
const (
	lreRunOnce  = iota // run once, from pc
	lreRunChar         // match at each position that holds the char at op
	lreRunRange        // match at each position that holds a character of the range at op
	lreRunSet          // match at each position that holds a unit of set
)

// lreStart is what lreSkip gives Xlre_exec: where to start, and how lreRun
// goes on from there.
type lreStart struct {
	index int32   // the position of the string to start from
	pc    uintptr // the offset in the bytecode of the first op to run, or 0: run nothing, return index
	kind  uint8   // lreRunOnce, ...
	op    uintptr // the first op of the regexp, for lreRunChar and lreRunRange
	set   lreSet  // for lreRunSet
}

// lreSet is a set of the units a match can start with: the units below 256
// exactly, the others all or none.
type lreSet struct {
	bits [4]uint64
	high bool // every unit from 256 up
}

func (f *lreSet) add(c uint32) {
	if c < 256 {
		f.bits[c>>6] |= 1 << (c & 63)
	} else {
		f.high = true
	}
}

func (f *lreSet) has(c uint32) bool {
	if c < 256 {
		return f.bits[c>>6]&(1<<(c&63)) != 0
	}
	return f.high
}

// lreSkip returns where lre_exec starts to look for a match of the regexp
// of bytecode bc in the string cbuf of clen units (8-bit when cbufType is 0,
// 16-bit when it is 1) from cindex, and how lreRun goes on from there. When
// its pc is 0, lre_exec runs nothing and returns its index: 0 for no match,
// or LRE_RET_TIMEOUT when an interrupt came first. It is called by Xlre_exec
// before anything else (see fixRegexSkip in internal/fixlib), with the
// opaque that lre_exec passes to lre_check_timeout.
//
// A match can only start at a position that holds a unit of the set of
// first units of the regexp (see lreFirstSet), which is exact when the
// first op of the regexp is one character or one class of characters. So
// lreSkip finds the first such position in Go, not one position at a time
// in the backtracking machine, and lreRun tries the match there without the
// loop, then at the next such position, and so on. When the first op is ^
// outside multiline mode, a match can only start at position 0, so
// lre_exec runs it from there without the loop. When no position can start
// a match, or an interrupt comes while lreSkip looks for one, lreSkip
// clears the captures, as lre_exec does first, and lre_exec returns.
//
// Every other regexp starts from cindex with the loop, as before. So do the
// unicode ones, whose loop steps over surrogate pairs, and the sticky ones,
// which have no loop. What lreSkip reads of the bytecode and of the ops is
// pinned by regexSites in internal/fixlib.
func lreSkip(tls *libc.TLS, capture, bc, cbuf uintptr, cindex, clen, cbufType int32, opaque uintptr) lreStart {
	asIs := lreStart{index: cindex, pc: MRE_HEADER_LEN}
	flags := (*Tpacked_u16)(unsafe.Pointer(bc + MRE_HEADER_FLAGS)).Fv
	if flags&(MLRE_FLAG_UNICODE|MLRE_FLAG_UNICODE_SETS|MLRE_FLAG_STICKY) != 0 || cbufType != 0 && cbufType != 1 {
		return asIs
	}
	// The bytecode holds at least the loop, save_start 0 and the op after
	// it: an empty regexp is followed by save_end 0, then match.
	if (*Tpacked_u32)(unsafe.Pointer(bc+MRE_HEADER_BYTECODE_LEN)).Fv < lreSkipLoop+2+1 {
		return asIs
	}
	loop := bc + MRE_HEADER_LEN
	if *(*uint8)(unsafe.Pointer(loop)) != EREOP_split_goto_first ||
		(*Tpacked_u32)(unsafe.Pointer(loop+1)).Fv != 1+5 ||
		*(*uint8)(unsafe.Pointer(loop + 5)) != EREOP_any ||
		*(*uint8)(unsafe.Pointer(loop + 6)) != EREOP_goto ||
		int32((*Tpacked_u32)(unsafe.Pointer(loop+7)).Fv) != -lreSkipLoop ||
		*(*uint8)(unsafe.Pointer(loop + lreSkipLoop)) != EREOP_save_start ||
		*(*uint8)(unsafe.Pointer(loop + lreSkipLoop + 1)) != 0 {
		return asIs
	}
	st := lreStart{pc: lreSkipMatch, op: bc + lreSkipBody}
	switch *(*uint8)(unsafe.Pointer(st.op)) {
	case EREOP_line_start:
		if cindex == 0 {
			return lreStart{index: 0, pc: lreSkipMatch}
		}
		return lreSkipNone(capture, bc, 0)
	case EREOP_char:
		st.kind = lreRunChar
	case EREOP_range:
		st.kind = lreRunRange
	default:
		var ok bool
		if st.set, ok = lreFirstSet(st.op); !ok {
			return asIs
		}
		st.kind = lreRunSet
	}
	i, ret := lreFind(tls, &st, cbuf, cindex, clen, cbufType, opaque)
	if ret != 1 {
		return lreSkipNone(capture, bc, ret)
	}
	st.index = i
	return st
}

// lreFind returns the first position from i to clen that can start a match
// as st says, and 1, or 0 when there is none, or LRE_RET_TIMEOUT when an
// interrupt comes first. It checks for one after every lreSkipChunk units
// it looks at.
func lreFind(tls *libc.TLS, st *lreStart, cbuf uintptr, i, clen, cbufType int32, opaque uintptr) (int32, int32) {
	for i < clen {
		end := clen
		if clen-i > lreSkipChunk {
			end = i + lreSkipChunk
		}
		var found bool
		switch st.kind {
		case lreRunChar:
			i, found = lreFindChar(st.op, cbuf, i, end, cbufType)
		case lreRunRange:
			i, found = lreFindRange(st.op, cbuf, i, end, cbufType)
		default:
			i, found = lreFindSet(&st.set, cbuf, i, end, cbufType)
		}
		if found {
			return i, 1
		}
		if i < clen && Xlre_check_timeout(tls, opaque) != 0 {
			return 0, MLRE_RET_TIMEOUT
		}
	}
	return 0, 0
}

// lreRun runs the backtracking machine of lre_exec, with its context s,
// from the position cptr of the string as st says, and returns what
// lre_exec_backtrack returns. Xlre_exec calls it in place of
// lre_exec_backtrack (see fixRegexSkip in internal/fixlib).
//
// For lreRunOnce it runs the machine once from st.pc. Otherwise it tries
// the match at the position, without the loop over positions, and when
// that fails, at each next position that can start a match. That is what
// the loop does, which tries the match at each position in turn, with the
// captures as they were before the loop, all cleared: the machine undoes
// what a failed match changed in the captures down to the state of the
// loop, so lreRun clears them before it tries the next position. Like the
// loop, lreRun polls the interrupt counter of the machine after each
// failed match, and lreFind checks for an interrupt while it looks for the
// next position. When no position is left, lreRun clears the captures, as
// they are when the loop runs out of the string.
func lreRun(tls *libc.TLS, s, capture, bc, cptr uintptr, st *lreStart) Tintptr_t {
	if st.kind == lreRunOnce {
		return _lre_exec_backtrack(tls, s, capture, bc+st.pc, cptr)
	}
	x := (*TREExecContext)(unsafe.Pointer(s))
	cbufType := x.Fcbuf_type
	clen := int32((x.Fcbuf_end - x.Fcbuf) >> cbufType)
	i := st.index
	n := 2 * uintptr(x.Fcapture_count)
	for {
		if ret := _lre_exec_backtrack(tls, s, capture, bc+lreSkipMatch, x.Fcbuf+uintptr(i)<<cbufType); ret != 0 {
			return ret
		}
		lreClear(capture, n)
		if _lre_poll_timeout(tls, s) != 0 {
			return MLRE_RET_TIMEOUT
		}
		next, ret := lreFind(tls, st, x.Fcbuf, i+1, clen, cbufType, x.Fopaque)
		if ret != 1 {
			return Tintptr_t(ret)
		}
		i = next
	}
}

// lreFirstSet returns the set of units a match of the bytecode from pc can
// start with, and whether it knows them: a match can start at a position
// only when it holds a unit of the set. It follows both ways of each split,
// steps over the ops that consume nothing, and stops on each path at the
// first op that consumes a unit: one character, or a class of characters.
// It skips lookaheads and lookbehinds to the op after them, and steps over
// the other assertions: they can only stop a match. It gives up when a path
// can reach the end of the regexp without consuming a unit (the regexp can
// match the empty string, at every position), at a backward jump (a loop
// that consumed nothing), at an op that consumes a unit it does not know
// (., \s, a back reference, a class that ignores case, ...), at an op that
// moves back (a lookbehind's own ops, which lreFirstSet does not enter, are
// all of them), and after lreFirstBudget ops.
//
// A character that ignores case matches the units whose canonical form,
// from lre_canonicalize outside unicode mode, is that character. For the
// ASCII characters lreFirstSet knows them: the character, and its lower
// case form for the upper case letters (lre_canonicalize gives the upper
// case of an ASCII letter and an ASCII unit for no other unit, which
// TestCanonicalizeASCII checks for every unit).
func lreFirstSet(pc uintptr) (lreSet, bool) {
	var set lreSet
	budget := lreFirstBudget
	return set, lreFirstWalk(&set, pc, &budget)
}

func lreFirstWalk(set *lreSet, pc uintptr, budget *int) bool {
	for {
		if *budget--; *budget < 0 {
			return false
		}
		switch op := *(*uint8)(unsafe.Pointer(pc)); op {
		case EREOP_save_start, EREOP_save_end, EREOP_set_char_pos, EREOP_check_advance:
			pc += 2
		case EREOP_save_reset:
			pc += 3
		case EREOP_set_i32:
			pc += 6
		case EREOP_line_start, EREOP_line_start_m, EREOP_line_end, EREOP_line_end_m,
			EREOP_word_boundary, EREOP_word_boundary_i, EREOP_not_word_boundary, EREOP_not_word_boundary_i:
			pc++
		case EREOP_lookahead, EREOP_negative_lookahead:
			off := int32((*Tpacked_u32)(unsafe.Pointer(pc + 1)).Fv)
			if off < 0 {
				return false
			}
			pc += 5 + uintptr(off)
		case EREOP_goto:
			off := int32((*Tpacked_u32)(unsafe.Pointer(pc + 1)).Fv)
			if off < 0 {
				return false
			}
			pc += 5 + uintptr(off)
		case EREOP_split_goto_first, EREOP_split_next_first:
			off := int32((*Tpacked_u32)(unsafe.Pointer(pc + 1)).Fv)
			if off < 0 || !lreFirstWalk(set, pc+5+uintptr(off), budget) {
				return false
			}
			pc += 5
		case EREOP_char:
			set.add(uint32((*Tpacked_u16)(unsafe.Pointer(pc + 1)).Fv))
			return true
		case EREOP_char_i:
			c := uint32((*Tpacked_u16)(unsafe.Pointer(pc + 1)).Fv)
			if c >= 128 || c >= 'a' && c <= 'z' {
				return false
			}
			set.add(c)
			if c >= 'A' && c <= 'Z' {
				set.add(c + 'a' - 'A')
			}
			return true
		case EREOP_char32:
			// Outside unicode mode a character is one unit, which is
			// never above 0xffff.
			if c := (*Tpacked_u32)(unsafe.Pointer(pc + 1)).Fv; c <= 0xffff {
				set.add(c)
			}
			return true
		case EREOP_range:
			n := uintptr((*Tpacked_u16)(unsafe.Pointer(pc + 1)).Fv)
			for k := uintptr(0); k < n; k++ {
				lreAddRange(set, uint32((*Tpacked_u16)(unsafe.Pointer(pc+3+4*k)).Fv), uint32((*Tpacked_u16)(unsafe.Pointer(pc+5+4*k)).Fv))
			}
			return true
		case EREOP_range32:
			n := uintptr((*Tpacked_u16)(unsafe.Pointer(pc + 1)).Fv)
			for k := uintptr(0); k < n; k++ {
				lreAddRange(set, (*Tpacked_u32)(unsafe.Pointer(pc+3+8*k)).Fv, (*Tpacked_u32)(unsafe.Pointer(pc+7+8*k)).Fv)
			}
			return true
		default:
			return false
		}
	}
}

// lreAddRange adds the units from low to high, both included, to set.
func lreAddRange(set *lreSet, low, high uint32) {
	for c := low; c <= high && c < 256; c++ {
		set.add(c)
	}
	if high >= 256 {
		set.high = true
	}
}

// lreFindChar returns the first position from i to end that holds the
// character of the op char at pc, or end.
func lreFindChar(pc, cbuf uintptr, i, end, cbufType int32) (int32, bool) {
	want := uint32((*Tpacked_u16)(unsafe.Pointer(pc + 1)).Fv)
	for ; i < end; i++ {
		if lreUnit(cbuf, i, cbufType) == want {
			return i, true
		}
	}
	return end, false
}

// lreFindRange returns the first position from i to end that holds a
// character of the op range at pc, or end. The op holds n >= 1 pairs of
// 16-bit bounds, both included, in increasing order, searched as lre_exec
// does. The last bound 0xffff stands for every character above, but a unit
// is never above it.
func lreFindRange(pc, cbuf uintptr, i, end, cbufType int32) (int32, bool) {
	n := int((*Tpacked_u16)(unsafe.Pointer(pc + 1)).Fv)
	pairs := pc + 3
	bound := func(k, end int) uint32 {
		return uint32((*Tpacked_u16)(unsafe.Pointer(pairs + uintptr(4*k+2*end))).Fv)
	}
	low, high := bound(0, 0), bound(n-1, 1)
	for ; i < end; i++ {
		c := lreUnit(cbuf, i, cbufType)
		if c < low || c > high {
			continue
		}
		for min, max := 0, n-1; min <= max; {
			k := (min + max) / 2
			if c < bound(k, 0) {
				max = k - 1
			} else if c > bound(k, 1) {
				min = k + 1
			} else {
				return i, true
			}
		}
	}
	return end, false
}

// lreFindSet returns the first position from i to end that holds a unit of
// set, or end.
func lreFindSet(set *lreSet, cbuf uintptr, i, end, cbufType int32) (int32, bool) {
	if cbufType == 0 {
		for ; i < end; i++ {
			c := *(*uint8)(unsafe.Pointer(cbuf + uintptr(i)))
			if set.bits[c>>6]&(1<<(c&63)) != 0 {
				return i, true
			}
		}
		return end, false
	}
	for ; i < end; i++ {
		if set.has(uint32(*(*uint16)(unsafe.Pointer(cbuf + 2*uintptr(i))))) {
			return i, true
		}
	}
	return end, false
}

// lreSkipNone clears the captures of the regexp of bytecode bc and returns
// ret as what lre_exec returns.
func lreSkipNone(capture, bc uintptr, ret int32) lreStart {
	lreClear(capture, 2*uintptr(*(*uint8)(unsafe.Pointer(bc + MRE_HEADER_CAPTURE_COUNT))))
	return lreStart{index: ret}
}

// lreClear sets the n captures at capture to NULL.
func lreClear(capture, n uintptr) {
	for i := uintptr(0); i < n; i++ {
		*(*uintptr)(unsafe.Pointer(capture + i*unsafe.Sizeof(uintptr(0)))) = 0
	}
}

// lreUnit returns unit i of cbuf.
func lreUnit(cbuf uintptr, i, cbufType int32) uint32 {
	if cbufType == 0 {
		return uint32(*(*uint8)(unsafe.Pointer(cbuf + uintptr(i))))
	}
	return uint32(*(*uint16)(unsafe.Pointer(cbuf + 2*uintptr(i))))
}

// lreSplit returns the two ways of a split op of the backtracking machine
// of lre_exec, with its context s, at the position cptr of the string: pc,
// the way the machine takes now, and pc1, the way it pushes to try later,
// or 0 to push nothing. The split ops of the machine call it before they
// push when the op at pc1 is one of lrePeekOps (see fixRegexSkip in
// internal/fixlib).
//
// A way that would fail right away when it is popped is not pushed: the
// ops before the first one that reads a unit only save captures, which the
// machine undoes when it fails, and that op is a character or a class of
// characters, of any kind, that the unit at cptr does not match. lreSplit
// reads the unit as GET_CHAR does and tests it as the op does, through the
// same lre_canonicalize when it ignores case. When the way taken now is
// another split op, which an alternation of many alternatives is, lreSplit
// goes on with that one, as the machine would, and so on. A regexp of many
// alternatives that start with a character so pushes nothing for the ones
// that cannot match at a position, rather than one state each, and steps
// over them in one call. After lreSplitBudget split ops in one call it
// gives back the one it is at, which the machine pushes as usual, so a
// chain of splits that never ends, which lre_compile does not write, still
// cannot hold it.
//
// The machine counts a step toward its interrupt check, and checks, at
// each jump back, loop and failed pop. A greedy x+ over a long string
// jumps back by a split op and so counts only through the pops of the
// states its splits push. So when lreSplit pushes nothing, the split op
// counts a step and checks itself (see fixRegexSkip), and such a match
// still stops when the eval timeout fires or Interrupt is called.
//
// What differs is how many steps are counted, and so after how many ops an
// interrupt stops a match: the machine counted a step for each state it
// popped, and now a call of lreSplit that pushes nothing counts one step,
// however many splits it stepped over. And a state that is not pushed
// takes no room on the backtracking stack, so near the memory limit a
// match can go on where it ran out of memory.
func lreSplit(tls *libc.TLS, s, pc, pc1, cptr uintptr) (uintptr, uintptr) {
	x := (*TREExecContext)(unsafe.Pointer(s))
	end := cptr >= x.Fcbuf_end
	var c, folded uint32
	if !end {
		c = lreGetChar(cptr, x.Fcbuf_end, x.Fcbuf_type)
	}
	canonical := false
	for range lreSplitBudget {
		q := pc1
		op := *(*uint8)(unsafe.Pointer(q))
		for op == EREOP_save_start || op == EREOP_save_end || op == EREOP_save_reset {
			if op == EREOP_save_reset {
				q += 3
			} else {
				q += 2
			}
			op = *(*uint8)(unsafe.Pointer(q))
		}
		if lreReadOps>>op&1 == 0 {
			return pc, pc1
		}
		if !end {
			k := c
			if op == EREOP_char_i || op == EREOP_char32_i || op == EREOP_range_i || op == EREOP_range32_i {
				if !canonical {
					canonical, folded = true, uint32(Xlre_canonicalize(tls, c, x.Fis_unicode))
				}
				k = folded
			}
			var can bool
			switch op {
			case EREOP_char, EREOP_char_i:
				can = k == uint32((*Tpacked_u16)(unsafe.Pointer(q+1)).Fv)
			case EREOP_char32, EREOP_char32_i:
				can = k == (*Tpacked_u32)(unsafe.Pointer(q+1)).Fv
			case EREOP_range, EREOP_range_i:
				can = lreInRange(q, k)
			default:
				can = lreInRange32(q, k)
			}
			if can {
				return pc, pc1
			}
		}
		// pc1 cannot start here: push nothing, and go on with pc.
		op = *(*uint8)(unsafe.Pointer(pc))
		if op != EREOP_split_goto_first && op != EREOP_split_next_first {
			return pc, 0
		}
		next := pc + 5
		far := next + uintptr(int32((*Tpacked_u32)(unsafe.Pointer(pc+1)).Fv))
		if op == EREOP_split_next_first {
			pc, pc1 = next, far
		} else {
			pc, pc1 = far, next
		}
	}
	return pc, pc1
}

// lreSplitBudget is how many split ops lreSplit steps over at most in one
// call. An alternation of 148 alternatives takes 148.
const lreSplitBudget = 4096

// lreReadOps has bit op set for each op that reads one unit and tests it
// alone: the characters and the classes of characters, of every kind.
// lrePeekOps adds the saves that lreSplit steps over before such an op.
// Every op is below 64.
const (
	lreReadOps uint64 = 1<<EREOP_char | 1<<EREOP_char_i | 1<<EREOP_char32 | 1<<EREOP_char32_i |
		1<<EREOP_range | 1<<EREOP_range_i | 1<<EREOP_range32 | 1<<EREOP_range32_i
	lrePeekOps uint64 = lreReadOps | 1<<EREOP_save_start | 1<<EREOP_save_end | 1<<EREOP_save_reset
)

// lreGetChar returns the character at cptr, before cbufEnd, as GET_CHAR of
// lre_exec_backtrack reads it: a unit, or in a string of type 2 (16-bit,
// unicode) a surrogate pair as one character.
func lreGetChar(cptr, cbufEnd uintptr, cbufType int32) uint32 {
	if cbufType == 0 {
		return uint32(*(*uint8)(unsafe.Pointer(cptr)))
	}
	c := uint32(*(*uint16)(unsafe.Pointer(cptr)))
	if c>>10 == 0xd800>>10 && cbufType == 2 && cptr+2 < cbufEnd {
		if d := uint32(*(*uint16)(unsafe.Pointer(cptr + 2))); d>>10 == 0xdc00>>10 {
			c = 0x10000 + 0x400*(c-0xd800) + (d - 0xdc00)
		}
	}
	return c
}

// lreInRange reports whether c is in the op range or range_i at pc, as
// lre_exec_backtrack tests it: n >= 1 pairs of 16-bit bounds, both
// included, where a last bound of 0xffff stands for every character above.
func lreInRange(pc uintptr, c uint32) bool {
	n := uint32((*Tpacked_u16)(unsafe.Pointer(pc + 1)).Fv)
	pairs := pc + 3
	if c < uint32((*Tpacked_u16)(unsafe.Pointer(pairs)).Fv) {
		return false
	}
	high := uint32((*Tpacked_u16)(unsafe.Pointer(pairs + uintptr(4*(n-1)+2))).Fv)
	if c >= 0xffff && high == 0xffff {
		return true
	}
	if c > high {
		return false
	}
	for min, max := int32(0), int32(n-1); min <= max; {
		k := uint32(min+max) / 2
		if c < uint32((*Tpacked_u16)(unsafe.Pointer(pairs+uintptr(4*k))).Fv) {
			max = int32(k) - 1
		} else if c > uint32((*Tpacked_u16)(unsafe.Pointer(pairs+uintptr(4*k+2))).Fv) {
			min = int32(k) + 1
		} else {
			return true
		}
	}
	return false
}

// lreInRange32 reports whether c is in the op range32 or range32_i at pc,
// as lre_exec_backtrack tests it: n >= 1 pairs of 32-bit bounds, both
// included.
func lreInRange32(pc uintptr, c uint32) bool {
	n := uint32((*Tpacked_u16)(unsafe.Pointer(pc + 1)).Fv)
	pairs := pc + 3
	if c < (*Tpacked_u32)(unsafe.Pointer(pairs)).Fv || c > (*Tpacked_u32)(unsafe.Pointer(pairs+uintptr(8*(n-1)+4))).Fv {
		return false
	}
	for min, max := int32(0), int32(n-1); min <= max; {
		k := uint32(min+max) / 2
		if c < (*Tpacked_u32)(unsafe.Pointer(pairs+uintptr(8*k))).Fv {
			max = int32(k) - 1
		} else if c > (*Tpacked_u32)(unsafe.Pointer(pairs+uintptr(8*k+4))).Fv {
			min = int32(k) + 1
		} else {
			return true
		}
	}
	return false
}
