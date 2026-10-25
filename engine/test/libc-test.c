/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/*
 * Tests the string functions and snprintf of engine/libc against the system's.
 * libc-test.sh builds the library with every function renamed to sl_NAME, so
 * both can be in one program. The inputs come from a generator with a fixed
 * seed. The math functions are tested by math.sh.
 */
#include <inttypes.h>
#include <limits.h>
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

void *sl_memcpy(void *restrict d, const void *restrict s, size_t n);
void *sl_memmove(void *d, const void *s, size_t n);
void *sl_memset(void *d, int c, size_t n);
int sl_memcmp(const void *l, const void *r, size_t n);
int sl_bcmp(const void *l, const void *r, size_t n);
void *sl_memchr(const void *s, int c, size_t n);
size_t sl_strlen(const char *s);
int sl_strcmp(const char *l, const char *r);
char *sl_strchr(const char *s, int c);
char *sl_strrchr(const char *s, int c);
int sl_snprintf(char *restrict buf, size_t size, const char *restrict fmt, ...);
int sl_vsnprintf(char *restrict buf, size_t size, const char *restrict fmt, va_list ap);

static uint64_t rng_state = 0x9e3779b97f4a7c15ull;

static uint64_t rng(void)
{
    uint64_t x = rng_state;

    x ^= x << 13;
    x ^= x >> 7;
    x ^= x << 17;
    rng_state = x;
    return x * 0x2545f4914f6cdd1dull;
}

static int failures;
static long checks;

#define CHECK(cond, ...) do { \
        checks++; \
        if (!(cond)) { \
            if (failures++ < 20) { \
                fprintf(stderr, "libc-test: %s:%d: ", __FILE__, __LINE__); \
                fprintf(stderr, __VA_ARGS__); \
                fputc('\n', stderr); \
            } \
        } \
    } while (0)

static int sign(int v) { return (v > 0) - (v < 0); }

/* Fill buf with bytes from a small alphabet, so that equal bytes, zeros and
   matches are common. */
static void fill(unsigned char *buf, size_t n, int alphabet)
{
    for (size_t i = 0; i < n; i++)
        buf[i] = alphabet ? "\0abc\x80\xff"[rng() % (alphabet > 6 ? 6 : alphabet)] : (unsigned char)rng();
}

#define MAXN 300
#define PAD 16

static void test_mem(void)
{
    static unsigned char a[MAXN + 4 * PAD], b[MAXN + 4 * PAD], c[MAXN + 4 * PAD], d[MAXN + 4 * PAD];

    for (int iter = 0; iter < 20000; iter++) {
        size_t n = rng() % (MAXN + 1);
        size_t so = rng() % 9, dof = rng() % 9;
        int alphabet = rng() % 4 ? 3 : 0;

        /* memcpy and memmove into disjoint buffers */
        fill(a, sizeof a, alphabet);
        fill(b, sizeof b, 0);
        memcpy(c, b, sizeof b);
        memcpy(d, b, sizeof b);
        memcpy(c + PAD + dof, a + PAD + so, n);
        sl_memcpy(d + PAD + dof, a + PAD + so, n);
        CHECK(!memcmp(c, d, sizeof c), "memcpy n=%zu so=%zu do=%zu", n, so, dof);
        memcpy(c, b, sizeof b);
        memcpy(d, b, sizeof b);
        CHECK(sl_memmove(d + PAD + dof, a + PAD + so, n) == d + PAD + dof, "memmove result");
        memmove(c + PAD + dof, a + PAD + so, n);
        CHECK(!memcmp(c, d, sizeof c), "memmove disjoint n=%zu", n);

        /* memmove inside one buffer, in both directions */
        {
            size_t from = rng() % 40, to = rng() % 40;
            memcpy(c, a, sizeof a);
            memcpy(d, a, sizeof a);
            memmove(c + PAD + to, c + PAD + from, n);
            sl_memmove(d + PAD + to, d + PAD + from, n);
            CHECK(!memcmp(c, d, sizeof c), "memmove overlap n=%zu from=%zu to=%zu", n, from, to);
        }

        /* memset */
        {
            int v = (int)(rng() % 512) - 100; /* the byte is v converted to unsigned char */
            memcpy(c, a, sizeof a);
            memcpy(d, a, sizeof a);
            memset(c + PAD + dof, v, n);
            CHECK(sl_memset(d + PAD + dof, v, n) == d + PAD + dof, "memset result");
            CHECK(!memcmp(c, d, sizeof c), "memset n=%zu v=%d do=%zu", n, v, dof);
        }

        /* memcmp, bcmp, memchr */
        {
            memcpy(b, a, sizeof a);
            if (n && rng() % 2)
                b[PAD + so + rng() % n] ^= (unsigned char)(1 + rng() % 255);
            int r1 = memcmp(a + PAD + so, b + PAD + so, n);
            int r2 = sl_memcmp(a + PAD + so, b + PAD + so, n);
            CHECK(sign(r1) == sign(r2), "memcmp n=%zu: %d %d", n, r1, r2);
            CHECK((memcmp(a + PAD + so, b + PAD + so, n) != 0) == (sl_bcmp(a + PAD + so, b + PAD + so, n) != 0), "bcmp n=%zu", n);
            int ch = (int)(rng() % 600) - 100;
            CHECK(sl_memchr(a + PAD + so, ch, n) == memchr(a + PAD + so, ch, n), "memchr n=%zu ch=%d", n, ch);
        }
    }
}

