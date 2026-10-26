// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Narrow arguments and results behind a global alias (TestSampleRuns). The
// optimizer folds a direct call of a strong alias into a call of the function
// it names, so what stays is the alias in a table of function pointers, beside
// a second function so that the call cannot be folded, and the calls through
// the table. The generator takes them and they give what native C gives. A
// call of a weak alias cannot be folded, and the generator refuses it
// (alias_weak.c).
typedef unsigned char uint8_t;
typedef signed char int8_t;
typedef unsigned short uint16_t;
typedef short int16_t;

__attribute__((noinline)) uint16_t mix16(uint8_t a, int16_t b, _Bool c, int8_t d)
{
    return (uint16_t)(a + b * 3 + c * 5 + d * 7);
}

__attribute__((noinline)) uint16_t alt16(uint8_t a, int16_t b, _Bool c, int8_t d)
{
    return (uint16_t)(a * 9 - b + c * 13 + d);
}

__attribute__((noinline)) int8_t mix8(uint8_t a, int16_t b, _Bool c, int8_t d)
{
    return (int8_t)(a * 3 - b + c * 11 - d * 5);
}

__attribute__((noinline)) int8_t alt8(uint8_t a, int16_t b, _Bool c, int8_t d)
{
    return (int8_t)(a + b + c + d);
}

__attribute__((noinline)) _Bool mixb(uint8_t a, int16_t b, _Bool c, int8_t d)
{
    return (a ^ b ^ c ^ d) & 1;
}

__attribute__((noinline)) _Bool altb(uint8_t a, int16_t b, _Bool c, int8_t d)
{
    return (a & b & c & d) != 0;
}

uint16_t alt16_alias(uint8_t a, int16_t b, _Bool c, int8_t d) __attribute__((alias("alt16")));
int8_t alt8_alias(uint8_t a, int16_t b, _Bool c, int8_t d) __attribute__((alias("alt8")));
_Bool altb_alias(uint8_t a, int16_t b, _Bool c, int8_t d) __attribute__((alias("altb")));

typedef uint16_t (*f16)(uint8_t, int16_t, _Bool, int8_t);
typedef int8_t (*f8)(uint8_t, int16_t, _Bool, int8_t);
typedef _Bool (*fb)(uint8_t, int16_t, _Bool, int8_t);
static f16 const t16[] = {mix16, alt16_alias};
static f8 const t8[] = {mix8, alt8_alias};
static fb const tb[] = {mixb, altb_alias};

static const struct {
    uint8_t a;
    int16_t b;
    _Bool c;
    int8_t d;
} cases[] = {
    {1, 2, 1, 3},
    {0xff, -1, 0, -1},
    {0x80, -32768, 1, -128},
    {0x7f, 32767, 0, 127},
};

// run writes, for each case, the three functions and the three aliases called
// directly and through the table, whose entry one - 1 is the function and
// entry one the alias. one is 1.
long run(long *r, int one)
{
    int n = 0;
    for (unsigned i = 0; i < sizeof cases / sizeof cases[0]; i++) {
        uint8_t a = (uint8_t)(cases[i].a * one);
        int16_t b = (int16_t)(cases[i].b * one);
        _Bool c = cases[i].c * one;
        int8_t d = (int8_t)(cases[i].d * one);
        r[n++] = mix16(a, b, c, d) * 100000L + t16[one - 1](a, b, c, d);
        r[n++] = alt16(a, b, c, d) * 100000L + alt16_alias(a, b, c, d) * 10L + (t16[one](a, b, c, d) == alt16(a, b, c, d));
        r[n++] = mix8(a, b, c, d) * 1000L + t8[one - 1](a, b, c, d);
        r[n++] = alt8(a, b, c, d) * 1000L + alt8_alias(a, b, c, d) * 10L + (t8[one](a, b, c, d) == alt8(a, b, c, d));
        r[n++] = mixb(a, b, c, d) * 100 + tb[one - 1](a, b, c, d) * 10 + altb_alias(a, b, c, d);
        r[n++] = altb(a, b, c, d) * 10 + tb[one](a, b, c, d);
    }
    return n;
}
