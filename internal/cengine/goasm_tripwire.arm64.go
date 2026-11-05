// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build arm64

package cengine

// goasmTripwireVar is read by the tripwire function of goasm_tripwire.arm64.s.
var goasmTripwireVar uintptr

// goasmTripwire is never called and never linked. It exists so that the
// assembler stops a build that would break the engine (see
// goasm_tripwire.arm64.s and docs/upgrading.md).
func goasmTripwire()
