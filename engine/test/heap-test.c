/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Tests of the allocator: heap.c.

     heap-test scenarios          the cases of limit, budget, resize, host
                                  refusal and release
     heap-test stress SEED STEPS  random sequences, checked as they go
     heap-test fuzz FILE...       replay the inputs of the fuzz target

   Built with -DHEAP_FUZZ it is the libFuzzer target of the same sequences. The
   host is a fake one: it maps chunks with mmap, remembers them, can refuse the
   nth request, and fills every range given back with 0xdd. Every block holds a
   pattern over its whole usable size, which is checked when it is freed or
   resized. Every few thousand steps the heap checks itself (TLSF, the counts,
   the footprint) and compares its counts with the sum over the blocks the
   test holds. */
#include <assert.h>
#include <inttypes.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>

#include "../host.h"
#include "heap.h"
#include "tlsf.h"

/* ---- the fake host ---- */

/* The engine's abort, which a failed assert of the heap calls, asks the host
   with host_call. The test ends there. */
JSValue host_call(uint32_t vm, JSValueConst this_val, int argc, JSValueConst *argv, int magic)
{
    (void)vm;
    (void)this_val;
    (void)argc;
    (void)argv;
    fprintf(stderr, "heap-test: the heap called abort (host_call %d)\n", magic);
    fflush(stderr);
    _exit(134);
}

#define MAX_VM 4
#define MAX_CHUNK 8192

typedef struct HostChunk {
    uintptr_t addr;
    size_t size;
} HostChunk;

typedef struct HostVM {
    int open;
    HostChunk chunk[MAX_CHUNK];
    int n;
    size_t live;      /* bytes of the chunks it holds */
    long grows;       /* requests so far */
    long refused;     /* requests it refused */
    long refuse_at;   /* the request with this number is refused (1 is the first), 0 for none */
    int refuse_all;   /* every request is refused */
    size_t most;      /* the most bytes it ever held */
} HostVM;

static HostVM vms[MAX_VM];

static uint32_t host_open(void)
{
    for (uint32_t i = 0; i < MAX_VM; i++) {
        if (!vms[i].open) {
            memset(&vms[i], 0, sizeof vms[i]);
            vms[i].open = 1;
            return i;
        }
    }
    fprintf(stderr, "heap-test: no free VM slot\n");
    abort();
}

static void host_close(uint32_t vm)
{
    HostVM *h = &vms[vm];

    for (int i = 0; i < h->n; i++) {
        heap_unpoison((void *)h->chunk[i].addr, h->chunk[i].size);
        munmap((void *)h->chunk[i].addr, h->chunk[i].size);
    }
    memset(h, 0, sizeof *h);
}

uintptr_t host_memory_grow(uint32_t vm, uintptr_t size)
{
    HostVM *h = &vms[vm];
    void *p;

    assert(h->open);
    assert(size % 65536 == 0 && size > 0);
    h->grows++;
    if (h->refuse_all || h->refuse_at == h->grows) {
        h->refused++;
        return 0;
    }
    assert(h->n < MAX_CHUNK);
    p = mmap(NULL, size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON, -1, 0);
    if (p == MAP_FAILED) {
        h->refused++;
        return 0;
    }
    heap_unpoison(p, size);
    /* the engine never assumes zeroed memory */
    memset(p, 0xcd, size < ((size_t)1 << 20) ? size : ((size_t)1 << 20));
    h->chunk[h->n].addr = (uintptr_t)p;
    h->chunk[h->n].size = size;
    h->n++;
    h->live += size;
    if (h->live > h->most)
        h->most = h->live;
    return (uintptr_t)p;
}

static int host_find(HostVM *h, uintptr_t addr)
{
    for (int i = 0; i < h->n; i++)
        if (h->chunk[i].addr == addr)
            return i;
    return -1;
}

static int host_inside(HostVM *h, uintptr_t addr, size_t len)
{
    for (int i = 0; i < h->n; i++)
        if (addr >= h->chunk[i].addr && addr + len <= h->chunk[i].addr + h->chunk[i].size)
            return 1;
    return 0;
}

/* What Go does after a release: unmap the chunks, and take the pages of the
   ranges. Here the whole range is filled, not only the pages inside it, so
   that the heap is caught reading or keeping anything of it. */
typedef struct Releaser {
    void *obj;
    void (*start)(void *obj, size_t keep);
    JSMemRange (*chunk)(void *obj);
    JSMemRange (*range)(void *obj);
} Releaser;

typedef struct ReleaseResult {
    size_t chunks, chunk_bytes, ranges, candidate, released;
} ReleaseResult;

static ReleaseResult host_release(uint32_t vm, const Releaser *rl, size_t keep)
{
    HostVM *h = &vms[vm];
    ReleaseResult res = { 0, 0, 0, 0, 0 };
    JSMemRange r;

    rl->start(rl->obj, keep);
    while ((r = rl->chunk(rl->obj)).len) {
        int i = host_find(h, r.addr);
        if (i < 0 || h->chunk[i].size != r.len) {
            fprintf(stderr, "heap-test: the heap gave back a chunk the host did not give\n");
            abort();
        }
        heap_unpoison((void *)r.addr, r.len);
        munmap((void *)r.addr, r.len);
        h->live -= r.len;
        h->chunk[i] = h->chunk[--h->n];
        res.chunks++;
        res.chunk_bytes += r.len;
        res.candidate += r.len;
        res.released += r.len;
    }
    while ((r = rl->range(rl->obj)).len) {
        uintptr_t a = (r.addr + 4095) & ~(uintptr_t)4095;
        uintptr_t e = (r.addr + r.len) & ~(uintptr_t)4095;
        if (!host_inside(h, r.addr, r.len)) {
            fprintf(stderr, "heap-test: the heap offered a range outside its chunks\n");
            abort();
        }
        res.ranges++;
        res.candidate += r.len;
        heap_unpoison((void *)r.addr, r.len);
        memset((void *)r.addr, 0xdd, r.len);
        heap_poison((void *)r.addr, r.len);
        if (e > a)
            res.released += e - a;
    }
    return res;
}

/* ---- random numbers: a seeded generator, or the bytes of a fuzz input ---- */

typedef struct Src {
    const uint8_t *data;
    size_t len, pos;
    uint64_t s;
    int fuzz;
    int done;
} Src;

static uint64_t src_next(Src *r)
{
    if (r->fuzz) {
        uint64_t v = 0;
        if (r->pos >= r->len) {
            r->done = 1;
            return 0;
        }
        for (int i = 0; i < 2 && r->pos < r->len; i++)
            v = (v << 8) | r->data[r->pos++];
        return v;
    }
    r->s ^= r->s >> 12;
    r->s ^= r->s << 25;
    r->s ^= r->s >> 27;
    return r->s * 0x2545F4914F6CDD1Dull;
}

static uint64_t src_below(Src *r, uint64_t n)
{
    return n ? src_next(r) % n : 0;
}

/* ---- blocks the test holds ---- */

#define MAX_LIVE 4096

typedef struct Block {
    uint8_t *p;
    size_t usable;
    uint32_t seed;
} Block;

