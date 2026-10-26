// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A trap in the C: __builtin_trap() is ud2 on amd64 and brk on arm64, and in
// Go either one ends the process (SIGILL and SIGTRAP cannot be recovered).
// The shadow-stack pass turns it into a call to host_trap, which panics. With
// one = 1 the trap is not taken, so the native run and the generated one print
// the same. TestSampleRuns also runs it with one = 2 on the generated code
// and requires that call.
__attribute__((noinline)) static long guarded(long x, int one)
{
    if (one == 2)
        __builtin_trap();
    return x * 3 + 1;
}

long run(long *r, int one)
{
    r[0] = guarded(5, one);
    r[1] = guarded(r[0], one);
    return 2;
}
