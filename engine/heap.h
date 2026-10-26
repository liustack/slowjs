/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef JS_HEAP_H
#define JS_HEAP_H

#include <stddef.h>
#include <stdint.h>

/* The memory of a VM: chunks the host maps for it, managed by TLSF (tlsf.c),
   counted and limited here. A block too big to share a chunk gets a chunk of
   its own, a dedicated chunk, which is not under TLSF: it goes back to the host
   as soon as its block is freed or moved (host_call, see host.h). quickjs.c sits on top (small blocks in arenas, the
   front cache). The state of a heap is the start of its JSRuntime, which lives
   in the first chunk, so there is no global. See docs/allocator.md. */

/* The one thing the heap asks of its host: size bytes (a multiple of 64 KiB)
   of read and write memory for the VM vm, aligned to the page size and not
   necessarily zeroed, or 0 when the system has none. It never calls back into
   the engine. */
uintptr_t host_memory_grow(uint32_t vm, uintptr_t size);

/* A range of memory handed back to the host. */
typedef struct JSMemRange {
    uintptr_t addr;
    size_t len;
} JSMemRange;

/* The largest request the heap serves: TLSF's. */
#define JS_HEAP_MAX_REQUEST ((((size_t)1) << 32) - 8)

typedef struct JSHeapChunk JSHeapChunk;

typedef struct JSHeap {
    void *tlsf;                /* the TLSF control, behind the runtime in the first chunk */
    JSHeapChunk *chunks;       /* the shared chunks, the first one first. It holds the runtime and cannot be released */
    JSHeapChunk *dedicated;    /* the dedicated chunks: one block each, outside TLSF */
    size_t rt_size;            /* the runtime, which is part of count and not a block */
    size_t count;              /* the runtime and the blocks in use, and for each block its size and the 8 byte size word before it: MemorySize, what the limit and the GC count */
    size_t limit;              /* (size_t)-1 for none */
    size_t blocks;             /* blocks in use, the runtime not counted */
    size_t footprint;          /* the sum of the chunks, shared and dedicated */
    size_t footprint_limit;    /* 0 for none. Only stops growth, never takes memory back */
    size_t chunk_count;        /* shared and dedicated */
    size_t release_candidate;  /* bytes offered to the host so far */
    size_t release_target;     /* the release in progress */
    size_t release_offered;
    void *release_prev;
    uint32_t vm;
} JSHeap;

/* Start a heap for the VM vm: map the first chunk and put in it a zeroed
   block of rt_size bytes, which is the JSHeap and the rest of the runtime
   (the heap is its first field). Returns the heap, or NULL when the host
   gives no memory. Nothing is freed on failure: the host drops its chunks
   when it closes the VM. */
JSHeap *heap_create(uint32_t vm, size_t rt_size);

/* The end of the heap. The host takes the chunks back after this. */
void heap_destroy(JSHeap *h);

/* Like malloc, free and realloc, for a size of at least 1 and at most
   JS_HEAP_MAX_REQUEST. They fail (NULL) when count plus the size would pass
   the limit, when a new chunk is needed and the budget or the host refuses
   it, and for a size over the maximum. A failed heap_realloc leaves the block,
   its content and the count as they were. A block is 8 byte aligned. A block
   too big to share a chunk gets a dedicated chunk, and the chunk goes back to the
   host at once when the block is freed or moved by heap_realloc. */
void *heap_alloc(JSHeap *h, size_t size);
void heap_free(JSHeap *h, void *ptr);
void *heap_realloc(JSHeap *h, void *ptr, size_t size);

/* The size of the block, at least what was asked for. */
size_t heap_usable_size(const void *ptr);

/* Give memory back. heap_release_start(keep) plans to offer the free memory
   but keep bytes of it. Then heap_release_next_chunk returns whole free chunks
   (taken out of the heap, never the first) until its length is 0, and
   heap_release_next_range returns the inside of free blocks, which the host
   may drop the pages of, until its length is 0. The host does not touch the
   heap in between, and unmaps a chunk or drops the pages of a range as it
   likes. A range may be offered again by a later release. */
void heap_release_start(JSHeap *h, size_t keep);
JSMemRange heap_release_next_chunk(JSHeap *h);
JSMemRange heap_release_next_range(JSHeap *h);

/* Test builds (-DJS_HEAP_CHECK). heap_contains tells whether ptr lies in a
   chunk of the heap. heap_check walks the heap and returns 0 when TLSF is
   consistent and the counts, the blocks and the footprint are what the
   blocks and chunks add up to. Otherwise it returns a message. */
#ifdef JS_HEAP_CHECK
int heap_contains(const JSHeap *h, const void *ptr);
const char *heap_check(const JSHeap *h);
#endif

/* Test builds with AddressSanitizer (-DJS_HEAP_POISON): memory nobody may
   touch is poisoned, so that a use after free inside a chunk or an arena is
   found. */
#ifdef JS_HEAP_POISON
void __asan_poison_memory_region(void const volatile *addr, size_t size);
void __asan_unpoison_memory_region(void const volatile *addr, size_t size);
#define heap_poison(p, n) __asan_poison_memory_region((p), (n))
#define heap_unpoison(p, n) __asan_unpoison_memory_region((p), (n))
#else
#define heap_poison(p, n) ((void)0)
#define heap_unpoison(p, n) ((void)0)
#endif

#endif
