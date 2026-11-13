// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

// Limits are the checks a long piece of work makes for the call it is done
// for, where the work cannot be cut into pieces a caller hands it one at a
// time: a run of characters a dictionary divides, and a run of combining
// marks put in order. Check ends the work when the call is to end, and Room
// when the call has no room for n bytes more, each by a panic its caller
// recovers. A nil Limits, or a nil func, checks nothing. The work calls them
// where it has changed nothing it shares with other work, so that a panic
// leaves only its own state behind.
type Limits struct {
	Check func()
	Room  func(n int64)
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
		l.check()
	}
}

// room calls Room.
func (l *Limits) room(n int64) {
	if l != nil && l.Room != nil {
		l.Room(n)
	}
}
