/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_INTTYPES_H
#define SLOWJS_LIBC_INTTYPES_H

#include <stdint.h>

/* The conversions of the formatter in stdio.c, for the 64-bit types. clang
   spells the right length modifier of each target in __INT64_FMTd__. */
#define PRId32 "d"
#define PRIu32 "u"
#define PRIx32 "x"
#define PRIX32 "X"
#define PRId64 __INT64_FMTd__
#define PRIu64 __UINT64_FMTu__
#define PRIx64 __UINT64_FMTx__
#define PRIX64 __UINT64_FMTX__

#endif
