// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// forInNoEnum reports whether p, an object of the prototype chain of a for
// in loop, has no enumerable property, read from its shape. When it has
// none it also stores an empty list of names in *ptab and *plen, as
// JS_GetOwnPropertyNamesInternal does for it with JS_GPN_STRING_MASK and
// JS_GPN_ENUM_ONLY, so that js_for_in_prepare_prototype_chain_enum can skip
// that call (internal/fixlib puts the call there, see fixForIn).
//
// For an object that is not exotic, JS_GetOwnPropertyNamesInternal counts
// the shape properties with an atom, the enumerable flag and a string key,
// and nothing else: no method of the class is called and no exception is
// thrown for a property it does not count. So when no shape property has an
// atom and the flag, its list is empty. forInNoEnum answers false for every
// other object, also for one whose only enumerable keys are symbols, which
// JS_GetOwnPropertyNamesInternal leaves out, and the caller lists the names
// as before. The empty list it gives needs no memory, where
// JS_GetOwnPropertyNamesInternal mallocs one entry, so with no memory left
// the loop goes on where it used to fail.
//
// forInExoticOff, forInExotic and the flag constants are the bit fields of
// the target, which fixForIn reads from the file's
// JS_GetOwnPropertyNamesInternal.
func forInNoEnum(tls *libc.TLS, p, ptab, plen uintptr) bool {
	if *(*uint8)(unsafe.Pointer(p + forInExoticOff))&forInExotic != 0 {
		return false
	}
	sh := (*TJSObject)(unsafe.Pointer(p)).Fshape
	prs := _get_shape_prop(tls, sh)
	for i := int32(0); i < (*TJSShape)(unsafe.Pointer(sh)).Fprop_count; i++ {
		pr := (*TJSShapeProperty)(unsafe.Pointer(prs + uintptr(i)*unsafe.Sizeof(TJSShapeProperty{})))
		if pr.Fatom != MJS_ATOM_NULL && *(*uint32)(unsafe.Pointer(pr))&forInFlagsMask>>forInFlagsShift&MJS_PROP_ENUMERABLE != 0 {
			return false
		}
	}
	*(*uintptr)(unsafe.Pointer(ptab)) = 0
	*(*Tuint32_t)(unsafe.Pointer(plen)) = 0
	return true
}
