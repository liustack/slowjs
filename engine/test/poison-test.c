/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/*
 * What the poison of build.sh asan-heap covers. In that mode the engine's own
 * allocator marks free memory as poisoned for AddressSanitizer, and this
 * program asks AddressSanitizer which bytes are poisoned (it does not touch
 * them, so nothing is reported). It checks both what must be poisoned and
 * what must stay readable, because the second is where the poison stops:
 *
 *   poisoned:  the user bytes of a free small block (in its arena and in the
 *              front cache but for the first 8 bytes), a freed large block,
 *              the memory of an arena that was given back, the tail a resize
 *              cut off, the old block after a resize that moved
 *   readable:  a live block up to its usable size, the first 8 bytes of a
 *              block in the front cache (they hold the link), the header of
 *              the slot after a live small block (the allocator reads and
 *              writes it), a block that was taken from the cache or the arena
 *              again, the tail a resize grew into
 *
 * So an access that stays within the 8 bytes after a live small block, or that
 * goes into another live block, is not found here. build.sh asan, with every
 * block alone from the system malloc and red zones around each, finds those,
 * and has no front cache and no arenas. Both modes run.
 */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "host.h"

int __asan_address_is_poisoned(void const volatile *addr);

static int failures;
static long checks;

#define CHECK(cond, ...) do { \
        checks++; \
        if (!(cond)) { \
            if (failures++ < 20) { \
                fprintf(stderr, "poison-test: %s:%d: ", __FILE__, __LINE__); \
                fprintf(stderr, __VA_ARGS__); \
                fputc('\n', stderr); \
            } \
        } \
    } while (0)

static int poisoned(const void *p)
{
    return __asan_address_is_poisoned(p) != 0;
}

/* how many of the n bytes at p are poisoned */
static size_t count_poisoned(const void *p, size_t n)
{
    size_t i, k = 0;

    for (i = 0; i < n; i++)
        k += poisoned((const uint8_t *)p + i);
    return k;
}

#define ALL_POISONED(p, n, what) \
    CHECK(count_poisoned((p), (n)) == (n), "%s: %zu of %zu bytes poisoned", what, count_poisoned((p), (n)), (size_t)(n))
#define NONE_POISONED(p, n, what) \
    CHECK(count_poisoned((p), (n)) == 0, "%s: %zu of %zu bytes poisoned", what, count_poisoned((p), (n)), (size_t)(n))

/* more blocks than an arena of the smallest class holds (253), so that part of
   them are in arenas of their own */
enum { K = 400 };
/* fewer than a list of the front cache holds (256), so that all of them wait there */
enum { KC = 200 };
static const size_t small_sizes[] = { 1, 8, 9, 24, 100, 248, 504 };
#define NSMALL (sizeof small_sizes / sizeof *small_sizes)

/* With the front cache off, a freed small block goes straight back to its
   arena. */
static void test_small_in_arena(JSRuntime *rt)
{
    size_t s;

    JS_SetFrontCache(rt, 0);
    for (s = 0; s < NSMALL; s++) {
        size_t n = small_sizes[s], u, pairs = 0, headers = 0;
        uint8_t *p[K];
        int i, j;

        for (i = 0; i < K; i++)
            p[i] = js_malloc_rt(rt, n);
        u = js_malloc_usable_size_rt(rt, p[0]);
        for (i = 0; i < K; i++)
            NONE_POISONED(p[i], u, "a live small block");
        for (i = 1; i < K; i += 2)
            js_free_rt(rt, p[i]);
        for (i = 1; i < K; i += 2)
            ALL_POISONED(p[i], u, "a free small block in its arena");
        /* a live block with a free one right after it in the arena: the 8
           bytes between them are the header of the free slot */
        for (i = 0; i < K; i += 2) {
            for (j = 1; j < K; j += 2) {
                if (p[j] == p[i] + u + 8) {
                    pairs++;
                    CHECK(!poisoned(p[i] + u) && !poisoned(p[i] + u + 7),
                          "the header of the slot after a live block (size %zu) is poisoned", n);
                    ALL_POISONED(p[j], u, "the free slot after a live block");
                }
            }
        }
        CHECK(pairs > 0, "size %zu: no live block with a free slot after it, the test checked nothing", n);
        /* a freed slot is taken again and readable */
        for (i = 1; i < K; i += 2) {
            p[i] = js_malloc_rt(rt, n);
            NONE_POISONED(p[i], u, "a small block taken from its arena again");
        }
        for (i = 0; i < K; i++)
            js_free_rt(rt, p[i]);
        /* the arenas that are empty go back to the heap, which poisons them
           whole, the headers of the slots too */
        for (i = 0; i < K; i++)
            headers += poisoned(p[i] - 8);
        CHECK(headers > 0, "size %zu: no empty arena was given back and poisoned", n);
        for (i = 0; i < K; i++)
            p[i] = js_malloc_rt(rt, n);
        for (i = 0; i < K; i++) {
            NONE_POISONED(p[i], u, "a small block in an arena taken from the heap again");
            CHECK(!poisoned(p[i] - 8), "the header of a slot in an arena taken from the heap again is poisoned");
        }
        for (i = 0; i < K; i++)
            js_free_rt(rt, p[i]);
    }
}

