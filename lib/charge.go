// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/quickjs/lib"

// Work of built-ins on the interrupt counter.
//
// The interpreter calls the interrupt handler once every
// JS_INTERRUPT_COUNTER_INIT polls, and a call of a built-in is one poll,
// however much it does. A loop that parsed, compiled or built long strings
// ran thousands of calls between two calls of the handler. This file is the
// Go form of js_charge_interrupts in internal/patch/quickjs.diff, which
// counts that work on the same counter:
//
//   - js_parse_init counts a step for each byte of the source it is given
//     to parse, for eval, new Function, scripts, modules and JSON.parse:
//     parsing and compiling cost far more for each byte than copying.
//   - string_buffer_init2 and string_buffer_realloc count a step for each 8
//     characters they make room for, for every built-in that builds a
//     string in a StringBuffer, such as JSON.stringify,
//     Array.prototype.join, String.prototype.repeat, padStart and
//     replaceAll.
//
// A step counted too many costs only an earlier call of the handler, which
// reads a flag and the clock: a step counted too few lets the work run on
// past the timeout.
//
// internal/fixlib wires these into every ccgo_*.go file (see
// fixWorkCharge).

import "unsafe"

// _js_charge_interrupts counts cost steps of work toward the interrupt
// counter of ctx and leaves the call of the interrupt handler to the next
// poll of the interpreter, for work that cannot stop where it is. The
// counter stops at 0, so that it cannot wrap.
func _js_charge_interrupts(ctx uintptr, cost Tsize_t) {
	c := &(*TJSContext)(unsafe.Pointer(ctx)).Finterrupt_counter
	if *c > 0 && cost < Tsize_t(*c) {
		*c -= int32(cost)
	} else {
		*c = 0
	}
}
