// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"strings"
	"unicode/utf8"
	"unsafe"
)

// Limits are the checks a long piece of work makes for the call it is done
// for, where the work cannot be cut into pieces a caller hands it one at a
// time: a run of characters a dictionary divides, and a run of combining
// marks put in order. Check ends the work when the call is to end, and Room
// when the call has no room for n bytes more, each by a panic its caller
// recovers. A nil Limits, or a nil func, checks nothing. The work calls them
// where it has changed nothing it shares with other work, so that a panic
// leaves only its own state behind.
//
// OutOfMemory, when it is not nil, has the work take each array the length
// of its text from the OS, as a block (qjs_blocks.go), and ends the work by
// a panic when the OS refuses one. The owner of such Limits calls Done once
// nothing the work made is used, and uses them for one piece of work only.
type Limits struct {
	Check       func()
	Room        func(n int64)
	OutOfMemory func()

	gives []func()
}

// limitEvery is how many turns of a loop go between two checks, where a
// turn takes some hundreds of nanoseconds at most: a code point a
// dictionary divides, or a combining mark put in order.
const limitEvery = 1 << 12

// check calls Check.
func (l *Limits) check() {
	if l != nil && l.Check != nil {
		l.Check()
	}
}

// every counts a turn of a loop in *n, and calls Check every limitEvery
// turns.
func (l *Limits) every(n *int) {
	if *n++; *n&(limitEvery-1) == 0 {
		l.checkAt()
	}
}

// checkAt is every's call of Check, apart so that every inlines into the
// loops that count their turns.
//
//go:noinline
func (l *Limits) checkAt() { l.check() }

// room calls Room.
func (l *Limits) room(n int64) {
	if l != nil && l.Room != nil {
		l.Room(n)
	}
}

// runesOf is []rune(s), with the room for it asked first and the call
// checked every limitEvery code points: a conversion of the whole text of a
// compare, 8 Mi units, ran some milliseconds with no check.
func runesOf(s string, l *Limits) []rune {
	if l == nil {
		return []rune(s)
	}
	l.room(4 * int64(len(s)))
	r := take[rune](l, len(s))[:0]
	turns := 0
	for _, c := range s {
		l.every(&turns)
		r = append(r, c)
	}
	return r
}

// stringOf is string(r), with the room for it asked first and the call
// checked every limitEvery code points.
func stringOf(r []rune, l *Limits) string {
	if l == nil {
		return string(r)
	}
	l.room(utf8.UTFMax * int64(len(r)))
	if l.blocks() {
		b := take[byte](l, utf8.UTFMax*len(r))[:0]
		turns := 0
		for _, c := range r {
			l.every(&turns)
			b = utf8.AppendRune(b, c)
		}
		return unsafe.String(unsafe.SliceData(b), len(b))
	}
	var b strings.Builder
	b.Grow(len(r))
	turns := 0
	for _, c := range r {
		l.every(&turns)
		b.WriteRune(c)
	}
	return b.String()
}

// turns counts the turns of the loops of one piece of work, which share the
// limits l, and calls Check every limitEvery turns of them all.
type turns struct {
	l *Limits
	n int
}

func (t *turns) turn() { t.l.every(&t.n) }
