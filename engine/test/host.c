/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository. js_load_file, js_module_test_json and
 * js_std_dump_error follow quickjs-libc.c of QuickJS (Copyright (c) 2017-2021
 * Fabrice Bellard and Charlie Gordon, MIT, see engine/LICENSE).
 */
#include <errno.h>
#include <limits.h>
#include <pthread.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <time.h>
#include <unistd.h>

#include "host.h"

#define HOST_STACK_SIZE (1024 * 1024) /* JS_DEFAULT_STACK_SIZE of QuickJS */
#define HOST_NOW_SEC 1767225600       /* 2026-01-01T00:00:00Z */
#define HOST_TIMEOUT_SEC 120          /* the longest a test may run */

/* When the runtime of this thread was made. A thread has one runtime at a
   time: the runner makes one per test, and one per agent in its own thread. */
static _Thread_local struct timespec host_started;

/* The memory of the VMs. The heap asks for chunks through host_memory_grow,
   which maps them and keeps a list for each VM, so that they can be unmapped
   when the runtime is freed. The mutex guards the list of VMs only: a VM is
   used by one thread at a time. */
typedef struct HostChunk {
    struct HostChunk *next;
    uintptr_t addr;
    size_t size;
} HostChunk;

typedef struct HostVM {
    struct HostVM *next;
    uint32_t id;
    JSRuntime *rt;
    HostChunk *chunks;
    size_t mapped;   /* bytes in the chunks */
    size_t released; /* mapped after the last release */
} HostVM;

static HostVM *host_vms;
static uint32_t host_next_vm = 1;
static pthread_mutex_t host_lock = PTHREAD_MUTEX_INITIALIZER;
static int host_leaks;
/* HOST_MAX_MEMORY=bytes: the most a VM may map, past which the host answers
   as a system that has no more memory would */
static size_t host_max_memory;
/* HOST_RELEASE_OVER=bytes (default 512 MiB, 0 for never): a VM that holds
   more than that gives its free chunks back, 64 MiB of growth apart. A buffer
   that grows by realloc, such as the 1 GiB string of test262's
   staging/sm/String/replace-math.js, moves into a chunk of its own at each
   step and leaves the smaller one behind, free but mapped: the test takes 6.7
   GB, six times what it takes on the system malloc, and does not run in a
   machine of 4 GB. The Go host will not release from inside
   host_memory_grow (that would reenter the engine): the pool of tifa calls
   ReleaseMemory between renders. This host does, to run that test. */
static size_t host_release_over = (size_t)512 << 20;

#ifdef JS_HEAP_POISON
void __asan_unpoison_memory_region(void const volatile *addr, size_t size);
#define host_unpoison(p, n) __asan_unpoison_memory_region((p), (n))
#else
#define host_unpoison(p, n) ((void)0)
#endif

static HostVM *host_vm_find(uint32_t id)
{
    HostVM *vm;

    pthread_mutex_lock(&host_lock);
    for (vm = host_vms; vm && vm->id != id; vm = vm->next)
        ;
    pthread_mutex_unlock(&host_lock);
    return vm;
}

uintptr_t host_memory_grow(uint32_t id, uintptr_t size)
{
    HostVM *vm = host_vm_find(id);
    HostChunk *c;
    void *p;

    if (!vm)
        return 0;
    if (host_release_over && vm->rt && vm->mapped > host_release_over &&
        vm->mapped >= vm->released + ((size_t)64 << 20)) {
        /* the heap is between two steps here: it asks for a chunk because
           its search failed, and changes nothing before this returns */
        host_release_memory(vm->rt, 0, NULL, NULL);
        vm->released = vm->mapped;
    }
    if (host_max_memory && size > host_max_memory - vm->mapped)
        return 0;
    p = mmap(NULL, size, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON, -1, 0);
    if (p == MAP_FAILED)
        return 0;
    c = malloc(sizeof(*c));
    if (!c) {
        munmap(p, size);
        return 0;
    }
    host_unpoison(p, size);
    /* the engine may not count on zeroed memory: the system gives zeroes, so
       write something else where the first blocks are */
    memset(p, 0xcd, size < 262144 ? size : 262144);
    c->addr = (uintptr_t)p;
    c->size = size;
    c->next = vm->chunks;
    vm->chunks = c;
    vm->mapped += size;
    if (getenv("HOST_TRACE"))
        fprintf(stderr, "host: vm %u maps %zu, holds %zu\n", vm->id, (size_t)size, vm->mapped);
    return c->addr;
}

static void host_chunk_forget(HostVM *vm, uintptr_t addr)
{
    HostChunk **pp, *c;

    for (pp = &vm->chunks; (c = *pp) != NULL; pp = &c->next) {
        if (c->addr == addr) {
            *pp = c->next;
            vm->mapped -= c->size;
            host_unpoison((void *)c->addr, c->size);
            munmap((void *)c->addr, c->size);
            free(c);
            return;
        }
    }
    fprintf(stderr, "host: the engine gave back a chunk it was not given\n");
    _exit(87);
}

