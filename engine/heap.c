/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#include <assert.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include "heap.h"

#include "tlsf.h"

#define HEAP_UNIT 65536u                   /* every chunk is a multiple of it */
#define HEAP_CHUNK_MAX ((size_t)8 << 20)   /* the biggest chunk the policy asks for */
#define HEAP_CHUNK_HEADER 32u
#define HEAP_CHUNK_MAGIC ((uintptr_t)0x68656170636b6e6bull)
#define HEAP_MIN_RANGE 4096u               /* the smallest inside of a free block worth offering */

struct JSHeapChunk {
    JSHeapChunk *next;
    size_t size;
    uintptr_t magic;
    /* TLSF keeps the prev_phys_block of the first block of the pool right
       before the pool and never uses it, so the pool starts after this word. */
    uintptr_t reserved;
};

_Static_assert(sizeof(JSHeapChunk) == HEAP_CHUNK_HEADER, "chunk header size");
_Static_assert(sizeof(size_t) == 8, "the heap is for 64 bit targets");

static size_t heap_round_unit(size_t n)
{
    return (n + (HEAP_UNIT - 1)) & ~(size_t)(HEAP_UNIT - 1);
}

/* What a block in use costs in count and MemorySize: its size and the size
   word TLSF keeps in front of it. */
static size_t heap_block_cost(const void *ptr)
{
    return tlsf_block_size((void *)ptr) + tlsf_alloc_overhead();
}

size_t heap_usable_size(const void *ptr)
{
    return tlsf_block_size((void *)ptr);
}

/* Would count become base plus request, past the limit? The limit may be
   below the count (it can be lowered), and request may be huge: subtract. */
static int heap_over(const JSHeap *h, size_t base, size_t request)
{
    return base > h->limit || request > h->limit - base;
}

JSHeap *heap_create(uint32_t vm, size_t rt_size)
{
    uintptr_t base;
    JSHeapChunk *c;
    JSHeap *h;
    char *control;
    void *tlsf;

    base = host_memory_grow(vm, HEAP_UNIT);
    if (!base)
        return NULL;
    assert((base & 7) == 0);
    rt_size = (rt_size + 7) & ~(size_t)7;
    assert(HEAP_CHUNK_HEADER + rt_size + tlsf_size() + 4096 <= HEAP_UNIT);
    c = (JSHeapChunk *)base;
    c->next = NULL;
    c->size = HEAP_UNIT;
    c->magic = HEAP_CHUNK_MAGIC;
    c->reserved = 0;
    h = (JSHeap *)(base + HEAP_CHUNK_HEADER);
    memset(h, 0, rt_size);
    control = (char *)h + rt_size;
    tlsf = tlsf_create_with_pool(control, (char *)(base + HEAP_UNIT) - control);
    if (!tlsf)
        return NULL;
    /* the pool is the free memory after the control: nobody may touch it */
    heap_poison(tlsf_get_pool(tlsf), (char *)(base + HEAP_UNIT) - (char *)tlsf_get_pool(tlsf));
    h->tlsf = tlsf;
    h->chunks = c;
    h->chunk_count = 1;
    h->footprint = HEAP_UNIT;
    h->limit = (size_t)-1;
    h->rt_size = rt_size;
    h->count = rt_size;
    h->vm = vm;
    return h;
}

void heap_destroy(JSHeap *h)
{
    (void)h;
}

/* Ask the host for a chunk that serves a request of need bytes. The size
   comes from the policy: a quarter of the footprint, 64 KiB to 8 MiB. A
   request of more than half of that gets a chunk of its own, with the room
   TLSF needs to find it (it rounds a request up by at most 1/32). Near the
   footprint budget the chunk shrinks to what the request needs, and when
   even that does not fit, nothing is asked. */
