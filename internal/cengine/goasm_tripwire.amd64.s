// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build amd64

#include "textflag.h"

// This function is never called and never linked. It assembles only when the
// assembler keeps each instruction as written, which the generated engine
// (engine_*_amd64.s) needs: it writes the bodies of its functions as bytes with
// the branch offsets fixed, and leaves only the calls and the loads of symbol
// addresses between them to the assembler. A build option that makes the
// assembler encode those with other lengths moves the targets of the fixed
// branches, and the engine crashes on its first call, though the build
// succeeds. Both options below are refused here instead.
// See docs/upgrading.md ("Moving to a newer Go").
// The name has a dot and no underscore before the architecture, because the
// generator deletes every file that matches *_amd64.s when it makes the engine
// again.
TEXT ·goasmTripwire(SB), NOSPLIT|NOFRAME, $0-0
	// BUILD ERROR: liustack/quickjs does not build with -asmflags=-spectre=ret (or all).
	// It turns the 2 byte CALL through a register into a 5 byte call through
	// a retpoline, and the engine's branches land wrong. Build without it.
	CALL (AX)
	// BUILD ERROR: liustack/quickjs does not build with -dynlink, which the go command
	// passes for -buildmode=plugin, -buildmode=shared and -linkshared. It
	// splits a load of a symbol address into two instructions and uses R15
	// for it, and the engine's branches land wrong. Build without it.
	MOVQ ·goasmTripwireVar(SB), AX
	MOVQ R15, BX
	RET