/* The engine calls this from time to time, between two steps of the script.
   It ends a test that runs over two minutes. */
static int host_interrupt(JSRuntime *rt, void *opaque)
{
    struct timespec now;

    (void)rt;
    (void)opaque;
    clock_gettime(CLOCK_MONOTONIC, &now);
    return now.tv_sec - host_started.tv_sec > HOST_TIMEOUT_SEC;
}

static void host_atexit(void)
{
    if (host_leaks) {
        fprintf(stderr, "host: %d runtimes freed with blocks still in use\n", host_leaks);
        _exit(87);
    }
}

static void host_register_exit(void)
{
    const char *max = getenv("HOST_MAX_MEMORY");
    const char *over = getenv("HOST_RELEASE_OVER");

    if (max)
        host_max_memory = (size_t)strtoull(max, NULL, 10);
    if (over)
        host_release_over = (size_t)strtoull(over, NULL, 10);
    atexit(host_atexit);
}

/* Takes vm out of the list of VMs. The caller holds host_lock. */
static void host_vm_unlink(HostVM *vm)
{
    HostVM **pp;

    for (pp = &host_vms; *pp; pp = &(*pp)->next) {
        if (*pp == vm) {
            *pp = vm->next;
            return;
        }
    }
}

/* Unmaps what is left of a VM that is out of the list, and frees it. */
static void host_vm_free(HostVM *vm)
{
    while (vm->chunks) {
        HostChunk *c = vm->chunks;
        vm->chunks = c->next;
        host_unpoison((void *)c->addr, c->size);
        munmap((void *)c->addr, c->size);
        free(c);
    }
    free(vm);
}

/* The VM is in the list from the start: the heap asks for its first chunk
   from inside JS_NewRuntime, by the id. It gets its runtime in the list
   under the lock the readers of the list take, once the engine has made it.
   When the engine cannot make one, the VM leaves the list and its chunks go
   back: the caller has no runtime to free. */
JSRuntime *host_new_runtime(void)
{
    static pthread_once_t once = PTHREAD_ONCE_INIT;
    HostVM *vm = calloc(1, sizeof(*vm));
    JSRuntime *rt;

    if (!vm)
        return NULL;
    pthread_once(&once, host_register_exit);
    pthread_mutex_lock(&host_lock);
    vm->id = host_next_vm++;
    vm->next = host_vms;
    host_vms = vm;
    pthread_mutex_unlock(&host_lock);
    rt = JS_NewRuntime(vm->id, (uint64_t)HOST_NOW_SEC * 1000000);
    pthread_mutex_lock(&host_lock);
    if (rt)
        vm->rt = rt;
    else
        host_vm_unlink(vm);
    pthread_mutex_unlock(&host_lock);
    if (!rt) {
        host_vm_free(vm);
        return NULL;
    }
    JS_SetMaxStackSize(rt, HOST_STACK_SIZE);
    clock_gettime(CLOCK_MONOTONIC, &host_started);
    JS_SetInterruptHandler(rt, host_interrupt, NULL);
    return rt;
}

void host_free_runtime(JSRuntime *rt)
{
    HostVM *vm;
    size_t leaked = JS_FreeRuntime(rt);

    pthread_mutex_lock(&host_lock);
    for (vm = host_vms; vm && vm->rt != rt; vm = vm->next)
        ;
    if (vm)
        host_vm_unlink(vm);
    if (leaked)
        host_leaks++;
    pthread_mutex_unlock(&host_lock);
    if (!vm) {
        fprintf(stderr, "host: a runtime the host did not make\n");
        _exit(87);
    }
    host_vm_free(vm);
}

size_t host_live_vms(void)
{
    HostVM *vm;
    size_t n = 0;

    pthread_mutex_lock(&host_lock);
    for (vm = host_vms; vm; vm = vm->next)
        n++;
    pthread_mutex_unlock(&host_lock);
    return n;
}

size_t host_mapped_bytes(void)
{
    HostVM *vm;
    size_t n = 0;

    pthread_mutex_lock(&host_lock);
    for (vm = host_vms; vm; vm = vm->next)
        n += vm->mapped;
    pthread_mutex_unlock(&host_lock);
    return n;
}

void host_release_memory(JSRuntime *rt, size_t keep, size_t *candidate, size_t *released)
{
    HostVM *vm;
    JSMemRange r;
    long page = sysconf(_SC_PAGESIZE);
    size_t cand = 0, rel = 0;

    pthread_mutex_lock(&host_lock);
    for (vm = host_vms; vm && vm->rt != rt; vm = vm->next)
        ;
    pthread_mutex_unlock(&host_lock);
    if (!vm)
        return;
    JS_ReleaseStart(rt, keep);
    while ((r = JS_ReleaseNextChunk(rt)).len) {
        host_chunk_forget(vm, r.addr);
        cand += r.len;
        rel += r.len;
    }
    while ((r = JS_ReleaseNextRange(rt)).len) {
        uintptr_t a = (r.addr + page - 1) & ~(uintptr_t)(page - 1);
        uintptr_t e = (r.addr + r.len) & ~(uintptr_t)(page - 1);

        cand += r.len;
        if (e > a) {
#ifdef __APPLE__
            madvise((void *)a, e - a, MADV_FREE);
#else
            madvise((void *)a, e - a, MADV_DONTNEED);
#endif
            rel += e - a;
        }
    }
    if (candidate)
        *candidate = cand;
    if (released)
        *released = rel;
}