static int heap_grow(JSHeap *h, size_t need)
{
    size_t overhead = HEAP_CHUNK_HEADER + tlsf_pool_overhead();
    size_t own, chunk, avail;
    uintptr_t base;
    JSHeapChunk *c, **pp;

    own = heap_round_unit(need + need / 32 + overhead + 64);
    chunk = heap_round_unit(h->footprint / 4);
    if (chunk < HEAP_UNIT)
        chunk = HEAP_UNIT;
    if (chunk > HEAP_CHUNK_MAX)
        chunk = HEAP_CHUNK_MAX;
    if (need + need / 32 + 64 > (chunk - overhead) / 2)
        chunk = own;
    if (h->footprint_limit) {
        if (h->footprint > h->footprint_limit)
            return 0;
        avail = h->footprint_limit - h->footprint;
        if (chunk > avail)
            chunk = own;
        if (chunk > avail)
            return 0;
    }
    if (chunk - overhead >= tlsf_block_size_max())
        return 0;
    base = host_memory_grow(h->vm, chunk);
    if (!base)
        return 0;
    assert((base & 7) == 0);
    c = (JSHeapChunk *)base;
    c->next = NULL;
    c->size = chunk;
    c->magic = HEAP_CHUNK_MAGIC;
    c->reserved = 0;
    if (!tlsf_add_pool(h->tlsf, (char *)base + HEAP_CHUNK_HEADER, chunk - HEAP_CHUNK_HEADER)) {
        /* cannot happen for the sizes above: the host drops the chunk when it closes the VM */
        return 0;
    }
    heap_poison((char *)base + HEAP_CHUNK_HEADER, chunk - HEAP_CHUNK_HEADER);
    for (pp = &h->chunks; *pp; pp = &(*pp)->next)
        ;
    *pp = c;
    h->footprint += chunk;
    h->chunk_count++;
    return 1;
}

void *heap_alloc(JSHeap *h, size_t size)
{
    void *p;

    if (size == 0 || size > JS_HEAP_MAX_REQUEST || heap_over(h, h->count, size))
        return NULL;
    p = tlsf_malloc(h->tlsf, size);
    if (!p) {
        if (!heap_grow(h, size))
            return NULL;
        p = tlsf_malloc(h->tlsf, size);
        assert(p);
        if (!p)
            return NULL;
    }
    heap_unpoison(p, tlsf_block_size(p));
    h->count += heap_block_cost(p);
    h->blocks++;
    return p;
}

void heap_free(JSHeap *h, void *ptr)
{
    size_t cost;

    if (!ptr)
        return;
#ifdef JS_HEAP_CHECK
    assert(heap_contains(h, ptr));
#endif
    cost = heap_block_cost(ptr);
    assert(h->blocks > 0 && h->count >= cost);
    h->count -= cost;
    h->blocks--;
    heap_poison(ptr, tlsf_block_size(ptr));
    tlsf_free(h->tlsf, ptr);
}

/* A block may grow into a free neighbor, shrink, or move. The count is checked
   for the size after the resize, as upstream checks it: count + the request -
   the usable size of the old block, against the limit (the old block does not
   count twice), and it is committed by the difference once the resize is
   done. A block that has to move needs room for both copies in the chunks
   while the data moves: when the chunks have none, a new one is mapped, and
   the budget applies to it. A failure leaves the old block, its content and
   the count of the heap alone. */
void *heap_realloc(JSHeap *h, void *ptr, size_t size)
{
    size_t old_size, new_size, base;
    void *p;

    if (!ptr)
        return heap_alloc(h, size);
    if (size == 0) {
        heap_free(h, ptr);
        return NULL;
    }
    if (size > JS_HEAP_MAX_REQUEST)
        return NULL;
#ifdef JS_HEAP_CHECK
    assert(heap_contains(h, ptr));
#endif
    old_size = tlsf_block_size(ptr);
    /* count + request - the usable size of the old block, against the limit,
       as upstream checks a resize: the 8 byte size word of the block stays in
       count, since the block is still a block after the resize */
    base = h->count - old_size;
    if (heap_over(h, base, size))
        return NULL;
    p = tlsf_realloc_inplace(h->tlsf, ptr, size);
    if (p) {
        new_size = tlsf_block_size(p);
        if (new_size > old_size)
            heap_unpoison((char *)p + old_size, new_size - old_size);
        else if (new_size < old_size)
            heap_poison((char *)p + new_size, old_size - new_size);
    } else {
        p = tlsf_malloc(h->tlsf, size);
        if (!p) {
            if (!heap_grow(h, size))
                return NULL;
            p = tlsf_malloc(h->tlsf, size);
            assert(p);
            if (!p)
                return NULL;
        }
        new_size = tlsf_block_size(p);
        heap_unpoison(p, new_size);
        memcpy(p, ptr, old_size < size ? old_size : size);
        heap_poison(ptr, old_size);
        tlsf_free(h->tlsf, ptr);
    }
    h->count = base + new_size;
    return p;
}

