/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_FLOAT_H
#define SLOWJS_LIBC_FLOAT_H

/* double and float only: the C tree has no long double. */
#define FLT_RADIX 2
#define FLT_EVAL_METHOD 0

#define FLT_MANT_DIG __FLT_MANT_DIG__
#define FLT_EPSILON __FLT_EPSILON__
#define FLT_MIN __FLT_MIN__
#define FLT_MAX __FLT_MAX__
#define DBL_MANT_DIG __DBL_MANT_DIG__
#define DBL_EPSILON __DBL_EPSILON__
#define DBL_MIN __DBL_MIN__
#define DBL_MAX __DBL_MAX__
#define DBL_MIN_EXP __DBL_MIN_EXP__
#define DBL_MAX_EXP __DBL_MAX_EXP__

#endif