static void test_str(void)
{
    static char a[MAXN + 4 * PAD], b[MAXN + 4 * PAD];

    for (int iter = 0; iter < 30000; iter++) {
        size_t n = rng() % MAXN;
        size_t so = rng() % 9;
        int alphabet = rng() % 3 ? 3 : 5;
        char *s, *t;
        int ch;

        fill((unsigned char *)a, sizeof a, alphabet);
        /* a string of n bytes of a's alphabet at an offset, ended by a zero */
        s = a + PAD + so;
        for (size_t i = 0; i < n; i++)
            if (!s[i]) s[i] = 'a';
        s[n] = 0;
        CHECK(sl_strlen(s) == strlen(s) && strlen(s) == n, "strlen n=%zu", n);

        t = b + PAD + (rng() % 9);
        strcpy(t, s);
        if (n && rng() % 2)
            t[rng() % n] ^= (char)(1 + rng() % 255);
        CHECK(sign(strcmp(s, t)) == sign(sl_strcmp(s, t)), "strcmp");

        ch = (int)(rng() % 600) - 100;
        CHECK(strchr(s, ch) == sl_strchr(s, ch), "strchr n=%zu ch=%d", n, ch);
        CHECK(strrchr(s, ch) == sl_strrchr(s, ch), "strrchr n=%zu ch=%d", n, ch);
        CHECK(sl_strchr(s, 0) == s + n && sl_strrchr(s, 0) == s + n, "strchr of 0");
    }
}

static int vwrap(char *buf, size_t size, const char *fmt, ...)
{
    va_list ap;
    int r;

    va_start(ap, fmt);
    r = sl_vsnprintf(buf, size, fmt, ap);
    va_end(ap);
    return r;
}

/* Compare snprintf with the system's on one format and its arguments, with
   the buffer cut at every size. */
#define SNP(fmt, ...) do { \
        char want[300], got[300]; \
        int rw = snprintf(want, sizeof want, fmt, __VA_ARGS__); \
        int rg = sl_snprintf(got, sizeof got, fmt, __VA_ARGS__); \
        CHECK(rw == rg && !strcmp(want, got), "snprintf %s: want [%s] %d, got [%s] %d", fmt, want, rw, got, rg); \
        for (size_t cut = 0; cut < 12 && cut < sizeof want; cut++) { \
            char w2[16], g2[16]; \
            memset(w2, 'x', sizeof w2); \
            memset(g2, 'x', sizeof g2); \
            int r1 = snprintf(w2, cut, fmt, __VA_ARGS__); \
            int r2 = sl_snprintf(g2, cut, fmt, __VA_ARGS__); \
            CHECK(r1 == r2 && !memcmp(w2, g2, sizeof w2), "snprintf %s cut %zu: want [%.16s] %d, got [%.16s] %d", fmt, cut, w2, r1, g2, r2); \
        } \
    } while (0)

