// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

import "modernc.org/libc"

// X__JS_FreeValueRT is __JS_FreeValueRT, which ccgo does not export. The
// inline JS_FreeValue of the test262 runner in internal/run-test262 calls it
// when a reference count drops to zero, and internal/fixlib makes the runner
// call the engine in this package.
func X__JS_FreeValueRT(tls *libc.TLS, rt uintptr, v TJSValue) {
	x__JS_FreeValueRT(tls, rt, v)
}
