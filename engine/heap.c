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

#include "host.h"
#include "tlsf.h"

#define HEAP_UNIT 65536u                   /* every chunk is a multiple of it */
#define HEAP_CHUNK_MAX ((size_t)8 << 20)   /* the biggest chunk the policy asks for */
#define HEAP_CHUNK_HEADER 32u
#define HEAP_CHUNK_MAGIC 0x68656170u
#define HEAP_SHARED 0u                     /* a chunk under TLSF, with many blocks */
#define HEAP_DEDICATED 1u                  /* a chunk of one block, outside TLSF */
#define HEAP_MIN_RANGE 4096u               /* the smallest inside of a free block worth offering */

struct JSHeapChunk {
    JSHeapChunk *next;
    size_t size;
    uint32_t magic;
    uint32_t kind;     /* HEAP_SHARED or HEAP_DEDICATED: who owns the chunk, set when it is made */
    /* TLSF keeps the prev_phys_block of the first block of the pool right
       before the pool and never uses it, so the pool starts after this word.
       A dedicated chunk has the same layout, see heap_dedicated_set. */
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
    c->kind = HEAP_SHARED;
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

void heap_set_remap(JSHeap *h, int on)
{
    h->remap = on != 0;
}

/* ---- dedicated chunks ----

   A block too big to share a chunk gets a chunk of its own (see heap_grow_block
   for the size). The chunk is not under TLSF: it is the header (32 bytes), the
   size word of the block, the block, and an end marker word, which is how TLSF
   lays out a pool with one block in use, so that tlsf_block_size reads the size
   of the block as for any other. The block fits its request (to 8 bytes) and
   the rest of the chunk is room it can grow into, never lent to other
   blocks. When the block is freed, or moves to another chunk, the chunk goes
   back to the host at once. */

#define HEAP_DEDICATED_START (HEAP_CHUNK_HEADER + 8)   /* the block starts after the header and its size word */
#define HEAP_DEDICATED_OVERHEAD (HEAP_CHUNK_HEADER + 16) /* header, size word, end marker */

static uint8_t *heap_dedicated_block(const JSHeapChunk *c)
{
    return (uint8_t *)c + HEAP_DEDICATED_START;
}

/* the most a block can be in this chunk */
static size_t heap_dedicated_room(const JSHeapChunk *c)
{
    return c->size - HEAP_DEDICATED_OVERHEAD;
}

/* Write the block of size bytes (a multiple of 8, at most the room): its size
   word, the end marker after it, and let nobody touch the room after that. */
static void heap_dedicated_set(JSHeapChunk *c, size_t size)
{
    uint8_t *p = heap_dedicated_block(c);

    *(size_t *)(p - 8) = size;
    heap_unpoison(p, size + 8);
    *(size_t *)(p + size) = 0;
    heap_poison(p + size + 8, heap_dedicated_room(c) - size);
}

/* The dedicated chunk whose block is ptr, or NULL. *link gets the pointer that
   points at it, to take it out of the list. There are few dedicated chunks. */
static JSHeapChunk *heap_dedicated_of(JSHeap *h, const void *ptr, JSHeapChunk ***link)
{
    JSHeapChunk **pp, *c;

    for (pp = &h->dedicated; (c = *pp) != NULL; pp = &c->next) {
        if ((const uint8_t *)ptr == heap_dedicated_block(c)) {
            if (link)
                *link = pp;
            return c;
        }
    }
    return NULL;
}

/* The room a block that is growing gets in its chunk beyond what it needs, so
   that a buffer that grows a little at a time does not get a new chunk (and a
   copy, or a system call) at each step: an eighth of its size. A new block has
   none. The room is footprint only, the block counts by its size. */
#define HEAP_GROW_ROOM(size) ((size) / 8)

/* The size of a dedicated chunk for a block of size bytes (a multiple of 8) with
   room bytes of room after it. When the budget has less than that, the room is
   what is left of it, down to none. 0 when even the block does not fit.
   have is the footprint the chunk adds. */
static size_t heap_dedicated_chunk_size(const JSHeap *h, size_t size, size_t room, size_t have)
{
    size_t chunk = heap_round_unit(size + room + HEAP_DEDICATED_OVERHEAD);
    size_t avail;

    if (!h->footprint_limit)
        return chunk;
    if (h->footprint > h->footprint_limit)
        return 0;
    avail = h->footprint_limit - h->footprint + have;
    if (chunk > avail)
        chunk = heap_round_unit(size + HEAP_DEDICATED_OVERHEAD);
    return chunk > avail ? 0 : chunk;
}

/* A new dedicated chunk for a block of need bytes, with room bytes of room
   after it if the budget allows. The budget applies to it as to any chunk.
   Returns the block, or NULL. */
static void *heap_new_dedicated(JSHeap *h, size_t need, size_t room)
{
    size_t size = (need + 7) & ~(size_t)7;
    size_t chunk = heap_dedicated_chunk_size(h, size, room, 0);
    uintptr_t base;
    JSHeapChunk *c;

    if (!chunk)
        return NULL;
    base = host_memory_grow(h->vm, chunk);
    if (!base)
        return NULL;
    assert((base & 7) == 0);
    c = (JSHeapChunk *)base;
    c->next = h->dedicated;
    c->size = chunk;
    c->magic = HEAP_CHUNK_MAGIC;
    c->kind = HEAP_DEDICATED;
    c->reserved = 0;
    heap_dedicated_set(c, size);
    h->dedicated = c;
    h->footprint += chunk;
    h->chunk_count++;
    return heap_dedicated_block(c);
}

/* The chunk c is out of the heap: its block is freed or has moved. Take it out
   of the lists and the counts, then give it to the host. Save what is needed
   first: the chunk is gone when the call returns, and nothing here reads it
   again. link is the pointer that points at c in the list. */
static void heap_give_back(JSHeap *h, JSHeapChunk *c, JSHeapChunk **link)
{
    JSValue argv[2];
    uintptr_t addr = (uintptr_t)c;
    size_t size = c->size;

    *link = c->next;
    h->footprint -= size;
    h->chunk_count--;
    argv[0] = HOST_WORD(addr);
    argv[1] = HOST_WORD(size);
    host_call(h->vm, NULL, 2, argv, HOST_CALL_MEMORY_RELEASE);
}

/* ---- shared chunks ---- */

/* A block of need bytes from a new chunk, or NULL. The size comes from the
   policy: a quarter of the footprint, 64 KiB to 8 MiB. A request of more than
   half of that gets a dedicated chunk. Otherwise the chunk is shared: near the
   footprint budget it shrinks to what the request needs (it holds the
   request, with the room TLSF needs to find it, it rounds a request up by at
   most 1/32), and when even that does not fit, nothing is asked. */
static void *heap_grow_block(JSHeap *h, size_t need, size_t room)
{
    size_t overhead = HEAP_CHUNK_HEADER + tlsf_pool_overhead();
    size_t own, chunk, avail;
    uintptr_t base;
    JSHeapChunk *c, **pp;
    void *p;

    own = heap_round_unit(need + need / 32 + overhead + 64);
    chunk = heap_round_unit(h->footprint / 4);
    if (chunk < HEAP_UNIT)
        chunk = HEAP_UNIT;
    if (chunk > HEAP_CHUNK_MAX)
        chunk = HEAP_CHUNK_MAX;
    if (need + need / 32 + 64 > (chunk - overhead) / 2)
        return heap_new_dedicated(h, need, room);
    if (h->footprint_limit) {
        if (h->footprint > h->footprint_limit)
            return NULL;
        avail = h->footprint_limit - h->footprint;
        if (chunk > avail)
            chunk = own;
        if (chunk > avail)
            return NULL;
    }
    base = host_memory_grow(h->vm, chunk);
    if (!base)
        return NULL;
    assert((base & 7) == 0);
    c = (JSHeapChunk *)base;
    c->next = NULL;
    c->size = chunk;
    c->magic = HEAP_CHUNK_MAGIC;
    c->kind = HEAP_SHARED;
    c->reserved = 0;
    if (!tlsf_add_pool(h->tlsf, (char *)base + HEAP_CHUNK_HEADER, chunk - HEAP_CHUNK_HEADER)) {
        /* cannot happen for the sizes above: the host drops the chunk when it closes the VM */
        return NULL;
    }
    heap_poison((char *)base + HEAP_CHUNK_HEADER, chunk - HEAP_CHUNK_HEADER);
    for (pp = &h->chunks; *pp; pp = &(*pp)->next)
        ;
    *pp = c;
    h->footprint += chunk;
    h->chunk_count++;
    p = tlsf_malloc(h->tlsf, need);
    assert(p);
    return p;
}

/* A block of at least size bytes, from the chunks the heap has or from a new
   one. Nothing is counted: that is for the caller. */
static void *heap_take(JSHeap *h, size_t size, size_t room)
{
    void *p = tlsf_malloc(h->tlsf, size);

    return p ? p : heap_grow_block(h, size, room);
}

void *heap_alloc(JSHeap *h, size_t size)
{
    void *p;

    if (size == 0 || size > JS_HEAP_MAX_REQUEST || heap_over(h, h->count, size))
        return NULL;
    p = heap_take(h, size, 0);
    if (!p)
        return NULL;
    heap_unpoison(p, tlsf_block_size(p));
    h->count += heap_block_cost(p);
    h->blocks++;
    return p;
}

void heap_free(JSHeap *h, void *ptr)
{
    JSHeapChunk *c, **link;
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
    c = heap_dedicated_of(h, ptr, &link);
    if (c) {
        heap_give_back(h, c, link);
        return;
    }
    heap_poison(ptr, tlsf_block_size(ptr));
    tlsf_free(h->tlsf, ptr);
}

/* The block of the dedicated chunk c outgrows it, and the host remaps: ask for
   a bigger chunk, with the content, where it is or elsewhere. The budget is
   checked first, for the growth alone (the old chunk is replaced, not kept).
   The host refusing is out of memory, not a reason to allocate and copy, which
   takes more: nothing changes then, and the caller gets NULL. The chunk may
   have another address afterwards, and the header the old one had is gone:
   what is needed is saved before the call, and the list gets the new address.
   link is the pointer that points at c in the list. */
static void *heap_remap_dedicated(JSHeap *h, JSHeapChunk *c, JSHeapChunk **link, size_t size, size_t base)
{
    size_t new_size = (size + 7) & ~(size_t)7;
    size_t old_chunk = c->size;
    size_t chunk = heap_dedicated_chunk_size(h, new_size, HEAP_GROW_ROOM(new_size), old_chunk);
    JSValue argv[3], ret;
    uintptr_t addr;

    if (!chunk)
        return NULL;
    assert(chunk > old_chunk);
    argv[0] = HOST_WORD((uintptr_t)c);
    argv[1] = HOST_WORD(old_chunk);
    argv[2] = HOST_WORD(chunk);
    ret = host_call(h->vm, NULL, 3, argv, HOST_CALL_MEMORY_REMAP);
    addr = HOST_WORD_VALUE(ret);
    if (!addr)
        return NULL;
    assert((addr & 7) == 0);
    c = (JSHeapChunk *)addr;
    *link = c;
    c->size = chunk;
    h->footprint += chunk - old_chunk;
    heap_dedicated_set(c, new_size);
    h->count = base + new_size;
    return heap_dedicated_block(c);
}

/* A block may grow into a free neighbor, shrink, or move. The count is checked
   for the size after the resize, as upstream checks it: count + the request -
   the usable size of the old block, against the limit (the old block does not
   count twice), and it is committed by the difference once the resize is
   done. A block that has to move needs room for both copies while the data
   moves: when the chunks have none, a new one is mapped, and the budget
   applies to it. A block that moves away from a dedicated chunk gives the
   chunk to the host once the data is in the new block. A failure leaves the
   old block, its content and the count of the heap alone. */
void *heap_realloc(JSHeap *h, void *ptr, size_t size)
{
    size_t old_size, new_size, base;
    JSHeapChunk *c, **link;
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
    c = heap_dedicated_of(h, ptr, &link);
    if (c && size <= heap_dedicated_room(c) && size >= heap_dedicated_room(c) / 2) {
        /* the block stays where it is, as big as the request needs */
        new_size = (size + 7) & ~(size_t)7;
        heap_dedicated_set(c, new_size);
        h->count = base + new_size;
        return ptr;
    }
    if (c && size > heap_dedicated_room(c) && h->remap)
        return heap_remap_dedicated(h, c, link, size, base);
    p = c ? NULL : tlsf_realloc_inplace(h->tlsf, ptr, size);
    if (p) {
        new_size = tlsf_block_size(p);
        if (new_size > old_size)
            heap_unpoison((char *)p + old_size, new_size - old_size);
        else if (new_size < old_size)
            heap_poison((char *)p + new_size, old_size - new_size);
    } else {
        p = heap_take(h, size, HEAP_GROW_ROOM(size));
        if (!p)
            return NULL;
        new_size = tlsf_block_size(p);
        heap_unpoison(p, new_size);
        memcpy(p, ptr, old_size < size ? old_size : size);
        h->count = base + new_size;
        if (c) {
            /* the chunk list may have a new head: find the link again */
            c = heap_dedicated_of(h, ptr, &link);
            heap_give_back(h, c, link);
        } else {
            heap_poison(ptr, old_size);
            tlsf_free(h->tlsf, ptr);
        }
        return p;
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
    for (c = h->dedicated; c; c = c->next) {
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

size_t heap_dedicated_size(const JSHeap *h, const void *ptr)
{
    const JSHeapChunk *c;

    for (c = h->dedicated; c; c = c->next)
        if ((const uint8_t *)ptr == heap_dedicated_block(c))
            return c->size;
    return 0;
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
        if (c->magic != HEAP_CHUNK_MAGIC || c->kind != HEAP_SHARED)
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
    for (c = h->dedicated; c; c = c->next) {
        const uint8_t *p = heap_dedicated_block(c);
        size_t size;

        if (c->magic != HEAP_CHUNK_MAGIC || c->kind != HEAP_DEDICATED)
            return "dedicated chunk header overwritten";
        if (c->size < HEAP_UNIT || c->size % HEAP_UNIT)
            return "dedicated chunk size is not a multiple of 64 KiB";
        size = *(const size_t *)(p - 8);
        if (size == 0 || size % 8 || size > heap_dedicated_room(c))
            return "the block of a dedicated chunk has a wrong size";
        if (*(const size_t *)(p + size) != 0)
            return "the end marker of a dedicated chunk was overwritten";
        footprint += c->size;
        chunks++;
        w.used_blocks++;
        w.used_bytes += size + tlsf_alloc_overhead();
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
