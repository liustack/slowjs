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
#include <signal.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/wait.h>
#include <unistd.h>

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

/* memcpy at every size from 0 to 1100 bytes, with the source and the destination
   each at 16 offsets from a multiple of 64 (so the destination reaches every
   position the loop of 64 bytes aligns it to, and the source every one it can be
   misaligned by). The destination holds a byte that no source byte has, so a byte
   written outside the range, before or after it, shows. The source must not
   change, and the result is the destination. Then bigger sizes at random
   offsets, which run the loop many times. */
#define CPMAX 1100
#define CPLEAD 64
#define CPBUF 2048
#define CPBIG 70000

static void test_memcpy_all(void)
{
    static unsigned char src[CPBUF] __attribute__((aligned(64)));
    static unsigned char src0[CPBUF];
    static unsigned char dst[CPBUF] __attribute__((aligned(64)));
    static unsigned char ref[CPBUF];
    static unsigned char big_src[CPBIG + 256], big_dst[CPBIG + 256], big_ref[CPBIG + 256];
    const unsigned char guard = 0xAA;

    for (size_t i = 0; i < sizeof src; i++) {
        src[i] = (unsigned char)rng();
        if (src[i] == guard)
            src[i] = guard + 1;
    }
    memcpy(src0, src, sizeof src);
    for (size_t n = 0; n <= CPMAX; n++) {
        for (size_t so = 0; so < 16; so++) {
            for (size_t dof = 0; dof < 16; dof++) {
                memset(dst, guard, sizeof dst);
                memset(ref, guard, sizeof ref);
                memcpy(ref + CPLEAD + dof, src + CPLEAD + so, n);
                void *r = sl_memcpy(dst + CPLEAD + dof, src + CPLEAD + so, n);
                CHECK(r == dst + CPLEAD + dof, "memcpy result n=%zu so=%zu do=%zu", n, so, dof);
                CHECK(!memcmp(dst, ref, sizeof dst), "memcpy n=%zu so=%zu do=%zu", n, so, dof);
                CHECK(!memcmp(src, src0, sizeof src), "memcpy changed its source n=%zu so=%zu do=%zu", n, so, dof);
            }
        }
    }

    for (int iter = 0; iter < 300; iter++) {
        size_t n = 1025 + rng() % (CPBIG - 1025);
        size_t so = rng() % 64, dof = rng() % 64;
        size_t total = so + dof + n;

        for (size_t i = 0; i < total + 128; i++) {
            big_src[i] = (unsigned char)rng();
            if (big_src[i] == guard)
                big_src[i] = guard + 1;
        }
        memset(big_dst, guard, total + 128);
        memset(big_ref, guard, total + 128);
        memcpy(big_ref + 64 + dof, big_src + 64 + so, n);
        sl_memcpy(big_dst + 64 + dof, big_src + 64 + so, n);
        CHECK(!memcmp(big_dst, big_ref, total + 128), "memcpy n=%zu so=%zu do=%zu", n, so, dof);
    }
}

/* memcpy with its source at the very end and at the very start of readable
   memory: a page that cannot be read is on either side, so a read past the
   last byte of the source or before its first faults and ends the test, even
   one inside a word the copy loads whole. The source ends at the page after
   it for every size up to CPMAX (which puts its start at every alignment) and
   for sizes up to CPBIG, and starts at the page before it for the same
   sizes, with the destination at 16 offsets. */
