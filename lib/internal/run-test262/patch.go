// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

func _js_check_stack_overflow(tls *libc.TLS, rt uintptr, sz libquickjs.Size_t) int32 {
	if tls.StackSlots() < libquickjs.MaxStackSlots {
		return 0
	}

	return 1
}
