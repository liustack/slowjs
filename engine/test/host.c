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
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "host.h"

#define HOST_STACK_SIZE (1024 * 1024) /* JS_DEFAULT_STACK_SIZE of QuickJS */
#define HOST_NOW_SEC 1767225600       /* 2026-01-01T00:00:00Z */
#define HOST_TIMEOUT_SEC 120          /* the longest a test may run */

/* When the runtime of this thread was made. A thread has one runtime at a
   time: the runner makes one per test, and one per agent in its own thread. */
static _Thread_local struct timespec host_started;

static int host_interrupt(JSRuntime *rt, void *opaque)
{
    struct timespec now;

    (void)rt;
    (void)opaque;
    clock_gettime(CLOCK_MONOTONIC, &now);
    return now.tv_sec - host_started.tv_sec > HOST_TIMEOUT_SEC;
}

JSRuntime *host_new_runtime(void)
{
    JSRuntime *rt = JS_NewRuntime();
    if (rt) {
        JS_SetMaxStackSize(rt, HOST_STACK_SIZE);
        clock_gettime(CLOCK_MONOTONIC, &host_started);
        JS_SetInterruptHandler(rt, host_interrupt, NULL);
    }
    return rt;
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
