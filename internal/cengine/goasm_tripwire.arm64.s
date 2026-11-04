// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build arm64

#include "textflag.h"

// This function is never called and never linked. It assembles only when the
// assembler keeps each instruction as written, which the generated engine
// (engine_*_arm64.s) needs: it writes the bodies of its functions as words with
// the branch offsets fixed, and leaves only the loads of symbol addresses
// between them to the assembler. A build option that makes the assembler encode
// those with other lengths moves the targets of the fixed branches, and the
// engine crashes on its first call, though the build succeeds.
// See docs/upgrading.md ("Moving to a newer Go").
// The name has a dot and no underscore before the architecture, because the
// generator deletes every file that matches *_arm64.s when it makes the engine
// again.
TEXT ·goasmTripwire(SB), NOSPLIT|NOFRAME, $0-0
	// BUILD ERROR: liustack/quickjs does not build with -dynlink, which the go command
	// passes for -buildmode=plugin, -buildmode=shared and -linkshared. It
	// rewrites a load of a symbol address into a load through the GOT, and the
	// engine's branches land wrong. Build without it.
	MOVWU $·goasmTripwireVar(SB), R0
	RET
