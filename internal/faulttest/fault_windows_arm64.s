// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

#include "textflag.h"

// func fillAndFault(wild uintptr, top *uintptr, span uintptr)
TEXT ·fillAndFault(SB), NOSPLIT|NOFRAME, $0-24
	MOVD	wild+0(FP), R0
	MOVD	top+8(FP), R1
	MOVD	span+16(FP), R2
	MOVD	RSP, R3
	MOVD	R3, (R1)
	SUB	R2, R3, R4
	SUB	$16, R3, R5
	MOVD	$0x5a5a5a5a5a5a5a5a, R6
fill:
	CMP	R5, R4
	BHS	fault
	EOR	R4, R6, R7
	MOVD	R7, (R4)
	ADD	$8, R4
	B	fill
fault:
	MOVD	(R0), R7
	MOVD	R7, R0
	RET

// func written(top, span uintptr) uintptr
TEXT ·written(SB), NOSPLIT|NOFRAME, $0-24
	MOVD	top+0(FP), R3
	MOVD	span+8(FP), R2
	SUB	R2, R3, R4
	SUB	$16, R3, R5
	MOVD	$0x5a5a5a5a5a5a5a5a, R6
scan:
	CMP	R5, R4
	BHS	none
	MOVD	(R4), R7
	EOR	R4, R6, R8
	CMP	R8, R7
	BNE	found
	ADD	$8, R4
	B	scan
found:
	SUB	R4, R3, R0
	MOVD	R0, ret+16(FP)
	RET
none:
	MOVD	ZR, ret+16(FP)
	RET
