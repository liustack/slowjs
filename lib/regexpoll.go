// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

// Interrupts while compiling a regexp.
//
// The interpreter calls the interrupt handler once every
// JS_INTERRUPT_COUNTER_INIT polls, and compiling a regexp is one
// instruction, however long the pattern. A loop that compiled long patterns
// ran thousands of compiles between two calls of the handler, so the eval
// timeout and Interrupt stopped it hundreds of milliseconds late. This file
// is the Go form of the change in internal/patch/quickjs.diff that makes
// the regexp parser count its steps on the same counter:
//
//   - Each term, alternative and atom of a class counts one step, and one
//     more for each 256 bytes of code or ranges it moves, through
//     re_poll_interrupts.
//   - When the handler asks to stop, the parser fails with the message
//     "interrupted", and js_compile_regexp throws the interrupt for it, as
//     js_regexp_exec does when lre_exec times out.
//   - A lookup in the regexp cache of lib/regexcache.go counts a step for
//     each 32 bytes of the pattern it hashes and compares, through
//     _js_charge_interrupts of lib/charge.go. That change has no C form, as the cache has none.
//
// internal/fixlib wires these into every ccgo_*.go file (see fixRegexPoll).

import (
	"unsafe"

	"modernc.org/libc"
)

// Xlre_poll_interrupts counts cost steps toward the interrupt counter of
// the context opaque, and calls the interrupt handler when the counter runs
// out, as js_poll_interrupts does. It reports whether the handler asks to
// stop, and throws nothing.
func Xlre_poll_interrupts(tls *libc.TLS, opaque uintptr, cost int32) int32 {
	ctx := (*TJSContext)(unsafe.Pointer(opaque))
	ctx.Finterrupt_counter -= cost
	if ctx.Finterrupt_counter > 0 {
		return 0
	}
	ctx.Finterrupt_counter = MJS_INTERRUPT_COUNTER_INIT
	return Xlre_check_timeout(tls, opaque)
}

// stringBytes returns the bytes of the characters of the string p, a
// JSString: the length is the low 31 bits of its first word, and the top
// bit says the characters are 16-bit (see _hash_string).
func stringBytes(p uintptr) uint32 {
	w := *(*uint32)(unsafe.Pointer(p))
	return (w & 0x7fffffff) << (w >> 31)
}

// reInterrupted is the message the regexp parser fails with when the
// interrupt handler asks to stop.
var reInterrupted = [...]byte{'i', 'n', 't', 'e', 'r', 'r', 'u', 'p', 't', 'e', 'd', 0}

// _re_poll_interrupts counts one step of the regexp parser that moves
// moved bytes. When the interrupt handler asks to stop, it fails the parse
// with the message _js_regexp_interrupted looks for.
func _re_poll_interrupts(tls *libc.TLS, s uintptr, moved Tsize_t) int32 {
	if Xlre_poll_interrupts(tls, (*TREParseState)(unsafe.Pointer(s)).Fopaque, 1+int32(moved>>8)) != 0 {
		return _re_parse_error(tls, s, uintptr(unsafe.Pointer(&reInterrupted)), 0)
	}
	return 0
}

// _js_regexp_interrupted reports whether the error message lre_compile
// wrote to msg is the one for an interrupt.
func _js_regexp_interrupted(msg uintptr) bool {
	return libc.GoString(msg) == "interrupted"
}
