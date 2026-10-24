// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import "unsafe"

// TforkRuntime is what this fork keeps in each runtime: the field Ffork
// that internal/fixlib adds at the end of JSRuntime, after the fields of the
// C struct, which keep their offsets (see fixRuntimeField). JS_NewRuntime2
// allocates it with the C struct and clears it.
type TforkRuntime struct {
	ephemeron uint8        // temporary collector phase, see ephemeron.go
	clock     bool         // whether clocks holds a custom clock for this runtime
	front     TfrontCache  // see frontcache.go
	shape     TshapeCache  // see shapecache.go
	fn        TfuncShapes  // see funcshape.go
	regexp    TregexpCache // see regexcache.go
	young     Tyoung       // see younggc.go
	keys      TownKeyCache // see ownkeys.go
	// sharedDebug is set while JS_ReadObject reads with ReadObjSharedDebug,
	// see shareddebug.go.
	sharedDebug bool
}

// forkRuntimeSize is the size of TforkRuntime, which JS_NewRuntime2
// allocates with the C struct.
const forkRuntimeSize = unsafe.Sizeof(TforkRuntime{})

// forkOf returns the state of the runtime whose malloc_ctx is at rt: the
// allocator passes the address of malloc_ctx around, and malloc_ctx is the
// first field of JSRuntime.
func forkOf(rt uintptr) *TforkRuntime {
	return &(*TJSRuntime)(unsafe.Pointer(rt)).Ffork
}
