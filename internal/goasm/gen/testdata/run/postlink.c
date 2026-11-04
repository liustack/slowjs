// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The optimizer that runs over the linked module (TestPostLinkOpt,
// TestSampleRuns). This file calls functions of postlink_lib.c, which clang
// cannot see into, so only the second run of the optimizer can inline them:
//   - pl_add is small and external: it is inlined into run and stays defined.
//   - walk is static and takes a function pointer that is a constant at each of
//     its two calls: the optimizer clones it (walk.specialized.N), with the
//     test's options that ask for it, and the cloned function is new to the
//     generator.
//   - pl_sum (postlink_lib.c) is the register-entry leaf of the test, called from
//     run with arguments that are not constants: it keeps its name and its leaf
//     entry, and the calls go to it.
typedef long (*step_fn)(long);

long pl_add(long a, long b);
long pl_sum(const long *p, long n, long k);

static long inc(long x) { return x + 1; }
static long dbl(long x) { return x * 2; }

__attribute__((noinline)) static long walk(long n, step_fn f)
{
    long s = 0;
    for (long i = 0; i < n; i++)
        s += f(i) ^ i;
    return s;
}

__attribute__((noinline)) long pl_inc_sum(long n) { return walk(n, inc); }
__attribute__((noinline)) long pl_dbl_sum(long n) { return walk(n, dbl); }

long run(long *r, int one)
{
    /* Not a constant address and not a constant length, so that nothing of
       pl_sum is a constant for the optimizer to clone it for. */
    long *v = r + 5;
    for (int i = 0; i < 8; i++)
        v[i] = i * one + 3;
    r[0] = pl_add(one, 41);
    r[1] = pl_inc_sum(10 * one);
    r[2] = pl_dbl_sum(10 * one);
    r[3] = pl_sum(v, 8 * one, 2 + one);
    r[4] = pl_sum(v, 5 * one, 4 + one);
    return 5;
}
