// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/quickjs/lib"

import "unsafe"

// pollDue counts down the interrupt counter at p, a JSContext's
// interrupt_counter, and reports whether it ran out, as js_poll_interrupts
// does before it calls __js_poll_interrupts. internal/fixlib puts it into
// each poll of JS_CallInternal (see fixPoll), so that the countdown is
// inlined there: JS_CallInternal is too large for Go to inline
// js_poll_interrupts into it, and every poll was a call.
func pollDue(p uintptr) bool {
	c := (*int32)(unsafe.Pointer(p))
	*c--
	return *c <= 0
}
