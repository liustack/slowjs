// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Freestanding linux driver for narrow.c: prints run's results, one per
// line.
typedef unsigned char uint8_t;
typedef unsigned short uint16_t;
typedef short int16_t;

long run(long *r, int one);

static long sys3(long nr, long a, long b, long c)
{
    long r;
    __asm__ volatile("syscall" : "=a"(r) : "a"(nr), "D"(a), "S"(b), "d"(c) : "rcx", "r11", "memory");
    return r;
}

long host_cb(long x) { return x; }

long host_narrow(uint8_t c, int16_t d, _Bool e, long tag) { return c * 1000003L + d * 1009L + e * 17L + tag; }

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
