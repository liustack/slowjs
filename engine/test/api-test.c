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

/* The one decimal of the averages is what printf's %.1f gives for the same
   double. The report is made from numbers set here, with no runtime, and its
   text is compared with the C library's, for the values that round the wrong
   way when a number is rounded twice (1.05 is a little over 1.05 as a double,
   0.15 and 0.05 a little under, and 1.25 is halfway). */
static void test_tenths(void)
{
    /* sprintf, and not snprintf: the snprintf of the program is the engine's,
       which does no floating point, and sprintf is the system's. */
    static const struct {
        int64_t num, den;
        const char *want;
    } known[] = {
        { 21, 20, "1.1" }, { 3, 20, "0.1" }, { 1, 20, "0.1" }, { 49, 20, "2.5" }, { 5, 4, "1.2" },
        { 1, 8, "0.1" }, { 3, 8, "0.4" }, { 5, 8, "0.6" }, { 7, 8, "0.9" }, { 0, 1, "0.0" },
    };
    char text[1024];
    JSMemoryUsage s;
    int64_t num, den;
    size_t i;
    long compared = 0;

    for (i = 0; i < sizeof known / sizeof *known; i++) {
        char want[32];
        const char *p, *q;
        size_t len;

        memset(&s, 0, sizeof s);
        s.atom_count = known[i].den;
        s.atom_size = known[i].num;
        JS_DumpMemoryUsage(text, sizeof text, &s, NULL);
        p = strstr(text, "  (");
        q = strstr(text, " per atom)");
        len = p && q && q > p + 3 ? (size_t)(q - p - 3) : 0;
        sprintf(want, "%.1f", (double)known[i].num / known[i].den);
        CHECK(!strcmp(want, known[i].want), "the C library gives %s for %lld/%lld, not %s", want, (long long)known[i].num,
              (long long)known[i].den, known[i].want);
        CHECK(len == strlen(want) && !strncmp(p + 3, want, len), "%lld/%lld: the report has [%.*s], printf %s",
              (long long)known[i].num, (long long)known[i].den, (int)len, p ? p + 3 : "", want);
    }
    /* many more, with the atoms line and with the line of memory slack, whose
       numbers go below 0 */
    for (den = 1; den <= 70; den++) {
        for (num = -300; num <= 2500; num++) {
            char want[32];
            const char *p, *q;
            size_t len;

            memset(&s, 0, sizeof s);
            s.malloc_count = 1;
            s.malloc_size = num;
            s.memory_used_count = den;
            JS_DumpMemoryUsage(text, sizeof text, &s, NULL);
            p = strstr(text, "overhead, ");
            q = strstr(text, " average slack)");
            len = p && q && q > p + 10 ? (size_t)(q - p - 10) : 0;
            sprintf(want, "%.1f", (double)num / den);
            compared++;
            if (len != strlen(want) || strncmp(p + 10, want, len)) {
                CHECK(0, "%lld/%lld: the report has [%.*s], printf %s", (long long)num, (long long)den, (int)len, p ? p + 10 : "", want);
            }
            if (num < 0)
                continue;
            memset(&s, 0, sizeof s);
            s.atom_count = den;
            s.atom_size = num;
            JS_DumpMemoryUsage(text, sizeof text, &s, NULL);
            p = strstr(text, "  (");
            q = strstr(text, " per atom)");
            len = p && q && q > p + 3 ? (size_t)(q - p - 3) : 0;
            compared++;
            if (len != strlen(want) || strncmp(p + 3, want, len)) {
                CHECK(0, "%lld/%lld: the atoms line has [%.*s], printf %s", (long long)num, (long long)den, (int)len, p ? p + 3 : "", want);
            }
        }
    }
    CHECK(compared > 190000, "only %ld values were compared", compared);
    /* a value printf would write with many digits, or that is not a number, has a word */
    memset(&s, 0, sizeof s);
    s.malloc_count = 1;
    JS_DumpMemoryUsage(text, sizeof text, &s, NULL);
    CHECK(strstr(text, "nan average slack)") != NULL, "0/0 is not written as nan");
}

int main(void)
{
    test_dump();
    test_tenths();
    printf("api-test: %ld checks, %d failures\n", checks, failures);
    return failures != 0;
}
