// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

// expect64 and expect32 stand for the likely and unlikely of QuickJS,
// __builtin_expect(!!(x), c), which ccgo writes as
// libc.BoolInt64(!!(x)) != 0, or as libc.BoolInt32(!!(x)) != 0 where a
// long has 32 bits. For such a test Go makes the int and tests it, when x
// has && or ||. internal/fixlib writes each one as expect64(!!(x)) or
// expect32(!!(x)) instead (see fixExpect). Go inlines these, so it
// branches on x itself.

func expect64(b bool) bool { return b }

func expect32(b bool) bool { return b }
