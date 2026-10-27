/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Tests of the allocator: heap.c and, with HEAP_TEST_RUNTIME, the small block
   layer and the front cache of quickjs.c.

     heap-test scenarios          the cases of limit, budget, resize, host
                                  refusal, release and retry
     heap-test stress SEED STEPS  random sequences, checked as they go
     heap-test stress-runtime SEED STEPS
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
#ifdef HEAP_TEST_RUNTIME
#include "quickjs.h"
#endif

/* ---- the fake host ---- */

/* The engine reads its stop flags through this (poll_flag in cutils.h).
   Nothing here asks a runtime to stop. */
int host_poll(const volatile int *flag)
{
    return *flag;
}

/* The runtime asks the wall clock for Date. Any time will do here. */
int64_t host_now(uint32_t vm, int kind)
{
    (void)vm;
    (void)kind;
    return (int64_t)1767225600 * 1000000000;
}

/* The bytes of stack left, which the runtime checks at each call. The scripts
   here do not recurse: say there is plenty. */
long __shadow_remaining(void)
{
    return 1L << 30;
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
    long releases;    /* dedicated chunks the heap gave back */
    long remaps;      /* requests to remap a chunk */
    long refuse_remap_at; /* the remap with this number is refused (1 is the first), 0 for none */
    int move_remap;   /* a remap maps a new chunk, copies and unmaps the old one: the chunk always moves (see next_move) */
    long refuse_at;   /* the request with this number is refused (1 is the first), 0 for none */
    int refuse_all;   /* every request is refused */
    size_t most;      /* the most bytes it ever held */
} HostVM;

static HostVM vms[MAX_VM];
/* dedicated chunks the heaps of this run gave back: a run that makes none tested nothing of them */
static long total_releases, total_remaps;
/* requests the cache of large blocks served in the runs on the runtime, so that a run can tell it exercised the cache */
static long total_large_hits;

/* How the fake host remaps: with mremap, which moves a chunk only when it must
   (linux), or by mapping a new chunk, copying and unmapping the old one, which
   always moves it and is the only way where there is no mremap. The heaps of
   one run take turns on linux, so that a run goes both ways. */
static int move_turn;

static int next_move(void)
{
#ifdef __linux__
    return move_turn++ & 1;
#else
    return 1;
#endif
}