static uint8_t pat(uint32_t seed, size_t i)
{
    return (uint8_t)(((uint32_t)i * 2654435761u + seed * 40503u) >> 13);
}

/* The pattern covers the whole block up to 64 KiB. Of a bigger block the first
   and the last 32 KiB are covered, and 8 bytes of every 4 KiB between, so
   that a few big blocks do not take the time of the run. */
#define PAT_EDGE 32768u
#define PAT_STRIDE 4096u

static size_t pat_next(size_t i, size_t n)
{
    i++;
    if (n <= 2 * PAT_EDGE || i < PAT_EDGE || i >= n - PAT_EDGE)
        return i;
    if (i % PAT_STRIDE < 8)
        return i;
    i = (i / PAT_STRIDE + 1) * PAT_STRIDE;
    return i < n - PAT_EDGE ? i : n - PAT_EDGE;
}

static void fill(Block *b)
{
    for (size_t i = 0; i < b->usable; i = pat_next(i, b->usable))
        b->p[i] = pat(b->seed, i);
}

static void check_content(const Block *b, size_t n, const char *what)
{
    for (size_t i = 0; i < n; i = pat_next(i, b->usable)) {
        if (b->p[i] != pat(b->seed, i)) {
            fprintf(stderr, "heap-test: %s: byte %zu of a block of %zu changed\n", what, i, b->usable);
            abort();
        }
    }
}

/* The blocks the test holds, as spans of addresses in order: no two blocks
   may overlap, whatever the patterns say. A block of size 0 (the shared zero
   size block of the runtime) is not put in. */
typedef struct Span {
    uintptr_t lo, hi;
} Span;

static void span_add(Span *sp, int *n, uintptr_t lo, uintptr_t hi)
{
    int i = 0, j = *n;

    while (i < j) { /* the first span that starts after lo */
        int k = (i + j) / 2;
        if (sp[k].lo <= lo)
            i = k + 1;
        else
            j = k;
    }
    if ((i > 0 && sp[i - 1].hi > lo) || (i < *n && sp[i].lo < hi)) {
        fprintf(stderr, "heap-test: a new block [%#lx, %#lx) overlaps a block in use\n", (unsigned long)lo, (unsigned long)hi);
        abort();
    }
    memmove(sp + i + 1, sp + i, (size_t)(*n - i) * sizeof *sp);
    sp[i].lo = lo;
    sp[i].hi = hi;
    (*n)++;
}

static void span_remove(Span *sp, int *n, uintptr_t lo)
{
    for (int i = 0; i < *n; i++) {
        if (sp[i].lo == lo) {
            memmove(sp + i, sp + i + 1, (size_t)(*n - i - 1) * sizeof *sp);
            (*n)--;
            return;
        }
    }
    fprintf(stderr, "heap-test: a block that is not in the set was freed or resized\n");
    abort();
}

/* a request the heap may refuse for its size alone: past the maximum, or so
   close to it that the chunk of its own would be a pool bigger than TLSF takes */
#define TOO_BIG(sz) ((sz) > ((size_t)1 << 32) - ((size_t)1 << 27))

#define FAIL(...) do { fprintf(stderr, "heap-test: " __VA_ARGS__); fprintf(stderr, "\n"); abort(); } while (0)
#define CHECK(c, ...) do { if (!(c)) FAIL(__VA_ARGS__); } while (0)

static const size_t edge_sizes[] = {
    1, 2, 7, 8, 9, 15, 16, 17, 504, 505, 511, 512, 513, 1000, 4088, 4096, 4104, 32760, 32768, 65520, 65536, 65537,
    (size_t)1 << 20, (size_t)3 << 20, (size_t)8 << 20,
};
#define N_EDGE (sizeof edge_sizes / sizeof edge_sizes[0])

/* A size from the mix of a render (nearly all small, a few medium, very few
   large) plus the edges, and now and then a size that must fail. */
static size_t pick_size(Src *r, int allow_zero)
{
    uint64_t k = src_below(r, 1000);

    if (k < 560)
        return 1 + src_below(r, 504);
    if (k < 800)
        return 505 + src_below(r, 3600);
    if (k < 900)
        return 4097 + src_below(r, 61440);
    if (k < 920)
        return 65537 + src_below(r, (size_t)4 << 20);
    if (k < 975)
        return edge_sizes[src_below(r, N_EDGE)];
    if (k < 990)
        return allow_zero ? 0 : 1;
    if (k < 995)
        return SIZE_MAX - src_below(r, 16);
    if (k < 997)
        return JS_HEAP_MAX_REQUEST + 1 + src_below(r, 4);
    if (k < 999)
        return (size_t)1 << (33 + src_below(r, 30));
    /* 64 MiB is rare: filling it with a pattern takes a while */
    return src_below(r, 40) == 0 ? (size_t)64 << 20 : edge_sizes[src_below(r, N_EDGE)];
}

/* ---- the heap level ---- */

typedef struct HeapModel {
    uint32_t vm;
    JSHeap *h;
    size_t rt_size;
    Block live[MAX_LIVE];
    int n;
    Span spans[MAX_LIVE];
    int nspans;
    size_t bytes;       /* usable bytes the test holds */
    uint32_t seed;
    size_t budget;      /* the footprint budget the test set, 0 for none */
    size_t budget_top;  /* the largest footprint since the budget was set */
} HeapModel;

#define HEAP_RT_SIZE 1000

static void heap_model_check(HeapModel *m)
{
    JSHeap *h = m->h;
    HostVM *host = &vms[m->vm];
    size_t sum = 0;
    const char *msg;

    msg = heap_check(h);
    CHECK(!msg, "heap_check: %s", msg);
    for (int i = 0; i < m->n; i++) {
        Block *b = &m->live[i];
        CHECK(heap_contains(h, b->p), "a block is outside the chunks");
        CHECK((uintptr_t)b->p % 8 == 0, "a block is not 8 byte aligned");
        CHECK(heap_usable_size(b->p) == b->usable, "usable size of a block changed");
        sum += b->usable + 8;
    }
    CHECK(m->nspans == m->n, "the set of spans holds %d blocks, the test %d", m->nspans, m->n);
    for (int i = 0; i + 1 < m->nspans; i++)
        CHECK(m->spans[i].hi <= m->spans[i + 1].lo, "two blocks overlap");
    CHECK(h->blocks == (size_t)m->n, "blocks is %zu, the test holds %d", h->blocks, m->n);
    CHECK(h->count == m->rt_size + sum, "count is %zu, the blocks and the runtime add up to %zu", h->count, m->rt_size + sum);
    CHECK(h->footprint == host->live, "footprint is %zu, the host holds %zu", h->footprint, host->live);
    CHECK((size_t)h->chunk_count == (size_t)host->n, "chunk_count %zu, the host holds %d", h->chunk_count, host->n);
    if (m->budget)
        CHECK(m->budget_top <= m->budget || h->footprint <= m->budget_top, "the footprint passed the budget");
}

