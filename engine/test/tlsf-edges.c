/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The edge cases of TLSF that engine/tlsf.c handles and the pinned file of
   Matthew Conte's project does not. tlsf-edges.sh builds this file against
   both, with the sanitizers, and runs it twice:

     tlsf-edges patched   every case passes
     tlsf-edges pinned    the cases marked FIX do not pass, the others do

   Each case runs in a child process, so a crash shows as CRASH for that case
   only. It includes tlsf.c, found through -I, to test its static functions. */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/wait.h>
#include <unistd.h>

#include <tlsf.c>

#ifndef MAP_ANON
#define MAP_ANON MAP_ANONYMOUS
#endif
#ifndef MAP_NORESERVE
#define MAP_NORESERVE 0
#endif

static tlsf_t make(size_t bytes, void **mem)
{
    *mem = malloc(bytes);
    return tlsf_create_with_pool(*mem, bytes);
}

static int same(const unsigned char *p, size_t n, unsigned char v)
{
    for (size_t i = 0; i < n; i++)
        if (p[i] != v) return 0;
    return 1;
}

/* issue 22: a request near SIZE_MAX wraps in align_up and gets a tiny block */
static int t_malloc_huge(void)
{
    void *mem;
    tlsf_t t = make(1 << 20, &mem);
    size_t sizes[] = { SIZE_MAX, SIZE_MAX - 3, SIZE_MAX - 7, (size_t)1 << 62,
                       tlsf_block_size_max(), tlsf_block_size_max() - 1, tlsf_block_size_max() - 8 };
    for (size_t i = 0; i < sizeof sizes / sizeof sizes[0]; i++) {
        void *p = tlsf_malloc(t, sizes[i]);
        if (p) { printf("  malloc(%zu) returned a block of %zu\n", sizes[i], tlsf_block_size(p)); return 0; }
    }
    return tlsf_check(t) == 0;
}

/* realloc with a size TLSF cannot serve must fail and leave the block alone */
static int t_realloc_huge(void)
{
    void *mem;
    tlsf_t t = make(1 << 20, &mem);
    unsigned char *p = tlsf_malloc(t, 1000);
    size_t before = tlsf_block_size(p);
    memset(p, 0xab, 1000);
    size_t sizes[] = { SIZE_MAX - 3, (size_t)1 << 62, tlsf_block_size_max() };
    for (size_t i = 0; i < sizeof sizes / sizeof sizes[0]; i++) {
        void *q = tlsf_realloc(t, p, sizes[i]);
        if (q) { printf("  realloc(p, %zu) returned %s, block now %zu (was %zu)\n", sizes[i],
                        q == p ? "p" : "a new block", tlsf_block_size(q), before); return 0; }
        if (tlsf_block_size(p) != before || !same(p, 1000, 0xab)) { printf("  old block changed\n"); return 0; }
    }
    return tlsf_check(t) == 0;
}

/* a move that finds no room fails and keeps the old block */
static int t_realloc_no_room(void)
{
    void *mem;
    tlsf_t t = make(tlsf_size() + 65536, &mem);
    unsigned char *p = tlsf_malloc(t, 30000);
    unsigned char *guard = tlsf_malloc(t, 64);  /* so p cannot grow in place */
    memset(p, 0x3c, 30000);
    void *q = tlsf_realloc(t, p, 40000);
    if (q) { printf("  realloc found room it should not have\n"); return 0; }
    (void)guard;
    return tlsf_block_size(p) >= 30000 && same(p, 30000, 0x3c) && tlsf_check(t) == 0;
}

/* issue 35: create_with_pool on a buffer smaller than the control writes past it */
static int t_create_small(void)
{
    size_t sizes[] = { 40, 4096, tlsf_size(), tlsf_size() + tlsf_pool_overhead() };
    for (size_t i = 0; i < sizeof sizes / sizeof sizes[0]; i++) {
        void *mem = malloc(sizes[i]);
        tlsf_t t = tlsf_create_with_pool(mem, sizes[i]);
        if (t) { printf("  create_with_pool(%zu) succeeded\n", sizes[i]); return 0; }
        free(mem);
    }
    return 1;
}

/* a pool smaller than its overhead plus one minimum block: bytes - overhead wraps */
static int t_add_pool_small(void)
{
    void *mem;
    tlsf_t t = make(1 << 16, &mem);
    for (size_t n = 0; n < tlsf_pool_overhead() + tlsf_block_size_min(); n += 8) {
        void *m = malloc(n ? n : 1);
        if (tlsf_add_pool(t, m, n)) { printf("  add_pool(%zu) succeeded\n", n); return 0; }
        free(m);
    }
    return tlsf_check(t) == 0;
}

/* issue 9: a pool of exactly block_size_max bytes indexes past the free lists */
static int t_pool_max(void)
{
    void *cmem = malloc(tlsf_size());
    tlsf_t t = tlsf_create(cmem);
    size_t n = tlsf_block_size_max() + tlsf_pool_overhead();
    void *m = mmap(NULL, n + 4096, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON | MAP_NORESERVE, -1, 0);
    if (m == MAP_FAILED) { perror("mmap"); return 0; }
    if (tlsf_add_pool(t, m, n)) { printf("  add_pool of block_size_max succeeded\n"); return 0; }
    return 1;
}

