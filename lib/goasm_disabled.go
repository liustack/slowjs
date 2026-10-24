//go:build !goasm || !linux || !amd64

// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package libquickjs

import "modernc.org/libc"

const goasmEnabled = false

func goasmCall(tls *libc.TLS, ctx uintptr, fn, this, newTarget TJSValue, argc int32, argv uintptr, flags int32) TJSValue {
	panic("unreachable: goasm disabled")
}
