/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_ASSERT_H
#define SLOWJS_LIBC_ASSERT_H

/* A failed assertion is a trap: there is no stderr to write to. */
#ifdef NDEBUG
#define assert(x) ((void)0)
#else
#define assert(x) ((void)((x) || (__builtin_trap(), 0)))
#endif

#endif
