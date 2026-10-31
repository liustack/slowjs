// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The other file of the optimizer test (see postlink.c).
long pl_add(long a, long b) { return a + b + (a ^ b); }

// A leaf: no call, a loop. It is not inlined, as memcpy is not when its length
// is large: the generated code calls it, which it must for the post-link check
// to find it in the binary.
__attribute__((noinline)) long pl_sum(const long *p, long n, long k)
{
    long s = 0;
    for (long i = 0; i < n; i++)
        s += p[i] * k + (p[i] >> 1);
    return s;
}