static void test_snprintf(void)
{
    static const int ints[] = { 0, 1, -1, 7, -7, 42, 99, 100, 123, -123, 4095, 65535, 1000000, INT_MAX, INT_MIN };
    static const long long lls[] = { 0, 1, -1, 123456789012LL, -123456789012LL, LLONG_MAX, LLONG_MIN };
    static const char *strs[] = { "", "a", "hello", "a longer string with spaces", "tab\there" };
    static const int widths[] = { 0, 1, 5, 12 };
    static const int precs[] = { 0, 1, 3, 9 };

    for (size_t i = 0; i < sizeof ints / sizeof *ints; i++) {
        int v = ints[i];
        unsigned u = (unsigned)v;
        SNP("%d", v); SNP("%i", v); SNP("%u", u); SNP("%x", u); SNP("%X", u); SNP("%o", u);
        SNP("%#x", u); SNP("%#X", u); SNP("%#o", u); SNP("%+d", v); SNP("% d", v);
        SNP("%5d|", v); SNP("%-5d|", v); SNP("%05d", v); SNP("%-05d|", v); SNP("%+07d", v);
        SNP("%02d", v); SNP("%03d", v); SNP("%04d", v); SNP("%04x", u); SNP("%08x", u); SNP("%02x", u);
        SNP("%.0d", v); SNP("%.3d", v); SNP("%8.3d", v); SNP("%-8.3d|", v); SNP("%08.3d", v); SNP("%.0u", u);
        SNP("%#.0x", u); SNP("%#.0o", u);
        SNP("%hhd", v); SNP("%hd", v); SNP("%hhu", u); SNP("%hu", u); SNP("%hx", u);
        SNP("%ld", (long)v); SNP("%lu", (unsigned long)u); SNP("%lx", (unsigned long)v);
        SNP("%zu", (size_t)u); SNP("%zd", (ptrdiff_t)v); SNP("%jd", (intmax_t)v); SNP("%td", (ptrdiff_t)v);
        SNP("%c", v & 0x7f); SNP("%5c|", 'z'); SNP("%-5c|", 'z');
        SNP("%02d:%02d:%02d.%03dZ", v & 31, v & 63, v & 63, v & 1023);
        SNP("%.3s, %02d %.3s %0*d ", "Wednesday", v & 31, "September", 4, v & 4095);
        SNP("%0*d|%*d|%-*d|", 6, v, 6, v, 6, v);
        SNP("%*d|", -6, v);
        for (size_t w = 0; w < sizeof widths / sizeof *widths; w++)
            for (size_t p = 0; p < sizeof precs / sizeof *precs; p++) {
                SNP("%*.*d", widths[w], precs[p], v);
                SNP("%-*.*d|", widths[w], precs[p], v);
                SNP("%*.*x", widths[w], precs[p], u);
                SNP("%#*.*x", widths[w], precs[p], u);
                SNP("%+*.*d", widths[w], precs[p], v);
            }
    }
    for (size_t i = 0; i < sizeof lls / sizeof *lls; i++) {
        long long v = lls[i];
        SNP("%lld", v); SNP("%llu", (unsigned long long)v); SNP("%llx", (unsigned long long)v);
        SNP("%" PRId64, (int64_t)v); SNP("%" PRIu64, (uint64_t)v); SNP("%" PRIx64, (uint64_t)v);
        SNP("%016" PRIx64, (uint64_t)v); SNP("%20" PRId64 "|", (int64_t)v); SNP("%-20" PRId64 "|", (int64_t)v);
        SNP("%8" PRId64 " %8" PRId64, (int64_t)v, (int64_t)v);
    }
    for (size_t i = 0; i < sizeof strs / sizeof *strs; i++) {
        const char *s = strs[i];
        SNP("%s", s); SNP("<%10s>", s); SNP("<%-10s>", s); SNP("%.3s", s); SNP("<%10.3s>", s); SNP("<%-10.3s>", s);
        SNP("%.0s|", s); SNP("%.*s", 2, s); SNP("<%*.*s>", 8, 4, s); SNP("<%-*s>", 8, s);
        SNP("a%sb%sc", s, s); SNP("%%%s%%", s);
    }
    SNP("%s", "");
    SNP("%d%%", 50);
    SNP("expecting '%c'", 'x');
    SNP("unexpected token in expression: '%.*s'", 3, "abcdef");
    SNP("invalid opcode: pc=%u opcode=0x%02x", 1234u, 0x7bu);
    SNP("%d:%d", 12, 345);
    SNP("%s", "nothing to see");
    SNP("%s", "x");

    /* NULL strings and pointers: printed as the engine prints them */
    {
        char buf[32];
        sl_snprintf(buf, sizeof buf, "%s", (char *)NULL);
        CHECK(!strcmp(buf, "(null)"), "%%s of NULL: [%s]", buf);
        sl_snprintf(buf, sizeof buf, "%p", (void *)0);
        CHECK(!strcmp(buf, "0x0"), "%%p of NULL: [%s]", buf);
        sl_snprintf(buf, sizeof buf, "%p", (void *)(uintptr_t)0x1234abcd);
        CHECK(!strcmp(buf, "0x1234abcd"), "%%p: [%s]", buf);
    }
    /* no buffer at all: only the length */
    CHECK(sl_snprintf(NULL, 0, "%d-%s", 1234, "abc") == 8, "length only");
    /* vsnprintf takes the same arguments */
    {
        char want[64], got[64];
        int rw = snprintf(want, sizeof want, "%s=%d (%c) %llx", "key", -77, 'k', 0xabcdefull);
        int rg = vwrap(got, sizeof got, "%s=%d (%c) %llx", "key", -77, 'k', 0xabcdefull);
        CHECK(rw == rg && !strcmp(want, got), "vsnprintf: [%s] [%s]", want, got);
    }
}

int main(void)
{
    test_mem();
    test_str();
    test_snprintf();
    printf("libc-test: %ld checks, %d failures\n", checks, failures);
    return failures != 0;
}