/* the largest pool TLSF accepts still works */
static int t_pool_largest(void)
{
    void *cmem = malloc(tlsf_size());
    tlsf_t t = tlsf_create(cmem);
    size_t n = tlsf_block_size_max() + tlsf_pool_overhead() - 8;
    void *m = mmap(NULL, n, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON | MAP_NORESERVE, -1, 0);
    if (m == MAP_FAILED) { perror("mmap"); return 0; }
    if (!tlsf_add_pool(t, m, n)) { printf("  add_pool refused the largest pool\n"); return 0; }
    void *a = tlsf_malloc(t, (size_t)1 << 31);
    return a && tlsf_check(t) == 0;
}

/* a block that fills a dedicated pool: need + need/32 + overhead is enough */
static int t_dedicated(void)
{
    size_t needs[] = { 600, 4096, 65536, 100000, 1 << 20, 3 << 20, (1 << 24) + 8, 80 << 20 };
    for (size_t i = 0; i < sizeof needs / sizeof needs[0]; i++) {
        size_t need = needs[i];
        size_t bytes = need + need / 32 + tlsf_pool_overhead() + 64;
        void *cmem = malloc(tlsf_size());
        tlsf_t t = tlsf_create(cmem);
        void *m = mmap(NULL, bytes, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON, -1, 0);
        if (!tlsf_add_pool(t, m, bytes)) { printf("  add_pool(%zu) failed\n", bytes); return 0; }
        void *a = tlsf_malloc(t, need);
        if (!a) { printf("  need %zu did not fit a pool of %zu\n", need, bytes); return 0; }
        munmap(m, bytes);
        free(cmem);
    }
    return 1;
}

/* remove_pool and walk_pool on a whole free pool */
static int walk_n;
static void walker(void *ptr, size_t size, int used, void *user) { (void)ptr; (void)size; (void)used; (void)user; walk_n++; }
static int t_remove_pool(void)
{
    void *mem;
    tlsf_t t = make(1 << 16, &mem);
    void *m = malloc(1 << 16);
    pool_t p = tlsf_add_pool(t, m, 1 << 16);
    walk_n = 0;
    tlsf_walk_pool(p, walker, NULL);
    if (walk_n != 1) return 0;
    tlsf_remove_pool(t, p);
    /* the pool is gone: a request of its size must fail */
    void *a = tlsf_malloc(t, 60000);
    return !a && tlsf_check(t) == 0;
}

/* the bit functions the pinned file checks in its debug self test, which the
   port takes out */
static int t_bits(void)
{
    int rv = 0;
    rv += (tlsf_ffs(0) == -1) ? 0 : 0x1;
    rv += (tlsf_fls(0) == -1) ? 0 : 0x2;
    rv += (tlsf_ffs(1) == 0) ? 0 : 0x4;
    rv += (tlsf_fls(1) == 0) ? 0 : 0x8;
    rv += (tlsf_ffs(0x80000000) == 31) ? 0 : 0x10;
    rv += (tlsf_ffs(0x80008000) == 15) ? 0 : 0x20;
    rv += (tlsf_fls(0x80000008) == 31) ? 0 : 0x40;
    rv += (tlsf_fls(0x7FFFFFFF) == 30) ? 0 : 0x80;
#if defined(TLSF_64BIT)
    rv += (tlsf_fls_sizet(0x80000000) == 31) ? 0 : 0x100;
    rv += (tlsf_fls_sizet(0x100000000) == 32) ? 0 : 0x200;
    rv += (tlsf_fls_sizet(0xffffffffffffffff) == 63) ? 0 : 0x400;
#endif
    if (rv) printf("  ffs and fls: %x tests failed\n", rv);
    return rv == 0;
}

static struct { const char *name; int fix; int (*fn)(void); } cases[] = {
    { "malloc of a huge size fails (issue 22)", 1, t_malloc_huge },
    { "realloc to a huge size fails, block kept", 1, t_realloc_huge },
    { "realloc with no room fails, block kept", 0, t_realloc_no_room },
    { "create_with_pool on a small buffer fails (issue 35)", 1, t_create_small },
    { "add_pool smaller than its overhead fails", 0, t_add_pool_small },
    { "add_pool of exactly block_size_max fails (issue 9)", 1, t_pool_max },
    { "add_pool just below block_size_max works", 0, t_pool_largest },
    { "dedicated pool of need + need/32 + overhead fits", 0, t_dedicated },
    { "remove_pool of a whole free pool", 0, t_remove_pool },
    { "ffs and fls", 0, t_bits },
};

int main(int argc, char **argv)
{
    int pinned;
    int bad = 0;

    if (argc != 2 || (strcmp(argv[1], "pinned") && strcmp(argv[1], "patched"))) {
        fprintf(stderr, "usage: tlsf-edges pinned|patched\n");
        return 2;
    }
    pinned = !strcmp(argv[1], "pinned");
    for (size_t i = 0; i < sizeof cases / sizeof cases[0]; i++) {
        int ok, expect_pass;
        const char *r;
        pid_t pid;
        int st;

        fflush(stdout);
        pid = fork();
        if (pid == 0) {
            int res = cases[i].fn();
            fflush(stdout);
            _exit(res ? 0 : 1);
        }
        waitpid(pid, &st, 0);
        r = WIFEXITED(st) ? (WEXITSTATUS(st) == 0 ? "PASS" : "FAIL") : "CRASH";
        if (WIFEXITED(st) && WEXITSTATUS(st) > 1) r = "CRASH"; /* a sanitizer exits with 86 */
        ok = !strcmp(r, "PASS");
        expect_pass = !(pinned && cases[i].fix);
        printf("%-5s %s%s\n", r, cases[i].name, ok == expect_pass ? "" : "   <- unexpected");
        if (ok != expect_pass) bad++;
    }
    printf("%s: %s\n", argv[1], bad ? "unexpected results" : "as expected");
    return bad != 0;
}