static void heap_model_start(HeapModel *m)
{
    memset(m, 0, sizeof *m);
    m->vm = host_open();
    m->h = heap_create(m->vm, HEAP_RT_SIZE);
    CHECK(m->h, "heap_create failed");
    m->rt_size = (HEAP_RT_SIZE + 7) & ~7u;
    CHECK(m->h->count == m->rt_size && m->h->blocks == 0 && m->h->footprint == 65536 && m->h->chunk_count == 1,
          "a new heap is not what it should be");
    CHECK(m->h->limit == (size_t)-1 && m->h->footprint_limit == 0, "a new heap has a limit");
    heap_model_check(m);
}

static void model_note_footprint(HeapModel *m)
{
    if (m->h->footprint > m->budget_top)
        m->budget_top = m->h->footprint;
}

static int model_alloc(HeapModel *m, size_t sz)
{
    JSHeap *h = m->h;
    HostVM *host = &vms[m->vm];
    size_t count = h->count, fp = h->footprint, blocks = h->blocks;
    long refused = host->refused;
    uint8_t *p;
    Block *b;

    p = heap_alloc(h, sz);
    model_note_footprint(m);
    if (!p) {
        /* the limit fails a request when count + request > limit, in numbers that do not wrap */
        int over = h->limit != (size_t)-1 && (unsigned __int128)count + sz > h->limit;
        int explained = sz == 0 || TOO_BIG(sz) || over || host->refused != refused || h->footprint_limit != 0;
        CHECK(explained, "heap_alloc(%zu) failed for no reason (count %zu, footprint %zu)", sz, count, fp);
        CHECK(h->count == count && h->footprint == fp && h->blocks == blocks, "a failed heap_alloc changed the heap");
        return 0;
    }
    CHECK(sz != 0 && sz <= JS_HEAP_MAX_REQUEST, "heap_alloc(%zu) succeeded", sz);
    CHECK(h->limit == (size_t)-1 || (unsigned __int128)count + sz <= h->limit, "heap_alloc(%zu) passed the limit", sz);
    CHECK((uintptr_t)p % 8 == 0, "block not aligned");
    CHECK(heap_usable_size(p) >= sz, "usable size %zu is below the request %zu", heap_usable_size(p), sz);
    b = &m->live[m->n++];
    b->p = p;
    b->usable = heap_usable_size(p);
    span_add(m->spans, &m->nspans, (uintptr_t)p, (uintptr_t)p + b->usable);
    b->seed = ++m->seed;
    fill(b);
    m->bytes += b->usable;
    return 1;
}

static void model_free(HeapModel *m, int i)
{
    Block *b = &m->live[i];

    check_content(b, b->usable, "free");
    span_remove(m->spans, &m->nspans, (uintptr_t)b->p);
    heap_free(m->h, b->p);
    m->bytes -= b->usable;
    m->live[i] = m->live[--m->n];
}

static void model_realloc(HeapModel *m, int i, size_t sz)
{
    JSHeap *h = m->h;
    HostVM *host = &vms[m->vm];
    Block *b = &m->live[i];
    size_t count = h->count, fp = h->footprint, blocks = h->blocks, old = b->usable;
    uint32_t old_seed = b->seed;
    long refused = host->refused;
    uint8_t *p;

    p = heap_realloc(h, b->p, sz);
    model_note_footprint(m);
    if (!p) {
        /* the rule of upstream: count + request - the usable size of the old block > limit */
        int over = h->limit != (size_t)-1 && (unsigned __int128)count + sz - old > h->limit;
        int explained = sz == 0 || TOO_BIG(sz) || over || host->refused != refused || h->footprint_limit != 0;
        CHECK(explained, "heap_realloc(%zu) failed for no reason", sz);
        CHECK(h->count == count && h->footprint == fp && h->blocks == blocks, "a failed heap_realloc changed the heap");
        CHECK(heap_usable_size(b->p) == old, "a failed heap_realloc changed the block");
        check_content(b, old, "failed realloc");
        return;
    }
    CHECK(sz != 0 && sz <= JS_HEAP_MAX_REQUEST, "heap_realloc(%zu) succeeded", sz);
    CHECK(h->limit == (size_t)-1 || (unsigned __int128)count + sz - old <= h->limit, "heap_realloc(%zu) passed the limit", sz);
    CHECK((uintptr_t)p % 8 == 0, "block not aligned");
    CHECK(heap_usable_size(p) >= sz, "usable size below the request");
    {
        Block was = { p, old, old_seed };
        check_content(&was, old < sz ? old : sz, "realloc");
    }
    span_remove(m->spans, &m->nspans, (uintptr_t)b->p);
    span_add(m->spans, &m->nspans, (uintptr_t)p, (uintptr_t)p + heap_usable_size(p));
    m->bytes += heap_usable_size(p) - old;
    b->p = p;
    b->usable = heap_usable_size(p);
    b->seed = ++m->seed;
    fill(b);
    CHECK(h->blocks == blocks, "heap_realloc changed the number of blocks");
}

static void model_release(HeapModel *m, size_t keep)
{
    JSHeap *h = m->h;
    Releaser rl = { h, (void (*)(void *, size_t))heap_release_start, (JSMemRange (*)(void *))heap_release_next_chunk,
                    (JSMemRange (*)(void *))heap_release_next_range };
    size_t count = h->count, blocks = h->blocks, fp = h->footprint, cand = h->release_candidate;
    ReleaseResult r = host_release(m->vm, &rl, keep);

    CHECK(h->count == count && h->blocks == blocks, "a release changed the count");
    CHECK(h->footprint == fp - r.chunk_bytes, "footprint did not drop by the chunks released");
    CHECK(h->release_candidate == cand + r.candidate, "release_candidate is not what was offered");
    CHECK(r.released <= r.candidate, "more released than offered");
    heap_model_check(m);
    /* the content of about 100 blocks: all of them at each release would take most of the run */
    for (int i = 0; i < m->n; i += m->n / 100 + 1)
        check_content(&m->live[i], m->live[i].usable, "release");
}

static void heap_model_step(HeapModel *m, Src *r)
{
    uint64_t op = src_below(r, 100);
    JSHeap *h = m->h;

    if (op < 40 || m->n == 0) {
        size_t sz = pick_size(r, 0);
        /* keep what the test holds in bounds, but still try the sizes that must fail */
        if (m->n < MAX_LIVE && (m->bytes < ((size_t)192 << 20) || sz > JS_HEAP_MAX_REQUEST))
            model_alloc(m, sz);
    } else if (op < 75) {
        model_free(m, (int)src_below(r, m->n));
    } else if (op < 92) {
        size_t sz = pick_size(r, 0);
        int i = (int)src_below(r, m->n);
        if (m->bytes < ((size_t)192 << 20) || sz > JS_HEAP_MAX_REQUEST)
            model_realloc(m, i, sz);
    } else if (op < 94) {
        model_release(m, (size_t)src_below(r, 3) == 0 ? 0 : (size_t)src_below(r, (size_t)1 << 20));
    } else if (op < 96) {
        /* a limit around the count, then none */
        h->limit = src_below(r, 2) ? h->count + src_below(r, 100000) : (size_t)-1;
    } else if (op < 98) {
        /* a footprint budget around the footprint (it never takes memory back), or none */
        if (src_below(r, 3) == 0) {
            h->footprint_limit = 0;
            m->budget = 0;
        } else {
            h->footprint_limit = h->footprint + src_below(r, 4) * 65536;
            m->budget = h->footprint_limit;
            m->budget_top = h->footprint;
        }
    } else {
        /* the host refuses the next request, or the one after */
        vms[m->vm].refuse_at = vms[m->vm].grows + 1 + (long)src_below(r, 2);
    }
}