static uint32_t host_open(void)
{
    for (uint32_t i = 0; i < MAX_VM; i++) {
        if (!vms[i].open) {
            memset(&vms[i], 0, sizeof vms[i]);
            vms[i].open = 1;
            vms[i].move_remap = next_move();
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
    total_releases += h->releases;
    total_remaps += h->remaps;
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

/* The remap that always moves: a new mapping while the old one is still there,
   so at another address, the content copied, and the old one unmapped, so that
   a heap that goes on with the old address finds nothing there. */
static void *host_remap_by_moving(uintptr_t addr, size_t old, size_t size)
{
    void *p = mmap(NULL, size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON, -1, 0);

    if (p == MAP_FAILED)
        return p;
    heap_unpoison(p, size);
    /* the heap poisoned the room after its block: the host reads it all */
    heap_unpoison((void *)addr, old);
    memcpy(p, (void *)addr, old);
    munmap((void *)addr, old);
    return p;
}

static void *host_remap_by_system(uintptr_t addr, size_t old, size_t size)
{
#ifdef __linux__
    return mremap((void *)addr, old, size, MREMAP_MAYMOVE);
#else
    (void)addr;
    (void)old;
    (void)size;
    return MAP_FAILED;
#endif
}

/* host_call: the heap gives back a dedicated chunk (HOST_CALL_MEMORY_RELEASE),
   and abort, which a failed assert of the heap calls (the test ends there). */
JSValue host_call(uint32_t vm, const JSValue *this_val, int argc, JSValueConst *argv, int magic)
{
    if (this_val) {
        fprintf(stderr, "heap-test: the heap passed a this value to host_call\n");
        abort();
    }
    if (magic == HOST_CALL_MEMORY_RELEASE && argc == 2) {
        HostVM *h = &vms[vm];
        uintptr_t addr = HOST_WORD_VALUE(argv[0]);
        size_t size = HOST_WORD_VALUE(argv[1]);
        int i = host_find(h, addr);

        if (i < 0 || h->chunk[i].size != size) {
            fprintf(stderr, "heap-test: the heap gave back a chunk the host did not give, or not whole\n");
            abort();
        }
        /* the chunk is unmapped: a heap that reads it after this call is caught */
        heap_unpoison((void *)addr, size);
        munmap((void *)addr, size);
        h->live -= size;
        h->chunk[i] = h->chunk[--h->n];
        h->releases++;
        return JS_UNDEFINED;
    }
    if (magic == HOST_CALL_MEMORY_REMAP && argc == 3) {
        HostVM *h = &vms[vm];
        uintptr_t addr = HOST_WORD_VALUE(argv[0]);
        size_t old = HOST_WORD_VALUE(argv[1]), size = HOST_WORD_VALUE(argv[2]);
        int i = host_find(h, addr);
        void *p;

        if (i < 0 || h->chunk[i].size != old || size <= old || size % 65536) {
            fprintf(stderr, "heap-test: the heap remapped a chunk the host did not give, or not whole, or to a size that is not bigger\n");
            abort();
        }
        h->remaps++;
        if (h->refuse_remap_at == h->remaps) {
            h->refused++;
            return HOST_WORD(0);
        }
        p = h->move_remap ? host_remap_by_moving(addr, old, size) : host_remap_by_system(addr, old, size);
        if (p == MAP_FAILED) {
            h->refused++;
            return HOST_WORD(0);
        }
        heap_unpoison(p, size);
        memset((char *)p + old, 0xcd, size - old < ((size_t)1 << 20) ? size - old : ((size_t)1 << 20));
        h->chunk[i].addr = (uintptr_t)p;
        h->chunk[i].size = size;
        h->live += size - old;
        if (h->live > h->most)
            h->most = h->live;
        return HOST_WORD(p);
    }
    fprintf(stderr, "heap-test: the heap called abort (host_call %d)\n", magic);
    fflush(stderr);
    _exit(134);
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

/* the three calls of a release on the heap, with the type Releaser wants */
static void heap_start(void *h, size_t keep) { heap_release_start(h, keep); }
static JSMemRange heap_chunk(void *h) { return heap_release_next_chunk(h); }
static JSMemRange heap_range(void *h) { return heap_release_next_range(h); }

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
    unsigned char large; /* runtime level: a block the heap holds on its own */
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

/* A request the heap refuses for its size alone: past the maximum. A dedicated
   chunk is not a pool of TLSF, so nothing else limits it. */
#define TOO_BIG(sz) ((sz) > JS_HEAP_MAX_REQUEST)

/* ---- what a growth costs, from docs/allocator.md and not from heap.c ----

   The rules the model holds the heap to when a footprint budget leaves little
   room. A chunk is a multiple of 64 KiB. The next chunk the policy takes is a
   quarter of the footprint, 64 KiB to 8 MiB. A request of more than half of it
   (less the header of 32 bytes and what a pool of TLSF needs) gets a chunk of its
   own, the dedicated chunk: the header, the size word of the block, the block
   to 8 bytes and an end marker (48 bytes in all). Otherwise the chunk is
   shared, and near the budget it shrinks to the request, with the 1/32 TLSF may
   round it up by, 64 bytes, the header and the overhead of a pool. */
#define T_UNIT ((size_t)65536)
#define T_CHUNK_HEADER ((size_t)32)
#define T_DEDICATED_OVERHEAD ((size_t)48)

static size_t round_unit(size_t n)
{
    return (n + T_UNIT - 1) / T_UNIT * T_UNIT;
}

/* the chunk the policy takes for a footprint */
static size_t policy_chunk(size_t footprint)
{
    size_t chunk = round_unit(footprint / 4);

    if (chunk < T_UNIT)
        chunk = T_UNIT;
    return chunk > ((size_t)8 << 20) ? ((size_t)8 << 20) : chunk;
}

/* does a request of need bytes get a chunk of its own when the footprint is fp */
static int gets_own_chunk(size_t fp, size_t need)
{
    return need + need / 32 + 64 > (policy_chunk(fp) - (T_CHUNK_HEADER + tlsf_pool_overhead())) / 2;
}

/* the dedicated chunk of a block of need bytes with no room to grow */
static size_t own_chunk(size_t need)
{
    return round_unit(((need + 7) & ~(size_t)7) + T_DEDICATED_OVERHEAD);
}

/* The least a request that finds no room in the chunks adds to the footprint. A
   request the budget does not leave this much for is refused, to the byte. */
static size_t min_growth(size_t fp, size_t need)
{
    if (gets_own_chunk(fp, need))
        return own_chunk(need);
    return round_unit(need + need / 32 + 64 + T_CHUNK_HEADER + tlsf_pool_overhead());
}

/* Whether the budget alone refuses a resize to sz of a block that lives in a
   dedicated chunk of the size chunk (0 for a block of the shared chunks), when
   the footprint is fp and the host can remap or not. A block that stays in its
   chunk needs nothing, one that outgrows it and can be remapped needs the
   growth of the chunk alone, and any other needs a new block. */
static int budget_refuses_resize(size_t budget, size_t fp, size_t chunk, size_t sz, int remap)
{
    if (!budget)
        return 0;
    if (chunk) {
        size_t room = chunk - T_DEDICATED_OVERHEAD;

        if (sz <= room && sz >= room / 2)
            return 0;
        if (sz > room && remap)
            return fp + own_chunk(sz) - chunk > budget;
    }
    return fp + min_growth(fp, sz) > budget;
}

#define FAIL(...) do { fprintf(stderr, "heap-test: " __VA_ARGS__); fprintf(stderr, "\n"); abort(); } while (0)
#define CHECK(c, ...) do { if (!(c)) FAIL(__VA_ARGS__); } while (0)

/* The block the test holds that is the biggest, or -1 when none has 1 MiB. A
   resize of it to a bigger size is how a run grows a dedicated block, which the
   mix of sizes does by chance only now and then. */
static int biggest_block(const Block *live, int n)
{
    int best = -1;

    for (int i = 0; i < n; i++)
        if (live[i].usable >= ((size_t)1 << 20) && (best < 0 || live[i].usable > live[best].usable))
            best = i;
    return best;
}

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

/* The heaps of one run tell their host they can remap by turns, so that a run
   goes both ways. A random run sets the turn from its seed, or from its input. */
static int remap_turn;

static int next_remap(void)
{
    return remap_turn++ & 1;
}

static void heap_model_start(HeapModel *m)
{
    memset(m, 0, sizeof *m);
    m->vm = host_open();
    m->h = heap_create(m->vm, HEAP_RT_SIZE);
    CHECK(m->h, "heap_create failed");
    heap_set_remap(m->h, next_remap());
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
        int stopped = h->footprint_limit != 0 && fp + min_growth(fp, sz) > h->footprint_limit;
        int explained = sz == 0 || TOO_BIG(sz) || over || host->refused != refused || stopped;
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
    size_t chunk = heap_dedicated_size(h, b->p), budget = h->footprint_limit;
    int remap = h->remap;
    uint32_t old_seed = b->seed;
    long refused = host->refused;
    uint8_t *p;

    p = heap_realloc(h, b->p, sz);
    model_note_footprint(m);
    if (!p) {
        /* the rule of upstream: count + request - the usable size of the old block > limit */
        int over = h->limit != (size_t)-1 && (unsigned __int128)count + sz - old > h->limit;
        int stopped = budget_refuses_resize(budget, fp, chunk, sz, remap);
        int explained = sz == 0 || TOO_BIG(sz) || over || host->refused != refused || stopped;
        CHECK(explained, "heap_realloc(%zu) of a block of %zu in a chunk of %zu failed for no reason (footprint %zu, budget %zu)", sz, old, chunk, fp, budget);
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
    Releaser rl = { h, heap_start, heap_chunk, heap_range };
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
        int i = (int)src_below(r, m->n), big = src_below(r, 6) == 0 ? biggest_block(m->live, m->n) : -1;
        if (big >= 0) { /* grow the biggest block, up to 64 MiB */
            i = big;
            sz = m->live[big].usable + 1 + src_below(r, m->live[big].usable);
            if (sz > ((size_t)64 << 20))
                sz = (size_t)64 << 20;
        }
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
    } else if (src_below(r, 3) == 0) {
        /* the host refuses the next remap, or the one after */
        vms[m->vm].refuse_remap_at = vms[m->vm].remaps + 1 + (long)src_below(r, 2);
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

#ifdef HEAP_TEST_RUNTIME
/* ---- the runtime level: the small blocks, the front cache and the retry ---- */

/* A request of 505 bytes or more is a large block, which goes to the heap on
   its own. A smaller one is a slot of an arena, and what it asks of the heap
   is a new arena of 4096 bytes at most. */
#define LARGE_FROM 505
#define ARENA_SIZE 4096

typedef struct RtModel {
    uint32_t vm;
    JSRuntime *rt;
    Block live[MAX_LIVE];
    int n;
    Span spans[MAX_LIVE];
    int nspans;
    size_t bytes;
    uint32_t seed;
    size_t limit;     /* the memory limit set now, (size_t)-1 for none */
    size_t fp_limit;  /* the footprint budget set now, 0 for none */
    int front;        /* the most blocks of a list of the front cache */
    size_t large_hdr; /* what a large block costs on top of its usable size and the 8 byte word of the heap */
} RtModel;

/* How many freed blocks the two caches hold. A request that fails at the heap
   gives all of them back, and so do a collection, a release and a change of the
   caches. */
static size_t rt_cached(JSRuntime *rt)
{
    return JS_FrontCacheBlocks(rt) + JS_LargeCacheBlocks(rt);
}

/* What a request of sz bytes asks of the heap: a large block of 505 to 4096
   bytes is rounded up to a multiple of 64 while the cache is on. */
static size_t rt_large_request(const RtModel *m, size_t sz)
{
    return m->front && sz >= LARGE_FROM && sz <= 4096 ? (sz + 63) / 64 * 64 : sz;
}

static size_t rt_footprint(JSRuntime *rt)
{
    JSMemoryUsage u;

    JS_ComputeMemoryUsage(rt, &u);
    return (size_t)u.footprint;
}

static void rt_check(RtModel *m)
{
    const char *msg = JS_CheckMemory(m->rt);
    int nonzero = 0;

    CHECK(!msg, "JS_CheckMemory: %s", msg);
    for (int i = 0; i < m->n; i++) {
        Block *b = &m->live[i];
        CHECK((uintptr_t)b->p % 8 == 0, "a block is not 8 byte aligned");
        CHECK(js_malloc_usable_size_rt(m->rt, b->p) == b->usable, "usable size of a block changed");
        nonzero += b->usable != 0;
    }
    CHECK(m->nspans == nonzero, "the set of spans holds %d blocks, the test %d", m->nspans, nonzero);
    for (int i = 0; i + 1 < m->nspans; i++)
        CHECK(m->spans[i].hi <= m->spans[i + 1].lo, "two blocks overlap");
    CHECK(JS_FrontCacheBlocks(m->rt) <= (size_t)m->front * 32, "the front cache holds more than %d blocks a list", m->front);
    CHECK(m->front || JS_LargeCacheBlocks(m->rt) == 0, "the large cache holds blocks while the caches are off");
}

/* Why a request the runtime refused may be refused, from the state after the
   call and not from what happened before: a size past the maximum, the host
   refusing a request during the call, the footprint budget set now (the least
   a growth takes is min_growth, and a large block that grows asks the rule of
   budget_refuses_resize, with the dedicated chunk it lives in, chunk, 0 for
   none), or the memory limit set now, which the request (an arena, or the
   block and its header) would pass. The front cache is given back when a
   request fails, so count is the one after the call. */
static int rt_refusal_explained(RtModel *m, size_t sz, size_t count, long refused, int large_growth, size_t chunk)
{
    size_t request = sz < LARGE_FROM ? ARENA_SIZE : rt_large_request(m, sz) + m->large_hdr;
    size_t fp = rt_footprint(m->rt);

    if (sz > JS_HEAP_MAX_REQUEST - m->large_hdr || vms[m->vm].refused != refused)
        return 1;
    if (large_growth) {
        /* the heap is asked to resize the block and its header */
        if (budget_refuses_resize(m->fp_limit, fp, chunk, sz + m->large_hdr, ((const JSHeap *)m->rt)->remap))
            return 1;
    } else if (m->fp_limit && fp + min_growth(fp, request) > m->fp_limit) {
        return 1;
    }
    return m->limit != (size_t)-1 && (unsigned __int128)count + request > m->limit;
}

/* A request the runtime takes to the heap (and so gives the front cache back
   for, once, when the heap fails it): not the size 0 and not a size that is
   refused before the heap. */
static int rt_reaches_heap(const RtModel *m, size_t sz)
{
    return sz != 0 && sz <= JS_HEAP_MAX_REQUEST - m->large_hdr;
}

/* Whether the memory limit alone decides a request for a large block: the
   caches are empty (nothing is served from them, nothing is given back and
   tried again), no budget,
   no refusal of the host (of a chunk or of a remap) is waiting. Then the rule of upstream gives the
   verdict to the byte. */
static int rt_limit_alone(RtModel *m)
{
    return rt_cached(m->rt) == 0 && !m->fp_limit && vms[m->vm].refuse_at <= vms[m->vm].grows &&
        vms[m->vm].refuse_remap_at <= vms[m->vm].remaps;
}

static int rt_alloc(RtModel *m, size_t sz)
{
    size_t count = JS_GetMemorySize(m->rt), cached = rt_cached(m->rt), large_cached = JS_LargeCacheBlocks(m->rt);
    size_t hits = JS_LargeCacheHits(m->rt), hit;
    int alone = rt_limit_alone(m) && sz >= LARGE_FROM && sz <= JS_HEAP_MAX_REQUEST - m->large_hdr;
    long refused = vms[m->vm].refused;
    uint8_t *p = js_malloc_rt(m->rt, sz);
    Block *b;

    /* a request the cache served took one block from it */
    hit = JS_LargeCacheHits(m->rt) - hits;
    CHECK(hit == 0 || (p && hit == 1 && JS_LargeCacheBlocks(m->rt) == large_cached - 1),
          "js_malloc_rt(%zu): %zu hits of the cache of large blocks, which held %zu blocks and holds %zu", sz, hit,
          large_cached, JS_LargeCacheBlocks(m->rt));

    if (alone) {
        /* count + request > limit, request = the block and its header */
        int over = m->limit != (size_t)-1 && (unsigned __int128)count + rt_large_request(m, sz) + m->large_hdr > m->limit;
        CHECK((p == NULL) == (over != 0), "js_malloc_rt(%zu) %s with count %zu and limit %zu", sz, p ? "passed" : "failed", count, m->limit);
    }
    if (!p) {
        CHECK(rt_refusal_explained(m, sz, JS_GetMemorySize(m->rt), refused, 0, 0),
              "js_malloc_rt(%zu) failed for no reason (count %zu, limit %zu, footprint %zu, budget %zu, refused %ld)", sz,
              JS_GetMemorySize(m->rt), m->limit, rt_footprint(m->rt), m->fp_limit, vms[m->vm].refused - refused);
        if (alone)
            CHECK(JS_GetMemorySize(m->rt) == count, "a refused large block changed the count");
        /* the heap failed it, the cache was given back and it was tried again */
        if (cached && rt_reaches_heap(m, sz))
            CHECK(rt_cached(m->rt) == 0, "js_malloc_rt(%zu) failed and left %zu blocks in the caches", sz, rt_cached(m->rt));
        return 0;
    }
    CHECK(sz <= JS_HEAP_MAX_REQUEST - 8, "js_malloc_rt(%zu) succeeded", sz);
    CHECK((uintptr_t)p % 8 == 0, "block not aligned");
    CHECK(JS_LargeCacheBlocks(m->rt) <= large_cached, "js_malloc_rt(%zu) put a block in the large cache", sz);
    total_large_hits += hit;
    b = &m->live[m->n++];
    b->p = p;
    b->usable = js_malloc_usable_size_rt(m->rt, p);
    b->large = sz >= LARGE_FROM;
    CHECK(b->usable >= sz, "usable size %zu is below the request %zu", b->usable, sz);
    CHECK(sz != 0 || b->usable == 0, "the block of size 0 has a usable size");
    if (alone)
        CHECK(JS_GetMemorySize(m->rt) == count + b->usable + m->large_hdr + 8, "a large block costs %zu, not its usable size, its header and 8",
              JS_GetMemorySize(m->rt) - count);
    if (b->usable)
        span_add(m->spans, &m->nspans, (uintptr_t)p, (uintptr_t)p + b->usable);
    b->seed = ++m->seed;
    fill(b);
    m->bytes += b->usable;
    return 1;
}

static void rt_free(RtModel *m, int i)
{
    Block *b = &m->live[i];

    check_content(b, b->usable, "js_free_rt");
    if (b->usable)
        span_remove(m->spans, &m->nspans, (uintptr_t)b->p);
    js_free_rt(m->rt, b->p);
    m->bytes -= b->usable;
    m->live[i] = m->live[--m->n];
}

static void rt_realloc(RtModel *m, int i, size_t sz)
{
    Block *b = &m->live[i];
    size_t old = b->usable, count = JS_GetMemorySize(m->rt), cached = rt_cached(m->rt);
    uint32_t old_seed = b->seed;
    long refused = vms[m->vm].refused;
    /* a large block resized to a large size or a small one stays one block of
       the heap: its rule is count + the new request - its usable size */
    int alone = rt_limit_alone(m) && b->large && old != 0 && sz != 0 && sz <= JS_HEAP_MAX_REQUEST - m->large_hdr;
    /* a large block that grows: in a dedicated chunk, the budget may be
       asked for the growth of the chunk alone (budget_refuses_resize) */
    int large_growth = b->large && sz > old;
    size_t chunk = b->large ? heap_dedicated_size((const JSHeap *)m->rt, (const uint8_t *)b->p - m->large_hdr) : 0;
    uint8_t *p;

    p = js_realloc_rt(m->rt, b->p, sz);
    if (alone) {
        int over = m->limit != (size_t)-1 && (unsigned __int128)count + sz - old > m->limit;
        CHECK((p == NULL) == (over != 0), "js_realloc_rt(%zu) of a block of %zu %s with count %zu and limit %zu", sz, old, p ? "passed" : "failed", count, m->limit);
    }
    if (!p) {
        if (sz == 0) {
            /* realloc(ptr, 0) frees */
            if (old)
                span_remove(m->spans, &m->nspans, (uintptr_t)b->p);
            m->bytes -= old;
            m->live[i] = m->live[--m->n];
            return;
        }
        CHECK(rt_refusal_explained(m, sz, JS_GetMemorySize(m->rt), refused, large_growth, chunk),
              "js_realloc_rt(%zu) of a block of %zu failed for no reason (count %zu, limit %zu, footprint %zu, budget %zu, refused %ld)", sz, old,
              JS_GetMemorySize(m->rt), m->limit, rt_footprint(m->rt), m->fp_limit, vms[m->vm].refused - refused);
        CHECK(js_malloc_usable_size_rt(m->rt, b->p) == old, "a failed js_realloc_rt changed the block");
        check_content(b, old, "failed js_realloc_rt");
        if (alone)
            CHECK(JS_GetMemorySize(m->rt) == count, "a refused resize changed the count");
        /* a block that fits its slot is not a request to the heap, and a large block is one at any size */
        if (cached && rt_reaches_heap(m, sz) && (b->large || sz > old))
            CHECK(rt_cached(m->rt) == 0, "js_realloc_rt(%zu) failed and left %zu blocks in the caches", sz, rt_cached(m->rt));
        return;
    }
    CHECK(sz <= JS_HEAP_MAX_REQUEST - 8, "js_realloc_rt(%zu) succeeded", sz);
    CHECK((uintptr_t)p % 8 == 0, "block not aligned");
    {
        Block was = { p, old, old_seed };
        check_content(&was, old < sz ? old : sz, "js_realloc_rt");
    }
    if (old)
        span_remove(m->spans, &m->nspans, (uintptr_t)b->p);
    /* a large block stays one. A small one stays in its slot when the new size
       fits it, and is a new block when not */
    b->large = b->large ? 1 : p != b->p ? sz >= LARGE_FROM : 0;
    b->p = p;
    b->usable = js_malloc_usable_size_rt(m->rt, p);
    CHECK(b->usable >= sz, "usable size below the request");
    if (alone)
        CHECK(JS_GetMemorySize(m->rt) == count - old + b->usable, "a large block resized: the count changed by %zd, the usable size by %zd",
              (ssize_t)(JS_GetMemorySize(m->rt) - count), (ssize_t)(b->usable - old));
    span_add(m->spans, &m->nspans, (uintptr_t)p, (uintptr_t)p + b->usable);
    m->bytes += b->usable - old;
    b->seed = ++m->seed;
    fill(b);
}

static void rt_start(void *rt, size_t keep) { JS_ReleaseStart(rt, keep); }
static JSMemRange rt_chunk(void *rt) { return JS_ReleaseNextChunk(rt); }
static JSMemRange rt_range(void *rt) { return JS_ReleaseNextRange(rt); }

static void rt_release(RtModel *m, size_t keep)
{
    Releaser rl = { m->rt, rt_start, rt_chunk, rt_range };
    size_t size, cached = rt_cached(m->rt);
    ReleaseResult r;

    /* a release gives the caches back first, which can change MemorySize
       only by what they held: the rest of it must not change */
    size = JS_GetMemorySize(m->rt);
    r = host_release(m->vm, &rl, keep);
    CHECK(rt_cached(m->rt) == 0, "the caches hold %zu blocks after a release", rt_cached(m->rt));
    CHECK(JS_GetMemorySize(m->rt) <= size && (cached || JS_GetMemorySize(m->rt) == size), "a release changed MemorySize from %zu to %zu with %zu blocks cached",
          size, JS_GetMemorySize(m->rt), cached);
    rt_check(m);
    for (int i = 0; i < m->n; i += m->n / 100 + 1)
        check_content(&m->live[i], m->live[i].usable, "release");
    (void)r;
}

/* A collection of the objects gives the caches back and takes nothing
   else. The runtime of this test holds no objects, so it is the caches alone. */
static void rt_gc(RtModel *m, int young)
{
    size_t count = JS_GetMemorySize(m->rt), cached = rt_cached(m->rt);

    if (young)
        JS_RunYoungGC(m->rt);
    else
        JS_RunGC(m->rt);
    CHECK(rt_cached(m->rt) == 0, "the caches hold %zu blocks after %s", rt_cached(m->rt), young ? "JS_RunYoungGC" : "JS_RunGC");
    CHECK(JS_GetMemorySize(m->rt) <= count && (cached || JS_GetMemorySize(m->rt) == count), "%s changed the count from %zu to %zu with %zu blocks cached",
          young ? "JS_RunYoungGC" : "JS_RunGC", count, JS_GetMemorySize(m->rt), cached);
}

static void rt_step(RtModel *m, Src *r)
{
    uint64_t op = src_below(r, 100);

    if (op < 40 || m->n == 0) {
        size_t sz = pick_size(r, 1);
        /* the front cache and the arenas are what this level is about: small sizes most of the time */
        if (src_below(r, 3)) sz = src_below(r, 2) ? 1 + src_below(r, 520) : 505 + src_below(r, 3800);
        if (m->n < MAX_LIVE && (m->bytes < ((size_t)96 << 20) || sz > JS_HEAP_MAX_REQUEST))
            rt_alloc(m, sz);
    } else if (op < 78) {
        rt_free(m, (int)src_below(r, m->n));
    } else if (op < 92) {
        size_t sz = pick_size(r, 1);
        int i = (int)src_below(r, m->n), big = src_below(r, 6) == 0 ? biggest_block(m->live, m->n) : -1;
        if (src_below(r, 2)) sz = src_below(r, 2) ? 1 + src_below(r, 600) : 505 + src_below(r, 3800);
        if (big >= 0) {
            i = big;
            sz = m->live[big].usable + 1 + src_below(r, m->live[big].usable);
            if (sz > ((size_t)48 << 20))
                sz = (size_t)48 << 20;
        }
        if (m->bytes < ((size_t)96 << 20) || sz > JS_HEAP_MAX_REQUEST)
            rt_realloc(m, i, sz);
    } else if (op < 93) {
        rt_gc(m, (int)src_below(r, 2));
    } else if (op < 94) {
        m->front = src_below(r, 4) == 0 ? 0 : (int)src_below(r, 300);
        if (m->front > 256) m->front = 256;
        JS_SetFrontCache(m->rt, m->front);
        CHECK(rt_cached(m->rt) == 0, "JS_SetFrontCache left blocks in the caches");
    } else if (op < 95) {
        rt_release(m, src_below(r, 2) ? 0 : (size_t)src_below(r, (size_t)1 << 20));
    } else if (op < 97) {
        size_t size = JS_GetMemorySize(m->rt);
        m->limit = src_below(r, 2) ? size + src_below(r, 200000) : (size_t)-1;
        JS_SetMemoryLimit(m->rt, m->limit);
    } else if (op < 98) {
        JSMemoryUsage u;
        JS_ComputeMemoryUsage(m->rt, &u);
        CHECK((size_t)u.malloc_size == JS_GetMemorySize(m->rt) && u.footprint >= 65536 && u.chunk_count >= 1,
              "JS_ComputeMemoryUsage differs from the heap");
    } else if (op < 99) {
        size_t footprint = rt_footprint(m->rt);
        m->fp_limit = src_below(r, 3) == 0 ? 0 : footprint + src_below(r, 3) * 65536;
        JS_SetFootprintLimit(m->rt, m->fp_limit);
    } else if (src_below(r, 3) == 0) {
        vms[m->vm].refuse_remap_at = vms[m->vm].remaps + 1 + (long)src_below(r, 2);
    } else {
        vms[m->vm].refuse_at = vms[m->vm].grows + 1 + (long)src_below(r, 2);
    }
}

static void run_runtime(Src *r, long steps)
{
    RtModel *m = calloc(1, sizeof *m);
    size_t leaked, count;
    void *p;

    m->vm = host_open();
    m->rt = JS_NewRuntime(m->vm, 1);
    CHECK(m->rt, "JS_NewRuntime failed");
    JS_SetHostRemap(m->rt, next_remap());
    m->limit = (size_t)-1;
    /* the header of a large block, from what one block costs: the count goes
       up by its usable size, its header and the 8 byte word of the heap */
    JS_SetFrontCache(m->rt, 0);
    count = JS_GetMemorySize(m->rt);
    p = js_malloc_rt(m->rt, 1000);
    CHECK(p, "a large block");
    m->large_hdr = JS_GetMemorySize(m->rt) - count - js_malloc_usable_size_rt(m->rt, p) - 8;
    CHECK(m->large_hdr >= 8 && m->large_hdr <= 32, "the header of a large block is %zu bytes", m->large_hdr);
    js_free_rt(m->rt, p);
    CHECK(JS_GetMemorySize(m->rt) == count, "a large block was freed and the count did not come back");
    m->front = 256;
    JS_SetFrontCache(m->rt, m->front);
    rt_check(m);
    for (long i = 0; i < steps && !r->done; i++) {
        rt_step(m, r);
        if (i % 1500 == 1499)
            rt_check(m);
    }
    rt_check(m);
    while (m->n)
        rt_free(m, m->n - 1);
    JS_SetMemoryLimit(m->rt, (size_t)-1);
    JS_SetFootprintLimit(m->rt, 0);
    rt_check(m);
    leaked = JS_FreeRuntime(m->rt);
    CHECK(leaked == 0, "JS_FreeRuntime reports %zu blocks in use", leaked);
    host_close(m->vm);
    free(m);
}
#endif /* HEAP_TEST_RUNTIME */

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
    /* a block over half of the next chunk gets a chunk of its own, sized for it
       (its 48 bytes of header, size word and end marker, rounded up to 64 KiB),
       and the chunk goes back to the host when the block is freed */
    prev_fp = h->footprint;
    {
        size_t need = (size_t)12 << 20, n = vms[m.vm].n;
        void *p = heap_alloc(h, need);
        size_t got = h->footprint - prev_fp;
        CHECK(p, "a block of 12 MiB failed");
        CHECK(got == ((need + 48 + 65535) / 65536) * 65536, "the chunk of its own is %zu for a block of %zu", got, need);
        CHECK(heap_usable_size(p) == need, "the block of its own has a usable size of %zu, not %zu", heap_usable_size(p), need);
        heap_free(h, p);
        CHECK(h->footprint == prev_fp && vms[m.vm].n == n, "the chunk of its own did not go back with its block");
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

/* The chunk of a block that grew: an eighth of room more (the budget permitting) */
static size_t grown_chunk(size_t n)
{
    size_t s = (n + 7) & ~(size_t)7;

    return (s + s / 8 + 48 + 65535) / 65536 * 65536;
}

/* Use up the free space of the chunks, so that the next block of more than 30000
   bytes needs a new chunk: with no room to grow (a budget at the footprint),
   take blocks from big to small until none fits. */
static void fill_up(HeapModel *m)
{
    static const size_t sizes[] = { (size_t)2 << 20, (size_t)1 << 20, 512 << 10, 256 << 10, 128 << 10, 64 << 10, 30000 };
    JSHeap *h = m->h;

    h->footprint_limit = h->footprint;
    for (size_t i = 0; i < sizeof sizes / sizeof *sizes; i++)
        while (model_alloc(m, sizes[i]))
            ;
    h->footprint_limit = 0;
}

/* A dedicated chunk holds one block that is too big to share (over half of the
   next chunk, which is over 4 MiB for good). It is the chunk's own, flagged in
   its header, and it goes back to the host in the call that frees the block or
   moves it. Nothing keeps a freed one. A refused resize leaves the old chunk
   as it was. */
static void scenario_dedicated(void)
{
    HeapModel m;
    JSHeap *h;
    HostVM *host;
    size_t fp, count, size = (size_t)5 << 20, chunk, room;
    long releases;
    uint8_t *p;

    heap_model_start(&m);
    h = m.h;
    host = &vms[m.vm];
    fp = h->footprint;
    count = h->count;

    /* made, then freed: the host loses the chunk in the same call */
    CHECK(model_alloc(&m, size), "a block of 5 MiB failed");
    p = m.live[0].p;
    chunk = ((size + 48 + 65535) / 65536) * 65536;
    room = chunk - 48;
    CHECK(h->footprint == fp + chunk && host->n == 2 && host->live == fp + chunk && h->chunk_count == 2,
          "a block of 5 MiB did not get a chunk of %zu: footprint %zu", chunk, h->footprint);
    CHECK(heap_usable_size(p) == size && h->count == count + size + 8 && h->blocks == 1, "the block of a dedicated chunk is counted by its size");
    heap_model_check(&m);
    releases = host->releases;
    model_free(&m, 0);
    CHECK(host->n == 1 && host->live == fp && host->releases == releases + 1 && h->footprint == fp &&
          h->chunk_count == 1 && h->count == count && h->blocks == 0,
          "the chunk did not go back to the host with its block (host holds %zu chunks, %zu bytes)", (size_t)host->n, host->live);
    heap_model_check(&m);

    /* resize inside the chunk: the same chunk, the same pointer, the count by the size */
    CHECK(model_alloc(&m, size), "alloc");
    p = m.live[0].p;
    fp = h->footprint;
    releases = host->releases;
    model_realloc(&m, 0, (size_t)4 << 20);
    CHECK(m.live[0].p == p && heap_usable_size(p) == (size_t)4 << 20 && h->footprint == fp && host->releases == releases &&
          h->count == count + ((size_t)4 << 20) + 8, "a shrink to 4 MiB should stay in the chunk");
    model_realloc(&m, 0, room);
    CHECK(m.live[0].p == p && heap_usable_size(p) == room && h->footprint == fp && host->releases == releases,
          "a resize to the room of the chunk should stay in it");
    /* past the room, or under half of it: a new block, copied, and the chunk is given back at once */
    model_realloc(&m, 0, room + 1);
    CHECK(h->footprint > fp && host->releases == releases + 1 && host->n == 2, "a resize past the room should move to a new chunk and give the old one back");
    heap_model_check(&m);
    fp = h->footprint;
    releases = host->releases;
    model_realloc(&m, 0, (size_t)100);
    CHECK(host->releases == releases + 1 && host->n == 1 && h->footprint == 65536 && h->chunk_count == 1 &&
          h->count == m.rt_size + heap_usable_size(m.live[0].p) + 8,
          "a shrink to 100 bytes should move to the chunks and give the dedicated chunk back");
    model_free(&m, 0);

    /* a refused resize leaves the old chunk, block, content and count */
    CHECK(model_alloc(&m, size), "alloc");
    p = m.live[0].p;
    fp = h->footprint;
    count = h->count;
    releases = host->releases;
    vms[m.vm].refuse_at = host->grows + 1;
    model_realloc(&m, 0, (size_t)12 << 20);
    CHECK(m.live[0].p == p && h->footprint == fp && h->count == count && host->releases == releases && host->n == 2, "a refusal of the host changed the old chunk");
    h->footprint_limit = fp + 65536;
    model_realloc(&m, 0, (size_t)12 << 20);
    CHECK(m.live[0].p == p && h->footprint == fp && h->count == count && host->releases == releases, "the budget refused, and the old chunk changed");
    h->footprint_limit = 0;
    h->limit = count + 1000;
    model_realloc(&m, 0, (size_t)12 << 20);
    CHECK(m.live[0].p == p && h->footprint == fp && h->count == count && host->releases == releases, "the limit refused, and the old chunk changed");
    h->limit = (size_t)-1;
    model_free(&m, 0);

    /* a block of the shared chunks that grows past what they hold moves to a chunk of its own */
    while (h->footprint < ((size_t)16 << 20))
        CHECK(model_alloc(&m, 30000), "alloc 30000");
    while (m.n > 1)
        model_free(&m, m.n - 1);
    model_free(&m, 0);
    CHECK(model_alloc(&m, 100000), "alloc 100000");
    fp = h->footprint;
    releases = host->releases;
    model_realloc(&m, 0, (size_t)6 << 20);
    CHECK(h->footprint == fp + grown_chunk((size_t)6 << 20) && host->releases == releases,
          "a block of the shared chunks that grows to 6 MiB should get a chunk of its own, with room to grow");
    model_free(&m, 0);
    CHECK(h->footprint == fp && host->releases == releases + 1, "the chunk of its own should go back with the block");

    /* the footprint decides what is too big to share. With 4 MiB held the next
       chunk would be 1 MiB, and a block of 1.2 MiB gets a chunk of its own */
    while (m.n)
        model_free(&m, m.n - 1);
    heap_model_finish(&m);
    heap_model_start(&m);
    h = m.h;
    host = &vms[m.vm];
    while (h->footprint < ((size_t)4 << 20))
        CHECK(model_alloc(&m, 30000), "alloc 30000");
    fill_up(&m);
    {
        size_t need = 1258291, before = h->footprint;
        long r0 = host->releases;

        CHECK(model_alloc(&m, need), "alloc 1.2 MiB");
        CHECK(h->footprint == before + (need + 48 + 65535) / 65536 * 65536 && heap_usable_size(m.live[m.n - 1].p) == ((need + 7) & ~(size_t)7),
              "a block of 1.2 MiB with 4 MiB held should get a chunk of its own");
        model_free(&m, m.n - 1);
        CHECK(h->footprint == before && host->releases == r0 + 1, "the chunk of its own should go back with the block");
    }
    /* with 32 MiB held the chunks are 8 MiB, and a block of 3 MiB shares one: it stays when the block is freed,
       until the host asks for it back with a release */
    while (h->footprint < ((size_t)32 << 20))
        CHECK(model_alloc(&m, 30000), "alloc 30000");
    fill_up(&m);
    {
        size_t need = (size_t)3 << 20, before = h->footprint;
        long r0 = host->releases;

        CHECK(model_alloc(&m, need), "alloc 3 MiB");
        CHECK(h->footprint == before + ((size_t)8 << 20), "a block of 3 MiB with 32 MiB held should share a chunk of 8 MiB");
        model_free(&m, m.n - 1);
        CHECK(h->footprint == before + ((size_t)8 << 20) && host->releases == r0, "a shared chunk should stay when its block is freed");
        model_release(&m, 0);
        CHECK(h->footprint == before && host->releases == r0, "a release should take the free shared chunk, and not as a dedicated one");
        /* near the budget the shared chunk shrinks to the block, and is still shared */
        h->footprint_limit = before + ((size_t)3 << 20) + 400000;
        CHECK(model_alloc(&m, need), "alloc 3 MiB near the budget");
        CHECK(h->footprint < before + ((size_t)4 << 20), "the chunk should have shrunk to the block");
        model_free(&m, m.n - 1);
        CHECK(host->releases == r0, "a chunk that the budget shrank is still a shared chunk");
        h->footprint_limit = 0;
    }
    while (m.n)
        model_free(&m, m.n - 1);
    heap_model_check(&m);
    heap_model_finish(&m);
    ok("dedicated chunks: made for one block, given back at once, resized, refused");
}

/* The size of the dedicated chunk a block of n bytes gets */
static size_t dedicated_chunk(size_t n)
{
    return (((n + 7) & ~(size_t)7) + 48 + 65535) / 65536 * 65536;
}

/* A dedicated block that outgrows its chunk, with a host that remaps: the chunk
   gets bigger where it is or elsewhere, with no new chunk, no copy by the heap
   and no release. The host refusing is out of memory: the resize fails and
   nothing changes, and the heap does not try to copy, which takes more. The
   budget and the limit refuse before the host is asked. The host remaps with
   mremap (linux) or by mapping a new chunk, copying and unmapping the old one
   (move: the chunk is then at another address each time, on every system). */
static void remap_by(int move)
{
    HeapModel m;
    JSHeap *h;
    HostVM *host;
    size_t fp, count, grow;
    long releases, grows, remaps, refused;
    uint8_t *p;

    heap_model_start(&m);
    h = m.h;
    host = &vms[m.vm];
    host->move_remap = move;
    heap_set_remap(h, 1);

    /* three dedicated chunks, and the middle of the list grows */
    CHECK(model_alloc(&m, (size_t)5 << 20), "alloc 5 MiB");
    CHECK(model_alloc(&m, (size_t)6 << 20), "alloc 6 MiB");
    CHECK(model_alloc(&m, (size_t)7 << 20), "alloc 7 MiB");
    CHECK(host->n == 4, "three chunks of their own");
    fp = h->footprint;
    count = h->count;
    releases = host->releases;
    grows = host->grows;
    remaps = host->remaps;
    p = m.live[1].p;
    model_realloc(&m, 1, (size_t)20 << 20);
    CHECK(!move || m.live[1].p != p, "a host that moves the chunk gave the block its old address");
    CHECK(host->remaps == remaps + 1 && host->releases == releases && host->grows == grows && host->n == 4,
          "a dedicated block that outgrows its chunk should remap it: %ld remaps, %ld releases, %ld grows", host->remaps - remaps,
          host->releases - releases, host->grows - grows);
    CHECK(h->footprint == fp - dedicated_chunk((size_t)6 << 20) + grown_chunk((size_t)20 << 20) && host->live == h->footprint,
          "the footprint after a remap is %zu", h->footprint);
    CHECK(heap_usable_size(m.live[1].p) == (size_t)20 << 20 && h->count == count - ((size_t)6 << 20) + ((size_t)20 << 20) &&
          h->blocks == 3, "the count after a remap");
    heap_model_check(&m);
    /* the content is checked by model_realloc, and the other two chunks are still where the heap thinks */
    model_free(&m, 0);
    model_free(&m, 1);
    model_free(&m, 0);
    CHECK(host->n == 1 && host->releases == releases + 3, "the three chunks should be given back");
    heap_model_check(&m);

    /* the host refuses the remap: out of memory, the old chunk as it was, and no copy */
    CHECK(model_alloc(&m, (size_t)5 << 20), "alloc 5 MiB");
    p = m.live[0].p;
    fp = h->footprint;
    count = h->count;
    releases = host->releases;
    grows = host->grows;
    remaps = host->remaps;
    refused = host->refused;
    host->refuse_remap_at = remaps + 1;
    model_realloc(&m, 0, (size_t)30 << 20);
    CHECK(m.live[0].p == p && h->footprint == fp && h->count == count && heap_usable_size(p) == (size_t)5 << 20,
          "a refused remap changed the block or the heap");
    CHECK(host->remaps == remaps + 1 && host->refused == refused + 1 && host->grows == grows && host->releases == releases,
          "a refused remap should end the resize: no new chunk, no copy, no release");
    heap_model_check(&m);
    model_realloc(&m, 0, (size_t)30 << 20);
    CHECK(host->remaps == remaps + 2 && heap_usable_size(m.live[0].p) == (size_t)30 << 20, "the same resize should pass when the host does not refuse");

    /* the budget refuses, to the byte, for the growth alone, before the host is asked */
    fp = h->footprint;
    /* the budget leaves no room for the eighth of room, so the chunk is the block's alone */
    grow = dedicated_chunk((size_t)40 << 20) - grown_chunk((size_t)30 << 20);
    remaps = host->remaps;
    h->footprint_limit = fp + grow - 1;
    model_realloc(&m, 0, (size_t)40 << 20);
    CHECK(host->remaps == remaps && h->footprint == fp, "the budget one byte short should refuse before the host is asked");
    h->footprint_limit = fp + grow;
    model_realloc(&m, 0, (size_t)40 << 20);
    CHECK(host->remaps == remaps + 1 && h->footprint == fp + grow, "the budget at the growth should pass");
    h->footprint_limit = 0;

    /* the limit refuses before the host is asked */
    count = h->count;
    remaps = host->remaps;
    h->limit = count + ((size_t)50 << 20) - heap_usable_size(m.live[0].p) - 1;
    model_realloc(&m, 0, (size_t)50 << 20);
    CHECK(host->remaps == remaps && h->count == count, "the limit one byte short should refuse before the host is asked");
    h->limit = (size_t)-1;

    /* a host that does not remap: a new chunk, a copy, and the old chunk back at once */
    heap_set_remap(h, 0);
    releases = host->releases;
    grows = host->grows;
    remaps = host->remaps;
    model_realloc(&m, 0, (size_t)50 << 20);
    CHECK(host->remaps == remaps && host->grows == grows + 1 && host->releases == releases + 1 && host->n == 2,
          "without remap a dedicated block that outgrows its chunk should get a new chunk and give the old one back");
    heap_model_check(&m);
    /* a buffer that grows by 64 KiB at a time gets a new chunk (or a remap) once in a
       while, not at each step: the chunk has an eighth of room for the block */
    while (m.n)
        model_free(&m, m.n - 1);
    for (int mode = 0; mode < 2; mode++) {
        size_t n;

        heap_set_remap(h, mode);
        CHECK(model_alloc(&m, (size_t)5 << 20), "alloc 5 MiB");
        grows = host->grows;
        remaps = host->remaps;
        for (n = (size_t)5 << 20; n < ((size_t)20 << 20); n += 65536)
            model_realloc(&m, 0, n);
        CHECK(host->grows - grows + host->remaps - remaps < 30, "%s: %ld new chunks and %ld remaps for a block that grew from 5 to 20 MiB by 64 KiB",
              mode ? "remap" : "copy", host->grows - grows, host->remaps - remaps);
        model_free(&m, 0);
    }
    heap_model_finish(&m);
}

static void scenario_remap(void)
{
    remap_by(1);
#ifdef __linux__
    remap_by(0);
#endif
    ok("remap: a dedicated chunk grows in place or moves, the host refusing is out of memory, the budget and the limit ask first");
}

/* The chunk of a dedicated block that grows is at the head, in the middle or at
   the tail of the list of dedicated chunks, and the list must follow it to its
   new address. Three blocks of 3, 4 and 5 MiB (the last is the head of the
   list) and each in turn grows to three times its size, in each way the heap
   has: remapped by a host that moves the chunk, by mremap (linux), and copied to
   a chunk of its own when the host does not remap. Each also meets a refusal
   of the host first, which must leave all three as they were. Then the three
   are freed in each of the six orders: the heap walks its list at each step
   (heap_check) and the host holds exactly the chunks that are left. A list
   that kept the old address would walk into an unmapped chunk. */
static void positions_run(int way, int pos, int refuse, int order)
{
    static const int orders[6][3] = { { 0, 1, 2 }, { 0, 2, 1 }, { 1, 0, 2 }, { 1, 2, 0 }, { 2, 0, 1 }, { 2, 1, 0 } };
    HeapModel m;
    JSHeap *h;
    HostVM *host;
    uint8_t *ptr[3], *old;
    size_t size[3] = { (size_t)3 << 20, (size_t)4 << 20, (size_t)5 << 20 };
    size_t fp, count;
    long releases, remaps, grows;

    heap_model_start(&m);
    h = m.h;
    host = &vms[m.vm];
    /* way 0: the host moves the chunk, 1: mremap, 2: no remap */
    host->move_remap = way == 0;
    heap_set_remap(h, way != 2);
    for (int i = 0; i < 3; i++) {
        CHECK(model_alloc(&m, size[i]), "alloc %zu", size[i]);
        ptr[i] = m.live[i].p;
    }
    CHECK(host->n == 4 && heap_dedicated_size(h, ptr[0]) == own_chunk(size[0]), "three chunks of their own");
    heap_model_check(&m);
    fp = h->footprint;
    count = h->count;
    old = m.live[pos].p;
    if (refuse) {
        if (way == 2)
            host->refuse_at = host->grows + 1;
        else
            host->refuse_remap_at = host->remaps + 1;
        model_realloc(&m, pos, size[pos] * 3);
        CHECK(m.live[pos].p == old && h->footprint == fp && h->count == count && m.live[pos].usable == size[pos],
              "a refusal of the host changed the block (way %d, position %d)", way, pos);
        heap_model_check(&m);
    }
    releases = host->releases;
    remaps = host->remaps;
    grows = host->grows;
    model_realloc(&m, pos, size[pos] * 3);
    CHECK(m.live[pos].usable == size[pos] * 3, "the block did not grow (way %d, position %d)", way, pos);
    if (way == 2)
        CHECK(m.live[pos].p != old && host->releases == releases + 1 && host->grows == grows + 1 && host->remaps == remaps,
              "a block that cannot be remapped gets a chunk of its own and gives the old one back");
    else
        CHECK(host->releases == releases && host->grows == grows && host->remaps == remaps + 1 && (way != 0 || m.live[pos].p != old),
              "a block that is remapped keeps its chunk (moved by the host: %d)", way == 0);
    ptr[pos] = m.live[pos].p;
    heap_model_check(&m);
    for (int k = 0; k < 3; k++) {
        int i, left = host->n;

        for (i = 0; i < m.n && m.live[i].p != ptr[orders[order][k]]; i++)
            ;
        CHECK(i < m.n, "block %d is not held", orders[order][k]);
        if (i == m.n)
            break;
        model_free(&m, i);
        CHECK(host->n == left - 1, "the chunk of the block did not go back to the host");
        heap_model_check(&m);
    }
    CHECK(host->n == 1 && h->footprint == 65536, "chunks are left");
    heap_model_finish(&m);
}

static void scenario_remap_positions(void)
{
    int ways = 3;

#ifndef __linux__
    ways = 1;  /* no mremap, and the heap's own copy is a way of its own */
#endif
    for (int way = 0; way < 3; way++) {
        if (way == 1 && ways == 1)
            continue;
        for (int pos = 0; pos < 3; pos++)
            for (int refuse = 0; refuse < 2; refuse++)
                for (int order = 0; order < 6; order++)
                    positions_run(way, pos, refuse, order);
    }
    ok("remap: the list of dedicated chunks follows a chunk that grows at its head, middle or tail, moved or copied, and after a refusal");
}

/* A request under a budget for the footprint: with the budget one unit short of
   the least growth it is refused, to the byte, with the host not asked and
   nothing changed, and with the growth it passes and the footprint gains it. */
static void budget_exact_alloc(HeapModel *m, size_t sz, const char *what)
{
    JSHeap *h = m->h;
    HostVM *host = &vms[m->vm];
    size_t fp = h->footprint, count = h->count, growth = min_growth(fp, sz);
    long grows = host->grows, refused = host->refused;

    h->footprint_limit = fp + growth - T_UNIT;
    CHECK(!model_alloc(m, sz), "%s: %zu passed with the budget one unit short", what, sz);
    CHECK(host->grows == grows && host->refused == refused && h->footprint == fp && h->count == count,
          "%s: %zu was refused by the budget, and the host was asked or the heap changed", what, sz);
    h->footprint_limit = fp + growth;
    CHECK(model_alloc(m, sz), "%s: %zu was refused with the budget at the growth, and the host refused nothing", what, sz);
    CHECK(host->grows == grows + 1 && host->refused == refused && h->footprint == fp + growth,
          "%s: %zu with the budget at the growth took %zu of footprint, not %zu", what, sz, h->footprint - fp, growth);
    h->footprint_limit = 0;
}

/* The same for the resize to sz of the block i. growth is what the heap needs
   room for while it resizes, fp_after the footprint when it is done (the old
   chunk of a block that moved is given back by then), and which the count of the
   host that goes up by one: 0 the grows, 1 the remaps. */
static void budget_exact_resize(HeapModel *m, int i, size_t sz, size_t growth, size_t fp_after, int which, const char *what)
{
    JSHeap *h = m->h;
    HostVM *host = &vms[m->vm];
    size_t fp = h->footprint, count = h->count, usable = m->live[i].usable;
    uint8_t *p = m->live[i].p;
    long *counter = which ? &host->remaps : &host->grows, before = *counter, refused = host->refused;

    h->footprint_limit = fp + growth - T_UNIT;
    model_realloc(m, i, sz);
    CHECK(m->live[i].p == p && m->live[i].usable == usable && *counter == before && host->refused == refused && h->footprint == fp && h->count == count,
          "%s: the resize to %zu passed or was tried with the budget one unit short", what, sz);
    h->footprint_limit = fp + growth;
    model_realloc(m, i, sz);
    CHECK(m->live[i].usable >= sz && *counter == before + 1 && host->refused == refused && h->footprint == fp_after,
          "%s: the resize to %zu with the budget at the growth gave %zu usable, footprint %+ld, and the host was asked %ld times", what, sz, m->live[i].usable,
          (long)h->footprint - (long)fp, *counter - before);
    h->footprint_limit = 0;
}

/* The budget refuses a growth to the byte and passes it to the byte, for every
   way a block gets memory: a block of its own chunk, a chunk that is remapped
   (the growth of the chunk alone), a block that moves to a chunk of its own
   while the old one is still held, and a block of the shared chunks that
   outgrows them. A resize that needs nothing (inside the room of the chunk, or
   to a smaller block) passes with no budget left at all. */
static void scenario_budget_exact(void)
{
    static const size_t allocs[] = { (size_t)1 << 20, (size_t)5 << 20, ((size_t)5 << 20) + 1, (size_t)17 << 20 };
    HeapModel m;
    JSHeap *h;
    HostVM *host;
    size_t chunk, fp;

    for (size_t k = 0; k < sizeof allocs / sizeof *allocs; k++) {
        heap_model_start(&m);
        budget_exact_alloc(&m, allocs[k], "alloc");
        heap_model_check(&m);
        heap_model_finish(&m);
    }

    /* a remapped chunk grows by the difference of the chunks */
    for (int move = 0; move < 2; move++) {
        heap_model_start(&m);
        h = m.h;
        host = &vms[m.vm];
        host->move_remap = move;
        heap_set_remap(h, 1);
        CHECK(model_alloc(&m, (size_t)5 << 20), "alloc");
        chunk = own_chunk((size_t)5 << 20);
        fp = h->footprint;
        budget_exact_resize(&m, 0, (size_t)12 << 20, own_chunk((size_t)12 << 20) - chunk, fp + own_chunk((size_t)12 << 20) - chunk, 1, "remap");
        heap_model_check(&m);
        heap_model_finish(&m);
    }

    /* no remap: the new chunk is held with the old one while the data moves */
    heap_model_start(&m);
    h = m.h;
    heap_set_remap(h, 0);
    CHECK(model_alloc(&m, (size_t)5 << 20), "alloc");
    fp = h->footprint;
    chunk = heap_dedicated_size(h, m.live[0].p);
    budget_exact_resize(&m, 0, (size_t)12 << 20, min_growth(fp, (size_t)12 << 20), fp + min_growth(fp, (size_t)12 << 20) - chunk, 0, "copy");
    heap_model_check(&m);
    heap_model_finish(&m);

    /* a block of the shared chunks that outgrows them */
    heap_model_start(&m);
    h = m.h;
    CHECK(model_alloc(&m, 1000), "alloc");
    fp = h->footprint;
    budget_exact_resize(&m, 0, (size_t)5 << 20, min_growth(fp, (size_t)5 << 20), fp + min_growth(fp, (size_t)5 << 20), 0, "shared to own");
    heap_model_check(&m);
    heap_model_finish(&m);

    /* nothing is needed: a smaller block, and the room of the chunk, with no budget left at all */
    for (int remap = 0; remap < 2; remap++) {
        size_t room;
        uint8_t *p;

        heap_model_start(&m);
        h = m.h;
        host = &vms[m.vm];
        heap_set_remap(h, remap);
        CHECK(model_alloc(&m, (size_t)5 << 20), "alloc");
        p = m.live[0].p;
        room = heap_dedicated_size(h, p) - 48;
        h->footprint_limit = h->footprint;
        model_realloc(&m, 0, room);
        CHECK(m.live[0].p == p && m.live[0].usable == room, "a resize to the room of the chunk needs no budget");
        model_realloc(&m, 0, room / 2);
        CHECK(m.live[0].p == p && m.live[0].usable == room / 2, "a resize to half the room needs no budget");
        model_realloc(&m, 0, 100);
        CHECK(m.live[0].usable >= 100 && m.live[0].usable < 200 && h->footprint == 65536,
              "a shrink to 100 bytes needs no budget and gives the chunk back (footprint %zu)", h->footprint);
        h->footprint_limit = 0;
        heap_model_check(&m);
        heap_model_finish(&m);
    }
    ok("budget: a growth is refused one unit short and passes at its growth, for a new chunk, a remap, a copy and a move from the shared chunks");
}

/* The biggest request the heap serves is JS_HEAP_MAX_REQUEST, nearly 4 GiB, and a
   dedicated chunk takes it: a host that is not asked to refuse must give it,
   whole or not at all (a system with no room for the mapping is a skip). The
   mapping is sparse: the test writes the first and the last 64 bytes of the
   block and no more. One byte over is refused by the heap with the host not asked. */
static void check_edges(const uint8_t *p, size_t n, uint8_t first, uint8_t last, const char *what)
{
    for (size_t i = 0; i < 64; i++)
        CHECK(p[i] == first && p[n - 64 + i] == last, "%s: the edges of the block changed", what);
}

static void scenario_max_request(void)
{
    HeapModel m;
    JSHeap *h;
    HostVM *host;
    size_t fp, count, chunk, max = JS_HEAP_MAX_REQUEST;
    long grows, refused;
    uint8_t *p, *q;

    heap_model_start(&m);
    h = m.h;
    host = &vms[m.vm];
    fp = h->footprint;
    count = h->count;
    p = heap_alloc(h, max);
    if (!p) {
        CHECK(host->refused > 0 && h->footprint == fp && h->count == count, "the biggest request failed, and the host did not refuse it");
        heap_model_finish(&m);
        printf("SKIP  the biggest request: the system gave no mapping of 4 GiB\n");
        return;
    }
    chunk = own_chunk(max);
    CHECK(heap_usable_size(p) == max && h->footprint == fp + chunk && host->live == h->footprint && h->count == count + max + 8 && h->blocks == 1,
          "the biggest block: usable %zu, footprint %zu", heap_usable_size(p), h->footprint);
    CHECK(!heap_check(h), "heap_check of the biggest block");
    memset(p, 0xa5, 64);
    memset(p + max - 64, 0x5a, 64);
    check_edges(p, max, 0xa5, 0x5a, "biggest block");
    grows = host->grows;
    refused = host->refused;
    CHECK(!heap_alloc(h, max + 1) && !heap_realloc(h, p, max + 1), "one byte over the biggest request passed");
    CHECK(host->grows == grows && host->refused == refused && h->footprint == fp + chunk && heap_usable_size(p) == max, "one byte over changed the heap or asked the host");
    check_edges(p, max, 0xa5, 0x5a, "after the refused ones");
    heap_free(h, p);
    CHECK(host->n == 1 && h->footprint == fp && h->count == count && h->blocks == 0, "the chunk of the biggest block did not go back");
    CHECK(!heap_check(h), "heap_check after the biggest block");

    /* a dedicated block of 5 MiB that grows to the biggest, remapped and copied */
    for (int mode = 0; mode < 3; mode++) {
        heap_set_remap(h, mode != 2);
        host->move_remap = mode == 0;
        q = heap_alloc(h, (size_t)5 << 20);
        CHECK(q != NULL, "alloc 5 MiB");
        memset(q, 0x11, 64);
        memset(q + ((size_t)5 << 20) - 64, 0x22, 64);
        p = heap_realloc(h, q, max);
        CHECK(p != NULL || host->refused > 0, "the biggest resize failed, and the host did not refuse it");
        if (!p) {
            heap_free(h, q);
            continue;
        }
        CHECK(heap_usable_size(p) == max && h->footprint == fp + grown_chunk(max) && h->count == count + max + 8,
              "mode %d: the block grown to the biggest has usable %zu, footprint %zu", mode, heap_usable_size(p), h->footprint);
        CHECK(p[0] == 0x11 && p[63] == 0x11 && p[((size_t)5 << 20) - 64] == 0x22 && p[((size_t)5 << 20) - 1] == 0x22, "mode %d: the content of the block was lost", mode);
        heap_free(h, p);
        CHECK(host->n == 1 && h->footprint == fp && h->count == count, "mode %d: the chunk did not go back", mode);
    }
    heap_model_finish(&m);
    ok("the biggest request: a dedicated chunk takes it whole, one byte over is refused, a 5 MiB block grows to it");
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
    rl.start = heap_start;
    rl.chunk = heap_chunk;
    rl.range = heap_range;

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
    300000, 1000000,
};
#define N_FRAG (sizeof frag_sizes / sizeof frag_sizes[0])

/* free blocks of many sizes, each between two blocks in use so that they do not
   merge: the minimal block of 24 bytes (nothing inside it), the sizes around
   4096 + 24 (the smallest inside offered), sizes on both sides of the borders
   of the size classes, and a whole free chunk that is shared: with 36 MiB held in
   dedicated chunks the next chunk is 8 MiB, and a block of 3 MiB takes a new
   one that is a whole free pool once the block is freed */
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
    {
        void *big[3], *b;

        for (int i = 0; i < 3; i++) {
            CHECK(model_alloc(m, (size_t)12 << 20), "alloc 12 MiB");
            big[i] = m->live[m->n - 1].p;
        }
        CHECK(model_alloc(m, (size_t)3 << 20), "alloc 3 MiB");
        b = m->live[m->n - 1].p;
        model_free_ptr(m, b);
        for (int i = 0; i < 3; i++)
            model_free_ptr(m, big[i]);
    }
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

#ifdef HEAP_TEST_RUNTIME
static void scenario_runtime(void)
{
    uint32_t vm = host_open();
    JSRuntime *rt = JS_NewRuntime(vm, 1);
    uint8_t *small[600], *big;
    size_t size, leaked;
    JSMemoryUsage u;

    CHECK(rt, "JS_NewRuntime");
    CHECK(!JS_CheckMemory(rt), "a new runtime fails the check");

    /* the block of size 0 is one block, with usable size 0 */
    {
        void *z = js_malloc_rt(rt, 0), *z2 = js_malloc_rt(rt, 0);
        CHECK(z && z == z2 && js_malloc_usable_size_rt(rt, z) == 0, "zero size block");
        js_free_rt(rt, z);
        js_free_rt(rt, NULL);
        z = js_realloc_rt(rt, z, 10);
        CHECK(z != NULL, "realloc of the zero size block");
        js_free_rt(rt, z);
        js_free_rt(rt, js_realloc_rt(rt, NULL, 20));
    }
    CHECK(!js_malloc_rt(rt, SIZE_MAX) && !js_malloc_rt(rt, SIZE_MAX - 7) && !js_malloc_rt(rt, (size_t)1 << 40) &&
          !js_malloc_rt(rt, JS_HEAP_MAX_REQUEST), "a request near SIZE_MAX returned a block");

    /* the front cache keeps freed small blocks: they count, a GC gives them back */
    JS_RunGC(rt);
    size = JS_GetMemorySize(rt);
    for (int i = 0; i < 600; i++) {
        small[i] = js_malloc_rt(rt, 1 + (size_t)i % 500);
        CHECK(small[i], "small alloc");
    }
    CHECK(JS_GetMemorySize(rt) > size, "small blocks do not count");
    for (int i = 0; i < 600; i++)
        js_free_rt(rt, small[i]);
    CHECK(!JS_CheckMemory(rt), "check after frees");
    JS_ComputeMemoryUsage(rt, &u);
    CHECK((size_t)u.malloc_size > size, "freed blocks that the cache holds must keep counting");
    JS_RunGC(rt);
    CHECK(JS_GetMemorySize(rt) == size, "a GC did not give the front cache back: %zu, was %zu", JS_GetMemorySize(rt), size);
    CHECK(!JS_CheckMemory(rt), "check after a GC");

    /* with no cache, a freed block goes back to its arena at once */
    JS_SetFrontCache(rt, 0);
    for (int i = 0; i < 600; i++)
        small[i] = js_malloc_rt(rt, 1 + (size_t)i % 500);
    for (int i = 0; i < 600; i++)
        js_free_rt(rt, small[i]);
    CHECK(JS_GetMemorySize(rt) == size, "with no front cache the arenas should be freed at once");
    JS_SetFrontCache(rt, 256);

    /* retry: the limit counts the cache, and a request that fits once the cache
       is given back passes */
    JS_RunGC(rt);
    for (int i = 0; i < 600; i++)
        small[i] = js_malloc_rt(rt, 100 + (size_t)i % 400);
    for (int i = 0; i < 600; i++)
        js_free_rt(rt, small[i]);
    size = JS_GetMemorySize(rt);
    JS_SetMemoryLimit(rt, size + 10);
    big = js_malloc_rt(rt, 100000);
    CHECK(big != NULL, "a request that fits once the front cache is given back failed");
    CHECK(JS_GetMemorySize(rt) <= size + 10 + 4096 + 100000, "count after the retry");
    js_free_rt(rt, big);
    JS_SetMemoryLimit(rt, (size_t)-1);
    CHECK(!JS_CheckMemory(rt), "check after the retry");
    /* the same request with nothing to give back fails */
    JS_RunGC(rt);
    JS_SetFrontCache(rt, 0);
    for (int i = 0; i < 600; i++)
        small[i] = js_malloc_rt(rt, 100 + (size_t)i % 400);
    JS_SetMemoryLimit(rt, JS_GetMemorySize(rt) + 10);
    CHECK(!js_malloc_rt(rt, 100000), "a request past the limit passed");
    JS_SetMemoryLimit(rt, (size_t)-1);
    for (int i = 0; i < 600; i++)
        js_free_rt(rt, small[i]);
    JS_SetFrontCache(rt, 256);

    /* a resize that needs the cache given back */
    JS_RunGC(rt);
    big = js_malloc_rt(rt, 20000);
    memset(big, 0x5a, 20000);
    for (int i = 0; i < 600; i++)
        small[i] = js_malloc_rt(rt, 100 + (size_t)i % 400);
    for (int i = 0; i < 600; i++)
        js_free_rt(rt, small[i]);
    JS_SetMemoryLimit(rt, JS_GetMemorySize(rt) + 10);
    {
        uint8_t *q = js_realloc_rt(rt, big, 60000);
        CHECK(q, "a resize that fits once the front cache is given back failed");
        for (int i = 0; i < 20000; i++)
            CHECK(q[i] == 0x5a, "content");
        /* too big even then: the block stays as it was */
        JS_SetMemoryLimit(rt, JS_GetMemorySize(rt) + 10);
        big = js_realloc_rt(rt, q, 10000000);
        CHECK(!big, "a resize past the limit passed");
        CHECK(js_malloc_usable_size_rt(rt, q) >= 60000, "a failed resize changed the block");
        for (int i = 0; i < 20000; i++)
            CHECK(q[i] == 0x5a, "content after a failed resize");
        js_free_rt(rt, q);
    }
    JS_SetMemoryLimit(rt, (size_t)-1);
    CHECK(!JS_CheckMemory(rt), "check after the resize");

    /* JS_FreeRuntime says how many blocks of the heap were left, not how many
       the program allocated: a leaked arena is one block, whatever the number
       of slots in it that are in use */
    big = js_malloc_rt(rt, 1000);
    CHECK(big != NULL, "leak block");
    leaked = JS_FreeRuntime(rt);
    CHECK(leaked == 1, "JS_FreeRuntime reported %zu blocks, one was leaked", leaked);
    host_close(vm);

    vm = host_open();
    rt = JS_NewRuntime(vm, 1);
    CHECK(rt, "JS_NewRuntime");
    JS_SetFrontCache(rt, 0);
    for (int i = 0; i < 4; i++)
        CHECK(js_malloc_rt(rt, 100), "leak in an arena");      /* four slots of one arena */
    CHECK(js_malloc_rt(rt, 300) != NULL, "leak in another arena");
    CHECK(js_malloc_rt(rt, 1000) != NULL, "leak, large");
    leaked = JS_FreeRuntime(rt);
    CHECK(leaked == 3, "JS_FreeRuntime reported %zu blocks, two arenas and a large block were leaked", leaked);
    host_close(vm);

    vm = host_open();
    rt = JS_NewRuntime(vm, 1);
    CHECK(rt, "JS_NewRuntime");
    leaked = JS_FreeRuntime(rt);
    CHECK(leaked == 0, "JS_FreeRuntime reported %zu blocks in a new runtime", leaked);
    CHECK(vms[vm].live > 0, "the host holds the chunks until it closes the VM");
    host_close(vm);
    ok("runtime: front cache, retry after a failure, zero size block, huge sizes, leak count");
}

/* ---- the cache of freed large blocks ----

   A freed block of 505 to 4096 bytes waits in the list of its class (one class
   per 64 bytes) for the next request of that class. It stays a block in use as
   far as the heap goes: it counts in MemorySize and for the limit, and
   heap_check and JS_ComputeMemoryUsage count it. It is given back to the heap
   when the cache is turned off, by a collection, by a release, when a request
   fails at the heap, and when the runtime ends. The cache keeps at most
   JS_LargeCacheLimits blocks of a class and bytes in all. */

static size_t lc_cost(JSRuntime *rt, void *p, size_t hdr)
{
    /* what a large block adds to the count: its user size, its header and the
       8 byte word of the heap */
    return js_malloc_usable_size_rt(rt, p) + hdr + 8;
}

static void lc_release_all(uint32_t vm, JSRuntime *rt)
{
    Releaser rl = { rt, rt_start, rt_chunk, rt_range };

    host_release(vm, &rl, 0);
}

static void scenario_large_cache(void)
{
    uint32_t vm = host_open();
    JSRuntime *rt = JS_NewRuntime(vm, 1);
    JSMemoryUsage u0, u1;
    size_t per_class, cap, count0, hdr, cost, n;
    uint8_t *p, *q, *live[600];
    int i, k;

    CHECK(rt, "JS_NewRuntime");
    JS_LargeCacheLimits(&per_class, &cap);
    CHECK(per_class == 8 && cap == (size_t)256 << 10, "the limits are %zu blocks a class and %zu bytes", per_class, cap);

    /* what a large block costs on top of its user size, from the cache off */
    JS_SetFrontCache(rt, 0);
    count0 = JS_GetMemorySize(rt);
    p = js_malloc_rt(rt, 1000);
    hdr = JS_GetMemorySize(rt) - count0 - js_malloc_usable_size_rt(rt, p) - 8;
    CHECK(hdr >= 8 && hdr <= 32, "header of a large block %zu", hdr);
    js_free_rt(rt, p);
    CHECK(JS_GetMemorySize(rt) == count0 && JS_LargeCacheBlocks(rt) == 0, "with the cache off a freed large block is not kept");
    JS_SetFrontCache(rt, 256);
    CHECK(!JS_CheckMemory(rt), "check at the start");

    /* a freed block waits, counts, and is the next one asked for. A request is
       rounded up to its class, so the block holds the whole class */
    count0 = JS_GetMemorySize(rt);
    JS_ComputeMemoryUsage(rt, &u0);
    p = js_malloc_rt(rt, 1000);
    cost = lc_cost(rt, p, hdr);
    CHECK(js_malloc_usable_size_rt(rt, p) >= 1024, "a request of 1000 is rounded up to 1024, got %zu", js_malloc_usable_size_rt(rt, p));
    CHECK(JS_GetMemorySize(rt) == count0 + cost, "a large block costs %zu", JS_GetMemorySize(rt) - count0);
    js_free_rt(rt, p);
    CHECK(JS_LargeCacheBlocks(rt) == 1, "a freed large block is not in the cache");
    CHECK(JS_LargeCacheBytes(rt) == cost - 8, "the cache holds %zu bytes, a block is %zu", JS_LargeCacheBytes(rt), cost - 8);
    CHECK(JS_GetMemorySize(rt) == count0 + cost, "a cached block must keep counting: %zu, was %zu", JS_GetMemorySize(rt), count0 + cost);
    JS_ComputeMemoryUsage(rt, &u1);
    CHECK(u1.malloc_count == u0.malloc_count + 1 && (size_t)u1.malloc_size == count0 + cost, "JS_ComputeMemoryUsage does not count the cached block as a block in use");
    CHECK(!JS_CheckMemory(rt), "check with one cached block (heap_check counts it as in use)");
    q = js_malloc_rt(rt, 1000);
    CHECK(q == p, "the request did not get the cached block");
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_LargeCacheBytes(rt) == 0, "the cache is not empty after its block was taken");
    CHECK(JS_GetMemorySize(rt) == count0 + cost, "taking a block from the cache changed the count");
    js_free_rt(rt, q);

    /* one class is 64 bytes: 1001 to 1024 share the block, 1025 does not */
    CHECK(js_malloc_rt(rt, 1024) == p, "1024 is in the class of 1000");
    js_free_rt(rt, p);
    q = js_malloc_rt(rt, 1025);
    CHECK(q != p && JS_LargeCacheBlocks(rt) == 1, "1025 is in the next class");
    js_free_rt(rt, q);
    CHECK(JS_LargeCacheBlocks(rt) == 2, "two classes");
    CHECK(js_malloc_rt(rt, 1025) == q && js_malloc_rt(rt, 1001) == p && JS_LargeCacheBlocks(rt) == 0, "each class gives its own block");
    js_free_rt(rt, p);
    js_free_rt(rt, q);
    JS_SetFrontCache(rt, 256);
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_GetMemorySize(rt) == count0, "JS_SetFrontCache gives the cache back");

    /* the smallest and the largest class: 505 to 512 and 4033 to 4096 */
    p = js_malloc_rt(rt, 505);
    q = js_malloc_rt(rt, 4096);
    CHECK(js_malloc_usable_size_rt(rt, p) >= 512 && js_malloc_usable_size_rt(rt, q) >= 4096, "classes at the ends");
    js_free_rt(rt, p);
    js_free_rt(rt, q);
    CHECK(JS_LargeCacheBlocks(rt) == 2, "the ends of the range are cached");
    CHECK(js_malloc_rt(rt, 512) == p && js_malloc_rt(rt, 4033) == q, "the ends of the range are served from the cache");
    js_free_rt(rt, p);
    js_free_rt(rt, q);
    /* a block of 5000 bytes is out of the range: it goes to the heap at once */
    JS_SetFrontCache(rt, 256);
    count0 = JS_GetMemorySize(rt);
    p = js_malloc_rt(rt, 5000);
    js_free_rt(rt, p);
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_GetMemorySize(rt) == count0, "a block over 4159 bytes is not cached");

    /* a class keeps at most per_class blocks, the rest go to the heap */
    for (i = 0; i < 12; i++)
        live[i] = js_malloc_rt(rt, 1000);
    cost = lc_cost(rt, live[0], hdr);
    for (i = 0; i < 12; i++)
        js_free_rt(rt, live[i]);
    CHECK(JS_LargeCacheBlocks(rt) == per_class, "a class holds %zu blocks, not %zu", JS_LargeCacheBlocks(rt), per_class);
    CHECK(JS_GetMemorySize(rt) == count0 + per_class * cost, "the blocks that were not cached still count: %zu, want %zu", JS_GetMemorySize(rt), count0 + per_class * cost);
    CHECK(JS_LargeCacheBytes(rt) == per_class * (cost - 8), "bytes of a full class");
    CHECK(!JS_CheckMemory(rt), "check with a full class");
    JS_SetFrontCache(rt, 256);
    CHECK(JS_GetMemorySize(rt) == count0, "count after the cache was given back");

    /* the whole cache keeps at most cap bytes, however many classes: every
       class is offered per_class blocks, which is more than cap */
    n = 0;
    for (k = 0; k < 57; k++) {
        size_t sz = 505 + (size_t)k * 64;

        for (i = 0; i < (int)per_class; i++) {
            live[n] = js_malloc_rt(rt, sz > 4096 ? 4096 : sz);
            n++;
        }
    }
    {
        size_t offered = 0;

        for (i = 0; i < (int)n; i++)
            offered += lc_cost(rt, live[i], hdr) - 8;
        CHECK(offered > cap, "the test offers %zu bytes to a cache of %zu", offered, cap);
    }
    for (i = 0; i < (int)n; i++)
        js_free_rt(rt, live[i]);
    CHECK(JS_LargeCacheBytes(rt) <= cap, "the cache holds %zu bytes, past its %zu", JS_LargeCacheBytes(rt), cap);
    /* it is full: the biggest block would not fit (a block is at most 4096 bytes and a header) */
    CHECK(cap - JS_LargeCacheBytes(rt) < 4096 + 64, "the cache holds %zu bytes of %zu, and it refused blocks that fit", JS_LargeCacheBytes(rt), cap);
    CHECK(JS_LargeCacheBlocks(rt) < n, "the cache refused no block");
    {
        size_t bytes = JS_LargeCacheBytes(rt), blocks = JS_LargeCacheBlocks(rt);

        CHECK(JS_GetMemorySize(rt) == count0 + bytes + 8 * blocks, "count %zu with %zu bytes in %zu cached blocks", JS_GetMemorySize(rt), bytes, blocks);
    }
    CHECK(!JS_CheckMemory(rt), "check with a full cache");
    /* a block taken from it lowers the bytes by its size */
    {
        size_t bytes = JS_LargeCacheBytes(rt);

        p = js_malloc_rt(rt, 4096);
        q = js_malloc_rt(rt, 505);
        CHECK(JS_LargeCacheBytes(rt) < bytes, "taking blocks did not lower the bytes");
        js_free_rt(rt, p);
        js_free_rt(rt, q);
    }
    JS_SetFrontCache(rt, 0);
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_LargeCacheBytes(rt) == 0 && JS_GetMemorySize(rt) == count0, "turning the cache off did not give it all back");
    JS_SetFrontCache(rt, 256);

    /* a collection and a release give the cache back */
    for (i = 0; i < 5; i++)
        live[i] = js_malloc_rt(rt, 700 + (size_t)i * 70);
    for (i = 0; i < 5; i++)
        js_free_rt(rt, live[i]);
    CHECK(JS_LargeCacheBlocks(rt) == 5, "five cached");
    JS_RunYoungGC(rt);
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_GetMemorySize(rt) == count0, "JS_RunYoungGC did not give the cache back");
    for (i = 0; i < 5; i++)
        live[i] = js_malloc_rt(rt, 700 + (size_t)i * 70);
    for (i = 0; i < 5; i++)
        js_free_rt(rt, live[i]);
    JS_RunGC(rt);
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_GetMemorySize(rt) == count0, "JS_RunGC did not give the cache back");
    for (i = 0; i < 5; i++)
        live[i] = js_malloc_rt(rt, 700 + (size_t)i * 70);
    for (i = 0; i < 5; i++)
        js_free_rt(rt, live[i]);
    lc_release_all(vm, rt);
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_LargeCacheBytes(rt) == 0 && JS_GetMemorySize(rt) == count0, "a release did not give the cache back");
    CHECK(!JS_CheckMemory(rt), "check after the release");

    /* the limit counts the cached blocks, and a request that fits once they
       are given back passes: the heap fails it, the cache goes back, and the
       heap is asked again */
    for (i = 0; i < 8; i++)
        live[i] = js_malloc_rt(rt, 1000);
    for (i = 0; i < 8; i++)
        js_free_rt(rt, live[i]);
    CHECK(JS_LargeCacheBlocks(rt) == 8, "eight cached");
    count0 = JS_GetMemorySize(rt);
    JS_SetMemoryLimit(rt, count0);
    p = js_malloc_rt(rt, 3000);
    CHECK(p != NULL, "a request that fits once the large cache is given back failed");
    CHECK(JS_LargeCacheBlocks(rt) == 0, "the cache was not given back for the retry");
    js_free_rt(rt, p);
    JS_SetFrontCache(rt, 256);
    JS_SetMemoryLimit(rt, (size_t)-1);
    /* a request that fails anyway gives the cache back too, and leaves the count as the cache's return makes it */
    count0 = JS_GetMemorySize(rt);
    for (i = 0; i < 8; i++)
        live[i] = js_malloc_rt(rt, 1000);
    cost = lc_cost(rt, live[0], hdr);
    for (i = 0; i < 8; i++)
        js_free_rt(rt, live[i]);
    JS_SetMemoryLimit(rt, JS_GetMemorySize(rt));
    CHECK(js_malloc_rt(rt, 1 << 20) == NULL, "a request past the limit passed");
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_GetMemorySize(rt) == count0, "a failed request did not give the cache back");
    /* a resize that fails does the same */
    p = js_malloc_rt(rt, 2000);
    memset(p, 0x5a, 2000);
    for (i = 0; i < 8; i++)
        live[i] = js_malloc_rt(rt, 1000);
    for (i = 0; i < 8; i++)
        js_free_rt(rt, live[i]);
    JS_SetMemoryLimit(rt, JS_GetMemorySize(rt));
    CHECK(js_realloc_rt(rt, p, 1 << 20) == NULL, "a resize past the limit passed");
    CHECK(JS_LargeCacheBlocks(rt) == 0, "a failed resize did not give the cache back");
    for (i = 0; i < 2000; i++)
        CHECK(p[i] == 0x5a, "content after a failed resize");
    JS_SetMemoryLimit(rt, (size_t)-1);
    js_free_rt(rt, p);
    JS_SetFrontCache(rt, 256);
    /* a request the cache can serve adds nothing to the count, so the limit
       does not refuse it even when the count is over the limit */
    p = js_malloc_rt(rt, 1000);
    js_free_rt(rt, p);
    JS_SetMemoryLimit(rt, 1);
    q = js_malloc_rt(rt, 1000);
    CHECK(q == p, "a request served from the cache was refused by the limit");
    CHECK(js_malloc_rt(rt, 1000) == NULL, "a request the cache cannot serve passed a limit under the count");
    JS_SetMemoryLimit(rt, (size_t)-1);
    js_free_rt(rt, q);
    JS_SetFrontCache(rt, 256);

    /* a resize: a small block that grows into the range is served from the
       cache, and a large block that is cut under 512 bytes is not cached when
       it is freed. A resized block is filed under the class its size holds */
    p = js_malloc_rt(rt, 1500);
    js_free_rt(rt, p);
    q = js_malloc_rt(rt, 100);
    memset(q, 0x3c, 100);
    q = js_realloc_rt(rt, q, 1500);
    CHECK(q == p, "a small block that grew to 1500 did not take the cached block");
    for (i = 0; i < 100; i++)
        CHECK(q[i] == 0x3c, "content of a block that grew into a cached block");
    js_free_rt(rt, q);
    JS_SetFrontCache(rt, 256);
    count0 = JS_GetMemorySize(rt);
    p = js_malloc_rt(rt, 2000);
    p = js_realloc_rt(rt, p, 100);
    js_free_rt(rt, p);
    CHECK(JS_LargeCacheBlocks(rt) == 0 && JS_GetMemorySize(rt) == count0, "a large block cut to 100 bytes was cached");
    p = js_malloc_rt(rt, 600);
    p = js_realloc_rt(rt, p, 3000);
    n = js_malloc_usable_size_rt(rt, p);
    js_free_rt(rt, p);
    CHECK(JS_LargeCacheBlocks(rt) == 1, "a resized block is not cached");
    CHECK(js_malloc_rt(rt, n / 64 * 64) == p, "a resized block of %zu bytes did not serve the class under it", n);
    js_free_rt(rt, p);
    JS_SetFrontCache(rt, 256);

    /* two runtimes in one process each keep their own cache, and each is bound by the limit */
    {
        uint32_t vm2 = host_open();
        JSRuntime *rt2 = JS_NewRuntime(vm2, 1);
        size_t leaked;

        CHECK(rt2, "JS_NewRuntime");
        for (k = 0; k < 2; k++) {
            JSRuntime *r = k ? rt2 : rt;

            for (n = 0, i = 0; i < 400; i++)
                live[n++] = js_malloc_rt(r, 505 + (size_t)(i % 57) * 64);
            for (i = 0; i < (int)n; i++)
                js_free_rt(r, live[i]);
            CHECK(JS_LargeCacheBytes(r) <= cap && JS_LargeCacheBlocks(r) > 0, "a runtime holds %zu bytes in its cache", JS_LargeCacheBytes(r));
        }
        CHECK(!JS_CheckMemory(rt2), "check of the second runtime");
        /* the cached blocks are not leaks: the end of a runtime gives them back */
        leaked = JS_FreeRuntime(rt2);
        CHECK(leaked == 0, "JS_FreeRuntime reported %zu blocks with a cache", leaked);
        host_close(vm2);
    }
    CHECK(!JS_CheckMemory(rt), "check at the end");
    {
        size_t leaked = JS_FreeRuntime(rt);

        CHECK(leaked == 0, "JS_FreeRuntime reported %zu blocks with a cache", leaked);
    }
    host_close(vm);
    ok("large block cache: classes, limits per class and in bytes, counts, given back by every route, resizes");
}

