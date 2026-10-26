/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/*
 * Tests of the API the C tree changes, on real runtimes: JS_DumpMemoryUsage.
 * api-test.sh builds and runs it.
 */
#include <stdio.h>
#include <stdlib.h>
#include <signal.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

#include "host.h"

static int failures;
static long checks;

#define CHECK(cond, ...) do { \
        checks++; \
        if (!(cond)) { \
            if (failures++ < 20) { \
                fprintf(stderr, "api-test: %s:%d: ", __FILE__, __LINE__); \
                fprintf(stderr, __VA_ARGS__); \
                fputc('\n', stderr); \
            } \
        } \
    } while (0)

static JSContext *new_context(JSRuntime *rt)
{
    JSContext *ctx = JS_NewContext(rt);
    if (!ctx) {
        fprintf(stderr, "api-test: JS_NewContext failed\n");
        exit(2);
    }
    return ctx;
}

static void eval_ok(JSContext *ctx, const char *code)
{
    JSValue v = JS_Eval(ctx, code, strlen(code), "<api-test>", JS_EVAL_TYPE_GLOBAL);
    if (JS_IsException(v)) {
        js_std_dump_error(ctx);
        exit(2);
    }
    JS_FreeValue(ctx, v);
}

/* The report of a runtime that holds objects: the same text at every size, the
   length when there is no buffer, and a cut at every length from nothing to
   past the end, with a canary after the cut. */
static void test_dump(void)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = new_context(rt);
    JSMemoryUsage s;
    size_t n, n_null;
    char *full, *cut;

    eval_ok(ctx,
            "globalThis.keep = [];"
            "for (let i = 0; i < 200; i++) keep.push({ i, s: 'item' + i, a: [i, i + 1], f() { return i; } });"
            "keep.push(new Map([[1, 2]]), new Set([1]), /re/g, new Date(0), new Error('x'), Promise.resolve(1),"
            "          new Uint8Array(10), Symbol('s'), 10n ** 30n);");
    JS_ComputeMemoryUsage(rt, &s);

    n = JS_DumpMemoryUsage(NULL, 0, &s, rt);
    CHECK(n > 500, "the report is too short: %zu", n);
    full = malloc(n + 1);
    CHECK(JS_DumpMemoryUsage(full, n + 1, &s, rt) == n, "length with a buffer differs from the length without");
    CHECK(strlen(full) == n, "report length %zu, text length %zu", n, strlen(full));
    CHECK(strstr(full, "QuickJS memory usage") == full, "the report does not start as it should");
    CHECK(strstr(full, "JSObject classes") != NULL, "no class counts: the part that names classes did not run");
    CHECK(strstr(full, "Array") && strstr(full, "Map") && strstr(full, "RegExp"), "class names are missing");
    CHECK(strstr(full, "objects") && strstr(full, "shapes") && strstr(full, "bytecode"), "a table row is missing");
    CHECK(strchr(full, '%') == NULL, "a conversion was left in the text");

    /* Cut to every size from 0 to past the end. The text before the cut is the
       start of the full report, a 0 ends it, and every byte after what was
       written, to the end of the buffer, is untouched. */
    cut = malloc(n + 16);
    for (size_t size = 0; size <= n + 3; size++) {
        size_t r;
        memset(cut, 0x5a, n + 16);
        r = JS_DumpMemoryUsage(cut, size, &s, rt);
        CHECK(r == n, "size %zu: length %zu, want %zu", size, r, n);
        {
            /* size - 1 characters and the 0, or the whole report and the 0 */
            size_t written = size == 0 ? 0 : size - 1 < n ? size : n + 1;
            size_t i;

            if (size) {
                size_t keep = written - 1;
                CHECK(!memcmp(cut, full, keep), "size %zu: text differs from the full report", size);
                CHECK(cut[keep] == 0, "size %zu: no 0 after the text", size);
            }
            for (i = written; i < n + 16; i++)
                if ((unsigned char)cut[i] != 0x5a)
                    break;
            CHECK(i == n + 16, "size %zu: the byte %zu past the %zu written was changed", size, i - written, written);
        }
    }

    /* Without a runtime there are no class counts and no object sizes */
    n_null = JS_DumpMemoryUsage(NULL, 0, &s, NULL);
    CHECK(n_null > 100 && n_null < n, "report without a runtime: %zu (with: %zu)", n_null, n);

    free(cut);
    free(full);
    JS_FreeContext(ctx);
    JS_FreeRuntime(rt);
}

int main(void)
{
    test_dump();
    printf("api-test: %ld checks, %d failures\n", checks, failures);
    return failures != 0;
}