static void heap_model_finish(HeapModel *m)
{
    JSHeap *h = m->h;
    HostVM *host = &vms[m->vm];

    while (m->n)
        model_free(m, m->n - 1);
    h->limit = (size_t)-1;
    h->footprint_limit = 0;
    heap_model_check(m);
    CHECK(h->blocks == 0 && h->count == m->rt_size, "blocks are left after everything was freed");
    model_release(m, 0);
    CHECK(h->footprint == 65536 && h->chunk_count == 1 && host->n == 1,
          "a heap with nothing in use still holds %zu bytes in %zu chunks after a release", h->footprint, h->chunk_count);
    heap_destroy(h);
    host_close(m->vm);
}

static void run_heap(Src *r, long steps)
{
    HeapModel *m = calloc(1, sizeof *m);

    heap_model_start(m);
    for (long i = 0; i < steps && !r->done; i++) {
        heap_model_step(m, r);
        if (i % 1500 == 1499)
            heap_model_check(m);
    }
    heap_model_check(m);
    heap_model_finish(m);
    free(m);
}


/* ---- the scenarios ---- */

static void ok(const char *name)
{
    printf("PASS  %s\n", name);
    fflush(stdout);
}

/* The limit fails at the byte: a request passes when count plus the request
   is at most the limit, whatever block the request gets. */
static void scenario_limit(void)
{
    HeapModel m;
    JSHeap *h;
    void *p, *q;
    size_t count;

    heap_model_start(&m);
    h = m.h;
    CHECK(!heap_alloc(h, 0), "heap_alloc(0) returned a block");
    count = h->count;
    h->limit = count + 1000 - 1;
    CHECK(!heap_alloc(h, 1000) && h->count == count && h->blocks == 0, "a request one byte over the limit passed");
    h->limit = count + 1000;
    p = heap_alloc(h, 1000);
    CHECK(p, "a request at the limit failed");
    CHECK(h->count == count + heap_usable_size(p) + 8 && h->blocks == 1, "count is not the block size plus 8");
    /* the count may pass the limit now: the next request fails, and a free works */
    CHECK(!heap_alloc(h, 1), "a request past the limit passed");
    heap_free(h, p);
    CHECK(h->count == count && h->blocks == 0, "count did not come back after a free");
    /* lowering the limit frees nothing and fails new requests */
    h->limit = (size_t)-1;
    p = heap_alloc(h, 5000);
    CHECK(p, "alloc 5000");
    h->limit = 10;
    CHECK(!heap_alloc(h, 1), "alloc with a limit below the count");
    CHECK(!heap_realloc(h, p, 6000), "realloc with a limit below the count passed");
    CHECK(heap_usable_size(p) >= 5000, "block changed");
    q = heap_realloc(h, p, 16);
    CHECK(!q, "shrinking under a limit below the count passed");
    h->limit = (size_t)-1;
    q = heap_alloc(h, 100);
    heap_free(h, q);
    heap_free(h, p);
    /* an overflowing request is refused before any arithmetic */
    h->limit = (size_t)-1 - 5;
    CHECK(!heap_alloc(h, SIZE_MAX - 1) && !heap_alloc(h, SIZE_MAX) && !heap_alloc(h, JS_HEAP_MAX_REQUEST + 1), "a huge request passed");
    h->limit = (size_t)-1;
    while (m.n)
        model_free(&m, 0);
    heap_model_finish(&m);
    ok("limit fails at the byte, is not retroactive, huge requests fail");
}

/* The footprint budget stops growth only, is never passed, and is not
   retroactive. */
static void scenario_budget(void)
{
    HeapModel m;
    JSHeap *h;
    void *a;
    size_t fp;

    heap_model_start(&m);
    h = m.h;
    /* a budget below the footprint (the first chunk) keeps it, and refuses growth */
    h->footprint_limit = 1000;
    CHECK(h->footprint == 65536, "footprint");
    a = heap_alloc(h, 1000);
    CHECK(a, "an allocation that fits in the chunks failed under a small budget");
    CHECK(!heap_alloc(h, 100000), "growth under a budget below the footprint passed");
    CHECK(h->footprint == 65536 && vms[m.vm].n == 1, "the heap grew past a budget");
    heap_free(h, a);
    /* a budget of one more chunk: the chunk shrinks to what the block needs and fits */
    h->footprint_limit = h->footprint + 65536;
    CHECK(model_alloc(&m, 40000), "the first block that needs a chunk failed");
    CHECK(h->footprint <= h->footprint_limit, "the footprint passed the budget");
    while (model_alloc(&m, 30000))
        CHECK(h->footprint <= h->footprint_limit, "the footprint passed the budget");
    fp = h->footprint;
    CHECK(fp <= h->footprint_limit && vms[m.vm].refused == 0, "the budget was not what stopped it");
    CHECK(!heap_alloc(h, 70000) && h->footprint == fp, "the budget did not refuse a bigger chunk");
    h->footprint_limit = 0;
    CHECK((a = heap_alloc(h, 70000)) != NULL, "without a budget growth must work");
    heap_free(h, a);
    heap_model_check(&m);
    while (m.n)
        model_free(&m, 0);
    heap_model_finish(&m);
    ok("budget stops growth, is never passed, is not retroactive");
}

static void scenario_host_refuses(void)
{
    HeapModel m;
    JSHeap *h;
    void *a;
    size_t count, fp;

    heap_model_start(&m);
    h = m.h;
    count = h->count;
    fp = h->footprint;
    vms[m.vm].refuse_at = vms[m.vm].grows + 1;
    CHECK(!heap_alloc(h, 100000), "the host refused and the heap allocated");
    CHECK(h->count == count && h->footprint == fp && h->blocks == 0, "a refusal changed the heap");
    CHECK(vms[m.vm].refused == 1, "the host did not see the request");
    a = heap_alloc(h, 100000);
    CHECK(a, "the request after a refusal failed");
    heap_free(h, a);
    vms[m.vm].refuse_all = 1;
    CHECK(!heap_alloc(h, 3000000), "alloc with a host that refuses everything");
    vms[m.vm].refuse_all = 0;
    heap_model_finish(&m);
    ok("a refusal of the host leaves the heap as it was");
}

/* Sizes of chunks: 64 KiB first, then a quarter of the footprint up to 8 MiB,
   a block over half a chunk gets one of its own. */
