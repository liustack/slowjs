/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 *
 * dtoa-test: the shortest conversion of a float64 to a string in radix 10
 * (dtoa_shortest10 in engine/dtoa.c, the path of js_dtoa for
 * Number::toString) against the big number loop of js_dtoa it replaced
 * (JS_DTOA_BIGNUM), string for string. It stops at the first difference.
 *
 *   dtoa-test                    the set that compare.sh runs: every exponent
 *                                with a few significands, the values next to
 *                                every power of two and of ten, subnormals,
 *                                the extremes, the hard cases of Paxson's
 *                                tables, short decimals and the values next to
 *                                them, and 3,000,000 random values of a fixed
 *                                seed (bit patterns, float32, decimals)
 *   dtoa-test random N SEED      N random bit patterns from SEED
 *   dtoa-test float32 FROM TO    the float32 bit patterns in [FROM, TO),
 *                                widened, as hex numbers
 *   dtoa-test file PATH Z        the values of PATH, one a line (anything
 *                                strtod reads: decimal or hex), and the Z
 *                                float64 values below and above each
 */
#include <inttypes.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "dtoa.h"

static uint64_t checked;

static double from_bits(uint64_t b)
{
    double d;
    memcpy(&d, &b, sizeof(d));
    return d;
}

static uint64_t to_bits(double d)
{
    uint64_t b;
    memcpy(&b, &d, sizeof(b));
    return b;
}

static void check_flags(uint64_t bits, int flags)
{
    char a[80], b[80];
    JSDTOATempMem tmp;
    double d = from_bits(bits);
    int la, lb;

    la = js_dtoa(a, d, 10, 0, flags, &tmp);
    lb = js_dtoa(b, d, 10, 0, flags | JS_DTOA_BIGNUM, &tmp);
    if (la != lb || memcmp(a, b, la) != 0 || la >= 40) {
        fprintf(stderr, "dtoa-test: 0x%016" PRIx64 " flags %d: shortest \"%.*s\", big number loop \"%.*s\" (after %" PRIu64 " values the same)\n",
                bits, flags, la, a, lb, b, checked);
        exit(1);
    }
    checked++;
}

/* the free format (Number::toString) and the free format with an exponent
   (toExponential with no digits), on both signs */
static void check_all(uint64_t bits)
{
    check_flags(bits, JS_DTOA_FORMAT_FREE);
    check_flags(bits ^ ((uint64_t)1 << 63), JS_DTOA_FORMAT_FREE);
    check_flags(bits, JS_DTOA_FORMAT_FREE | JS_DTOA_EXP_ENABLED);
    check_flags(bits, JS_DTOA_FORMAT_FREE | JS_DTOA_MINUS_ZERO);
}

/* bits and the z values below and above it, inside the finite positive
   range (and 0) */
static void check_around(uint64_t bits, int z, int all)
{
    int64_t i;
    for (i = -(int64_t)z; i <= z; i++) {
        int64_t b = (int64_t)(bits & ~((uint64_t)1 << 63)) + i;
        if (b < 0 || b >= 0x7ff0000000000000)
            continue;
        if (all)
            check_all(b);
        else
            check_flags(b, JS_DTOA_FORMAT_FREE);
    }
}

static uint64_t rnd_state;

/* splitmix64 */
static uint64_t rnd(void)
{
    uint64_t z = (rnd_state += 0x9e3779b97f4a7c15);
    z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9;
    z = (z ^ (z >> 27)) * 0x94d049bb133111eb;
    return z ^ (z >> 31);
}

static double parse(const char *s)
{
    JSATODTempMem tmp;
    return js_atod(s, NULL, 10, 0, &tmp, NULL);
}

static double float32_bits(uint32_t b)
{
    float f;
    memcpy(&f, &b, sizeof(f));
    return f;
}

/* Paxson, "A Program for Testing IEEE Decimal-Binary Conversion" (1991),
   tables 3 and 4: significand * 2^exponent */
