// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

#include "textflag.h"

// func clientRequest(args *[6]uint64) uint64
//
// The client request of valgrind on amd64: AX holds the address of the
// arguments, DX the result outside valgrind, and the four rotations of DI,
// which add up to a whole turn, and the exchange of BX with itself change
// nothing when valgrind does not run the program.
TEXT ·clientRequest(SB), NOSPLIT, $0-16
	MOVQ args+0(FP), AX
	MOVQ $0, DX
	ROLQ $3, DI
	ROLQ $13, DI
	ROLQ $61, DI
	ROLQ $51, DI
	XCHGQ BX, BX
	MOVQ DX, ret+8(FP)
	RET
