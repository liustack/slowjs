// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package faulttest

// Room makes room below the stack pointer of its caller for the state of the
// CPU that windows writes at a fault: 12 KiB past Go's stack guard, more than
// 16.9 KiB in all, as every function of the generated engine has. A test calls
// it right before the access that faults, in the same function and with no
// call between: a call may stop the goroutine, and its stack may shrink then.
func Room()