static void scenario_chunks(void)
{
    HeapModel m;
    JSHeap *h;
    size_t prev_fp, want;
    int rounds = 0;

    heap_model_start(&m);
    h = m.h;
    /* fill the chunks with blocks of 30000 bytes and look at each new chunk */
    while (h->footprint < ((size_t)48 << 20) && rounds < 100000) {
        size_t before = h->footprint, nchunks = h->chunk_count;
        if (!model_alloc(&m, 30000)) {
            FAIL("alloc 30000 failed");
        }
        if (h->chunk_count != nchunks) {
            size_t got = h->footprint - before;
            want = (before / 4 + 65535) & ~(size_t)65535;
            if (want < 65536)
                want = 65536;
            if (want > ((size_t)8 << 20))
                want = (size_t)8 << 20;
            CHECK(got == want, "the chunk after a footprint of %zu is %zu, not %zu", before, got, want);
            CHECK(got % 65536 == 0, "chunk is not a multiple of 64 KiB");
        }
        rounds++;
    }
    CHECK(h->chunk_count > 8, "too few chunks");
    /* a block over half of the next chunk gets a chunk of its own, sized for it */
    prev_fp = h->footprint;
    {
        size_t need = (size_t)12 << 20;
        void *p = heap_alloc(h, need);
        size_t got = h->footprint - prev_fp;
        CHECK(p, "a block of 12 MiB failed");
        CHECK(got % 65536 == 0 && got >= need + need / 32 + 112 && got < need + need / 32 + 112 + 65536 + 64,
              "the chunk of its own is %zu for a block of %zu", got, need);
        heap_free(h, p);
    }
    while (m.n)
        model_free(&m, m.n - 1);
    heap_model_finish(&m);
    ok("chunk sizes follow the policy");
}

/* The limit of a resize at the byte, whichever way the block goes: the request
   fails with the limit one below count + request - the usable size of the old
   block, and passes with the limit at it. The numbers come from the rule of
   upstream, not from the heap. The block, its content and the count are as
   they were after the failure. Returns the new pointer. */
static uint8_t *realloc_at_the_byte(JSHeap *h, uint8_t *p, size_t size, const char *what)
{
    size_t old = heap_usable_size(p), count = h->count, blocks = h->blocks;
    size_t edge = count - old + size;
    uint8_t first = p[0], last = p[old - 1];
    uint8_t *q;

    h->limit = edge - 1;
    q = heap_realloc(h, p, size);
    CHECK(!q, "%s: a resize one byte over the limit passed", what);
    CHECK(h->count == count && h->blocks == blocks && heap_usable_size(p) == old && p[0] == first && p[old - 1] == last,
          "%s: a resize that failed changed the block or the count", what);
    h->limit = edge;
    q = heap_realloc(h, p, size);
    CHECK(q, "%s: a resize that ends at the limit failed", what);
    h->limit = (size_t)-1;
    return q;
}

static void scenario_resize_at_the_byte(void)
{
    HeapModel m;
    JSHeap *h;
    uint8_t *a, *b, *q;
    size_t count, fp, usable;

    heap_model_start(&m);
    h = m.h;

    /* in place, a move inside the chunks, a move to a new chunk, and the same
       size with the limit below the count */
    a = heap_alloc(h, 1000);
    memset(a, 0x55, 1000);
    q = realloc_at_the_byte(h, a, 3000, "growth in place");
    CHECK(q == a, "the growth at the byte of the limit moved");
    heap_free(h, q);
    a = heap_alloc(h, 1000);
    b = heap_alloc(h, 1000);
    memset(a, 0x55, 1000);
    q = realloc_at_the_byte(h, a, 20000, "move inside the chunks");
    CHECK(q != a, "the move at the byte of the limit stayed");
    heap_free(h, q);
    heap_free(h, b);
    a = heap_alloc(h, 40000);
    b = heap_alloc(h, 8000);
    memset(a, 0x55, 40000);
    fp = h->footprint;
    q = realloc_at_the_byte(h, a, 200000, "move to a new chunk");
    CHECK(h->footprint > fp, "the move to a new chunk mapped nothing");
    heap_free(h, q);
    heap_free(h, b);
    a = heap_alloc(h, 3000);
    memset(a, 0x55, 3000);
    count = h->count;
    usable = heap_usable_size(a);
    /* the same size with the limit just below the count: count - usable + usable = count */
    h->limit = count - 1;
    CHECK(!heap_realloc(h, a, usable), "a resize to the same size passed with the limit below the count");
    h->limit = count;
    q = heap_realloc(h, a, usable);
    CHECK(q == a, "a resize to the same size failed with the limit at the count");
    h->limit = (size_t)-1;
    heap_free(h, q);
    CHECK(h->blocks == 0 && h->count == m.rt_size, "count after the boundary resizes");
    heap_model_finish(&m);
    ok("a resize at the byte of the limit: in place, moved, moved to a new chunk, the same size");
}

static void scenario_resize(void)
{
    HeapModel m;
    JSHeap *h;
    uint8_t *a, *b, *c, *q;
    size_t count, fp, usable;

    heap_model_start(&m);
    h = m.h;

    /* in place: a block with a free neighbor grows where it is, and shrinks */
    a = heap_alloc(h, 1000);
    memset(a, 0x11, 1000);
    q = heap_realloc(h, a, 3000);
    CHECK(q == a, "a block with free room after it moved");
    CHECK(heap_usable_size(q) >= 3000, "usable size after growth");
    q = heap_realloc(h, q, 100);
    CHECK(q == a && heap_usable_size(q) < 3000, "a shrink moved or kept the size");
    heap_free(h, q);
    CHECK(h->blocks == 0 && h->count == m.rt_size, "count after in place resizes");

    /* a move inside the chunks: the neighbor is in use */
    a = heap_alloc(h, 1000);
    b = heap_alloc(h, 1000);
    memset(a, 0x22, 1000);
    fp = h->footprint;
    q = heap_realloc(h, a, 20000);
    CHECK(q && q != a, "growth against a used neighbor did not move");
    CHECK(h->footprint == fp, "a move that fits in the chunks mapped memory");
    for (int i = 0; i < 1000; i++)
        CHECK(q[i] == 0x22, "content lost in a move");
    heap_free(h, q);
    heap_free(h, b);

    /* a move that needs a new chunk, with the old block still in its chunk */
    c = heap_alloc(h, 40000);
    memset(c, 0x33, 40000);
    b = heap_alloc(h, 8000); /* in the way of c */
    fp = h->footprint;
    q = heap_realloc(h, c, 200000);
    CHECK(q && h->footprint > fp, "growth that needs a chunk did not map one");
    for (int i = 0; i < 40000; i++)
        CHECK(q[i] == 0x33, "content lost when the move needed a chunk");
    heap_free(h, q);
    heap_free(h, b);

    /* a refusal: budget or host, and the old block, content and count stay */
    c = heap_alloc(h, 40000);
    memset(c, 0x44, 40000);
    b = heap_alloc(h, 8000);
    count = h->count;
    fp = h->footprint;
    usable = heap_usable_size(c);
    h->footprint_limit = fp;
    q = heap_realloc(h, c, 3000000);
    CHECK(!q, "a move past the budget passed");
    h->footprint_limit = 0;
    CHECK(h->count == count && h->footprint == fp && heap_usable_size(c) == usable, "a refused move changed the heap");
    for (int i = 0; i < 40000; i++)
        CHECK(c[i] == 0x44, "content changed by a refused move");
    vms[m.vm].refuse_at = vms[m.vm].grows + 1;
    q = heap_realloc(h, c, 3000000);
    CHECK(!q && vms[m.vm].refused > 0, "a move past the host passed");
    CHECK(h->count == count && h->footprint == fp && heap_usable_size(c) == usable, "a move the host refused changed the heap");
    for (int i = 0; i < 40000; i++)
        CHECK(c[i] == 0x44, "content changed by a move the host refused");

    /* the count check is for the size after the move, not both copies: the rule
       of upstream, count + request - the usable size of the old block */
    h->limit = count - usable + 3000000 - 1;
    CHECK(!heap_realloc(h, c, 3000000), "a resize one byte over the limit passed");
    h->limit = count - usable + 3000000;
    q = heap_realloc(h, c, 3000000);
    CHECK(q, "a resize that ends exactly at the limit failed");
    h->limit = (size_t)-1;
    c = q;

    /* sizes the heap cannot serve */
    CHECK(!heap_realloc(h, c, SIZE_MAX) && !heap_realloc(h, c, SIZE_MAX - 7) && !heap_realloc(h, c, JS_HEAP_MAX_REQUEST + 1),
          "a huge resize passed");
    heap_free(h, c);
    heap_free(h, b);
    heap_model_finish(&m);
    ok("resize: in place, moved in the chunks, moved to a new chunk, refused with the old block intact");
}