/* Real objects: a script that makes objects with cycles, closures, strings,
   maps and typed arrays, collections between rounds (the full one and the
   young one), a script that runs out of memory under a limit, and the end of
   the context and the runtime, which must leave no block of the heap. */
static void scenario_js_objects(void)
{
    static const char script[] =
        "(function () {\n"
        "  var keep = [];\n"
        "  for (var i = 0; i < 3000; i++) {\n"
        "    var o = { a: i, b: 'str' + i, c: [i, i + 1, { d: i }], f: function () { return i; } };\n"
        "    o.self = o; o.c.push(o);\n"
        "    if (i % 100 == 0) keep.push(o);\n"
        "  }\n"
        "  var m = new Map(), s = new Set();\n"
        "  for (var i = 0; i < 2000; i++) { m.set('k' + i, { v: i }); s.add(i * 3); }\n"
        "  var t = new Uint8Array(100000); t.fill(7);\n"
        "  var back = JSON.parse(JSON.stringify(keep.map(function (x) { return { a: x.a, b: x.b }; })));\n"
        "  var str = ''; for (var i = 0; i < 2000; i++) str += 'x' + i;\n"
        "  globalThis.result = back.length + m.size + s.size + t.length + str.length;\n"
        "})();\n"
        "globalThis.result";
    static const char hog[] = "var a = []; for (;;) a.push(new Array(1000).fill(1));";
    static const char after[] = "globalThis.a = undefined; 1 + 1";
    uint32_t vm = host_open();
    JSRuntime *rt = JS_NewRuntime(vm, 1);
    JSContext *ctx;
    size_t leaked;

    CHECK(rt, "JS_NewRuntime");
    ctx = JS_NewContext(rt);
    CHECK(ctx, "JS_NewContext");
    for (int round = 0; round < 6; round++) {
        JSValue v = JS_Eval(ctx, script, sizeof script - 1, "<objects>", JS_EVAL_TYPE_GLOBAL);
        int32_t n = 0;

        CHECK(!JS_IsException(v), "the script threw in round %d", round);
        JS_ToInt32(ctx, &n, v);
        CHECK(n == 30 + 2000 + 2000 + 100000 + 8890, "the script gave %d", (int)n);
        JS_FreeValue(ctx, v);
        CHECK(!JS_CheckMemory(rt), "check after round %d", round);
        if (round % 2)
            JS_RunGC(rt);
        else
            JS_RunYoungGC(rt);
        CHECK(JS_FrontCacheBlocks(rt) == 0 && JS_LargeCacheBlocks(rt) == 0, "the caches hold %zu and %zu blocks after a collection", JS_FrontCacheBlocks(rt), JS_LargeCacheBlocks(rt));
        CHECK(!JS_CheckMemory(rt), "check after the collection of round %d", round);
    }
    /* out of memory in the middle of a script: an exception, and the runtime goes on */
    {
        JSValue v;

        JS_RunGC(rt);
        JS_SetMemoryLimit(rt, JS_GetMemorySize(rt) + ((size_t)2 << 20));
        v = JS_Eval(ctx, hog, sizeof hog - 1, "<hog>", JS_EVAL_TYPE_GLOBAL);
        CHECK(JS_IsException(v), "a script past the memory limit did not throw");
        JS_FreeValue(ctx, JS_GetException(ctx));
        JS_FreeValue(ctx, v);
        JS_SetMemoryLimit(rt, (size_t)-1);
        CHECK(!JS_CheckMemory(rt), "check after running out of memory");
        v = JS_Eval(ctx, after, sizeof after - 1, "<after>", JS_EVAL_TYPE_GLOBAL);
        CHECK(!JS_IsException(v), "the runtime does not go on after running out of memory");
        JS_FreeValue(ctx, v);
        JS_RunGC(rt);
        CHECK(!JS_CheckMemory(rt), "check after the collection that follows");
    }
    JS_FreeContext(ctx);
    JS_RunGC(rt);
    CHECK(!JS_CheckMemory(rt), "check after the context is freed");
    leaked = JS_FreeRuntime(rt);
    CHECK(leaked == 0, "JS_FreeRuntime reported %zu blocks after the objects of a script", leaked);
    host_close(vm);
    ok("real objects: cycles, collections, out of memory in a script, no block left");
}
#endif

