/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/* The two functions of math.h that are one instruction. musl's are in
   software. The parentheses keep the macros of math.h from rewriting the
   names. Built with -fno-math-errno, or sqrt of a negative number would call
   sqrt. */
#include <math.h>

#ifndef __NO_MATH_ERRNO__
#error build with -fno-math-errno
#endif

double (fabs)(double x)
{
	return __builtin_fabs(x);
}

double (sqrt)(double x)
{
	return __builtin_sqrt(x);
}