static void scenario_release(void)
{
    HeapModel m;
    JSHeap *h;
    uint8_t *keep_blocks[50];
    size_t fp;
    ReleaseResult r;
    Releaser rl;

    heap_model_start(&m);
    h = m.h;
    rl.obj = h;
    rl.start = (void (*)(void *, size_t))heap_release_start;
    rl.chunk = (JSMemRange (*)(void *))heap_release_next_chunk;
    rl.range = (JSMemRange (*)(void *))heap_release_next_range;

    /* fill 20 MiB, free all but 50 blocks */
    while (h->footprint < ((size_t)20 << 20))
        CHECK(model_alloc(&m, 30000), "alloc");
    for (int i = 0; i < 50; i++)
        keep_blocks[i] = m.live[i * 5].p;
    for (int i = m.n - 1; i >= 0; i--) {
        int is_kept = 0;
        for (int k = 0; k < 50; k++)
            if (m.live[i].p == keep_blocks[k])
                is_kept = 1;
        if (!is_kept)
            model_free(&m, i);
    }
    heap_model_check(&m);
    fp = h->footprint;

    /* keeping more than is free offers nothing */
    r = host_release(m.vm, &rl, (size_t)1 << 40);
    CHECK(r.candidate == 0 && h->footprint == fp, "a release with a huge keep offered %zu", r.candidate);

    /* keeping a part offers part */
    r = host_release(m.vm, &rl, (size_t)1 << 20);
    heap_model_check(&m);
    CHECK(r.candidate > 0 && h->footprint <= fp, "a release with keep 1 MiB offered nothing");
    for (int i = 0; i < m.n; i++)
        check_content(&m.live[i], m.live[i].usable, "release with keep");

    /* keeping nothing offers everything: whole free chunks leave, and what is left is usable */
    r = host_release(m.vm, &rl, 0);
    heap_model_check(&m);
    for (int i = 0; i < m.n; i++)
        check_content(&m.live[i], m.live[i].usable, "release");
    /* the free space inside is wiped by the host: the heap must work on it all the same */
    for (int i = 0; i < 400; i++)
        CHECK(model_alloc(&m, 1 + (size_t)(i * 977) % 70000), "alloc after a release");
    heap_model_check(&m);
    for (int i = 0; i < m.n; i++)
        check_content(&m.live[i], m.live[i].usable, "after a release");
    (void)r;
    while (m.n)
        model_free(&m, m.n - 1);
    heap_model_finish(&m);
    ok("release: keep is respected, whole chunks leave, wiped free space is reusable");
}

/* ---- the release iteration at its edges ---- */

/* The free blocks of every pool of a heap, read with TLSF's own walker, and what
   each pool holds. What a release must offer is worked out from this, and not
   from the code that offers it. */
typedef struct PoolInfo {
    uintptr_t chunk; /* the address of the chunk, which is where the host mapped it */
    size_t size;
    int nfree, nused;
} PoolInfo;

typedef struct FreeSet {
    struct {
        uintptr_t ptr;
        size_t size;
        int pool;
    } b[8192];
    int n;
    PoolInfo pool[256];
    int npool;
} FreeSet;

static void free_walker(void *ptr, size_t size, int used, void *user)
{
    FreeSet *f = user;
    PoolInfo *pi = &f->pool[f->npool - 1];

    if (used) {
        pi->nused++;
        return;
    }
    pi->nfree++;
    CHECK(f->n < 8192, "too many free blocks");
    f->b[f->n].ptr = (uintptr_t)ptr;
    f->b[f->n].size = size;
    f->b[f->n].pool = f->npool - 1;
    f->n++;
}

static void free_set(HeapModel *m, FreeSet *f)
{
    JSHeap *h = m->h;

    f->n = 0;
    f->npool = 0;
    memset(&f->pool[0], 0, sizeof f->pool[0]);
    f->pool[f->npool].chunk = (uintptr_t)h->chunks;
    f->npool++;
    tlsf_walk_pool(tlsf_get_pool(h->tlsf), free_walker, f);
    for (int i = 0; i < vms[m->vm].n; i++) {
        uintptr_t a = vms[m->vm].chunk[i].addr;

        if (a == (uintptr_t)h->chunks)
            continue;
        memset(&f->pool[f->npool], 0, sizeof f->pool[0]);
        f->pool[f->npool].chunk = a;
        f->pool[f->npool].size = vms[m->vm].chunk[i].size;
        f->npool++;
        tlsf_walk_pool((void *)(a + 32), free_walker, f);
    }
}

/* the size class TLSF files a free block under, worked out from its rules:
   below 256 bytes, one class for each 8 bytes, above, 32 classes for each
   power of two */
static void tlsf_class_of(size_t size, int *fl, int *sl)
{
    if (size < 256) {
        *fl = 0;
        *sl = (int)(size / 8);
    } else {
        int lg = 63 - __builtin_clzl(size);
        *fl = lg - 7;
        *sl = (int)((size >> (lg - 5)) ^ 32);
    }
}

typedef struct Item {
    uintptr_t addr;
    size_t len;
    int chunk; /* 1 for a chunk, 0 for a range */
} Item;

static Item items[4096];

/* One release as the host does it, with every call written down. The host
   unmaps the chunks. The ranges are left alone for the caller to look at. */
