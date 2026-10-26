/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* heap.h on the system malloc, for the sanitizer builds that want every block
   alone: red zones between blocks and LeakSanitizer see each of them. It is
   built in place of heap.c and tlsf.c (build.sh asan and asan-ubsan), with the
   system's headers, and counts as heap.c does: a block costs its usable size
   and 8 bytes. There are no chunks, so the footprint stays 0 and nothing can
   be released. quickjs.c is built with JS_HEAP_SYSTEM_MALLOC, which sends every
   block here. */
#include <stdlib.h>
#if defined(__APPLE__)
#include <malloc/malloc.h>
#else
#include <malloc.h>
#endif

#include "heap.h"

#define HEAP_BLOCK_OVERHEAD 8

size_t heap_usable_size(const void *ptr)
{
#if defined(__APPLE__)
    return malloc_size(ptr);
#else
    return malloc_usable_size((void *)ptr);
#endif
}

static int heap_over(const JSHeap *h, size_t base, size_t request)
{
    return base > h->limit || request > h->limit - base;
}

JSHeap *heap_create(uint32_t vm, size_t rt_size)
{
    JSHeap *h = calloc(1, rt_size);
    if (!h)
        return NULL;
    h->vm = vm;
    h->limit = (size_t)-1;
    h->rt_size = rt_size;
    h->count = rt_size;
    return h;
}

void heap_destroy(JSHeap *h)
{
    free(h);
}

/* the system malloc has no chunks to remap */
void heap_set_remap(JSHeap *h, int on)
{
    (void)h;
    (void)on;
}

void *heap_alloc(JSHeap *h, size_t size)
{
    void *p;

    if (size == 0 || size > JS_HEAP_MAX_REQUEST || heap_over(h, h->count, size))
        return NULL;
    p = malloc(size);
    if (!p)
        return NULL;
    h->count += heap_usable_size(p) + HEAP_BLOCK_OVERHEAD;
    h->blocks++;
    return p;
}

void heap_free(JSHeap *h, void *ptr)
{
    if (!ptr)
        return;
    h->count -= heap_usable_size(ptr) + HEAP_BLOCK_OVERHEAD;
    h->blocks--;
    free(ptr);
}

/* The count is checked as upstream checks a resize, count + the request - the
   usable size of the old block, against the limit (the 8 bytes of the block
   stay in the count, the block is still a block after the resize), and it
   changes by the difference once the system has resized the block. */
void *heap_realloc(JSHeap *h, void *ptr, size_t size)
{
    size_t old_size;
    void *p;

    if (!ptr)
        return heap_alloc(h, size);
    if (size == 0) {
        heap_free(h, ptr);
        return NULL;
    }
    if (size > JS_HEAP_MAX_REQUEST)
        return NULL;
    old_size = heap_usable_size(ptr);
    if (heap_over(h, h->count - old_size, size))
        return NULL;
    p = realloc(ptr, size);
    if (!p)
        return NULL;
    h->count = h->count - old_size + heap_usable_size(p);
    return p;
}

void heap_release_start(JSHeap *h, size_t keep)
{
    (void)h;
    (void)keep;
}

JSMemRange heap_release_next_chunk(JSHeap *h)
{
    JSMemRange none = { 0, 0 };
    (void)h;
    return none;
}

JSMemRange heap_release_next_range(JSHeap *h)
{
    JSMemRange none = { 0, 0 };
    (void)h;
    return none;
}
