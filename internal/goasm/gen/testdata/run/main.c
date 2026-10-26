// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Freestanding linux/amd64 driver of the samples in this folder (TestSampleRuns):
// calls the sample's run and prints what it wrote, one result per line. A
// sample needs no host callback.
long run(long *r, int one);

static long sys3(long nr, long a, long b, long c)
{
    long r;
    __asm__ volatile("syscall" : "=a"(r) : "a"(nr), "D"(a), "S"(b), "d"(c) : "rcx", "r11", "memory");
    return r;
}

static void put(long v)
{
    char buf[32];
    unsigned long u = v < 0 ? -(unsigned long)v : (unsigned long)v;
    int k = sizeof buf;
    buf[--k] = '\n';
    do
        buf[--k] = (char)('0' + u % 10);
    while (u /= 10);
    if (v < 0)
        buf[--k] = '-';
    sys3(1, 1, (long)(buf + k), sizeof buf - k);
}

void cmain(void)
{
    long r[64];
    int n = (int)run(r, 1);
    for (int i = 0; i < n; i++)
        put(r[i]);
    sys3(60, 0, 0, 0);
}

__attribute__((naked, noreturn)) void _start(void) { __asm__("xor %ebp, %ebp\n\tand $-16, %rsp\n\tcall cmain\n\tud2"); }
