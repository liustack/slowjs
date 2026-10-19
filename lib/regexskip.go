// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

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
// lreSkipLoop is the length of the loop, lreSkipBody the offset of the
// first op of the regexp itself from the start of the bytecode.
const (
	lreSkipLoop = 5 + 1 + 5
	lreSkipBody = MRE_HEADER_LEN + lreSkipLoop + 2
)

// lreSkipChunk is how many units of the string lreSkip looks at between two
// checks for an interrupt. The backtracking machine checks every
// INTERRUPT_COUNTER_INIT steps, about as many positions of the loop, so a
// regexp in a loop over a long string stops when the eval timeout fires or
// Interrupt is called. lreSkip looks at a unit in about a nanosecond, and
// checks less often than the machine so that the check costs nothing.
const lreSkipChunk = 1 << 16

// lreSkip returns where lre_exec starts to look for a match of the regexp
// of bytecode bc in the string cbuf of clen units (8-bit when cbufType is 0,
// 16-bit when it is 1) from cindex, and the offset in bc of the first op to
// run. When the offset is 0, lre_exec runs nothing and returns the first
// result: 0 for no match, or LRE_RET_TIMEOUT when an interrupt came first.
// It is called by Xlre_exec before anything else (see fixRegexSkip in
// internal/fixlib), with the opaque that lre_exec passes to
// lre_check_timeout.
//
// When the first op of the regexp is one character or one class of
// characters, a match can only start at a position that holds such a
// character, so the loop runs from the first such position, found here in
// Go and not one position at a time in the backtracking machine. When the
// first op is ^ outside multiline mode, a match can only start at position
// 0, so lre_exec runs it from there without the loop. When no position can
// start a match, or an interrupt comes while lreSkip looks for one, lreSkip
// clears the captures, as lre_exec does first, and lre_exec returns.
//
// Every other regexp starts from cindex with the loop, as before. So do the
// unicode ones, whose loop steps over surrogate pairs, the sticky ones,
// which have no loop, and the ones that ignore case or are multiline, so
// that lreSkip only meets the ops it knows the way lre_compile writes them
// without flags. What lreSkip reads of the bytecode and of the ops is
// pinned by regexSites in internal/fixlib.
func lreSkip(tls *libc.TLS, capture, bc, cbuf uintptr, cindex, clen, cbufType int32, opaque uintptr) (int32, uintptr) {
	flags := (*Tpacked_u16)(unsafe.Pointer(bc + MRE_HEADER_FLAGS)).Fv
	if flags&(MLRE_FLAG_UNICODE|MLRE_FLAG_UNICODE_SETS|MLRE_FLAG_STICKY|MLRE_FLAG_IGNORECASE|MLRE_FLAG_MULTILINE) != 0 || cbufType != 0 && cbufType != 1 {
		return cindex, MRE_HEADER_LEN
	}
	// The bytecode holds at least the loop, save_start 0 and the op after
	// it: an empty regexp is followed by save_end 0, then match.
	if (*Tpacked_u32)(unsafe.Pointer(bc+MRE_HEADER_BYTECODE_LEN)).Fv < lreSkipLoop+2+1 {
		return cindex, MRE_HEADER_LEN
	}
	loop := bc + MRE_HEADER_LEN
	if *(*uint8)(unsafe.Pointer(loop)) != EREOP_split_goto_first ||
		(*Tpacked_u32)(unsafe.Pointer(loop+1)).Fv != 1+5 ||
		*(*uint8)(unsafe.Pointer(loop + 5)) != EREOP_any ||
		*(*uint8)(unsafe.Pointer(loop + 6)) != EREOP_goto ||
		int32((*Tpacked_u32)(unsafe.Pointer(loop+7)).Fv) != -lreSkipLoop ||
		*(*uint8)(unsafe.Pointer(loop + lreSkipLoop)) != EREOP_save_start ||
		*(*uint8)(unsafe.Pointer(loop + lreSkipLoop + 1)) != 0 {
		return cindex, MRE_HEADER_LEN
	}
	pc := bc + lreSkipBody
	op := *(*uint8)(unsafe.Pointer(pc))
	switch op {
	case EREOP_line_start:
		if cindex == 0 {
			return 0, MRE_HEADER_LEN + lreSkipLoop
		}
		return lreSkipNone(capture, bc, 0)
	case EREOP_char, EREOP_range:
	default:
		return cindex, MRE_HEADER_LEN
	}
	for i := cindex; i < clen; {
		end := clen
		if clen-i > lreSkipChunk {
			end = i + lreSkipChunk
		}
		var found bool
		if op == EREOP_char {
			i, found = lreFindChar(pc, cbuf, i, end, cbufType)
		} else {
			i, found = lreFindRange(pc, cbuf, i, end, cbufType)
		}
		if found {
			return i, MRE_HEADER_LEN
		}
		if i < clen && Xlre_check_timeout(tls, opaque) != 0 {
			return lreSkipNone(capture, bc, MLRE_RET_TIMEOUT)
		}
	}
	return lreSkipNone(capture, bc, 0)
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

// lreSkipNone clears the captures of the regexp of bytecode bc and returns
// ret as what lre_exec returns.
func lreSkipNone(capture, bc uintptr, ret int32) (int32, uintptr) {
	for i := uintptr(0); i < 2*uintptr(*(*uint8)(unsafe.Pointer(bc + MRE_HEADER_CAPTURE_COUNT))); i++ {
		*(*uintptr)(unsafe.Pointer(capture + i*unsafe.Sizeof(uintptr(0)))) = 0
	}
	return ret, 0
}

// lreUnit returns unit i of cbuf.
func lreUnit(cbuf uintptr, i, cbufType int32) uint32 {
	if cbufType == 0 {
		return uint32(*(*uint8)(unsafe.Pointer(cbuf + uintptr(i))))
	}
	return uint32(*(*uint16)(unsafe.Pointer(cbuf + 2*uintptr(i))))
}
