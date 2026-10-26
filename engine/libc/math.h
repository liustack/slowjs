/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_LIBC_MATH_H
#define SLOWJS_LIBC_MATH_H

/* double only. FLT_EVAL_METHOD is 0, so double_t is double. The functions are
   musl's (see README.md), except sqrt, which is one instruction. */
typedef float float_t;
typedef double double_t;

#define NAN __builtin_nanf("")
#define INFINITY __builtin_inff()
#define HUGE_VAL __builtin_huge_val()

double acos(double);
double acosh(double);
double asin(double);
double asinh(double);
double atan(double);
double atan2(double, double);
double atanh(double);
double cbrt(double);
double ceil(double);
double copysign(double, double);
double cos(double);
double cosh(double);
double exp(double);
double expm1(double);
double fabs(double);
double floor(double);
double fmax(double, double);
double fmin(double, double);
double fmod(double, double);
double hypot(double, double);
double log(double);
double log10(double);
double log1p(double);
double log2(double);
long lrint(double);
double modf(double, double *);
double pow(double, double);
double rint(double);
double round(double);
double scalbn(double, int);
double sin(double);
double sinh(double);
double sqrt(double);
double tan(double);
double tanh(double);
double trunc(double);

/* The builtins give the same answers as the functions, and the compiler
   expands them in line. Taking the address of one (the table of Math
   functions does) still gives the function. -fno-math-errno keeps sqrt a
   plain instruction. */
#define isnan(x) __builtin_isnan(x)
#define isinf(x) __builtin_isinf(x)
#define isfinite(x) __builtin_isfinite(x)
#define signbit(x) __builtin_signbit(x)
#define fabs(x) __builtin_fabs(x)
#define sqrt(x) __builtin_sqrt(x)

#endif
