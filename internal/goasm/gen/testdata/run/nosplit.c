// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The shapes the NOSPLIT choice tells apart (TestSampleRuns, and
// TestNoSplitSample, which checks the choice and faults at the bottom of the
// chain). poke goes down a chain of 24 calls, too tall to go without a stack
// check as a whole, to a leaf that reads through the pointer it was given:
// the choice takes the bottom of the chain as far as it fits, leaves the
// function above with its stack check, and takes the ones above that again.
// self and the pair ping and pong recurse, so they keep their stack checks,
// and so do viaptr, whose address a table holds, and icaller, which calls
// through it. Their callers can go without one. Nothing here loops, so
// nothing polls.
__attribute__((noinline)) long down23(long *p, long k) { return *p + k; }

// Each step keeps k across its call, so that it has a frame.
#define STEP(n, m)                                    \
    __attribute__((noinline)) long down##n(long *p, long k) \
    {                                                 \
        long x = down##m(p, k + 1);                   \
        return x * 3 + k;                             \
    }
STEP(22, 23)
STEP(21, 22)
STEP(20, 21)
STEP(19, 20)
STEP(18, 19)
STEP(17, 18)
STEP(16, 17)
STEP(15, 16)
STEP(14, 15)
STEP(13, 14)
STEP(12, 13)
STEP(11, 12)
STEP(10, 11)
STEP(9, 10)
STEP(8, 9)
STEP(7, 8)
STEP(6, 7)
STEP(5, 6)
STEP(4, 5)
STEP(3, 4)
STEP(2, 3)
STEP(1, 2)
STEP(0, 1)

__attribute__((noinline)) long poke(long *p, long k) { return down0(p, k) + 1; }

// Not an accumulator that the optimizer would turn into a loop.
__attribute__((noinline)) long self(long n) { return n <= 0 ? 1 : (self(n - 1) * 3) ^ n; }

__attribute__((noinline)) long ping(long n);
__attribute__((noinline)) long pong(long n) { return n <= 0 ? 7 : ping(n - 1) * 2 + 1; }
__attribute__((noinline)) long ping(long n) { return n <= 0 ? 5 : pong(n - 1) * 3 - n; }
__attribute__((noinline)) long callsrec(long n) { return self(n) + ping(n) + 1; }

__attribute__((noinline)) long sq(long x) { return x * x; }
__attribute__((noinline)) long viaptr(long x) { return sq(x) * 11 + 3; }
static long (*const tab[])(long) = {viaptr, sq};
__attribute__((noinline)) long icaller(long i, long x) { return tab[i](x); }
__attribute__((noinline)) long callsptr(long x) { return icaller(0, x) + viaptr(x + 1); }

long run(long *r, int one)
{
    long v = one * 4;
    long n = 0;
    r[n++] = poke(&v, one);
    r[n++] = callsrec(10 * one);
    r[n++] = callsptr(5 * one);
    r[n++] = icaller(one, 9);
    return n;
}
