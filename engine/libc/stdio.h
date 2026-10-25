/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_STDIO_H
#define SLOWJS_LIBC_STDIO_H

#include <stdarg.h>
#include <stddef.h>

/* The formatter of stdio.c. It writes to a buffer and nothing else: the
   C tree has no files and no standard streams. */
int snprintf(char *restrict buf, size_t size, const char *restrict fmt, ...)
    __attribute__((format(printf, 3, 4)));
int vsnprintf(char *restrict buf, size_t size, const char *restrict fmt, va_list ap)
    __attribute__((format(printf, 3, 0)));

#endif
