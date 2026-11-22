// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !(windows && amd64)

package cengine

// makeRoom does nothing: only windows writes the state of the CPU at a fault
// onto the goroutine's stack, and on windows/arm64 it fits in the 4 KiB Go
// keeps (room_windows_amd64.go, docs/changes/c-engine.md).
func makeRoom() {}
