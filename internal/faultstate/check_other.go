// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !(windows && arm64)

package faultstate

// queried reports whether Check asks the system.
const queried = false

// Check returns nil: on linux and darwin the kernel writes the state of the
// CPU at a fault on the stack of the signal, and windows/amd64 makes the room
// in the generated code.
func Check() error { return nil }
