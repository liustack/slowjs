// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !testhooks

package main

import "github.com/liustack/beastjs"

// canAbort says whether this build can make the engine abort: it needs the
// tag testhooks.
const canAbort = false

func polls(*beastjs.VM) int64 { return 0 }

func abortAfterPolls(*beastjs.VM, int64) {
	panic("stress: an abort needs a build with -tags testhooks")
}