static const struct { uint64_t m; int e; } paxson[] = {
    { 8511030020275656, -342 }, { 5201988407066741, -824 }, { 6406892948269899, 237 },
    { 8431154198732492, 72 }, { 6475049196144587, 99 }, { 8274307542972842, 726 },
    { 5381065484265332, -456 }, { 6761728585499734, -57 }, { 7976538478610756, 376 },
    { 5982403858958067, 377 }, { 5536995190630837, 93 }, { 7225450889282194, 710 },
    { 7225450889282194, 709 }, { 8703372741147379, 117 }, { 8944262675275217, -1 },
    { 7459803696087692, -707 }, { 6080469016670379, -381 }, { 8385515147034757, 721 },
    { 7514216811389786, -828 }, { 8397297803260511, -345 }, { 6733459239310543, 202 },
    { 8091450587292794, -473 }, { 6567258882077402, 952 }, { 6712731423444934, 535 },
    { 6712731423444934, 534 }, { 5298405411573037, -957 }, { 5137311167659507, -144 },
    { 6722280709661868, 363 }, { 5344436398034927, -169 }, { 8369123604277281, -853 },
    { 8995822108487663, -780 }, { 8942832835564782, -383 }, { 8942832835564782, -384 },
    { 8942832835564782, -385 }, { 6965949469487146, -249 }, { 6965949469487146, -250 },
    { 6965949469487146, -251 }, { 7487252720986826, 548 }, { 5592117679628511, 164 },
    { 8887055249355788, 665 }, { 6994187472632449, 690 }, { 8797576579012143, 588 },
    { 7363326733505337, 272 }, { 8549497411294502, -448 },
};

/* values that other shortest conversions got wrong at some point: too long,
   or not the closest */
static const char *const anomalies[] = {
    "2.82879384806159E17", "1.387364135037754E18", "1.45800632428665E17",
    "1.6E-322", "6.3E-322", "7.3879E20", "2.0E23", "7.0E22", "9.2E22",
    "9.5E21", "3.1E22", "5.63E21", "8.41E21", "9.9E-324", "9.9E-323",
    "1.9400994884341945E25", "3.6131332396758635E25", "2.5138990223946153E25",
    "-2.109808898695963E16", "4.940656E-318", "1.18575755E-316",
    "2.989102097996E-312", "9.0608011534336E15", "4.708356024711512E18",
    "9.409340012568248E18", "1.2345678", "5.764607523034235E39",
    "1.152921504606847E40", "2.305843009213694E40", "1.7800590868057611E-307",
    "2.8480945388892175E-306", "2.446494580089078E-296", "4.8929891601781557E-296",
    "1.8014398509481984E16", "3.6028797018963964E16", "2.900835519859558E-216",
    "5.801671039719115E-216", "3.196104012172126E-27", "117.74058342906295",
};