static int run_release(HeapModel *m, size_t keep)
{
    JSHeap *h = m->h;
    HostVM *host = &vms[m->vm];
    JSMemRange r;
    int n = 0;

    heap_release_start(h, keep);
    while ((r = heap_release_next_chunk(h)).len) {
        int i = host_find(host, r.addr);

        CHECK(i >= 0 && host->chunk[i].size == r.len, "the heap gave back a chunk the host did not give");
        CHECK(n < 4096, "too many items");
        items[n].addr = r.addr;
        items[n].len = r.len;
        items[n++].chunk = 1;
        heap_unpoison((void *)r.addr, r.len);
        munmap((void *)r.addr, r.len);
        host->live -= r.len;
        host->chunk[i] = host->chunk[--host->n];
    }
    /* an empty answer stays empty */
    for (int k = 0; k < 3; k++)
        CHECK(heap_release_next_chunk(h).len == 0, "an empty NextChunk gave a chunk when asked again");
    while ((r = heap_release_next_range(h)).len) {
        CHECK(host_inside(host, r.addr, r.len), "the heap offered a range outside its chunks");
        CHECK(n < 4096, "too many items");
        items[n].addr = r.addr;
        items[n].len = r.len;
        items[n++].chunk = 0;
    }
    for (int k = 0; k < 3; k++)
        CHECK(heap_release_next_range(h).len == 0, "an empty NextRange gave a range when asked again");
    CHECK(heap_release_next_chunk(h).len == 0, "NextChunk gave a chunk after the ranges");
    return n;
}

/* What the host may do with the ranges: take every byte of them. The test
   wipes every byte, not only the pages inside, so that a byte of the ends of a
   free block that TLSF keeps, if a range reached it, would show. */
static void wipe_ranges(int n)
{
    for (int i = 0; i < n; i++) {
        if (items[i].chunk)
            continue;
        heap_unpoison((void *)items[i].addr, items[i].len);
        memset((void *)items[i].addr, 0xdd, items[i].len);
        heap_poison((void *)items[i].addr, items[i].len);
    }
}

/* Check what one release offered against the free blocks before it:
   - a chunk is a whole free pool other than the first, with its whole size
   - a range is the inside of a free block that is still there: 16 bytes after
     its start to 8 bytes before its end, 4096 bytes or more
   - every item once, chunks before ranges, chunks largest first, ranges from
     the largest size class down
   - nothing offered when keep is as much as the free blocks hold. Otherwise the
     offer reaches free - keep, unless nothing more can be offered, and the
     last item was needed to reach it */
static void check_release(const FreeSet *f, size_t keep, int n)
{
    size_t total = 0, offered = 0, target;
    int chunk_items = 0, range_items = 0, whole = 0, eligible = 0;
    int removed[256] = { 0 };
    int last_fl = 1 << 30, last_sl = 1 << 30;
    size_t last_chunk = SIZE_MAX;
    int seen_range = 0;

    for (int i = 0; i < f->n; i++)
        total += f->b[i].size;
    target = keep >= total ? 0 : total - keep;
    if (target == 0)
        CHECK(n == 0, "%d items were offered with nothing to offer", n);

    for (int i = 1; i < f->npool; i++)
        if (f->pool[i].nused == 0 && f->pool[i].nfree == 1)
            whole++;
    for (int k = 0; k < n; k++) {
        if (items[k].chunk) {
            int found = -1;

            CHECK(!seen_range, "a chunk came after a range");
            for (int i = 1; i < f->npool; i++)
                if (f->pool[i].chunk == items[k].addr)
                    found = i;
            CHECK(found > 0 && f->pool[found].nused == 0 && f->pool[found].nfree == 1 && !removed[found],
                  "a chunk was offered that is not a whole free pool, or twice");
            CHECK(items[k].len == f->pool[found].size, "a chunk was offered with a length other than its own");
            CHECK(items[k].len <= last_chunk, "chunks were not offered largest first");
            last_chunk = items[k].len;
            removed[found] = 1;
            chunk_items++;
        } else {
            int found = -1, fl, sl;

            seen_range = 1;
            for (int i = 0; i < f->n; i++)
                if (f->b[i].ptr + 16 == items[k].addr)
                    found = i;
            CHECK(found >= 0 && items[k].len == f->b[found].size - 24,
                  "a range is not the inside of a free block (16 bytes in, 8 before the end)");
            CHECK(!removed[f->b[found].pool], "a range was offered in a chunk that was given back");
            CHECK(items[k].len >= 4096, "a range under 4096 bytes was offered");
            tlsf_class_of(f->b[found].size, &fl, &sl);
            CHECK(fl < last_fl || (fl == last_fl && sl <= last_sl), "ranges were not offered from the largest size class down");
            last_fl = fl;
            last_sl = sl;
            for (int j = 0; j < k; j++)
                CHECK(items[j].chunk || items[j].addr != items[k].addr, "a range was offered twice");
            range_items++;
        }
        offered += items[k].len;
    }
    for (int i = 0; i < f->n; i++)
        if (!removed[f->b[i].pool] && f->b[i].size >= 4096 + 24)
            eligible++;
    if (range_items)
        CHECK(chunk_items == whole, "ranges were offered while %d of %d whole free chunks were kept", whole - chunk_items, whole);
    if (target) {
        CHECK(offered >= target || (chunk_items == whole && range_items == eligible),
              "the offer %zu stops short of %zu with more to offer", offered, target);
        if (n)
            CHECK(offered - items[n - 1].len < target, "the last item was not needed: %zu without it, %zu wanted", offered - items[n - 1].len, target);
    }
}

static void model_free_ptr(HeapModel *m, void *p)
{
    for (int i = 0; i < m->n; i++) {
        if (m->live[i].p == p) {
            model_free(m, i);
            return;
        }
    }
    FAIL("a block to free is not held");
}

static const size_t frag_sizes[] = {
    24, 100, 4096, 4112, 4120, 5000, 8191, 8192, 8200, 9000, 16384, 20000, 24, 33000, 65000, 130000,
    300000, 1000000, 3000000, 12000000,
};
#define N_FRAG (sizeof frag_sizes / sizeof frag_sizes[0])

/* free blocks of many sizes, each between two blocks in use so that they do not
   merge: the minimal block of 24 bytes (nothing inside it), the sizes around
   4096 + 24 (the smallest inside offered), sizes on both sides of the borders
   of the size classes, and a big one in a chunk of its own, which is a whole
   free pool once it is freed */
static void build_fragmented(HeapModel *m, int n_sizes)
{
    void *p[N_FRAG];

    for (int i = 0; i < n_sizes; i++) {
        CHECK(model_alloc(m, frag_sizes[i]), "alloc %zu", frag_sizes[i]);
        p[i] = m->live[m->n - 1].p;
        CHECK(model_alloc(m, 64), "guard");
    }
    for (int i = 0; i < n_sizes; i++)
        model_free_ptr(m, p[i]);
    heap_model_check(m);
}

