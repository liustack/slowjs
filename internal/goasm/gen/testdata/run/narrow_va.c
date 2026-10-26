// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Narrow fixed parameters and results in front of a variable-argument block
// (TestSampleRuns, with -varargs-block). The pass moves the variable
// arguments into a block on the shadow stack, then passes the fixed narrow
// ones as i32 words like any other unified call: a narrow value must reach the
// callee whole, before and after the block, at the shadow pointer, through a
// pointer to the variadic function, and from a function that forwards its
// va_list. Narrow values among the variable arguments are promoted to int by
// C, so they travel as ints.
typedef __builtin_va_list va_list;
#define va_start __builtin_va_start
#define va_arg __builtin_va_arg
#define va_end __builtin_va_end
typedef unsigned char uint8_t;
typedef signed char int8_t;
typedef unsigned short uint16_t;
typedef short int16_t;

// Two narrow fixed parameters and a bool, then a count and the variable
// arguments: ints, and a double after them.
__attribute__((noinline)) long tagged(uint8_t tag, int16_t d, _Bool flag, int n, ...)
{
    va_list ap;
    va_start(ap, n);
    long s = tag * 1000003L + d * 1009L + flag * 17L;
    for (int i = 0; i < n; i++)
        s = s * 7 + va_arg(ap, int);
    s += (long)(va_arg(ap, double) * 8);
    va_end(ap);
    return s;
}

// The same signature as tagged, so that a table holding both keeps the call
// through it.
__attribute__((noinline)) long tagged2(uint8_t tag, int16_t d, _Bool flag, int n, ...)
{
    va_list ap;
    va_start(ap, n);
    long s = tag * 13L - d * 3L + flag;
    for (int i = 0; i < n; i++)
        s = s * 5 - va_arg(ap, int);
    s += (long)(va_arg(ap, double) * 2);
    va_end(ap);
    return s;
}

// A narrow result as well: the sum of narrow fixed values and what follows.
__attribute__((noinline)) int8_t small(int8_t a, uint16_t b, int n, ...)
{
    va_list ap;
    va_start(ap, n);
    int s = a * 3 + b;
    for (int i = 0; i < n; i++)
        s += va_arg(ap, int) * (i + 2);
    va_end(ap);
    return (int8_t)s;
}

__attribute__((noinline)) _Bool oddsum(_Bool first, uint8_t n, ...)
{
    va_list ap;
    va_start(ap, n);
    int s = first;
    for (int i = 0; i < n; i++)
        s += va_arg(ap, int);
    va_end(ap);
    return s & 1;
}

// A function that forwards its va_list, with narrow fixed parameters of its
// own, to one that reads it.
__attribute__((noinline)) long readv(uint8_t tag, int16_t d, int n, va_list ap)
{
    long s = tag * 31L + d;
    for (int i = 0; i < n; i++)
        s = s * 5 + va_arg(ap, long);
    return s;
}

__attribute__((noinline)) long forward(uint8_t tag, int16_t d, int n, ...)
{
    va_list ap;
    va_start(ap, n);
    long s = readv(tag, d, n, ap);
    va_end(ap);
    return s;
}

__attribute__((noinline)) long forward2(uint8_t tag, int16_t d, int n, ...)
{
    va_list ap;
    va_start(ap, n);
    long s = readv(tag, d, n, ap) * 3 + tag;
    va_end(ap);
    return s;
}

typedef long (*vfn)(uint8_t, int16_t, _Bool, int, ...);
typedef long (*vfn2)(uint8_t, int16_t, int, ...);
static vfn const vt[] = {tagged, tagged2};
static vfn2 const vt2[] = {forward, forward2};

long run(long *r, int one)
{
    int n = 0;
    // Narrow values the caller leaves with their upper bits as they are, and
    // arguments that C promotes: a uint8_t, an int16_t and a _Bool as ints.
    uint8_t tag = (uint8_t)(0x1ff * one);
    int16_t d = (int16_t)(-2 * one);
    uint8_t v8 = (uint8_t)(200 * one);
    int16_t v16 = (int16_t)(-30000 * one);
    r[n++] = tagged(tag, d, one == 1, 3, v8, v16, one == 1, 2.5);
    r[n++] = vt[one - 1](tag, d, one == 1, 3, v8, v16, one == 1, 2.5);
    r[n++] = tagged2(tag, d, one == 1, 3, v8, v16, one == 1, 2.5);
    r[n++] = vt[one](tag, d, one == 1, 3, v8, v16, one == 1, 2.5);
    r[n++] = vt[one](0xfe, (int16_t)0x8001, 0, 1, (int)v16, -1.25);
    r[n++] = tagged((uint8_t)(0x80 * one), (int16_t)(0x7fff * one), one != 1, 0, 3.0);
    r[n++] = small((int8_t)(-0x7f * one), (uint16_t)(0xfffe * one), 2, (int)v8, (int)v16);
    r[n++] = small((int8_t)(0x40 * one), (uint16_t)(0x8001 * one), 0);
    r[n++] = oddsum(one == 1, 3, 1, 2, 4) * 10 + oddsum(one != 1, 2, (int)v8, (int)v16);
    r[n++] = forward(tag, d, 3, 11L, -12L, 13L);
    r[n++] = vt2[one - 1]((uint8_t)(0x181 * one), (int16_t)(0x9000 * one), 2, 5L, 6L);
    r[n++] = vt2[one]((uint8_t)(0x181 * one), (int16_t)(0x9000 * one), 2, 5L, 6L);
    return n;
}
