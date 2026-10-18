// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// objlitDefine is the start of JS_DefinePropertyValue (internal/fixlib puts
// the call there, see fixDefine). It handles the case every object literal,
// spread, class field, JSON.parse and new function goes through: a data
// property that an ordinary extensible object does not have yet. That is
// what JS_DefineProperty does when find_own_property finds nothing and the
// object is no fast array, and JS_CreateProperty when the object is neither
// exotic nor the global object and is extensible: add_property with the
// C_W_E bits of flags, then the value. It takes the reference of val, as
// JS_DefinePropertyValue does. ok is false, with nothing done, for every
// other case, which then runs the generic path.
//
// The generic path costs three calls with a TLS frame and a deferred free
// each, and copies of four JSValues per call.
//
// objlitFlagsOff and the three masks are the JSObject bit fields of the
// target, which fixDefine reads from the file's JS_NewObjectFromShape.
//
// It writes only the value of the slot add_property has just made and reads
// no slot, so the slot audit of fixlib (slotsites.go) has nothing to check
// here.
func objlitDefine(tls *libc.TLS, ctx uintptr, obj TJSValue, prop TJSAtom, val TJSValue, flags int32) (r int32, ok bool) {
	if jsvTag(obj) != EJS_TAG_OBJECT || flags&(MJS_PROP_HAS_GET|MJS_PROP_HAS_SET) != 0 {
		return 0, false
	}
	p := jsvPtr(obj)
	if *(*uint8)(unsafe.Pointer(p + objlitFlagsOff))&(objlitExtensible|objlitExotic|objlitFastArray) != objlitExtensible ||
		(*TJSObject)(unsafe.Pointer(p)).Fclass_id == EJS_CLASS_GLOBAL_OBJECT ||
		_find_own_property1(tls, p, prop) != 0 {
		return 0, false
	}
	pr := _add_property(tls, ctx, p, prop, flags&(MJS_PROP_CONFIGURABLE|MJS_PROP_WRITABLE|MJS_PROP_ENUMERABLE))
	if pr == 0 {
		// add_property threw out of memory.
		XFreeValue(tls, ctx, val)
		return -1, true
	}
	// pr->u.value: the value is the first member of the JSProperty union.
	*(*TJSValue)(unsafe.Pointer(pr)) = val
	return 1, true
}
