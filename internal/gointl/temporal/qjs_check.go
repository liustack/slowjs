// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal

// WithCheck is the calendar c that calls check every few thousand turns of
// its loops over months and years, which a duration or two dates far apart
// can make millions long: check ends the work of a call when it is to end,
// by a panic its caller recovers, and the loops call it where they have
// changed nothing shared. A date, a date and time or a year and month made
// of the calendar it answers, and those its arithmetic answers, reckon in it
// too, so a call takes it for the values it reads and drops it when it
// returns. The ISO calendar has no such loops, and is c itself.
func (c *Calendar) WithCheck(check func()) *Calendar {
	if c.id == "iso8601" || check == nil {
		return c
	}
	d := *c
	d.check = check
	return &d
}

// checkEvery is how many turns of a loop of the calendar go between two
// checks: a turn is a month or a year, which takes a few nanoseconds, or
// some hundreds where a lunisolar year is reckoned.
const checkEvery = 1 << 12

// every counts a turn of a loop in *n, and checks the call every
// checkEvery turns.
func (c *Calendar) every(n *int) {
	if *n++; *n&(checkEvery-1) == 0 && c.check != nil {
		c.check()
	}
}