/* With the front cache on, a freed small block waits in its list, with a link
   in its first 8 bytes. */
static void test_small_in_cache(JSRuntime *rt)
{
    size_t s;

    JS_SetFrontCache(rt, 256);
    for (s = 0; s < NSMALL; s++) {
        size_t n = small_sizes[s], u;
        uint8_t *p[KC], *q[KC];
        int i;

        for (i = 0; i < KC; i++)
            p[i] = js_malloc_rt(rt, n);
        u = js_malloc_usable_size_rt(rt, p[0]);
        for (i = 0; i < KC; i++)
            js_free_rt(rt, p[i]);
        for (i = 0; i < KC; i++) {
            NONE_POISONED(p[i], 8, "the link of a block in the front cache");
            if (u > 8)
                ALL_POISONED(p[i] + 8, u - 8, "a block in the front cache past its link");
        }
        /* taken again from the cache */
        for (i = 0; i < KC; i++) {
            q[i] = js_malloc_rt(rt, n);
            NONE_POISONED(q[i], u, "a block taken from the front cache");
        }
        for (i = 0; i < KC; i++)
            js_free_rt(rt, q[i]);
        /* given back to the arenas when the cache is turned off */
        JS_SetFrontCache(rt, 0);
        for (i = 0; i < KC; i++)
            ALL_POISONED(q[i], u, "a block the front cache gave back to its arena");
        JS_SetFrontCache(rt, 256);
    }
}

static void test_large(JSRuntime *rt)
{
    uint8_t *p, *q;
    size_t u0, u1, inplace = 0;
    int attempt;

    JS_SetFrontCache(rt, 0);
    p = js_malloc_rt(rt, 3000);
    u0 = js_malloc_usable_size_rt(rt, p);
    NONE_POISONED(p, u0, "a live large block");
    js_free_rt(rt, p);
    ALL_POISONED(p, u0, "a freed large block");

    /* a resize that shrinks cuts the tail off in place */
    p = js_malloc_rt(rt, 8000);
    u0 = js_malloc_usable_size_rt(rt, p);
    q = js_realloc_rt(rt, p, 2000);
    u1 = js_malloc_usable_size_rt(rt, q);
    CHECK(q == p && u1 < u0, "a large block that shrinks from 8000 to 2000 should stay in place");
    NONE_POISONED(q, u1, "a shrunk block");
    ALL_POISONED(q + u1, u0 - u1, "the tail a resize cut off");
    js_free_rt(rt, q);

    /* a resize that grows: into the free space after the block when there is
       any, or to another place, where the old block is free */
    for (attempt = 0; attempt < 20; attempt++) {
        uint8_t *guard = js_malloc_rt(rt, 100 + 8 * (size_t)attempt);

        p = js_malloc_rt(rt, 2000 + 16 * (size_t)attempt);
        u0 = js_malloc_usable_size_rt(rt, p);
        q = js_realloc_rt(rt, p, 9000);
        u1 = js_malloc_usable_size_rt(rt, q);
        if (q == p) {
            inplace++;
            NONE_POISONED(q, u1, "a block that grew in place");
        } else {
            NONE_POISONED(q, u1, "a block that moved when it grew");
            ALL_POISONED(p, u0, "the old block after a resize that moved");
        }
        js_free_rt(rt, q);
        js_free_rt(rt, guard);
    }
    CHECK(inplace > 0, "no resize grew in place in 20 attempts, the test checked only the move");

    /* a resize past the room the chunks have moves the block into a chunk of
       its own, and the old block is free */
    p = js_malloc_rt(rt, 2000);
    u0 = js_malloc_usable_size_rt(rt, p);
    q = js_realloc_rt(rt, p, (size_t)3 << 20);
    u1 = js_malloc_usable_size_rt(rt, q);
    CHECK(q != p, "a block of 2000 grown to 3 MiB should have moved");
    NONE_POISONED(q, 4096, "the start of a block that moved to a chunk of its own");
    NONE_POISONED(q + u1 - 4096, 4096, "the end of a block that moved to a chunk of its own");
    ALL_POISONED(p, u0, "the old block after a resize that moved");
    js_free_rt(rt, q);
}

int main(void)
{
    JSRuntime *rt = host_new_runtime();

    test_small_in_arena(rt);
    test_small_in_cache(rt);
    test_large(rt);
    host_free_runtime(rt);
    printf("poison-test: %ld checks, %d failures\n", checks, failures);
    return failures != 0;
}
