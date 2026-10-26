/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/*
 * Tests of the native test host (host.c) on its own. The engine is not linked:
 * JS_NewRuntime and the rest of what host.c calls are stubs here, so that the
 * host can be made to meet a runtime that does not start, and so that a build
 * with ThreadSanitizer (host-test.sh tsan) sees the host and nothing else.
 * These are tests of the host, not of the engine running out of memory.
 */
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "host.h"

static _Atomic int failures;
static _Atomic long checks;

#define CHECK(cond, ...) do { \
        checks++; \
        if (!(cond)) { \
            if (failures++ < 20) { \
                fprintf(stderr, "host-test: %s:%d: ", __FILE__, __LINE__); \
                fprintf(stderr, __VA_ARGS__); \
                fputc('\n', stderr); \
            } \
        } \
    } while (0)

/* ---- the stubs ---- */

/* What the next JS_NewRuntime does: asks the host for stub_chunks chunks, as
   the heap does, then fails or not. A runtime is the number 16 * (vm + 1), a
   pointer that is never dereferenced. */
static int stub_chunks;
static int stub_fails;
static _Atomic long stub_started;

JSRuntime *JS_NewRuntime(uint32_t vm, uint64_t seed)
{
    int i;

    (void)seed;
    for (i = 0; i < stub_chunks; i++)
        CHECK(host_memory_grow(vm, 65536), "host_memory_grow refused");
    stub_started++;
    return stub_fails ? NULL : (JSRuntime *)(uintptr_t)(16 * (vm + 1));
}

size_t JS_FreeRuntime(JSRuntime *rt) { (void)rt; return 0; }
void JS_SetHostRemap(JSRuntime *rt, int on) { (void)rt; (void)on; }
void JS_SetInterruptHandler(JSRuntime *rt, JSInterruptHandler *cb, void *opaque) { (void)rt; (void)cb; (void)opaque; }
void JS_ReleaseStart(JSRuntime *rt, size_t keep) { (void)rt; (void)keep; }
JSMemRange JS_ReleaseNextChunk(JSRuntime *rt) { JSMemRange r = { 0, 0 }; (void)rt; return r; }
JSMemRange JS_ReleaseNextRange(JSRuntime *rt) { JSMemRange r = { 0, 0 }; (void)rt; return r; }

/* host.c also holds the helpers of test262's runner, which use the engine's
   values: they are not called here. */
void __JS_FreeValue(JSContext *ctx, JSValue v) { (void)ctx; (void)v; abort(); }
void JS_FreeCString(JSContext *ctx, const char *p) { (void)ctx; (void)p; abort(); }
JSValue JS_GetException(JSContext *ctx) { (void)ctx; abort(); }
JSValue JS_GetPropertyStr(JSContext *ctx, JSValueConst o, const char *n) { (void)ctx; (void)o; (void)n; abort(); }
void JS_PrintValue(JSContext *ctx, JSPrintValueWrite *w, void *o, JSValueConst v, const JSPrintValueOptions *opt)
{
    (void)ctx; (void)w; (void)o; (void)v; (void)opt;
    abort();
}
const char *JS_ToCStringLen2(JSContext *ctx, size_t *plen, JSValueConst v, JS_BOOL cesu8)
{
    (void)ctx; (void)plen; (void)v; (void)cesu8;
    abort();
}
void *js_malloc(JSContext *ctx, size_t size) { (void)ctx; (void)size; abort(); }
void js_free(JSContext *ctx, void *p) { (void)ctx; (void)p; abort(); }

/* ---- the tests ---- */

/* A runtime that does not start leaves nothing in the host: not its VM, not
   the chunks it was given before it failed. */
static void test_failed_start(void)
{
    int i, n;

    stub_fails = 1;
    for (n = 0; n <= 3; n += 3) {
        stub_chunks = n; /* none: the first grow was refused. Three: it failed after the heap had memory */
        for (i = 0; i < 100; i++)
            CHECK(!host_new_runtime(), "a runtime started that the engine refused");
        CHECK(host_live_vms() == 0, "%d chunks: %zu VMs left after 100 failed starts", n, host_live_vms());
        CHECK(host_mapped_bytes() == 0, "%d chunks: %zu bytes still mapped after 100 failed starts", n, host_mapped_bytes());
    }
    stub_fails = 0;
}

static void test_start_and_free(void)
{
    JSRuntime *a, *b;

    stub_chunks = 2;
    a = host_new_runtime();
    b = host_new_runtime();
    CHECK(a && b && a != b, "two runtimes");
    CHECK(host_live_vms() == 2 && host_mapped_bytes() == 4 * 65536, "%zu VMs, %zu bytes", host_live_vms(), host_mapped_bytes());
    host_free_runtime(a);
    CHECK(host_live_vms() == 1 && host_mapped_bytes() == 2 * 65536, "%zu VMs, %zu bytes after one is freed", host_live_vms(), host_mapped_bytes());
    host_free_runtime(b);
    CHECK(host_live_vms() == 0 && host_mapped_bytes() == 0, "%zu VMs, %zu bytes after both are freed", host_live_vms(), host_mapped_bytes());
}

/* Two threads make and free runtimes of their own at the same time, as the
   threads of test262's runner do. Under ThreadSanitizer this is the test of
   the lock around the list of VMs and what each VM publishes in it. */
static void *churn(void *arg)
{
    int i;

    (void)arg;
    for (i = 0; i < 300; i++) {
        JSRuntime *rt = host_new_runtime();

        if (!rt) {
            CHECK(0, "a runtime did not start");
            return NULL;
        }
        host_free_runtime(rt);
    }
    return NULL;
}

static void test_threads(void)
{
    pthread_t t[2];
    int i;

    stub_chunks = 1;
    for (i = 0; i < 2; i++)
        pthread_create(&t[i], NULL, churn, NULL);
    for (i = 0; i < 2; i++)
        pthread_join(t[i], NULL);
    CHECK(host_live_vms() == 0 && host_mapped_bytes() == 0, "%zu VMs, %zu bytes after the threads are done", host_live_vms(), host_mapped_bytes());
}

int main(void)
{
    test_start_and_free();
    test_failed_start();
    test_threads();
    printf("host-test: %ld checks, %d failures\n", (long)checks, (int)failures);
    return failures != 0;
}
