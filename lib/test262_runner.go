// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import "modernc.org/libc"

// X__JS_FreeValueRT is __JS_FreeValueRT, which ccgo does not export. The
// inline JS_FreeValue of the test262 runner in internal/run-test262 calls it
// when a reference count drops to zero, and internal/fixlib makes the runner
// call the engine in this package.
func X__JS_FreeValueRT(tls *libc.TLS, rt uintptr, v TJSValue) {
	x__JS_FreeValueRT(tls, rt, v)
}
