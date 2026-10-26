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

#define HOST_NOW_SEC 1767225600       /* 2026-01-01T00:00:00Z */
#define HOST_TIMEOUT_SEC 120          /* the longest a test may run, unless HOST_TIMEOUT_SEC says otherwise */

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
} HostVM;

static HostVM *host_vms;
static uint32_t host_next_vm = 1;
static pthread_mutex_t host_lock = PTHREAD_MUTEX_INITIALIZER;
static int host_leaks;
static long host_timeout = HOST_TIMEOUT_SEC; /* seconds, 0 for none */
/* HOST_MAX_MEMORY=bytes: the most a VM may map, past which the host answers
   as a system that has no more memory would */
static size_t host_max_memory;
/* HOST_NO_REMAP=1: do not tell the engine it can remap, so that it allocates,
   copies and releases when a dedicated block outgrows its chunk (linux has
   mremap and tells it by default) */
static int host_no_remap;
#ifdef JS_HEAP_POISON
void __asan_unpoison_memory_region(void const volatile *addr, size_t size);
#define host_unpoison(p, n) __asan_unpoison_memory_region((p), (n))
#else
#define host_unpoison(p, n) ((void)0)
#endif

/* The engine counts its recursion (JS_SetMaxCallDepth) and asks how many bytes
   of stack are left only to bound an alloca and to keep a margin at each
   level. In the generated assembly that is the room on the VM's shadow stack.
   Here it is the room on the stack of the thread, past HOST_STACK_RESERVE
   bytes kept for the host and the system, so that a recursion within the
   limits never runs off the stack. The bottom of the stack is found once per
   thread. */
#define HOST_STACK_RESERVE (64 * 1024)
static _Thread_local uintptr_t host_stack_low;

static uintptr_t host_find_stack_low(void)
{
#ifdef __APPLE__
    pthread_t self = pthread_self();
    return (uintptr_t)pthread_get_stackaddr_np(self) - pthread_get_stacksize_np(self);
#else
    pthread_attr_t attr;
    void *addr;
    size_t size;

    if (pthread_getattr_np(pthread_self(), &attr) != 0 ||
        pthread_attr_getstack(&attr, &addr, &size) != 0) {
        fprintf(stderr, "host: cannot find the stack of this thread\n");
        abort();
    }
    pthread_attr_destroy(&attr);
    return (uintptr_t)addr;
#endif
}

_Thread_local long host_stack_fail_at;

long __shadow_remaining(void)
{
    uintptr_t sp = (uintptr_t)__builtin_frame_address(0);

    if (host_stack_fail_at > 0 && --host_stack_fail_at == 0)
        return 0;
    if (!host_stack_low)
        host_stack_low = host_find_stack_low() + HOST_STACK_RESERVE;
    return sp > host_stack_low ? (long)(sp - host_stack_low) : 0;
}

typedef struct {
    int (*fn)(void *);
    void *arg;
    int ret;
} HostThread;

static void *host_thread_start(void *p)
{
    HostThread *t = p;

    t->ret = t->fn(t->arg);
    return NULL;
}

int host_run_thread(int (*fn)(void *), void *arg)
{
    HostThread t = { fn, arg, 0 };
    pthread_attr_t attr;
    pthread_t tid;

    pthread_attr_init(&attr);
    pthread_attr_setstacksize(&attr, HOST_THREAD_STACK);
    if (pthread_create(&tid, &attr, host_thread_start, &t) != 0) {
        fprintf(stderr, "host: cannot start a thread\n");
        exit(2);
    }
    pthread_attr_destroy(&attr);
    pthread_join(tid, NULL);
    return t.ret;
}

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

/* HOST_CALL_MEMORY_RELEASE: the engine gives back a whole chunk, size bytes at
   addr. */