static void test_memcpy_borders(void)
{
    static unsigned char dst[CPBIG + 256], ref[CPBIG + 256];
    const unsigned char guard = 0xAA;
    size_t page = (size_t)sysconf(_SC_PAGESIZE);
    size_t span = (CPBIG + page - 1) / page * page;
    unsigned char *m, *lo, *hi;

    m = mmap(NULL, span + 2 * page, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANONYMOUS, -1, 0);
    CHECK(m != MAP_FAILED, "mmap");
    if (m == MAP_FAILED)
        return;
    lo = m + page;
    hi = lo + span;
    for (size_t i = 0; i < span; i++) {
        lo[i] = (unsigned char)rng();
        if (lo[i] == guard)
            lo[i] = guard + 1;
    }
    CHECK(mprotect(m, page, PROT_NONE) == 0 && mprotect(hi, page, PROT_NONE) == 0, "mprotect");
    for (int end = 0; end < 2; end++) {
        for (size_t k = 0; k <= CPMAX + 300; k++) {
            /* every size to CPMAX, then 300 at random to CPBIG */
            size_t n = k <= CPMAX ? k : CPMAX + 1 + rng() % (CPBIG - CPMAX);
            const unsigned char *src = end ? hi - n : lo;

            for (size_t dof = 0; dof < 16; dof += k <= CPMAX ? 1 : 5) {
                memset(dst, guard, n + 128);
                memset(ref, guard, n + 128);
                memcpy(ref + 64 + dof, src, n);
                sl_memcpy(dst + 64 + dof, src, n);
                CHECK(!memcmp(dst, ref, n + 128), "memcpy n=%zu from the %s of readable memory do=%zu", n, end ? "end" : "start", dof);
            }
        }
    }
    munmap(m, span + 2 * page);
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
   the buffer cut at every size from nothing to two past the whole output. A
   canary byte after the cut shows a write that goes too far. */
#define SNP(fmt, ...) do { \
        char want[300], got[300]; \
        int rw = snprintf(want, sizeof want, fmt, __VA_ARGS__); \
        int rg = sl_snprintf(got, sizeof got, fmt, __VA_ARGS__); \
        CHECK(rw == rg && !strcmp(want, got), "snprintf %s: want [%s] %d, got [%s] %d", fmt, want, rw, got, rg); \
        for (size_t cut = 0; cut <= (size_t)rw + 2 && cut < 290; cut++) { \
            char w2[300], g2[300]; \
            memset(w2, 'x', sizeof w2); \
            memset(g2, 'x', sizeof g2); \
            int r1 = snprintf(w2, cut, fmt, __VA_ARGS__); \
            int r2 = sl_snprintf(g2, cut, fmt, __VA_ARGS__); \
            CHECK(r1 == r2 && !memcmp(w2, g2, sizeof w2), "snprintf %s cut %zu: want [%.20s] %d, got [%.20s] %d", fmt, cut, w2, r1, g2, r2); \
        } \
    } while (0)

/* The number of arguments the formatter has taken: libc-test.sh builds stdio.c
   with a stdarg.h that counts every va_arg (engine/test/counting). */
unsigned long sl_va_count;

static void on_abort(int sig)
{
    (void)sig;
    /* the number of arguments taken, as the exit status */
    _exit(100 + (int)sl_va_count);
}

/* Runs fn in a child process. Gives -1 when the process did not die of an abort
   (a trap, SIGILL or SIGTRAP, would end a Go process and does not count), and
   else the number of arguments the formatter had taken when it aborted. The
   library is built with its names changed, but not abort, so this is the
   system's abort. api-test.c checks the way through the engine's own abort and
   the host. */
static int abort_reads(void (*fn)(const void *), const void *arg)
{
    pid_t pid = fork();
    int status;

    if (pid == 0) {
        signal(SIGABRT, on_abort);
        fclose(stderr);
        sl_va_count = 0;
        fn(arg);
        _exit(0);
    }
    waitpid(pid, &status, 0);
    return WIFEXITED(status) && WEXITSTATUS(status) >= 100 ? WEXITSTATUS(status) - 100 : -1;
}

/* Arguments for the format, so that a formatter that reads them does not read
   what it should not: the ints are all the same, and a * that took one would
   find a width or a precision. */
typedef struct {
    const char *fmt;
    int arg;
} Case;

static void run_format(const void *arg)
{
    const Case *c = arg;
    char buf[64];

    sl_snprintf(buf, sizeof buf, c->fmt, c->arg, c->arg, c->arg);
}

static void run_int_min_width(const void *arg)
{
    char buf[64];

    (void)arg;
    sl_snprintf(buf, sizeof buf, "%*d", INT_MIN, 1);
}

static void run_too_long(const void *arg)
{
    char buf[8];

    (void)arg;
    sl_snprintf(buf, sizeof buf, "%2147483647dx", 1);
}

