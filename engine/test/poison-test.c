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
 *              front cache but for the first 8 bytes), a freed large block
 *              (also one that waits in the cache of large blocks, where the
 *              first 8 bytes are poisoned too and are opened only for the
 *              moment the allocator reads or writes the link), the memory of
 *              an arena that was given back, the tail a resize cut off, the
 *              old block after a resize that moved
 *   readable:  a live block up to its usable size, the first 8 bytes of a
 *              block in the front cache (they hold the link), the header of
 *              the slot after a live small block (the allocator reads and
 *              writes it), a block that was taken from the cache or the arena
 *              again, the tail a resize grew into
 *
 * Then it runs itself as a child process on a use after free of a block in
 * the cache of large blocks (a read in the middle, a read and a write of the
 * link, a write at the end) and of a block taken from it again, and checks that
 * AddressSanitizer reports the first four and not the last: the poison is not
 * just there, it is what makes a program that frees a block and goes on using it
 * stop.
 *
 * So an access that stays within the 8 bytes after a live small block, or that
 * goes into another live block, is not found here. build.sh asan, with every
 * block alone from the system malloc and red zones around each, finds those,
 * and has no front cache and no arenas. Both modes run.
 */
#include <fcntl.h>
#include <spawn.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

#include "host.h"

extern char **environ;

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

/* With the front cache on, a freed large block of 505 to 4096 bytes waits in
   the cache of large blocks, with a link in its first 8 bytes: all of it is
   poisoned, the link too. */
static void test_large_in_cache(JSRuntime *rt)
{
    static const size_t sizes[] = { 505, 512, 600, 1000, 1500, 3000, 4096 };
    uint8_t *p[8], *q[8];
    size_t s, i, u;

    JS_SetFrontCache(rt, 256);
    for (s = 0; s < sizeof sizes / sizeof *sizes; s++) {
        size_t n = sizes[s];

        for (i = 0; i < 8; i++)
            p[i] = js_malloc_rt(rt, n);
        u = js_malloc_usable_size_rt(rt, p[0]);
        for (i = 0; i < 8; i++)
            NONE_POISONED(p[i], u, "a live large block");
        for (i = 0; i < 8; i++)
            js_free_rt(rt, p[i]);
        for (i = 0; i < 8; i++)
            ALL_POISONED(p[i], u, "a large block in the cache of large blocks, its link included");
        /* the header in front of the user data stays readable, the allocator
           reads it when it takes the block */
        NONE_POISONED(p[0] - 8, 8, "the header of a cached large block");
        /* taken again from the cache */
        for (i = 0; i < 8; i++) {
            q[i] = js_malloc_rt(rt, n);
            NONE_POISONED(q[i], u, "a large block taken from the cache");
        }
        for (i = 0; i < 8; i++)
            js_free_rt(rt, q[i]);
        /* given back to the heap when the cache is turned off, which poisons the whole block */
        JS_SetFrontCache(rt, 0);
        for (i = 0; i < 8; i++)
            ALL_POISONED(q[i], u, "a large block the cache gave back to the heap");
        JS_SetFrontCache(rt, 256);
    }
    /* a block that is not cached because its class is full is poisoned by the heap */
    {
        uint8_t *many[12];

        for (i = 0; i < 12; i++)
            many[i] = js_malloc_rt(rt, 1000);
        u = js_malloc_usable_size_rt(rt, many[0]);
        for (i = 0; i < 12; i++)
            js_free_rt(rt, many[i]);
        for (i = 0; i < 12; i++)
            ALL_POISONED(many[i], u, "a freed large block, cached or not");
        JS_SetFrontCache(rt, 256);
    }
}

/* Child processes: the use after free of a large block that waits in the cache
   must be reported by AddressSanitizer, and the use of a block taken from the
   cache again, or of a live one, must not. */
