/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* unit checks through the public API what the scripts of compare.sh cannot
   see. It exits 0 when every check passes.

     unit */
#include <stdio.h>
#include <string.h>

#include "host.h"

static int failures;

#define CHECK(cond, ...)                                                 \
    do {                                                                \
        if (!(cond)) {                                                  \
            failures++;                                                 \
            printf("FAIL %s:%d: %s: ", __FILE__, __LINE__, #cond);      \
            printf(__VA_ARGS__);                                        \
            printf("\n");                                               \
        }                                                               \
    } while (0)

/* eval runs src as a global script in ctx and reports whether it ran. */
static int eval(JSContext *ctx, const char *src)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int ok = !JS_IsException(v);

    if (!ok) {
        JSValue e = JS_GetException(ctx);
        const char *s = JS_ToCString(ctx, e);
        printf("exception: %s\n", s ? s : "?");
        JS_FreeCString(ctx, s);
        JS_FreeValue(ctx, e);
    }
    JS_FreeValue(ctx, v);
    return ok;
}

/* eval_is runs src as a global script in ctx and reports whether its
   completion value is the string want. */
static int eval_is(JSContext *ctx, const char *src, const char *want)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    const char *s = JS_ToCString(ctx, v);
    int ok = s && strcmp(s, want) == 0;

    if (!ok)
        printf("got %s, want %s\n", s ? s : "an exception", want);
    JS_FreeCString(ctx, s);
    JS_FreeValue(ctx, v);
    return ok;
}

/* strings returns the number of strings rt holds. */
static int64_t strings(JSRuntime *rt)
{
    JSMemoryUsage m;

    JS_ComputeMemoryUsage(rt, &m);
    return m.str_count;
}

/* Regexps compiled from the same pattern and flags share the bytecode the
   cache kept, and with the cache off each has its own. */
static void test_regexp_cache(void)
{
    static const char src[] =
        "var keep = [];\n"
        "for (var i = 0; i < 200; i++) keep.push(new RegExp('x(y+)z' + (i % 4), 'g'));\n";
    int64_t on, off;
    int i;

    for (i = 0; i < 2; i++) {
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt);
        int64_t before;

        if (i == 1)
            JS_SetRegExpCache(rt, 0, 0);
        before = strings(rt);
        CHECK(eval(ctx, src), "the script threw");
        if (i == 0)
            on = strings(rt) - before;
        else
            off = strings(rt) - before;
        JS_FreeContext(ctx);
        JS_FreeRuntime(rt);
    }
    /* 200 patterns either way, 4 bytecodes with the cache, 200 without. */
    CHECK(off - on >= 190, "%lld strings with the cache, %lld without",
          (long long)on, (long long)off);
}

static int regexp_polls;

static int count_regexp_polls(JSRuntime *rt, void *opaque)
{
    regexp_polls++;
    return 0;
}

/* compile reports whether ctx compiled the regexp of pats[i] rather than
   took its bytecode from the cache: the compiler counts its steps toward
   the interrupt counter, and a pattern of 60 KB calls the interrupt handler
   many times, where the cache counts a step for each 32 bytes and calls it
   at most once. */
static int compile(JSContext *ctx, int i)
{
    char src[64];

    snprintf(src, sizeof(src), "new RegExp(pats[%d]), 0", i);
    regexp_polls = 0;
    CHECK(eval(ctx, src), "the script threw");
    return regexp_polls > 1;
}

/* hits compiles pats[from] down to pats[to] and returns how many came from
   the cache. */
static int hits(JSContext *ctx, int from, int to)
{
    int i, n = 0;

    for(i = from; i >= to; i--)
        n += !compile(ctx, i);
    return n;
}

/* fill compiles pats[0] to pats[n - 1]. */
static void fill(JSContext *ctx, int n)
{
    int i;

    for(i = 0; i < n; i++)
        compile(ctx, i);
}

/* the patterns of 60 KB, each its own string */
static const char pats_src[] =
    "var pats = [], big = '(?:a|b|c)+d'.repeat(5000)\n"
    "for (var i = 0; i < 40; i++) pats.push([big, i].join(''))\n";

/* new_regexp_context makes a runtime and a context with pats, whose regexp
   cache keeps at most n entries in at most bytes bytes */
static JSContext *new_regexp_context(int n, size_t bytes)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);

    CHECK(eval(ctx, pats_src), "the script threw");
    JS_SetRegExpCache(rt, n, bytes);
    JS_SetInterruptHandler(rt, count_regexp_polls, NULL);
    return ctx;
}

