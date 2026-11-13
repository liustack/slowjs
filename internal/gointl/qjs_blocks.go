// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"unsafe"

	"github.com/liustack/quickjs/internal/hostmem"
)

// The arrays the length of its text that a piece of work makes come from
// the OS, in blocks (hostmem.TakeKept), when its Limits have Take: Go
// clears an allocation in one call, and copies a slice that append grows
// in one call, with no Check, while the OS hands over its pages, 30 ms for
// 16 MiB on a slow virtual machine. The pages of a block come as the work
// writes them, and a copy into a block goes a piece at a time with a Check
// between two. The blocks live until the owner of the Limits calls Done.

// copyPiece is how many elements a copy of a long array takes between two
// checks: at most 512 KiB.
const copyPiece = 1 << 16

// blocks reports whether the work takes its long arrays from the OS.
func (l *Limits) blocks() bool { return l != nil && l.Take != nil }

// Done gives back the blocks of the work: nothing it made may be used
// after, a string over one included.
func (l *Limits) Done() {
	if l == nil {
		return
	}
	for _, give := range l.gives {
		give()
	}
	l.gives = nil
}

// take is n zero elements of T for the work: a block from hostmem.BlockMin
// bytes, when the work takes its long arrays from the OS, else a Go
// allocation.
func take[T hostmem.Elem](l *Limits, n int) []T {
	if !l.blocks() || !hostmem.Mapped[T](n) {
		return make([]T, n)
	}
	var t T
	p, give := l.Take(n * int(unsafe.Sizeof(t)))
	l.gives = append(l.gives, give)
	return unsafe.Slice((*T)(p), n)
}

// grown is s with room for n elements: s itself when it has it, else a new
// array of n, with the elements of s copied into it a piece at a time.
func grown[T hostmem.Elem](l *Limits, s []T, n int) []T {
	if cap(s) >= n {
		return s
	}
	return appendPieces(l, take[T](l, n)[:0], s)
}

// push appends v to s, in a new array of twice its length when it is full,
// which grown fills.
func push[T hostmem.Elem](l *Limits, s []T, v T) []T {
	if len(s) == cap(s) {
		s = grown(l, s, 2*len(s)+1)
	}
	return append(s, v)
}

// appendPieces appends from to s, a piece at a time with a check of l
// between two, in a new array of twice what it needs when s has no room.
func appendPieces[T hostmem.Elem](l *Limits, s, from []T) []T {
	if n := len(s) + len(from); cap(s) < n {
		s = grown(l, s, 2*n)
	}
	for lo := 0; lo < len(from); lo += copyPiece {
		if lo > 0 {
			l.check()
		}
		s = append(s, from[lo:min(lo+copyPiece, len(from))]...)
	}
	return s
}
