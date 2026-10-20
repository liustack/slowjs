// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
