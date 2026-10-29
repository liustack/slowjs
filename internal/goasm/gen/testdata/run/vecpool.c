// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions that read a vector constant of their own with an instruction that
// needs it on 16 bytes (TestSampleRuns, with -align64: see the entries of its
// table). LLVM folds each constant into a paddd or a pxor with a memory
// operand, or loads it with a movaps, and the generator keeps the constant in
// the body of the function. A body that starts 8 bytes past a multiple of 16
// (mixa, mixb and mixd) must still have the constant on 16 bytes where it
// runs, or the load faults.
typedef unsigned v4i __attribute__((vector_size(16)));
typedef unsigned long ul;

__attribute__((noinline)) long mixa(ul *p, ul k)
{
    v4i x;
    __builtin_memcpy(&x, p, 16);
    x = (x + (v4i){1, 2, 3, 4}) ^ (v4i){0x55, 0x66, 0x77, 0x88};
    __builtin_memcpy(p, &x, 16);
    return (long)(p[0] + p[1] * 3 + k);
}

__attribute__((noinline)) long mixb(ul *p, ul k)
{
    v4i x;
    __builtin_memcpy(&x, p, 16);
    x = (x - (v4i){10, 20, 30, 40}) ^ (v4i){0x1234, 0x2345, 0x3456, 0x4567};
    __builtin_memcpy(p, &x, 16);
    return (long)(p[0] * 5 + p[1] + k);
}

__attribute__((noinline)) long mixc(ul *p, ul k)
{
    v4i x;
    __builtin_memcpy(&x, p, 16);
    x = (x + (v4i){100, 200, 300, 400}) & (v4i){0xffff, 0x7fff, 0x3fff, 0x1fff};
    __builtin_memcpy(p, &x, 16);
    return (long)(p[0] + p[1] * 7 + k);
}

__attribute__((noinline)) long mixd(ul *p, ul k)
{
    v4i x = {0x11111111u, 0x22222222u, 0x33333333u, 0x44444444u};
    __builtin_memcpy(p, &x, 16);
    return (long)(p[0] ^ p[1] ^ k);
}

long run(long *r, int one)
{
    ul buf[2];
    int n = 0;
    for (ul i = 0; i < 3; i++) {
        buf[0] = 0x0102030405060708UL * (i + (ul)one);
        buf[1] = 0x1112131415161718UL ^ (i * 0x0101010101010101UL);
        r[n++] = mixa(buf, i);
        buf[0] = 0x2122232425262728UL * (i + (ul)one);
        buf[1] = 0x3132333435363738UL ^ (i * 0x0101010101010101UL);
        r[n++] = mixb(buf, i);
        buf[0] = 0x4142434445464748UL * (i + (ul)one);
        buf[1] = 0x5152535455565758UL ^ (i * 0x0101010101010101UL);
        r[n++] = mixc(buf, i);
        long m = mixd(buf, i);
        r[n++] = (long)((ul)m + buf[0]);
    }
    return n;
}
