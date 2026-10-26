// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"regexp"
	"strings"
)

// PollCounterOffset is where the poll counter is by default, from the base of
// the shadow-stack region (pass/poll.cpp, -poll-offset of cmd/engine). The
// host keeps the start of the region for what it passes the engine by
// address, and this word among it.
const PollCounterOffset = 384

// PollLLCFlags are the llc options an engine with polls is built with. The
// tick in the interpreter's dispatch gives the dispatch block more
// predecessors and successors than llc's tail duplication takes by default,
// and without the copies of the dispatch at the end of every handler the
// interpreter is much slower. CheckDispatchCopies holds the output to it.
var PollLLCFlags = []string{"-tail-dup-pred-size=1000", "-tail-dup-succ-size=1000"}

// DispatchCopies is the least number of indirect jumps each function with a
// computed goto table must have in llc's output: one at the end of each
// handler, where llc copied the dispatch. A function with a table and no
// entry here fails, so that a new one gets a number of its own.
// JS_CallInternal has 245 (204 in an engine without polls, which llc builds
// without these options), and its least number is about a tenth below.
var DispatchCopies = map[string]int{"JS_CallInternal": 220}

// CheckPollOffset checks -poll-offset: a word inside the region.
func CheckPollOffset(offset, sizeLog2 int) error {
	if offset < 0 || offset%8 != 0 || offset+8 > 1<<sizeLog2 {
		return fmt.Errorf("-poll-offset=%d: not a word inside the shadow-stack region", offset)
	}
	return nil
}

var reIndirectJmp = regexp.MustCompile(`^jmpq?\s+\*%r\w+$`)

// CheckDispatchCopies counts the indirect jumps in the llc text of a function
// that has a computed goto table and fails when there are fewer than
// DispatchCopies asks.
func CheckDispatchCopies(name string, lines []string) (int, error) {
	n := 0
	for _, l := range lines {
		if reIndirectJmp.MatchString(norm(clean(l))) {
			n++
		}
	}
	min, ok := DispatchCopies[name]
	if !ok {
		return n, fmt.Errorf("a computed goto table in %s, which has no least number of dispatch copies (gen.DispatchCopies)", name)
	}
	if n < min {
		return n, fmt.Errorf("%d indirect jumps, fewer than the %d copies of the dispatch it must have: llc stopped copying the dispatch into the handlers (%s)",
			n, min, strings.Join(PollLLCFlags, " "))
	}
	return n, nil
}
