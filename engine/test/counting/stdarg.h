/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/* libc-test.sh puts this folder in front of engine/libc when it builds stdio.c,
   so that the formatter counts the arguments it takes. The test can then tell
   a format that is refused before any argument is read from one that reads
   arguments and refuses after. */
#ifndef SLOWJS_TEST_COUNTING_STDARG_H
#define SLOWJS_TEST_COUNTING_STDARG_H

#include_next <stdarg.h>

extern unsigned long sl_va_count;
#undef va_arg
#define va_arg(ap, type) (sl_va_count++, __builtin_va_arg(ap, type))

#endif