/* HOST_CLOCK=running lets the time pass from the fixed start. A few tests wait
   for the clock to move, such as test262's dynamic-import/
   await-import-evaluation.js, and never end on a fixed one. */
static int host_clock_running;
static struct timespec host_clock_origin;

static void host_clock_init(void) __attribute__((constructor));
static void host_clock_init(void)
{
    const char *mode = getenv("HOST_CLOCK");

    host_clock_running = mode && !strcmp(mode, "running");
    clock_gettime(CLOCK_MONOTONIC, &host_clock_origin);
}

int64_t host_now(uint32_t vm, int kind)
{
    struct timespec now;
    int64_t ns;

    (void)vm;
    if (kind == HOST_NOW_MONOTONIC) {
        clock_gettime(CLOCK_MONOTONIC, &now);
        return (int64_t)now.tv_sec * 1000000000 + now.tv_nsec;
    }
    ns = (int64_t)HOST_NOW_SEC * 1000000000;
    if (host_clock_running) {
        clock_gettime(CLOCK_MONOTONIC, &now);
        ns += (int64_t)(now.tv_sec - host_clock_origin.tv_sec) * 1000000000 +
            (now.tv_nsec - host_clock_origin.tv_nsec);
    }
    return ns;
}

/* The test host registers no function, so the only request it answers is the
   engine's own abort. It ends the process with the signal abort() gives. The
   system's abort is not called: the engine's libc defines abort, and it is
   the one that calls this. */
JSValue host_call(uint32_t vm, JSValueConst this_val, int argc, JSValueConst *argv, int magic)
{
    (void)vm;
    (void)this_val;
    (void)argc;
    (void)argv;
    if (magic == HOST_CALL_ABORT)
        fputs("engine abort\n", stderr);
    else
        fprintf(stderr, "engine: host_call of %d, which the test host does not have\n", magic);
    fflush(stderr);
    signal(SIGABRT, SIG_DFL);
    raise(SIGABRT);
    _exit(134);
}

uint8_t *js_load_file(JSContext *ctx, size_t *pbuf_len, const char *filename)
{
    FILE *f;
    uint8_t *buf;
    size_t buf_len;
    long lret;

    f = fopen(filename, "rb");
    if (!f)
        return NULL;
    if (fseek(f, 0, SEEK_END) < 0)
        goto fail;
    lret = ftell(f);
    if (lret < 0)
        goto fail;
    /* on Linux, ftell() returns LONG_MAX for directories */
    if (lret == LONG_MAX) {
        errno = EISDIR;
        goto fail;
    }
    buf_len = lret;
    if (fseek(f, 0, SEEK_SET) < 0)
        goto fail;
    if (ctx)
        buf = js_malloc(ctx, buf_len + 1);
    else
        buf = malloc(buf_len + 1);
    if (!buf)
        goto fail;
    if (fread(buf, 1, buf_len, f) != buf_len) {
        errno = EIO;
        if (ctx)
            js_free(ctx, buf);
        else
            free(buf);
    fail:
        fclose(f);
        return NULL;
    }
    buf[buf_len] = '\0';
    fclose(f);
    *pbuf_len = buf_len;
    return buf;
}

/* return > 0 if the attributes indicate a JSON module */
int js_module_test_json(JSContext *ctx, JSValueConst attributes)
{
    JSValue str;
    const char *cstr;
    size_t len;
    int res;

    if (JS_IsUndefined(attributes))
        return 0;
    str = JS_GetPropertyStr(ctx, attributes, "type");
    if (!JS_IsString(str))
        return 0;
    cstr = JS_ToCStringLen(ctx, &len, str);
    JS_FreeValue(ctx, str);
    if (!cstr)
        return 0;
    if (len == 4 && !memcmp(cstr, "json", len)) {
        res = 1;
    } else if (len == 5 && !memcmp(cstr, "json5", len)) {
        res = 2;
    } else {
        res = 0;
    }
    JS_FreeCString(ctx, cstr);
    return res;
}

static void print_write(void *opaque, const char *buf, size_t len)
{
    fwrite(buf, 1, len, (FILE *)opaque);
}

void js_std_dump_error(JSContext *ctx)
{
    JSValue exception_val = JS_GetException(ctx);

    JS_PrintValue(ctx, print_write, stderr, exception_val, NULL);
    fputc('\n', stderr);
    JS_FreeValue(ctx, exception_val);
}