static void test_aborts(void)
{
    static const char *bad[] = {
        /* floating point, %n, and positional arguments */
        "%f", "%e", "%g", "%a", "%F", "%E", "%G", "%A", "%5.2f", "%lf", "%Lf", "%n", "%hhn", "%1$d", "%2$s", "%*1$d", "%.*1$d",
        /* a length where the conversion takes none */
        "%ls", "%lc", "%hs", "%hc", "%llc", "%zs", "%lp", "%hp", "%llp", "%l%", "%h%", "%ll%",
        /* flags, a width or a precision on %% */
        "%5%", "%-%", "%.1%", "%0%",
        /* a conversion that is not there */
        "%", "%l", "%ll", "%-", "%5", "%.", "%.5", "%*", "%.*", "%z", "%q", "%S", "%C", "%y", "% ", "%+",
        /* numbers that do not fit an int */
        "%2147483648d", "%99999999999999999999d", "%.2147483648d", "%.99999999999999999999s",
    };
    /* The same with a * in front: the conversion is refused, and no argument was
       taken for the *. The int given is what the * would have found: 3 is a
       width or precision that leaves the format as it was, and 0 and -1 are the
       values for which %% would have been printed. */
    static const Case star[] = {
        { "%*ls", 3 }, { "%.*f", 3 }, { "%*2$d", 3 }, { "%*%", 0 }, { "%.*%", -1 }, { "%*.*ls", 3 },
        { "%.*lc", 3 }, { "%*n", 3 }, { "%*.*f", 3 }, { "%*1$d", 3 }, { "%-*%", 0 }, { "%*lp", 3 },
        { "%*q", 3 }, { "%*.*%", 3 }, { "%.*5%", 3 }, { "%*l%", 3 }, { "%0*%", 0 },
    };
    size_t i;

    for (i = 0; i < sizeof bad / sizeof *bad; i++) {
        Case c = { bad[i], 3 };
        int reads = abort_reads(run_format, &c);

        CHECK(reads == 0, "format %s: %s", bad[i], reads < 0 ? "does not abort" : "takes an argument before it aborts");
    }
    for (i = 0; i < sizeof star / sizeof *star; i++) {
        int reads = abort_reads(run_format, &star[i]);

        CHECK(reads == 0, "format %s: %s", star[i].fmt, reads < 0 ? "does not abort" : "takes an argument before it aborts");
    }
    CHECK(abort_reads(run_int_min_width, NULL) >= 0, "a width of INT_MIN does not abort");
    CHECK(abort_reads(run_too_long, NULL) >= 0, "an output over INT_MAX does not abort");

    /* the count is real: a good format takes the arguments it names, the * first */
    {
        char buf[64];

        sl_va_count = 0;
        CHECK(sl_snprintf(buf, sizeof buf, "%*.*d|%s|%c", 6, 3, 42, "x", 'y') == 10 && !strcmp(buf, "   042|x|y"), "good format: [%s]", buf);
        CHECK(sl_va_count == 5, "a good format took %lu arguments, want 5", sl_va_count);
        sl_va_count = 0;
        CHECK(sl_snprintf(buf, sizeof buf, "100%%") == 4, "plain percent");
        CHECK(sl_va_count == 0, "%%%% took an argument");
    }
}

/* ---- the formats the generator accepts ----

   internal/goasm/gen/formats.go refuses, when the engine is built, a format
   that vsnprintf would abort on at run time. Its table is
   internal/goasm/gen/testdata/formats/cases.txt: "ok" or "bad", a tab and a
   format on each line, and a test of the generator holds the table to its
   checker. This holds it to vsnprintf: an "ok" format is done with no abort,
   after taking exactly the arguments it names, and a "bad" one ends in SIGABRT
   (and not in a trap). A change of stdio.c that the table does not follow, or
   the other way round, fails one of them. */

/* The arguments a format takes, one letter for each: i an int (also what a *
   takes, and %c), l a long, q a long long, z a size_t, j an intmax_t, t a
   ptrdiff_t, s a string and p a pointer. Gives the number of arguments, or -1
   when the format is not one that the table can say "ok" about. */