void heap_release_start(JSHeap *h, size_t keep)
{
    size_t size, total = 0;
    void *p = NULL;

    while ((p = tlsf_next_free(h->tlsf, p, &size)) != NULL)
        total += size;
    h->release_target = total > keep ? total - keep : 0;
    h->release_offered = 0;
    h->release_prev = NULL;
}

JSMemRange heap_release_next_chunk(JSHeap *h)
{
    JSMemRange r = { 0, 0 };
    JSHeapChunk *c, *best = NULL, **pp, **best_pp = NULL;

    if (h->release_offered >= h->release_target)
        return r;
    /* the largest chunk that is one free block, not the first one */
    for (pp = &h->chunks->next; (c = *pp) != NULL; pp = &c->next) {
        if (tlsf_pool_is_free((char *)c + HEAP_CHUNK_HEADER) && (!best || c->size > best->size)) {
            best = c;
            best_pp = pp;
        }
    }
    if (!best)
        return r;
    tlsf_remove_pool(h->tlsf, (char *)best + HEAP_CHUNK_HEADER);
    *best_pp = best->next;
    h->footprint -= best->size;
    h->chunk_count--;
    h->release_offered += best->size;
    h->release_candidate += best->size;
    r.addr = (uintptr_t)best;
    r.len = best->size;
    return r;
}

JSMemRange heap_release_next_range(JSHeap *h)
{
    JSMemRange r = { 0, 0 };
    size_t size, len;
    void *p, *start;

    while (h->release_offered < h->release_target) {
        p = tlsf_next_free(h->tlsf, h->release_prev, &size);
        h->release_prev = p;
        if (!p) {
            h->release_target = h->release_offered = 0;
            break;
        }
        start = tlsf_free_inside(p, size, &len);
        if (len < HEAP_MIN_RANGE)
            continue;
        h->release_offered += len;
        h->release_candidate += len;
        r.addr = (uintptr_t)start;
        r.len = len;
        return r;
    }
    return r;
}

#ifdef JS_HEAP_CHECK

int heap_contains(const JSHeap *h, const void *ptr)
{
    const JSHeapChunk *c;

    for (c = h->chunks; c; c = c->next) {
        if ((uintptr_t)ptr >= (uintptr_t)c + HEAP_CHUNK_HEADER &&
            (uintptr_t)ptr < (uintptr_t)c + c->size)
            return 1;
    }
    return 0;
}

typedef struct HeapWalk {
    size_t used_bytes, used_blocks;
} HeapWalk;

static void heap_walker(void *ptr, size_t size, int used, void *user)
{
    HeapWalk *w = user;

    (void)ptr;
    if (used) {
        w->used_bytes += size + tlsf_alloc_overhead();
        w->used_blocks++;
    }
}

const char *heap_check(const JSHeap *h)
{
    const JSHeapChunk *c;
    HeapWalk w = { 0, 0 };
    size_t footprint = 0, chunks = 0;
    void *pool;

    if (tlsf_check(h->tlsf))
        return "tlsf_check failed";
    for (c = h->chunks; c; c = c->next) {
        if (c->magic != HEAP_CHUNK_MAGIC)
            return "chunk header overwritten";
        if (c->size < HEAP_UNIT || c->size % HEAP_UNIT)
            return "chunk size is not a multiple of 64 KiB";
        footprint += c->size;
        chunks++;
        pool = c == h->chunks ? tlsf_get_pool(h->tlsf) : (void *)((char *)c + HEAP_CHUNK_HEADER);
        if (tlsf_check_pool(pool))
            return "tlsf_check_pool failed";
        tlsf_walk_pool(pool, heap_walker, &w);
    }
    if (footprint != h->footprint)
        return "footprint is not the sum of the chunks";
    if (chunks != h->chunk_count)
        return "chunk_count is not the number of chunks";
    if (w.used_blocks != h->blocks)
        return "blocks is not the number of blocks in use";
    if (h->count != h->rt_size + w.used_bytes)
        return "count is not the runtime and the blocks in use";
    return 0;
}

#endif /* JS_HEAP_CHECK */
