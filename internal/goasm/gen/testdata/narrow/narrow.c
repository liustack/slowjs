// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Narrow integer arguments and results (bool, 8 and 16 bits), run by
// TestNarrowArgs: natively, and through the generator in body and pn mode
// with Go calling in and called back. LLVM's preserve_none puts only 32 and
// 64-bit integers in its own registers and a narrower one where plain C
// would, so every value here must come out the same in every build.
// Freestanding: narrow_main.c drives the native build, a Go host the
// generated one.
typedef unsigned char uint8_t;
typedef signed char int8_t;
typedef unsigned short uint16_t;
typedef short int16_t;

// Host callbacks: the driver's in C, or Go's.
long host_cb(long x);
long host_narrow(uint8_t c, int16_t d, _Bool e, long tag);

// The sums are unsigned, so that they wrap by C's rules and the sample has
// no undefined behavior on any input (TestNarrowArgs also runs it under
// UBSan).
// Two pointers, then each narrow kind, then a 32 and a 64-bit integer.
__attribute__((noinline)) long f(void *a, void *b, uint8_t c, uint16_t d, _Bool e, int g, long h)
{
    unsigned long s = (unsigned long)a * 3 + (unsigned long)b * 5;
    s = s * 7 + c;
    s = s * 11 + d;
    s = s * 13 + e;
    s = s * 17 + (unsigned long)(long)g;
    return (long)(s * 19 + (unsigned long)h);
}

// Each narrow kind after one pointer: six words with the shadow pointer, so
// body mode passes them in registers too and Go can call it.
__attribute__((noinline)) long g6(void *a, uint8_t c, uint16_t d, _Bool e, int g)
{
    unsigned long s = (unsigned long)a * 3;
    s = s * 7 + c;
    s = s * 11 + d;
    s = s * 13 + e;
    return (long)(s * 17 + (unsigned long)(long)g);
}

// The shape of QuickJS's DynBuf writers (emit_op, __dbuf_put_u16): a
// buffer, then one narrow value. The call to grow keeps each one off the
// leaf path, so it begins with Go's stack check.
struct buf {
    uint8_t *p;
    long n, cap;
};

__attribute__((noinline)) long grow(struct buf *b, long need)
{
    return host_cb(need) >= 0 && b->n + need <= b->cap ? 0 : -1;
}

__attribute__((noinline)) int put8(struct buf *b, uint8_t v)
{
    if (b->n + 1 > b->cap && grow(b, 1))
        return -1;
    b->p[b->n++] = v;
    return 0;
}

__attribute__((noinline)) int put16(struct buf *b, uint16_t v)
{
    if (b->n + 2 > b->cap && grow(b, 2))
        return -1;
    b->p[b->n] = (uint8_t)v;
    b->p[b->n + 1] = (uint8_t)(v >> 8);
    b->n += 2;
    return 0;
}

__attribute__((noinline)) int putb(struct buf *b, _Bool v)
{
    if (b->n + 1 > b->cap && grow(b, 1))
        return -1;
    b->p[b->n++] = v ? 'T' : 'F';
    return 0;
}

__attribute__((noinline)) int puts8(struct buf *b, int8_t v)
{
    if (b->n + 1 > b->cap && grow(b, 1))
        return -1;
    b->p[b->n++] = (uint8_t)(v < 0 ? -v : v);
    return 0;
}

// Narrow results, sign and zero extended (tofp16 returns a uint16_t).
__attribute__((noinline)) uint16_t half(uint16_t v) { return (uint16_t)(v >> 1 | 0x8000); }
__attribute__((noinline)) int8_t neg8(int8_t v) { return (int8_t)-v; }
__attribute__((noinline)) _Bool odd(long v) { return v & 1; }

typedef long (*ffn)(void *, void *, uint8_t, uint16_t, _Bool, int, long);
static ffn const fns[] = {f};

// The cases of f, also in the Go host (narrowCases in narrow_test.go).
static const struct {
    long a, b;
    uint8_t c;
    uint16_t d;
    _Bool e;
    int g;
    long h;
} cases[] = {
    {1, 2, 3, 4, 1, 5, 6},
    {0x1000, -1, 0xff, 0xffff, 0, -1, -1},
    {-7, 0x7fffffffffffffffL, 0x80, 0x8000, 1, -2147483647 - 1, -0x7fffffffffffffffL - 1},
    {0, 0, 0x7f, 0x7fff, 0, 2147483647, 0x123456789abcdefL},
};

// run fills r and returns how many it wrote: f called directly and through
// a pointer and g6 for each case, the buffer the writers filled, a host callback
// with narrow arguments, the narrow results. one is 1.
long run(long *r, int one)
{
    int n = 0;
    for (unsigned i = 0; i < sizeof cases / sizeof cases[0]; i++) {
        r[n++] = f((void *)cases[i].a, (void *)cases[i].b, cases[i].c, cases[i].d, cases[i].e, cases[i].g, cases[i].h);
        r[n++] = fns[one - 1]((void *)cases[i].a, (void *)cases[i].b, cases[i].c, cases[i].d, cases[i].e, cases[i].g,
                              cases[i].h);
        r[n++] = g6((void *)cases[i].a, cases[i].c, cases[i].d, cases[i].e, cases[i].g);
    }
    uint8_t bytes[16] = {0};
    struct buf b = {bytes, 0, 12};
    long ok = put8(&b, (uint8_t)(0x1ff * one)) | put16(&b, (uint16_t)(0x1234 * one)) | putb(&b, one == 1) |
              putb(&b, one == 2) | puts8(&b, (int8_t)(-0x7f * one)) | put16(&b, (uint16_t)(0xfffe + one)) |
              put8(&b, (uint8_t)(0x80 * one)) | put16(&b, (uint16_t)(0x8001 * one));
    unsigned long w = 0;
    for (int i = 0; i < 8; i++)
        w = w << 8 | bytes[i];
    r[n++] = (long)w;
    r[n++] = (long)bytes[8] << 24 | (long)bytes[9] << 16 | (long)bytes[10] << 8 | b.n;
    r[n++] = ok;
    r[n++] = host_narrow((uint8_t)(0x1fe * one), (int16_t)(-2 * one), one == 1, 7);
    r[n++] = host_narrow((uint8_t)(0x7f * one), (int16_t)(0x7fff * one), one == 2, -9);
    r[n++] = half((uint16_t)(0xfffe * one)) + half((uint16_t)(0x1235 * one));
    r[n++] = neg8((int8_t)(-0x7f * one)) * 1000 + neg8((int8_t)(5 * one));
    r[n++] = odd(3 * one) * 10 + odd(4 * one);
    return n;
}