static void scenarios(void)
{
    scenario_limit();
    scenario_budget();
    scenario_host_refuses();
    scenario_chunks();
    scenario_resize();
    scenario_resize_at_the_byte();
    scenario_dedicated();
    scenario_remap();
    scenario_remap_positions();
    scenario_budget_exact();
    scenario_max_request();
    scenario_release();
    scenario_release_exact();
#ifdef HEAP_TEST_RUNTIME
    scenario_runtime();
    scenario_large_cache();
    scenario_js_objects();
#endif
}

#ifdef HEAP_FUZZ
/* The first byte tells the level: the heap, or with HEAP_TEST_RUNTIME the runtime. */
int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size)
{
    Src r;

    if (size < 2)
        return 0;
    memset(&r, 0, sizeof r);
    r.fuzz = 1;
    r.data = data + 1;
    r.len = size - 1;
    remap_turn = data[0] >> 1; /* the second bit: the host remaps or not */
    move_turn = data[0] >> 2;
#ifdef HEAP_TEST_RUNTIME
    if (data[0] & 1)
        run_runtime(&r, 4000);
    else
#endif
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
    if (argc == 4 && (!strcmp(argv[1], "stress") || !strcmp(argv[1], "stress-runtime"))) {
        Src r;
        uint64_t seed = strtoull(argv[2], NULL, 10);
        long steps = atol(argv[3]);
        char hits[64];

        memset(&r, 0, sizeof r);
        r.s = seed * 0x9E3779B97F4A7C15ull + 88172645463325252ull;
        remap_turn = (int)seed; /* odd seeds: the host remaps. Even seeds: it does not */
        move_turn = (int)(seed >> 1); /* of the odd ones, a host that moves the chunk and one that does not (linux) */
        if (!strcmp(argv[1], "stress")) {
            run_heap(&r, steps);
        } else {
#ifdef HEAP_TEST_RUNTIME
            run_runtime(&r, steps);
#else
            fprintf(stderr, "heap-test: built without the runtime\n");
            return 2;
#endif
        }
        if (steps >= 100000) {
            CHECK(total_releases > 0, "no dedicated chunk was given back in %ld steps", steps);
            if (!strcmp(argv[1], "stress-runtime"))
                CHECK(total_large_hits > 0, "the cache of large blocks served no request in %ld steps", steps);
            if (seed & 1)
                CHECK(total_remaps > 0, "no dedicated chunk was remapped in %ld steps", steps);
            else
                CHECK(total_remaps == 0, "a host that does not remap was asked to");
        }
        printf("PASS  %s seed %" PRIu64 " steps %ld (dedicated chunks given back: %ld, remapped: %ld%s)\n", argv[1], seed, steps, total_releases, total_remaps,
               !strcmp(argv[1], "stress-runtime") ? (snprintf(hits, sizeof hits, ", large cache hits: %ld", total_large_hits), hits) : "");
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
            remap_turn = n > 0 ? buf[0] >> 1 : 0;
            move_turn = n > 0 ? buf[0] >> 2 : 0;
#ifdef HEAP_TEST_RUNTIME
            if (n > 0 && (buf[0] & 1))
                run_runtime(&r, 4000);
            else
#endif
                run_heap(&r, 4000);
            free(buf);
        }
        return 0;
    }
    fprintf(stderr, "usage: heap-test scenarios | stress SEED STEPS | stress-runtime SEED STEPS | fuzz FILE...\n");
    return 2;
}
#endif