static void free_regexp_context(JSContext *ctx)
{
    JSRuntime *rt = JS_GetRuntime(ctx);

    JS_FreeContext(ctx);
    JS_FreeRuntime(rt);
}

/* kept_regexps returns how many of the 40 patterns, compiled in order, the
   cache of n entries in bytes bytes kept */
static int kept_regexps(int n, size_t bytes)
{
    JSContext *ctx = new_regexp_context(n, bytes);
    int k;

    fill(ctx, 40);
    /* from the newest down, so that a miss that pushes out the oldest does
       not push out one still to be asked for */
    k = hits(ctx, 39, 0);
    free_regexp_context(ctx);
    return k;
}

/* The limits of the regexp cache hold: at most n entries, and entries of at
   most bytes bytes in all. Setting the limits lets every entry go. A regexp
   keeps working once the cache let its bytecode go. When an allocation
   fails while a regexp is compiled and kept, the cache stays whole and keeps
   working. A runtime keeps its cache once the context that filled it is
   gone. */
static void test_regexp_cache_limits(void)
{
    static const char shared[] =
        "var r1 = new RegExp('x(y+)z', 'g'), r2 = new RegExp('x(y+)z', 'g')\n";
    static const char next[] = "new RegExp('q(r+)s' + k++)";
    JSRuntime *rt;
    JSContext *ctx, *ctx2;
    JSMemoryUsage m;
    JSValue v;
    int n, n1, n2;
    size_t slack;
    int ok = 0;

    ctx = new_regexp_context(0, 0);
    n = hits(ctx, 0, 0);
    CHECK(n == 0, "a hit with the cache off");
    n = compile(ctx, 0);
    CHECK(n == 1, "pats[0] was not compiled");
    free_regexp_context(ctx);
    n = kept_regexps(256, 64 << 20);
    CHECK(n == 40, "%d of 40 kept", n);
    n = kept_regexps(8, 64 << 20);
    CHECK(n == 8, "%d kept with 8 entries", n);
    n = kept_regexps(256, 1);
    CHECK(n == 0, "%d kept in 1 byte", n);
    n1 = kept_regexps(256, 400 << 10);
    n2 = kept_regexps(256, 800 << 10);
    CHECK(n1 > 0 && n1 < n2 && n2 < 40, "%d kept in 400 KB and %d in 800 KB",
          n1, n2);

    ctx = new_regexp_context(8, 64 << 20);
    rt = JS_GetRuntime(ctx);
    fill(ctx, 8);
    JS_SetRegExpCache(rt, 8, 64 << 20);
    n = hits(ctx, 7, 0);
    CHECK(n == 0, "%d kept after the limits were set again", n);

    CHECK(eval(ctx, shared), "the script threw");
    JS_SetRegExpCache(rt, 0, 0);
    JS_RunGC(rt);
    CHECK(eval_is(ctx, "r1.exec('axyyz')[1] + r2.exec('xyz')[1]", "yyy"),
          "regexps whose bytecode the cache let go");
    JS_SetRegExpCache(rt, 8, 64 << 20);

    /* Let each allocation fail in turn: the limit grows until the compile
       runs. */
    CHECK(eval(ctx, "var k = 0"), "the script threw");
    for(slack = 0; slack < 1 << 16 && !ok; slack += 16) {
        JS_ComputeMemoryUsage(rt, &m);
        JS_SetMemoryLimit(rt, m.malloc_size + slack);
        v = JS_Eval(ctx, next, strlen(next), "<unit>", JS_EVAL_TYPE_GLOBAL);
        ok = !JS_IsException(v);
        if (!ok)
            JS_FreeValue(ctx, JS_GetException(ctx));
        JS_FreeValue(ctx, v);
        JS_SetMemoryLimit(rt, (size_t)-1);
        CHECK(eval_is(ctx, "new RegExp('q(r+)s').exec('qrrs')[1]", "rr"),
              "a regexp after a failed allocation at %zu bytes", slack);
    }
    CHECK(ok, "the compile never ran");
    compile(ctx, 1);
    CHECK(!compile(ctx, 1), "no hit after the failed allocations");

    ctx2 = JS_NewContext(rt);
    CHECK(eval(ctx2, pats_src), "the script threw");
    CHECK(compile(ctx2, 2), "pats[2] was not compiled");
    JS_FreeContext(ctx2);
    JS_RunGC(rt);
    CHECK(!compile(ctx, 2), "the cache lost the entry of the freed context");
    free_regexp_context(ctx);
}

int main(void)
{
    test_regexp_cache();
    test_regexp_cache_limits();
    if (failures) {
        printf("unit: %d checks failed\n", failures);
        return 1;
    }
    printf("unit: all checks passed\n");
    return 0;
}