static JSValue host_memory_release(uint32_t id, uintptr_t addr, uintptr_t size)
{
    HostVM *vm = host_vm_find(id);
    HostChunk *c;

    if (vm) {
        for (c = vm->chunks; c && c->addr != addr; c = c->next)
            ;
        if (c && c->size == size) {
            host_chunk_forget(vm, addr);
            return JS_UNDEFINED;
        }
    }
    fprintf(stderr, "host: the engine gave back a chunk it was not given, or not whole\n");
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
    return now.tv_sec - host_started.tv_sec > host_timeout;
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
    const char *no_remap = getenv("HOST_NO_REMAP");
    const char *timeout = getenv("HOST_TIMEOUT_SEC");

    if (max)
        host_max_memory = (size_t)strtoull(max, NULL, 10);
    if (timeout)
        host_timeout = strtol(timeout, NULL, 10);
    host_no_remap = no_remap && *no_remap && strcmp(no_remap, "0");
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
JSRuntime *host_new_runtime_seed(uint64_t seed)
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
    rt = JS_NewRuntime(vm->id, seed);
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
#ifdef __linux__
    JS_SetHostRemap(rt, !host_no_remap);
#endif
    clock_gettime(CLOCK_MONOTONIC, &host_started);
    if (host_timeout)
        JS_SetInterruptHandler(rt, host_interrupt, NULL);
    return rt;
}

JSRuntime *host_new_runtime(void)
{
    return host_new_runtime_seed((uint64_t)HOST_NOW_SEC * 1000000);
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

_Thread_local long host_wall_reads;
_Thread_local uint32_t host_wall_vm;

uint32_t host_vm_id(JSRuntime *rt)
{
    HostVM *vm;
    uint32_t id = 0;

    pthread_mutex_lock(&host_lock);
    for (vm = host_vms; vm; vm = vm->next) {
        if (vm->rt == rt) {
            id = vm->id;
            break;
        }
    }
    pthread_mutex_unlock(&host_lock);
    return id;
}

int64_t host_now(uint32_t vm, int kind)
{
    struct timespec now;
    int64_t ns;

    if (kind == HOST_NOW_MONOTONIC) {
        clock_gettime(CLOCK_MONOTONIC, &now);
        return (int64_t)now.tv_sec * 1000000000 + now.tv_nsec;
    }
    host_wall_reads++;
    host_wall_vm = vm;
    ns = (int64_t)HOST_NOW_SEC * 1000000000;
    if (host_clock_running) {
        clock_gettime(CLOCK_MONOTONIC, &now);
        ns += (int64_t)(now.tv_sec - host_clock_origin.tv_sec) * 1000000000 +
            (now.tv_nsec - host_clock_origin.tv_nsec);
    }
    return ns;
}

/* HOST_CALL_MEMORY_REMAP: make the chunk of size old bytes at addr new bytes,
   keeping its content (linux only, and the host told the engine so). Returns
   the address as a word, or 0 when the system has no memory, and then the
   chunk is as it was. */
static JSValue host_memory_remap(uint32_t id, uintptr_t addr, uintptr_t old, uintptr_t new_size)
{
#ifdef __linux__
    HostVM *vm = host_vm_find(id);
    HostChunk *c;
    void *p;

    if (vm) {
        for (c = vm->chunks; c && c->addr != addr; c = c->next)
            ;
        if (c && c->size == old && new_size > old) {
            if (host_max_memory && new_size - old > host_max_memory - vm->mapped)
                return HOST_WORD(0);
            p = mremap((void *)addr, old, new_size, MREMAP_MAYMOVE);
            if (p == MAP_FAILED)
                return HOST_WORD(0);
            host_unpoison(p, new_size);
            /* the new part is not zeroed as far as the engine may know */
            memset((char *)p + old, 0xcd, new_size - old < 262144 ? new_size - old : 262144);
            c->addr = (uintptr_t)p;
            c->size = new_size;
            vm->mapped += new_size - old;
            return HOST_WORD(p);
        }
    }
#else
    (void)id;
    (void)addr;
    (void)old;
    (void)new_size;
#endif
    fprintf(stderr, "host: the engine remapped a chunk it was not given, or not whole, or asked for a smaller one\n");
    _exit(87);
}

/* The test host registers no function, so the requests it answers are the
   engine's own: the memory of the heap (a dedicated chunk goes back, see
   host.h), and abort. Abort ends the process with the signal abort() gives.
   The system's abort is not called: the engine's libc defines abort, and it is
   the one that calls this. */
JSValue host_call(uint32_t vm, JSValueConst this_val, int argc, JSValueConst *argv, int magic)
{
    (void)this_val;
    if (magic == HOST_CALL_MEMORY_RELEASE && argc == 2)
        return host_memory_release(vm, HOST_WORD_VALUE(argv[0]), HOST_WORD_VALUE(argv[1]));
    if (magic == HOST_CALL_MEMORY_REMAP && argc == 3)
        return host_memory_remap(vm, HOST_WORD_VALUE(argv[0]), HOST_WORD_VALUE(argv[1]), HOST_WORD_VALUE(argv[2]));
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
