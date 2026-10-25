/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_STDDEF_H
#define SLOWJS_LIBC_STDDEF_H

typedef __SIZE_TYPE__ size_t;
typedef __PTRDIFF_TYPE__ ptrdiff_t;

#define NULL ((void *)0)
#define offsetof(type, member) __builtin_offsetof(type, member)

#endif
