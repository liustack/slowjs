// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A register-entry leaf that traps. A leaf has no shadow pointer to call
// host_trap with, so the pass refuses it (TestSampleRuns, with -leaves lf).
long lf(long x)
{
    if (x == 77)
        __builtin_trap();
    return x + 1;
}

long run(long *r, int one)
{
    r[0] = lf(one);
    return 1;
}
