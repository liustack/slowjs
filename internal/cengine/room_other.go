// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build !(windows && amd64)

package cengine

// makeRoom does nothing: only windows writes the state of the CPU at a fault
// onto the goroutine's stack, and New runs on windows/arm64 only where it
// fits in the 4 KiB Go keeps (internal/faultstate, room_windows_amd64.go,
// docs/internals/changes/c-engine.md).
func makeRoom() {}
