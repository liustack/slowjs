// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import "unsafe"

// TforkRuntime is what this fork keeps in each runtime: the field Ffork
// that internal/fixlib adds at the end of JSRuntime, after the fields of the
// C struct, which keep their offsets (see fixRuntimeField). JS_NewRuntime2
// allocates it with the C struct and clears it.
type TforkRuntime struct {
	front TfrontCache // see frontcache.go
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
