// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine && cenginehammer && amd64

#include "textflag.h"

// hammerG returns the g of the calling goroutine, which the runtime keeps
// in TLS on amd64.
TEXT ·hammerG(SB), NOSPLIT, $0-8
	MOVQ (TLS), AX
	MOVQ AX, ret+0(FP)
	RET
