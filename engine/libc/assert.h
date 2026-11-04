/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef QUICKJS_LIBC_ASSERT_H
#define QUICKJS_LIBC_ASSERT_H

/* A failed assertion is an abort: there is no stderr to write to. */
#include <stdlib.h>
#ifdef NDEBUG
#define assert(x) ((void)0)
#else
#define assert(x) ((void)((x) || (abort(), 0)))
#endif

#endif
