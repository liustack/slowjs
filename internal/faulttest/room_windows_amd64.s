// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

#include "textflag.h"
#include "funcdata.h"

// Room does nothing, but its stack check grows the stack for a frame it never
// touches, as goasm_room of the generated engine does: its frame is
// gen.RoomFrame (internal/goasm/gen/emit.go), which a test of the generator
// holds it to.
TEXT ·Room(SB), $12416-0
	NO_LOCAL_POINTERS
	RET
