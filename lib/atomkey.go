// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// atomKey is the start of JS_ValueToAtom for a key that is a string, the
// JSString p (internal/fixlib puts the call there, see fixAtomKey). A string
// that is already an atom and does not start with a digit is its own atom.
// For it, JS_ValueToAtom dups the string (JS_ToPropertyKey), finds that it is
// no index (is_num_string is false for a string that does not start with a
// digit), and takes its index from the atom table (__JS_NewAtom), keeping the
// dup as the reference of the atom, or dropping it for a constant atom.
// atomKey gives the same atom and the same counts in one call, with no TLS
// frame. ok is false for every other string, which JS_ValueToAtom converts
// as before.
//
// atomKeyTypeOff, atomKeyTypeMask and atomKeyTypeShift are the atom type of
// a JSString, atomKeyLenMask and atomKeyLenShift its length, as the file's
// own __JS_NewAtom and is_num_string read them (see fixAtomKey).
func atomKey(tls *libc.TLS, ctx uintptr, p uintptr) (atom TJSAtom, ok bool) {
	if *(*uint32)(unsafe.Pointer(p + atomKeyTypeOff))&atomKeyTypeMask>>atomKeyTypeShift != EJS_ATOM_TYPE_STRING ||
		*(*uint32)(unsafe.Pointer(p))&atomKeyLenMask>>atomKeyLenShift == 0 {
		return 0, false
	}
	if c := _string_get(tls, p, 0); c >= '0' && c <= '9' {
		return 0, false
	}
	atom = _js_get_atom_index(tls, (*TJSContext)(unsafe.Pointer(ctx)).Frt, p)
	if ___JS_AtomIsConst(tls, atom) == 0 {
		(*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, p))).Fref_count++
	}
	return atom, true
}