static int plan_args(const char *f, char *kinds, size_t cap)
{
    size_t n = 0;

    for (; *f; f++) {
        char k = 'i', c;

        if (*f != '%')
            continue;
        f++;
        while (*f && strchr("-+ 0#", *f))
            f++;
        if (*f == '*') {
            if (n + 1 >= cap) return -1;
            kinds[n++] = 'i';
            f++;
        } else {
            while (*f >= '0' && *f <= '9')
                f++;
        }
        if (*f == '.') {
            f++;
            if (*f == '*') {
                if (n + 1 >= cap) return -1;
                kinds[n++] = 'i';
                f++;
            } else {
                while (*f >= '0' && *f <= '9')
                    f++;
            }
        }
        switch (*f) {
        case 'h': f++; if (*f == 'h') f++; break;
        case 'l': f++; if (*f == 'l') { f++; k = 'q'; } else k = 'l'; break;
        case 'z': f++; k = 'z'; break;
        case 'j': f++; k = 'j'; break;
        case 't': f++; k = 't'; break;
        }
        switch (c = *f) {
        case 'd': case 'i': case 'u': case 'x': case 'X': case 'o': break;
        case 'c': k = 'i'; break;
        case 's': k = 's'; break;
        case 'p': k = 'p'; break;
        case '%': k = 0; break;
        default: return -1;
        }
        if (k) {
            if (n + 1 >= cap) return -1;
            kinds[n++] = k;
        }
    }
    kinds[n] = 0;
    return (int)n;
}

/* vsnprintf of the format, given the arguments its kinds name. -2 when there
   is no call for these kinds: a new table entry may need one. */
static int call_kinds(const char *kinds, const char *fmt, char *buf, size_t size)
{
    static const char str[] = "abcdef";
    static int marker;

    if (!strcmp(kinds, "")) return vwrap(buf, size, fmt);
    if (!strcmp(kinds, "i")) return vwrap(buf, size, fmt, 3);
    if (!strcmp(kinds, "l")) return vwrap(buf, size, fmt, 3L);
    if (!strcmp(kinds, "q")) return vwrap(buf, size, fmt, 3LL);
    if (!strcmp(kinds, "z")) return vwrap(buf, size, fmt, (size_t)3);
    if (!strcmp(kinds, "j")) return vwrap(buf, size, fmt, (intmax_t)3);
    if (!strcmp(kinds, "t")) return vwrap(buf, size, fmt, (ptrdiff_t)3);
    if (!strcmp(kinds, "s")) return vwrap(buf, size, fmt, str);
    if (!strcmp(kinds, "p")) return vwrap(buf, size, fmt, (void *)&marker);
    if (!strcmp(kinds, "ii")) return vwrap(buf, size, fmt, 3, 3);
    if (!strcmp(kinds, "is")) return vwrap(buf, size, fmt, 3, str);
    if (!strcmp(kinds, "si")) return vwrap(buf, size, fmt, str, 3);
    if (!strcmp(kinds, "ss")) return vwrap(buf, size, fmt, str, str);
    if (!strcmp(kinds, "iis")) return vwrap(buf, size, fmt, 3, 3, str);
    if (!strcmp(kinds, "iii")) return vwrap(buf, size, fmt, 3, 3, 3);
    return -2;
}

/* Runs the format in a child process. Gives the number of arguments the
   formatter took, and sets *how to 0 when it returned, 1 when it died of
   SIGABRT, 2 for any other end (a trap, a signal), and 3 when there is no call
   for the kinds. */
static int table_run(const char *fmt, const char *kinds, int *how)
{
    pid_t pid = fork();
    int status, code;

    if (pid == 0) {
        char buf[64];

        signal(SIGABRT, on_abort);
        fclose(stderr);
        sl_va_count = 0;
        if (call_kinds(kinds, fmt, buf, sizeof buf) == -2)
            _exit(99);
        _exit((int)sl_va_count);
    }
    waitpid(pid, &status, 0);
    if (!WIFEXITED(status)) {
        *how = 2;
        return -1;
    }
    code = WEXITSTATUS(status);
    *how = code >= 100 ? 1 : code == 99 ? 3 : 0;
    return code >= 100 ? code - 100 : code;
}