static int uaf_child(const char *what)
{
    JSRuntime *rt = host_new_runtime();
    volatile uint8_t *p, *q;
    volatile uint8_t sink = 0;

    JS_SetFrontCache(rt, 256);
    p = js_malloc_rt(rt, 1000);
    if (!strcmp(what, "live")) {
        p[0] = 1;
        sink = p[500] + p[1023];
    } else if (!strcmp(what, "taken-again")) {
        js_free_rt(rt, (void *)p);
        q = js_malloc_rt(rt, 1000);
        if (q != p)
            return 3; /* the cache did not give the block back: the test is void */
        q[0] = 1;
        sink = q[500] + q[1023];
    } else if (!strcmp(what, "read-middle")) {
        js_free_rt(rt, (void *)p);
        sink = p[500];
    } else if (!strcmp(what, "read-link")) {
        js_free_rt(rt, (void *)p);
        sink = p[0];
    } else if (!strcmp(what, "write-link")) {
        js_free_rt(rt, (void *)p);
        p[0] = 1;
    } else if (!strcmp(what, "write-end")) {
        js_free_rt(rt, (void *)p);
        p[1023] = 1;
    } else {
        return 2;
    }
    (void)sink;
    _exit(0); /* no leak report: the runtime is not freed */
}

/* What a child run of `poison-test uaf WHAT` did */
typedef struct {
    int ran;          /* it ran, ended and its error output could be read whole */
    int exited_0;     /* it exited normally with status 0 */
    int use_after_poison; /* AddressSanitizer reported a use after poison */
    int any_report;   /* AddressSanitizer reported anything */
} UafRun;

/* Reads fd to its end into buf, which holds cap bytes, and ends the text with a
   NUL. Returns 1 when it read to the end, and 0 on a read error or when the
   output is longer than cap - 1 bytes (buf then holds the start of it). */
static int read_all(int fd, char *buf, size_t cap, size_t *lenp)
{
    size_t len = 0;
    ssize_t n = 0;
    char more;

    while (len < cap - 1 && (n = read(fd, buf + len, cap - 1 - len)) > 0)
        len += (size_t)n;
    buf[len] = 0;
    *lenp = len;
    if (len < cap - 1)
        return n == 0;
    /* the buffer is full: a read of no bytes would say 0 whatever is left */
    return read(fd, &more, 1) == 0;
}

/* Runs this program again as `poison-test uaf WHAT` with its error output in a
   file, and reads that output. */
static UafRun uaf_run(const char *self, const char *what)
{
    char path[] = "/tmp/poison-test-XXXXXX";
    char *argv[4];
    posix_spawn_file_actions_t fa;
    pid_t pid;
    int fd, status = 0, spawned, read_ok = 0;
    static char buf[1 << 16];
    size_t len = 0;
    UafRun r = { 0, 0, 0, 0 };

    fd = mkstemp(path);
    CHECK(fd >= 0, "mkstemp");
    if (fd < 0)
        return r;
    close(fd);
    argv[0] = (char *)self;
    argv[1] = (char *)"uaf";
    argv[2] = (char *)what;
    argv[3] = NULL;
    posix_spawn_file_actions_init(&fa);
    posix_spawn_file_actions_addopen(&fa, 2, path, O_WRONLY | O_TRUNC, 0600);
    spawned = posix_spawn(&pid, self, &fa, NULL, argv, environ);
    posix_spawn_file_actions_destroy(&fa);
    CHECK(spawned == 0, "cannot run %s", self);
    if (spawned == 0 && waitpid(pid, &status, 0) == pid) {
        fd = open(path, O_RDONLY);
        if (fd >= 0) {
            read_ok = read_all(fd, buf, sizeof buf, &len);
            close(fd);
        }
        buf[len] = 0;
        r.ran = read_ok;
        r.exited_0 = WIFEXITED(status) && WEXITSTATUS(status) == 0;
        r.use_after_poison = strstr(buf, "AddressSanitizer: use-after-poison") != NULL;
        r.any_report = strstr(buf, "AddressSanitizer") != NULL;
        CHECK(read_ok, "uaf %s: the error output could not be read whole", what);
        CHECK(!(WIFEXITED(status) && (WEXITSTATUS(status) == 2 || WEXITSTATUS(status) == 3)),
              "uaf %s: the child could not set up the case (status %d)", what, WEXITSTATUS(status));
    }
    unlink(path);
    return r;
}

