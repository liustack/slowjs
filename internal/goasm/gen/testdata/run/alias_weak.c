// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A call of a weak alias, which the optimizer cannot fold into a call of the
// function it names (another definition could replace the alias). The
// generator refuses it, narrow arguments or not (TestSampleRuns).
typedef unsigned char uint8_t;
typedef signed char int8_t;
typedef unsigned short uint16_t;
typedef short int16_t;

__attribute__((noinline)) uint16_t mix16(uint8_t a, int16_t b, _Bool c, int8_t d)
{
    return (uint16_t)(a + b * 3 + c * 5 + d * 7);
}

uint16_t mix16_weak(uint8_t a, int16_t b, _Bool c, int8_t d) __attribute__((weak, alias("mix16")));

long run(long *r, int one)
{
    r[0] = mix16_weak((uint8_t)(200 * one), (int16_t)(-3 * one), one == 1, (int8_t)(-100 * one));
    return 1;
}