static void test_format_table(const char *path)
{
    char line[256], *text = NULL, *cur;
    FILE *f = path ? fopen(path, "r") : NULL;
    size_t size = 0, cap = 0, got;
    int ok_n = 0, bad_n = 0;

    CHECK(f != NULL, "the table of formats %s cannot be read", path ? path : "(none given)");
    if (!f)
        return;
    /* The whole table is read before the first child process is made, and the
       file closed: a child that ends in the C library of macOS moves the offset
       of a stream that is still open (the offset is shared with the parent), and
       the parent then reads the same bytes again, for ever. */
    for (;;) {
        if (size + 4096 > cap) {
            cap = cap ? cap * 2 : 65536;
            text = realloc(text, cap);
            CHECK(text != NULL, "out of memory");
            if (!text)
                return;
        }
        got = fread(text + size, 1, cap - size - 1, f);
        if (!got)
            break;
        size += got;
    }
    fclose(f);
    text[size] = 0;
    for (cur = text; *cur; ) {
        char *nl = strchr(cur, '\n'), *tab, kinds[8];
        size_t len = nl ? (size_t)(nl - cur) : strlen(cur);
        int ok, how, took;

        if (len >= sizeof line)
            len = sizeof line - 1;
        memcpy(line, cur, len);
        line[len] = 0;
        cur = nl ? nl + 1 : cur + strlen(cur);
        tab = strchr(line, '\t');
        if (!len || line[0] == '#')
            continue;
        CHECK(tab != NULL, "line without a tab: %s", line);
        if (!tab)
            continue;
        *tab++ = 0;
        ok = !strcmp(line, "ok");
        CHECK(ok || !strcmp(line, "bad"), "verdict %s is not ok or bad", line);
        if (ok) {
            int want = plan_args(tab, kinds, sizeof kinds);

            ok_n++;
            CHECK(want >= 0, "ok format %s: cannot tell its arguments", tab);
            if (want < 0)
                continue;
            took = table_run(tab, kinds, &how);
            CHECK(how != 3, "ok format %s: no call for arguments \"%s\"", tab, kinds);
            CHECK(how != 1, "format %s is ok in the table, and vsnprintf aborts", tab);
            CHECK(how != 2, "format %s: the process died of something but an abort", tab);
            CHECK(how != 0 || took == want, "ok format %s: took %d arguments, want %d", tab, took, want);
        } else {
            bad_n++;
            took = table_run(tab, "iii", &how);
            CHECK(how != 3, "bad format %s: no call for arguments \"iii\"", tab);
            CHECK(how == 1 || how == 3, "format %s is bad in the table, and vsnprintf %s", tab, how == 0 ? "takes it" : "dies, but not of an abort");
        }
    }
    free(text);
    CHECK(ok_n >= 40 && bad_n >= 40, "only %d ok and %d bad formats read", ok_n, bad_n);
}

/* A huge width costs nothing when the buffer is small, and the result is its length. */
static void test_long_fill(void)
{
    char buf[8];

    CHECK(sl_snprintf(buf, sizeof buf, "%2000000000d", 1) == 2000000000 && !strcmp(buf, "       "), "long width: [%s]", buf);
    CHECK(sl_snprintf(buf, 0, "%-1000000000d|", 1) == 1000000001, "long left width, no buffer");
    CHECK(sl_snprintf(buf, sizeof buf, "%.1000000000d", 1) == 1000000000 && !strcmp(buf, "0000000"), "long precision: [%s]", buf);
    CHECK(sl_snprintf(buf, sizeof buf, "%*s", 1500000000, "abc") == 1500000000, "long width from an argument");
    CHECK(sl_snprintf(buf, sizeof buf, "%*d|", -3, 5) == 4 && !strcmp(buf, "5  |"), "negative width: [%s]", buf);
}

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

int main(int argc, char **argv)
{
    test_mem();
    test_memcpy_all();
    test_memcpy_borders();
    test_str();
    test_snprintf();
    test_aborts();
    test_format_table(argc > 1 ? argv[1] : NULL);
    test_long_fill();
    printf("libc-test: %ld checks, %d failures\n", checks, failures);
    return failures != 0;
}