/* The output of a child is read through read_all: output that does not fit in
   the buffer must be reported as cut, not read as if it were whole, as a report
   past the cut would not be seen and a check for no report would pass. */
static void test_read_all(void)
{
    static const size_t sizes[] = { 0, 1, 99, 100, 101, 5000 };
    char path[sizeof "/tmp/poison-test-XXXXXX"];
    char buf[100], big[5000];
    size_t i, len;
    int fd;

    memset(big, 'x', sizeof big);
    for (i = 0; i < sizeof sizes / sizeof *sizes; i++) {
        int ok;

        strcpy(path, "/tmp/poison-test-XXXXXX");
        fd = mkstemp(path);
        CHECK(fd >= 0, "mkstemp");
        if (fd < 0)
            return;
        CHECK(write(fd, big, sizes[i]) == (ssize_t)sizes[i], "write");
        lseek(fd, 0, SEEK_SET);
        ok = read_all(fd, buf, sizeof buf, &len);
        close(fd);
        unlink(path);
        /* the buffer keeps 99 bytes and a NUL */
        CHECK(ok == (sizes[i] <= 99), "read_all of %zu bytes into a buffer of 100 said %d", sizes[i], ok);
        CHECK(len == (sizes[i] < 99 ? sizes[i] : 99), "read_all of %zu bytes into a buffer of 100 kept %zu", sizes[i], len);
        CHECK(buf[len] == 0, "read_all of %zu bytes did not end the text", sizes[i]);
    }
}

/* A use after free must be reported as one. A correct use must end the child
   with status 0, an error output read whole, and no report of any kind: a
   crash or another report is not a pass. */
static void test_uaf(const char *self)
{
    static const char *const caught[] = { "read-middle", "read-link", "write-link", "write-end" };
    static const char *const quiet[] = { "live", "taken-again" };
    size_t i;

    for (i = 0; i < sizeof caught / sizeof *caught; i++) {
        UafRun r = uaf_run(self, caught[i]);
        CHECK(r.ran && r.use_after_poison, "a use after free of a block in the cache of large blocks (%s) was not reported", caught[i]);
    }
    for (i = 0; i < sizeof quiet / sizeof *quiet; i++) {
        UafRun r = uaf_run(self, quiet[i]);
        CHECK(r.ran && r.exited_0 && !r.any_report, "a correct use of a large block (%s) did not end cleanly (ran %d, status 0 %d, report %d)",
              quiet[i], r.ran, r.exited_0, r.any_report);
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

    /* a block of 5 MiB has a chunk of its own: readable to its usable size, and the
       room the chunk has after it is poisoned, until the block grows into it */
    p = js_malloc_rt(rt, ((size_t)5 << 20) + 1000);
    u0 = js_malloc_usable_size_rt(rt, p);
    NONE_POISONED(p, 4096, "the start of a block in a chunk of its own");
    NONE_POISONED(p + u0 - 4096, 4096, "the end of a block in a chunk of its own");
    CHECK(poisoned(p + u0 + 8) && poisoned(p + u0 + 108), "the room after a block of its own is not poisoned");
    q = js_realloc_rt(rt, p, ((size_t)5 << 20) + 5000);
    u1 = js_malloc_usable_size_rt(rt, q);
    CHECK(q == p && u1 > u0, "a block of its own should grow into the room of its chunk");
    NONE_POISONED(q + u0 - 8, 3000, "the tail a block of its own grew into");
    CHECK(poisoned(q + u1 + 8), "the room after a block of its own that grew is not poisoned");
    js_free_rt(rt, q);

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

int main(int argc, char **argv)
{
    JSRuntime *rt;

    if (argc == 3 && !strcmp(argv[1], "uaf"))
        return uaf_child(argv[2]);
    rt = host_new_runtime();
    test_small_in_arena(rt);
    test_small_in_cache(rt);
    test_large_in_cache(rt);
    test_large(rt);
    host_free_runtime(rt);
    test_read_all();
    test_uaf(argv[0]);
    printf("poison-test: %ld checks, %d failures\n", checks, failures);
    return failures != 0;
}