static void scenario_release_exact(void)
{
    static FreeSet f;
    HeapModel m;
    JSHeap *h;
    size_t total;
    int n, n2;

    /* the heap with free blocks of every kind: a full release */
    heap_model_start(&m);
    h = m.h;
    build_fragmented(&m, N_FRAG);
    free_set(&m, &f);
    CHECK(f.npool > 2, "the test heap has %d pools", f.npool);
    {
        int min_block = 0, small = 0, whole = 0, big = 0;

        for (int i = 0; i < f.n; i++) {
            min_block += f.b[i].size == 24;
            small += f.b[i].size > 24 && f.b[i].size - 24 < 4096;
            big += f.b[i].size - 24 >= 4096;
        }
        for (int i = 1; i < f.npool; i++)
            whole += f.pool[i].nused == 0 && f.pool[i].nfree == 1;
        CHECK(min_block >= 1 && small >= 1 && big >= 8 && whole >= 1,
              "the test heap lacks a kind of block: %d minimal, %d small, %d big, %d whole chunks", min_block, small, big, whole);
    }
    n = run_release(&m, 0);
    check_release(&f, 0, n);
    CHECK(n > 8, "a release of everything offered %d items", n);
    wipe_ranges(n);
    heap_model_check(&m);
    for (int i = 0; i < m.n; i++)
        check_content(&m.live[i], m.live[i].usable, "after a wiped release");
    /* the heap works on the wiped space: splits, merges and reuse */
    for (int i = 0; i < 300; i++)
        CHECK(model_alloc(&m, 1 + (size_t)(i * 1237) % 40000), "alloc after the wipe");
    for (int i = 0; i < 150; i++)
        model_free(&m, m.n - 1);
    heap_model_check(&m);

    /* a second release of what is left: the pools with blocks in use stay */
    free_set(&m, &f);
    n2 = run_release(&m, 0);
    check_release(&f, 0, n2);
    wipe_ranges(n2);
    heap_model_check(&m);
    for (int i = 0; i < m.n; i++)
        check_content(&m.live[i], m.live[i].usable, "after a second release");

    /* nothing in use: every pool but the first is a whole free pool and goes,
       then only the first pool is left and a release gives ranges of it only */
    while (m.n)
        model_free(&m, m.n - 1);
    free_set(&m, &f);
    CHECK(f.npool > 1, "no pool but the first is left to give back");
    n2 = run_release(&m, 0);
    check_release(&f, 0, n2);
    CHECK(h->chunk_count == 1 && h->footprint == 65536, "%zu chunks of %zu bytes are left after giving everything back", h->chunk_count, h->footprint);
    wipe_ranges(n2);
    free_set(&m, &f);
    CHECK(f.npool == 1, "%d pools are left", f.npool);
    n2 = run_release(&m, 0);
    check_release(&f, 0, n2);
    for (int i = 0; i < n2; i++)
        CHECK(!items[i].chunk, "a chunk came from a heap with only its first pool");
    CHECK(n2 >= 1, "the first pool gave no range");
    wipe_ranges(n2);
    heap_model_check(&m);
    heap_model_finish(&m);

    /* keep: nothing offered when it covers the free blocks, and the offer stops
       at free - keep (to the block) otherwise, on a new heap each time */
    heap_model_start(&m);
    build_fragmented(&m, N_FRAG);
    free_set(&m, &f);
    total = 0;
    for (int i = 0; i < f.n; i++)
        total += f.b[i].size;
    while (m.n)
        model_free(&m, m.n - 1);
    heap_model_finish(&m);
    {
        size_t keeps[] = { 0, 1, total / 3, total / 2, total - 5000000, total - 1, total, total + 1, (size_t)1 << 40, SIZE_MAX };

        for (size_t k = 0; k < sizeof keeps / sizeof keeps[0]; k++) {
            heap_model_start(&m);
            build_fragmented(&m, N_FRAG);
            free_set(&m, &f);
            n = run_release(&m, keeps[k]);
            check_release(&f, keeps[k], n);
            wipe_ranges(n);
            heap_model_check(&m);
            while (m.n)
                model_free(&m, m.n - 1);
            heap_model_finish(&m);
        }
    }

    /* only the first pool: free blocks in it and nothing else */
    heap_model_start(&m);
    {
        void *p1, *p2, *p3;

        CHECK(model_alloc(&m, 20000), "alloc");
        p1 = m.live[m.n - 1].p;
        CHECK(model_alloc(&m, 64), "guard");
        CHECK(model_alloc(&m, 8200), "alloc");
        p2 = m.live[m.n - 1].p;
        CHECK(model_alloc(&m, 64), "guard");
        CHECK(model_alloc(&m, 9000), "alloc");
        p3 = m.live[m.n - 1].p;
        CHECK(model_alloc(&m, 64), "guard");
        model_free_ptr(&m, p1);
        model_free_ptr(&m, p2);
        model_free_ptr(&m, p3);
    }
    free_set(&m, &f);
    CHECK(f.npool == 1 && f.n == 4, "the first pool holds %d free blocks in %d pools", f.n, f.npool);
    n = run_release(&m, 0);
    check_release(&f, 0, n);
    CHECK(n == 4, "%d ranges for the 4 free blocks of the first pool", n);
    wipe_ranges(n);
    heap_model_check(&m);
    while (m.n)
        model_free(&m, m.n - 1);
    heap_model_finish(&m);
    (void)h;
    ok("release at its edges: whole ranges wiped, the offer against the free blocks, keep, empty answers");
}

static void scenarios(void)
{
    scenario_limit();
    scenario_budget();
    scenario_host_refuses();
    scenario_chunks();
    scenario_resize();
    scenario_resize_at_the_byte();
    scenario_release();
    scenario_release_exact();
}

#ifdef HEAP_FUZZ
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size)
{
    Src r;

    if (size < 2)
        return 0;
    memset(&r, 0, sizeof r);
    r.fuzz = 1;
    r.data = data + 1;
    r.len = size - 1;
    run_heap(&r, 4000);
    return 0;
}
#else
int main(int argc, char **argv)
{
    if (argc >= 2 && !strcmp(argv[1], "scenarios")) {
        scenarios();
        return 0;
    }
    if (argc == 4 && !strcmp(argv[1], "stress")) {
        Src r;
        uint64_t seed = strtoull(argv[2], NULL, 10);
        long steps = atol(argv[3]);

        memset(&r, 0, sizeof r);
        r.s = seed * 0x9E3779B97F4A7C15ull + 88172645463325252ull;
        run_heap(&r, steps);
        printf("PASS  %s seed %" PRIu64 " steps %ld\n", argv[1], seed, steps);
        return 0;
    }
    if (argc >= 3 && !strcmp(argv[1], "fuzz")) {
        for (int i = 2; i < argc; i++) {
            FILE *f = fopen(argv[i], "rb");
            uint8_t *buf;
            long n;
            Src r;
            if (!f) {
                perror(argv[i]);
                return 2;
            }
            fseek(f, 0, SEEK_END);
            n = ftell(f);
            fseek(f, 0, SEEK_SET);
            buf = malloc(n + 1);
            if (fread(buf, 1, n, f) != (size_t)n)
                return 2;
            fclose(f);
            memset(&r, 0, sizeof r);
            r.fuzz = 1;
            r.data = buf + 1;
            r.len = n > 0 ? n - 1 : 0;
            run_heap(&r, 4000);
            free(buf);
        }
        return 0;
    }
    fprintf(stderr, "usage: heap-test scenarios | stress SEED STEPS | fuzz FILE...\n");
    return 2;
}
#endif
