// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// upSites are the statements of each generated file where fixUp drops the
// order between a read of memory in the address of a conversion and a call
// after it, and how many times each comes (see upOrderOf). Each was
// reviewed: no call after the read changes what it reads. The 13 files
// have the same list.
var upSites = map[string]int{
	// ctx->class_proto[i] = f(...): the calls make objects and strings and
	// do not move class_proto. Only JS_NewClass moves it, for a class id at
	// or past the class count, as the backtrace class of lib/backtrace.go
	// is, which a runtime registers with its first pending stack. Here that
	// needs a context that runs out of memory while it is made, and then
	// the store goes to the old array, as it did in the upstream form.
	"XJS_AddIntrinsicBaseObjects: (*TJSContext)(unsafe.Pointer(ctx6)).Fclass_proto + uintptr(EJS_CLASS_ARRAY_ITERATOR)*N -> _JS_NewObjectProtoList":       1,
	"XJS_AddIntrinsicBaseObjects: (*TJSContext)(unsafe.Pointer(ctx6)).Fclass_proto + uintptr(EJS_CLASS_GENERATOR)*N -> _JS_NewObjectProtoList":            1,
	"XJS_AddIntrinsicBaseObjects: (*TJSContext)(unsafe.Pointer(ctx6)).Fclass_proto + uintptr(EJS_CLASS_ITERATOR_CONCAT)*N -> _JS_NewObjectProtoList":      1,
	"XJS_AddIntrinsicBaseObjects: (*TJSContext)(unsafe.Pointer(ctx6)).Fclass_proto + uintptr(EJS_CLASS_ITERATOR_HELPER)*N -> _JS_NewObjectProtoList":      1,
	"XJS_AddIntrinsicBaseObjects: (*TJSContext)(unsafe.Pointer(ctx6)).Fclass_proto + uintptr(EJS_CLASS_ITERATOR_WRAP)*N -> _JS_NewObjectProtoList":        1,
	"XJS_AddIntrinsicBaseObjects: (*TJSContext)(unsafe.Pointer(ctx6)).Fclass_proto + uintptr(EJS_CLASS_STRING)*N -> XJS_AtomToString":                     1,
	"XJS_AddIntrinsicBaseObjects: (*TJSContext)(unsafe.Pointer(ctx6)).Fclass_proto + uintptr(EJS_CLASS_STRING_ITERATOR)*N -> _JS_NewObjectProtoList":      1,
	"XJS_AddIntrinsicMapSet: (*TJSContext)(unsafe.Pointer(ctx1)).Fclass_proto + uintptr(int32(EJS_CLASS_MAP_ITERATOR)+i)*N -> _JS_NewObjectProtoList":     1,
	"XJS_AddIntrinsicPromise: (*TJSContext)(unsafe.Pointer(ctx1)).Fclass_proto + uintptr(EJS_CLASS_ASYNC_FROM_SYNC_ITERATOR)*N -> _JS_NewObjectProtoList": 1,
	"XJS_AddIntrinsicPromise: (*TJSContext)(unsafe.Pointer(ctx1)).Fclass_proto + uintptr(EJS_CLASS_ASYNC_GENERATOR)*N -> _JS_NewObjectProtoList":          1,
	"XJS_AddIntrinsicRegExp: (*TJSContext)(unsafe.Pointer(ctx)).Fclass_proto + uintptr(EJS_CLASS_REGEXP_STRING_ITERATOR)*N -> _JS_NewObjectProtoList":     1,
	"_JS_AddIntrinsicBasicObjects: (*TJSContext)(unsafe.Pointer(ctx3)).Fclass_proto + uintptr(EJS_CLASS_OBJECT)*N -> _JS_NewObjectProtoClassAlloc":        1,
	// *p is the position in the text: the calls only classify a character.
	"Xjs_atod: *(*uintptr)(unsafe.Pointer(bp + N)) -> _to_digit1":                                                                                                               4,
	"_js_atof: *(*uintptr)(unsafe.Pointer(bp + N)) -> _to_digit":                                                                                                                1,
	"_js_atof: *(*uintptr)(unsafe.Pointer(bp + N)) ; *(*uintptr)(unsafe.Pointer(bp + N)) -> _is_digit":                                                                          1,
	"_js_atof: *(*uintptr)(unsafe.Pointer(bp + N)) ; *(*uintptr)(unsafe.Pointer(bp + N)) -> _to_digit":                                                                          1,
	"_js_atof: *(*uintptr)(unsafe.Pointer(bp + N)) ; *(*uintptr)(unsafe.Pointer(bp + N)) ; *(*uintptr)(unsafe.Pointer(bp + N)) + uintptr(-libc.Int32FromInt32(N)) -> _to_digit": 1,
	// rt->atom_array[i] = atom_set_free(...): it only computes a value.
	"_JS_FreeAtomStruct: (*TJSRuntime)(unsafe.Pointer(rt)).Fatom_array + uintptr(i)*N -> _atom_set_free": 1,
	"___JS_NewAtom: (*TJSRuntime)(unsafe.Pointer(rt)).Fatom_array + uintptr(i)*N -> _atom_set_free":      1,
	// The reads are on the left of &&, which Go evaluates first.
	"_js_date_parse_otherstring: sp + uintptr(*(*int32)(unsafe.Pointer(bp + N))) ; sp + uintptr(*(*int32)(unsafe.Pointer(bp + N))) -> _string_get_tzoffset": 1,
}
