// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !(windows && amd64)

package cengine

// makeRoom does nothing: only windows writes the state of the CPU at a fault
// onto the goroutine's stack (room_windows_amd64.go).
func makeRoom() {}