static void test_set(void)
{
    uint64_t be, i, b;
    int k, j;
    char buf[32];
    static const uint64_t sig[] = {
        0, 1, 2, 3, 4, 5, 0xfffffffffffff, 0xffffffffffffe, 0x8000000000000,
        0x7ffffffffffff, 0x123456789abcd, 0x5555555555555, 0xaaaaaaaaaaaaa,
    };

    /* zero, the infinities and NaN */
    check_all(0);
    check_all(0x7ff0000000000000);
    check_all(0x7ff8000000000000);
    check_all(0x7ff0000000000001);
    /* every exponent with a few significands, and the values next to every
       power of two */
    for (be = 0; be < 2047; be++) {
        for (i = 0; i < sizeof(sig) / sizeof(sig[0]); i++)
            check_all((be << 52) | sig[i]);
        check_around(be << 52, 64, 1);
    }
    /* the powers of ten and the values next to them, and 5^k */
    for (k = -330; k <= 310; k++) {
        snprintf(buf, sizeof(buf), "1e%d", k);
        check_around(to_bits(parse(buf)), 64, 1);
        snprintf(buf, sizeof(buf), "5e%d", k);
        check_around(to_bits(parse(buf)), 4, 1);
        snprintf(buf, sizeof(buf), "9.999999999999999e%d", k);
        check_around(to_bits(parse(buf)), 4, 1);
    }
    /* the smallest subnormals, and those under a power of two */
    for (b = 1; b < 100000; b++)
        check_all(b);
    for (b = 1; b < 0x10000000000000; b *= 3)
        check_around(b, 8, 1);
    /* Paxson's hard cases, and the values next to them */
    for (i = 0; i < sizeof(paxson) / sizeof(paxson[0]); i++) {
        double d = (double)paxson[i].m;
        int e = paxson[i].e;
        /* scale by 2^e in steps that stay exact (no subnormal in between) */
        while (e > 0) { j = e > 60 ? 60 : e; d *= (double)((uint64_t)1 << j); e -= j; }
        while (e < 0) { j = -e > 60 ? 60 : -e; d /= (double)((uint64_t)1 << j); e += j; }
        check_around(to_bits(d), 64, 1);
    }
    for (i = 0; i < sizeof(anomalies) / sizeof(anomalies[0]); i++)
        check_around(to_bits(parse(anomalies[i])), 64, 1);
    /* integers past 2^53 */
    for (i = 10000; i < 100000; i++)
        check_flags(to_bits((double)i * 1e15), JS_DTOA_FORMAT_FREE);
    /* 0.1 to 999.9, 0.01 to 99.99, 0.001 to 9.999 and the values next to them */
    for (i = 1; i < 10000; i++) {
        check_around(to_bits((double)i / 1e1), 4, 0);
        check_around(to_bits((double)i / 1e2), 4, 0);
        check_around(to_bits((double)i / 1e3), 4, 0);
    }
    /* random values of a fixed seed */
    rnd_state = 1;
    for (i = 0; i < 1500000; i++) {
        uint64_t r = rnd();
        if ((r & 0x7ff0000000000000) != 0x7ff0000000000000)
            check_flags(r, JS_DTOA_FORMAT_FREE);
    }
    for (i = 0; i < 500000; i++) {
        uint32_t r = (uint32_t)rnd();
        if ((r & 0x7f800000) != 0x7f800000)
            check_flags(to_bits(float32_bits(r)), JS_DTOA_FORMAT_FREE);
    }
    for (i = 0; i < 300000; i++) {
        uint64_t r = rnd();
        check_flags(to_bits((double)(r % 1000000000000000000) / 1e3), JS_DTOA_FORMAT_FREE);
        check_flags(to_bits((double)(r >> 1) / 1e6), JS_DTOA_FORMAT_FREE);
        check_flags(to_bits((double)(r >> 11) / 9007199254740992.0 * 1000), JS_DTOA_FORMAT_FREE);
    }
    /* short decimals at random exponents, and the values next to them */
    for (i = 0; i < 20000; i++) {
        uint64_t r = rnd();
        snprintf(buf, sizeof(buf), "%de%d", (int)(r % 9999 + 1), (int)((r >> 20) % 640) - 330);
        check_around(to_bits(parse(buf)), 4, 0);
    }
}

int main(int argc, char **argv)
{
    if (argc == 1) {
        test_set();
    } else if (argc == 4 && !strcmp(argv[1], "random")) {
        uint64_t n = strtoull(argv[2], NULL, 0), i;
        rnd_state = strtoull(argv[3], NULL, 0);
        for (i = 0; i < n; i++) {
            uint64_t r = rnd();
            if ((r & 0x7ff0000000000000) != 0x7ff0000000000000)
                check_flags(r, JS_DTOA_FORMAT_FREE);
        }
    } else if (argc == 4 && !strcmp(argv[1], "float32")) {
        uint64_t from = strtoull(argv[2], NULL, 16), to = strtoull(argv[3], NULL, 16), b;
        for (b = from; b < to; b++) {
            if ((b & 0x7f800000) != 0x7f800000)
                check_flags(to_bits(float32_bits((uint32_t)b)), JS_DTOA_FORMAT_FREE);
        }
    } else if (argc == 4 && !strcmp(argv[1], "file")) {
        FILE *f = fopen(argv[2], "r");
        char line[256];
        int z = atoi(argv[3]);
        if (!f) {
            perror(argv[2]);
            return 2;
        }
        while (fgets(line, sizeof(line), f))
            check_around(to_bits(strtod(line, NULL)), z, 1);
        fclose(f);
    } else {
        fprintf(stderr, "usage: dtoa-test [random N SEED | float32 FROM TO | file PATH Z]\n");
        return 2;
    }
    printf("dtoa-test: %" PRIu64 " conversions the same\n", checked);
    return 0;
}
