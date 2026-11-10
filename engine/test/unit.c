/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* unit checks through the public API what the scripts of compare.sh cannot
   see. It exits 0 when every check passes.

     unit */
#include <limits.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "host.h"
#include "cutils.h"
#include "libregexp.h"
#include "libunicode.h"

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

/* interrupt() sets the interrupt flag of the runtime of the script that
   calls it, as a host does to stop a script. */
static JSValue js_interrupt(JSContext *ctx, JSValueConst this_val, int argc,
                            JSValueConst *argv)
{
    (void)this_val;
    (void)argc;
    (void)argv;
    *JS_GetInterruptFlag(JS_GetRuntime(ctx)) = 1;
    return JS_UNDEFINED;
}

/* add_interrupt gives the global object of ctx interrupt(), and T(x), an
   object whose conversion to a string or a number calls interrupt() and
   gives x. A built-in that converts an argument first and then does its
   work sees the flag set only in that work. */
static void add_interrupt(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx);

    JS_SetPropertyStr(ctx, g, "interrupt",
                      JS_NewCFunction(ctx, js_interrupt, "interrupt", 0));
    JS_FreeValue(ctx, g);
    CHECK(eval(ctx, "var T = x => ({ toString() { interrupt(); return x }, "
                    "valueOf() { interrupt(); return x } })"),
          "the script threw");
}

/* interrupted runs src in ctx, reports whether it threw "interrupted", and
   clears the flag again. */
static int interrupted(JSContext *ctx, const char *src)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int r = 0;

    /* clear first: the conversion of the error to a string runs code */
    *JS_GetInterruptFlag(JS_GetRuntime(ctx)) = 0;
    if (JS_IsException(v)) {
        JSValue e = JS_GetException(ctx);
        const char *m = JS_ToCString(ctx, e);
        r = m && strstr(m, "interrupted") != NULL;
        if (!r)
            printf("%s threw %s\n", src, m ? m : "?");
        JS_FreeCString(ctx, m);
        JS_FreeValue(ctx, e);
    }
    JS_FreeValue(ctx, v);
    return r;
}

/* run_jobs ends the job that ran (JS_ClearKeptObjects), runs the jobs of rt,
   each its own job, until none is left, and reports whether none threw. */
static int run_jobs(JSRuntime *rt)
{
    JSContext *ctx;
    int r, ok = 1;

    for (;;) {
        JS_ClearKeptObjects(rt);
        r = JS_ExecutePendingJob(rt, &ctx);
        if (r == 0)
            break;
        if (r < 0) {
            JS_FreeValue(ctx, JS_GetException(ctx));
            ok = 0;
        }
    }
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
        host_free_runtime(rt);
    }
    /* 200 patterns either way, 4 bytecodes with the cache, 200 without. */
    CHECK(off - on >= 190, "%lld strings with the cache, %lld without",
          (long long)on, (long long)off);
}

/* compile reports whether ctx compiled the regexp of pats[i] rather than
   took its bytecode from the cache. A first try has the pattern set the
   interrupt flag as it becomes a string: the compiler reads the flag at its
   first step and stops, where the cache never reads it. When it stopped,
   the regexp is compiled again, so that the cache keeps it as after any
   miss. */
static int compile(JSContext *ctx, int i)
{
    char src[64];

    snprintf(src, sizeof(src), "new RegExp(T(pats[%d])), 0", i);
    if (!interrupted(ctx, src))
        return 0;
    snprintf(src, sizeof(src), "new RegExp(pats[%d]), 0", i);
    CHECK(eval(ctx, src), "the script threw");
    return 1;
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
    add_interrupt(ctx);
    return ctx;
}

static void free_regexp_context(JSContext *ctx)
{
    JSRuntime *rt = JS_GetRuntime(ctx);

    JS_FreeContext(ctx);
    host_free_runtime(rt);
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
   fails while a regexp is compiled and kept, the cache keeps the entries it
   had and keeps working. A runtime keeps its cache once the context that filled it is
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
       runs. After each failure the entry of pats[3] is still there. */
    CHECK(compile(ctx, 3), "pats[3] was not compiled");
    CHECK(!compile(ctx, 3), "no hit for pats[3]");
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
        CHECK(!compile(ctx, 3),
              "pats[3] lost after a failed allocation at %zu bytes", slack);
    }
    CHECK(ok, "the compile never ran");
    compile(ctx, 1);
    CHECK(!compile(ctx, 1), "no hit after the failed allocations");

    ctx2 = JS_NewContext(rt);
    CHECK(eval(ctx2, pats_src), "the script threw");
    add_interrupt(ctx2);
    CHECK(compile(ctx2, 2), "pats[2] was not compiled");
    JS_FreeContext(ctx2);
    JS_RunGC(rt);
    CHECK(!compile(ctx, 2), "the cache lost the entry of the freed context");
    free_regexp_context(ctx);
}

/* shapes returns the number of shapes rt holds. */
static int64_t shapes(JSRuntime *rt)
{
    JSMemoryUsage m;

    JS_ComputeMemoryUsage(rt, &m);
    return m.shape_count;
}

/* run_shapes runs src in a new runtime whose shape cache keeps n shapes and
   returns the number of shapes it left. With off, it turns the cache off
   again before counting. */
static int64_t run_shapes(int n, int off, const char *src)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    int64_t before, after;

    before = shapes(rt);
    CHECK(JS_SetShapeCache(ctx, n) == 0, "no ring of %d shapes", n);
    CHECK(eval(ctx, src), "the script threw");
    if (off)
        JS_SetShapeCache(ctx, 0);
    after = shapes(rt);
    JS_SetShapeCache(ctx, 0);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return after - before;
}

/* kept returns the number of shapes the cache keeps after src ran with a
   ring of n. */
static int64_t kept(int n, const char *src)
{
    return run_shapes(n, 0, src) - run_shapes(0, 0, src);
}

/* The shape cache keeps the shapes of plain objects whose objects are gone,
   in a ring, and only those that hold nothing else alive. */
static void test_shape_cache(void)
{
    /* the empty shape and the shapes of 1 to 10 keys */
    static const char ten[] =
        "{ let o = {}; o.k0 = 0; o.k1 = 1; o.k2 = 2; o.k3 = 3; o.k4 = 4;\n"
        "  o.k5 = 5; o.k6 = 6; o.k7 = 7; o.k8 = 8; o.k9 = 9; o = null }\n";
    /* The shapes of 0 to 31 keys are copied and kept. The shape of 32 keys
       grows in place, and the shape of 40 is not kept. */
    static const char forty[] =
        "{ let o = {}; for (let i = 0; i < 40; i++) o['p' + i] = i; o = null }\n";
    static const char symbol[] =
        "{ let s = Symbol('s'); let o = { a: 1 }; o[s] = 2; o = s = null }\n";
    static const char registered[] =
        "{ let s = Symbol.for('s'); let o = { a: 1 }; o[s] = 2; o = s = null }\n";
    static const char no_proto[] =
        "{ let o = Object.create(null); o.a = 1; o.b = 2; o = null }\n";
    /* Keys no other object has. The first object copies the empty shape,
       which the cache keeps, and the next ones share it. The first objects
       copy their shape of one key, which the cache keeps with the shape of
       two, until the 64 copies of credit are spent, 63 of them on these.
       Then each object grows its shape in place and only the shape of two
       keys is kept. */
    static const char unique[] =
        "for (let i = 0; i < 200; i++) { let o = {}; o['a' + i] = 1; o['b' + i] = 2 }\n";
    int64_t n;

    n = kept(2048, ten);
    CHECK(n == 11, "%lld shapes kept of 10 keys", (long long)n);
    n = kept(4, ten);
    CHECK(n == 4, "%lld shapes kept in a ring of 4", (long long)n);
    n = run_shapes(2048, 1, ten) - run_shapes(0, 0, ten);
    CHECK(n == 0, "%lld shapes left with the cache turned off", (long long)n);
    n = kept(2048, forty);
    CHECK(n == 32, "%lld shapes kept of 40 keys", (long long)n);
    n = kept(2048, registered) - kept(2048, symbol);
    CHECK(n == 1, "%lld more shapes kept with a registered symbol", (long long)n);
    n = kept(2048, no_proto);
    CHECK(n == 0, "%lld shapes kept without a prototype", (long long)n);
    n = kept(2048, unique);
    CHECK(n == 1 + 63 + 200, "%lld shapes kept of unique keys", (long long)n);
}

/* The shape cache across the collectors and the end of a runtime: a full
   and a young collection leave the kept shapes, and the cycle collector
   frees the shapes of the cycles it frees, which the cache cannot keep. A
   ring that is full lets its oldest shapes go, before and after a
   collection. A runtime freed with shapes in the ring frees them and its
   context. An allocation that fails leaves the ring whole. */
static void test_shape_cache_lifetime(void)
{
    static const char ten[] =
        "{ let o = {}; o.k0 = 0; o.k1 = 1; o.k2 = 2; o.k3 = 3; o.k4 = 4;\n"
        "  o.k5 = 5; o.k6 = 6; o.k7 = 7; o.k8 = 8; o.k9 = 9; o = null }\n";
    /* objects that hold each other, with keys no other object has */
    static const char cycle[] =
        "{ let a = {}, b = {}; a.cy0 = b; a.cy1 = 1; b.cz0 = a; b.cz1 = 1; a = b = null }\n";
    static const char grow[] =
        "{ let o = {}; for (let i = 0; i < 30; i++) o['g' + i] = i; o = null }\n";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSMemoryUsage m;
    JSValue v;
    int64_t base, n0, n;
    size_t slack;
    int ok = 0;

    CHECK(JS_SetShapeCache(ctx, 2048) == 0, "no ring");
    base = shapes(rt);
    CHECK(eval(ctx, ten), "the script threw");
    n0 = shapes(rt) - base;
    CHECK(n0 >= 11, "%lld shapes kept of 10 keys", (long long)n0);
    JS_RunGC(rt);
    n = shapes(rt) - base;
    CHECK(n == n0, "a full collection left %lld of %lld shapes", (long long)n,
          (long long)n0);
    JS_RunYoungGC(rt);
    n = shapes(rt) - base;
    CHECK(n == n0, "a young collection left %lld of %lld shapes", (long long)n,
          (long long)n0);
    CHECK(eval(ctx, cycle), "the script threw");
    JS_RunGC(rt);
    n = shapes(rt) - base;
    CHECK(n == n0 + 2, "%lld shapes more after the cycle collector, want 2 "
          "(the shapes of one key)", (long long)(n - n0));

    /* a ring of 4 keeps the last 4 shapes */
    CHECK(JS_SetShapeCache(ctx, 4) == 0, "no ring");
    base = shapes(rt);
    CHECK(eval(ctx, grow), "the script threw");
    n = shapes(rt) - base;
    CHECK(n <= 4, "%lld shapes kept in a ring of 4", (long long)n);
    JS_RunGC(rt);
    CHECK(shapes(rt) - base == n, "a collection changed the ring of 4");

    /* Let each allocation fail in turn while objects grow: the shapes kept
       before are still there after, so the shapes of ten are found again
       and none is made. */
    CHECK(JS_SetShapeCache(ctx, 2048) == 0, "no ring");
    CHECK(eval(ctx, ten), "the script threw");
    for(slack = 0; slack < 1 << 16 && !ok; slack += 16) {
        JS_ComputeMemoryUsage(rt, &m);
        JS_SetMemoryLimit(rt, m.malloc_size + slack);
        v = JS_Eval(ctx, grow, strlen(grow), "<unit>", JS_EVAL_TYPE_GLOBAL);
        ok = !JS_IsException(v);
        if (!ok)
            JS_FreeValue(ctx, JS_GetException(ctx));
        JS_FreeValue(ctx, v);
        JS_SetMemoryLimit(rt, (size_t)-1);
        n = shapes(rt);
        CHECK(eval(ctx, ten), "the script threw");
        CHECK(shapes(rt) == n, "the shapes of ten were made again after a "
              "failed allocation at %zu bytes", slack);
    }
    CHECK(ok, "the objects never grew");
    JS_SetShapeCache(ctx, 0);
    JS_FreeContext(ctx);
    host_free_runtime(rt);

    /* freed with shapes in the ring, which hold the context */
    rt = host_new_runtime();
    ctx = JS_NewContext(rt);
    CHECK(JS_SetShapeCache(ctx, 2048) == 0, "no ring");
    CHECK(eval(ctx, ten), "the script threw");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The shapes of new functions a context holds hold its Function.prototype,
   and with it the context: once the context is freed, the cycle collector
   frees it with everything it made, which it can only when JS_MarkContext
   marks the shapes. In a live context the cycle collector leaves them. */
static void test_func_shapes(void)
{
    static const char kinds[] =
        "var f = [function () {}, async function () {}, function* () {},\n"
        "  async function* () {}, (a, b) => a, class {}, { m() {} }.m]\n";
    static const char check[] =
        "var h = function (a, b, c) {};\n"
        "[h.length, h.name, Object.getOwnPropertyNames(h)].join()\n";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx1 = JS_NewContext(rt), *ctx2;
    JSMemoryUsage before, after;

    CHECK(eval(ctx1, kinds), "the script threw");
    JS_RunGC(rt);
    JS_ComputeMemoryUsage(rt, &before);
    ctx2 = JS_NewContext(rt);
    CHECK(eval(ctx2, kinds), "the script threw");
    JS_FreeContext(ctx2);
    JS_RunGC(rt);
    JS_ComputeMemoryUsage(rt, &after);
    CHECK(after.obj_count == before.obj_count &&
          after.shape_count == before.shape_count,
          "%lld objects and %lld shapes left of a freed context",
          (long long)(after.obj_count - before.obj_count),
          (long long)(after.shape_count - before.shape_count));
    CHECK(eval_is(ctx1, check, "3,h,length,name,prototype"),
          "a function after the cycle collector");
    JS_FreeContext(ctx1);
    host_free_runtime(rt);
}

/* Each of the four prototypes of new functions keeps two shapes, the one a
   new function starts with and the one with length and name, and new
   functions reuse them: once the functions are gone, the context holds the
   two, and a hundred more functions of the kind make no more. The built-in
   functions a context starts with made those of Function.prototype already.
   An object made with the prototype finds the empty shape the context
   keeps. Two contexts used by turns keep their own, and freeing one frees
   its shapes and leaves the other's, which new functions still use. */
static void test_func_shapes_kept(void)
{
    static const char *const kinds[] = {
        "function () {}", "async function () {}", "function* () {}",
        "async function* () {}",
    };
    char src[128];
    JSRuntime *rt;
    JSContext *ctx, *ctx2;
    int64_t base, one, many, both, two, n;
    int i;

    for(i = 0; i < 4; i++) {
        rt = host_new_runtime();
        ctx = JS_NewContext(rt);
        JS_RunGC(rt);
        base = shapes(rt);
        snprintf(src, sizeof(src), "(%s)", kinds[i]);
        CHECK(eval(ctx, src), "the script threw");
        JS_RunGC(rt);
        one = shapes(rt) - base;
        snprintf(src, sizeof(src), "for (var i = 0; i < 100; i++) (%s)",
                 kinds[i]);
        CHECK(eval(ctx, src), "the script threw");
        JS_RunGC(rt);
        many = shapes(rt) - base;
        CHECK(one == (i ? 2 : 0) && many == one, "%s: %lld shapes kept after "
              "one, %lld after a hundred", kinds[i], (long long)one,
              (long long)many);
        /* an object of the prototype finds the empty shape the context
           keeps */
        CHECK(eval(ctx, "var keep"), "the script threw");
        snprintf(src, sizeof(src),
                 "keep = Object.create(Object.getPrototypeOf(%s))", kinds[i]);
        n = shapes(rt);
        CHECK(eval(ctx, src), "the script threw");
        n = shapes(rt) - n;
        CHECK(n == 0, "%s: an object of its prototype made %lld shapes",
              kinds[i], (long long)n);
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }

    rt = host_new_runtime();
    ctx = JS_NewContext(rt);
    JS_RunGC(rt);
    base = shapes(rt);
    ctx2 = JS_NewContext(rt);
    JS_RunGC(rt);
    both = shapes(rt);
    for(i = 0; i < 4; i++) {
        snprintf(src, sizeof(src), "var f%d = %s", i, kinds[i]);
        CHECK(eval(ctx, src), "the script threw");
        CHECK(eval(ctx2, src), "the script threw");
    }
    JS_RunGC(rt);
    two = shapes(rt) - both;
    JS_FreeContext(ctx2);
    JS_RunGC(rt);
    n = shapes(rt) - base;
    CHECK(n > 0 && 2 * n == two, "%lld shapes for the functions of two "
          "contexts, %lld left with one freed", (long long)two, (long long)n);
    CHECK(eval_is(ctx, "[f0, f1, f2, f3].map((f) => f.length + f.name + "
                  "Object.getOwnPropertyNames(f).length).join()",
                  "0f03,0f12,0f23,0f33"), "the functions of the context");
    CHECK(eval(ctx, "for (var i = 0; i < 10; i++) [function () {}, async "
               "function () {}, function* () {}, async function* () {}]"),
          "the script threw");
    JS_RunGC(rt);
    CHECK(shapes(rt) - base == n, "%lld shapes after more functions, want "
          "%lld", (long long)(shapes(rt) - base), (long long)n);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A context holds the empty shape of the prototype of each built-in class
   once an object of the class is made with it, so the next objects of the
   class find it: once they are gone the context keeps that one shape, and a
   hundred more objects make no more. new Array(n) starts from the shape of
   arrays, which has length, and holds no other. Freeing a context frees the
   shapes it holds, and leaves those of another context. */
static void test_class_shapes(void)
{
    static const struct {
        const char *src;
        int kept;
    } kinds[] = {
        { "new Map()", 1 },
        { "new Set()", 1 },
        { "new Promise(function () {})", 1 },
        { "[1, 2][Symbol.iterator]()", 1 },
        { "new Map().entries()", 2 },
        { "new Date(0)", 1 },
        { "new Array(3)", 0 },
        { "Array(2)", 0 },
    };
    static const char all[] =
        "var keep = [new Map(), new Set(), new Promise(function () {}),\n"
        "  [1][Symbol.iterator](), new Date(0), new Array(3)]\n";
    char src[128];
    JSRuntime *rt;
    JSContext *ctx, *ctx2;
    int64_t base, one, many, n, two;
    int i;

    for(i = 0; i < (int)countof(kinds); i++) {
        rt = host_new_runtime();
        ctx = JS_NewContext(rt);
        JS_RunGC(rt);
        base = shapes(rt);
        snprintf(src, sizeof(src), "%s", kinds[i].src);
        CHECK(eval(ctx, src), "the script threw");
        JS_RunGC(rt);
        one = shapes(rt) - base;
        snprintf(src, sizeof(src), "for (var i = 0; i < 100; i++) %s",
                 kinds[i].src);
        CHECK(eval(ctx, src), "the script threw");
        JS_RunGC(rt);
        many = shapes(rt) - base;
        CHECK(one == kinds[i].kept && many == one, "%s: %lld shapes kept "
              "after one, %lld after a hundred, want %d", kinds[i].src,
              (long long)one, (long long)many, kinds[i].kept);
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }

    rt = host_new_runtime();
    ctx = JS_NewContext(rt);
    CHECK(eval(ctx, all), "the script threw");
    JS_RunGC(rt);
    base = shapes(rt);
    /* a context and its shapes both young: the young collection keeps
       them */
    ctx2 = JS_NewContext(rt);
    CHECK(eval(ctx2, all), "the script threw");
    JS_RunYoungGC(rt);
    CHECK(eval_is(ctx2, "keep.map((o) => Reflect.ownKeys(o).length).join()",
                  "0,0,0,0,0,1"), "the objects of a young context");
    JS_RunGC(rt);
    two = shapes(rt);
    JS_FreeContext(ctx2);
    JS_RunYoungGC(rt);
    JS_RunGC(rt);
    n = shapes(rt);
    CHECK(n == base && two > base, "%lld shapes with one context, %lld with "
          "two, %lld with the second freed", (long long)base, (long long)two,
          (long long)n);
    CHECK(eval_is(ctx, "keep.map((o) => Reflect.ownKeys(o).length).join()",
                  "0,0,0,0,0,1"), "the objects of the context left");
    /* an old context that holds a shape made since the last collection: the
       young collection keeps it */
    JS_RunGC(rt);
    base = shapes(rt);
    CHECK(eval(ctx, "new WeakSet()"), "the script threw");
    JS_RunYoungGC(rt);
    CHECK(shapes(rt) == base + 1, "%lld shapes after a WeakSet, want %lld",
          (long long)shapes(rt), (long long)(base + 1));
    CHECK(eval_is(ctx, "var w = new WeakSet(); w.add(keep[0]); "
                  "[w.has(keep[0]), Reflect.ownKeys(w).length].join()",
                  "true,0"), "a WeakSet after the young collection");
    JS_RunGC(rt);
    CHECK(shapes(rt) == base + 1, "%lld shapes after the collection, want "
          "%lld", (long long)shapes(rt), (long long)(base + 1));
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

static int proto_freed;

static void proto_finalizer(JSRuntime *rt, JSValue val)
{
    proto_freed++;
}

/* Whether the prototype of obj is want. */
static int proto_is(JSContext *ctx, JSValueConst obj, JSValueConst want)
{
    JSValue v = JS_GetPrototype(ctx, obj);
    int same = JS_VALUE_GET_PTR(v) == JS_VALUE_GET_PTR(want);

    JS_FreeValue(ctx, v);
    return same;
}

/* The empty shape a context holds for the prototype of a built-in class
   holds that prototype. JS_SetClassProto lets the shape go with the
   prototype it replaces, so that prototype is freed once the host drops it,
   and the next object of the class gets the new prototype. */
static void test_class_shape_set_proto(void)
{
    static const char src[] = "new Map()";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSClassDef def = { .class_name = "Proto", .finalizer = proto_finalizer };
    JSClassID pclass = 0, map_class;
    JSValue map, p, q, obj;

    JS_NewClassID(rt, &pclass);
    CHECK(JS_NewClass(rt, pclass, &def) == 0, "JS_NewClass failed");
    map = JS_Eval(ctx, src, sizeof(src) - 1, "<test>", JS_EVAL_TYPE_GLOBAL);
    CHECK(JS_IsObject(map), "new Map() threw");
    map_class = JS_GetClassID(map);
    JS_FreeValue(ctx, map);

    p = JS_NewObjectClass(ctx, pclass);
    q = JS_NewObject(ctx);
    CHECK(JS_IsObject(p) && JS_IsObject(q), "the prototypes");
    JS_SetClassProto(ctx, map_class, JS_DupValue(ctx, p));
    /* the context takes the empty shape of p */
    obj = JS_NewObjectClass(ctx, map_class);
    CHECK(JS_IsObject(obj) && proto_is(ctx, obj, p),
          "an object of the class with p");
    JS_FreeValue(ctx, obj);

    JS_SetClassProto(ctx, map_class, JS_DupValue(ctx, q));
    proto_freed = 0;
    JS_FreeValue(ctx, p);
    JS_RunGC(rt);
    CHECK(proto_freed == 1, "the prototype replaced was freed %d times, "
          "want 1", proto_freed);
    obj = JS_NewObjectClass(ctx, map_class);
    CHECK(JS_IsObject(obj) && proto_is(ctx, obj, q),
          "an object of the class with q");
    JS_FreeValue(ctx, obj);
    JS_FreeValue(ctx, q);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The bytes the property arrays of the objects that src makes take. */
static int64_t lit_prop_bytes(JSRuntime *rt, JSContext *ctx, const char *src)
{
    JSMemoryUsage before, after;

    JS_RunGC(rt);
    JS_ComputeMemoryUsage(rt, &before);
    CHECK(eval(ctx, src), "the script threw");
    JS_RunGC(rt);
    JS_ComputeMemoryUsage(rt, &after);
    return after.prop_size - before.prop_size;
}

/* An object literal with only plain keys gets a property array of exactly
   its size: a thousand objects of five keys take five fourths of the bytes
   of a thousand of four keys, give or take the few bytes of the other
   objects a run makes, where growing the array one key at a time gives eight
   slots to five keys. */
static void test_object_lit_size(void)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    int64_t four, five;

    CHECK(eval(ctx, "var a4 = [], a5 = []"), "the script threw");
    four = lit_prop_bytes(rt, ctx, "for (let i = 0; i < 1000; i++) "
                          "a4.push({ a: i, b: i, c: i, d: i })");
    five = lit_prop_bytes(rt, ctx, "for (let i = 0; i < 1000; i++) "
                          "a5.push({ a: i, b: i, c: i, d: i, e: i })");
    CHECK(four > 0 && llabs(4 * five - 5 * four) < four / 100,
          "%lld bytes for five keys, %lld for four", (long long)five,
          (long long)four);
    CHECK(eval_is(ctx, "JSON.stringify([a4[999], a5[7]])",
                  "[{\"a\":999,\"b\":999,\"c\":999,\"d\":999},"
                  "{\"a\":7,\"b\":7,\"c\":7,\"d\":7,\"e\":7}]"),
          "the objects");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}


/* The blocks the heap holds, and in *bytes the bytes they take, after a full
   collection. */
static int64_t blocks_now(JSRuntime *rt, int64_t *bytes)
{
    JSMemoryUsage u;

    JS_RunGC(rt);
    JS_ComputeMemoryUsage(rt, &u);
    if (bytes)
        *bytes = u.malloc_size;
    return u.malloc_count;
}

/* The properties of an object literal sit in the block of the object: a
   thousand objects of four keys take 112 bytes each, not 120. When they get
   more keys than the block has room for, lose keys, are frozen or given
   another prototype, they keep their properties and all the memory is given
   back once they are dropped. */
static void test_object_lit_one_block(void)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    int64_t base, blocks, base_bytes, bytes, json_bytes;

    JS_SetShapeCache(ctx, 0);
    CHECK(eval(ctx, "var a4 = new Array(1000).fill(0), a5 = new Array(1000).fill(0), m = [];\n"
               "function fill() { for (let i = 0; i < 1000; i++) a4[i] = { a: i, b: i, c: i, d: i } }\n"
               "function fill_json() { for (let i = 0; i < 1000; i++) a5[i] = JSON.parse('{\"a\":1,\"b\":2,\"c\":3,\"d\":4}') }"),
          "the script threw");
    base = blocks_now(rt, &base_bytes);
    CHECK(eval(ctx, "fill()"), "the script threw");
    blocks_now(rt, &bytes);
    bytes -= base_bytes;
    CHECK(eval_is(ctx, "JSON.stringify(a4[999])", "{\"a\":999,\"b\":999,\"c\":999,\"d\":999}"),
          "the object");
    /* objects of the same four keys made one key at a time keep their
       properties in a block of their own, which has a header of its own */
    CHECK(eval(ctx, "a4.fill(0); fill_json()"), "the script threw");
    blocks_now(rt, &json_bytes);
    json_bytes -= base_bytes;
    CHECK(bytes + 1000 * 4 < json_bytes, "%lld bytes for 1000 objects of 4 keys in a block, %lld in two",
          (long long)bytes, (long long)json_bytes);
    /* the same kind of objects, then grown, trimmed, frozen and moved on */
    CHECK(eval(ctx,
               "for (let i = 0; i < 300; i++) {\n"
               "  let o = { a: i, b: [i], c: 'x' + i };\n"
               "  if (i % 6 == 0) { o.d = i; o.e = i; o.f = i; o.g = i; o.h = i }\n"
               "  if (i % 6 == 1) { delete o.a; o.z = i }\n"
               "  if (i % 6 == 2) Object.freeze(o);\n"
               "  if (i % 6 == 3) Object.setPrototypeOf(o, { p: 1 });\n"
               "  if (i % 6 == 4) { for (let k = 0; k < 40; k++) o['k' + k] = k;\n"
               "    for (let k = 0; k < 40; k += 2) delete o['k' + k] }\n"
               "  if (i % 6 == 5) Object.defineProperty(o, 'b', { get() { return 7 } });\n"
               "  m.push(o)\n"
               "}"), "the script threw");
    CHECK(eval_is(ctx,
                  "[m[0], m[1], m[2], m[3], m[5], m[4]].map(o => Object.keys(o).length + ':' + o.c + ':' + o.b).join()",
                  "8:x0:0,3:x1:1,3:x2:2,3:x3:3,3:x5:7,23:x4:4"),
          "the objects after their changes");
    CHECK(eval_is(ctx, "m[3].p + ' ' + Object.isFrozen(m[2]) + ' ' + Object.isFrozen(m[8]) + ' ' + ('a' in m[1])",
                  "1 true true false"), "prototype and frozen");
    CHECK(eval(ctx, "a4 = a5 = m = null"), "the script threw");
    blocks = blocks_now(rt, NULL) - base;
    CHECK(blocks < 40, "%lld blocks left after dropping the objects", (long long)blocks);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A string key that is already an atom takes the reference JS_DupAtom
   gives: an atom made from a string at run time and used as a key many
   times is freed with the last string and object that hold it. */
static void test_atom_keys(void)
{
    static const char src[] =
        "{ let k = '#zzkey'.slice(1), o = {}; o[k] = 1;\n"
        "  for (let i = 0; i < 100; i++) { o[k] += i; k in o; delete o[k]; o[k] = i }\n"
        "  o = k = null }\n";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSMemoryUsage before, after;

    JS_ComputeMemoryUsage(rt, &before);
    CHECK(eval(ctx, src), "the script threw");
    JS_ComputeMemoryUsage(rt, &after);
    CHECK(after.atom_count == before.atom_count, "%lld atoms left",
          (long long)(after.atom_count - before.atom_count));
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The key cache across the cycle collector: an object in a cycle whose keys
   were listed is freed by the collector with its shape, and the memory of
   that shape goes to the shapes of new objects with other kinds of keys,
   with the shape cache off. A listing must not take the kinds of keys of
   the freed shape. */
static void test_own_keys_collected(void)
{
    static const char setup[] =
        "var bad = 0, r = 0\n"
        "function make() { var c = {}; c.self = c; c['q' + r] = 1; c[r % 3] = 2;\n"
        "  Object.keys(c); Reflect.ownKeys(c) }\n"
        "function check() { var d = {}; d[Symbol()] = 1; d['w' + r] = 2; d.self = d;\n"
        "  if (Object.keys(d).join() !== 'w' + r + ',self' ||\n"
        "      Reflect.ownKeys(d).map(String).join() !== 'w' + r + ',self,Symbol()') bad++\n"
        "  d.self = null; r++ }\n";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    int i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < 200; i++) {
        CHECK(eval(ctx, "make()"), "the script threw");
        JS_RunGC(rt);
        CHECK(eval(ctx, "check()"), "the script threw");
    }
    CHECK(eval_is(ctx, "r + ' ' + bad", "200 0"), "listings after collections");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The interrupt flag stops a script where the engine reads it: at calls
   and backward jumps of the interpreter, and inside the long work of a
   built-in, which T(x) reaches with the flag set after its arguments were
   converted. Each one throws "interrupted", which try does not catch, and
   the context runs scripts again once the flag is clear. A short call that
   reads no flag returns, and the flag stays for the next read. */
static void test_interrupt_points(void)
{
    static const char setup[] =
        "var big = 'x'.repeat(1 << 22), ta = new Int32Array(1 << 22), u8 = new Uint8Array(1 << 23)\n"
        "var f64 = new Float64Array(1 << 21), json = '[' + Array(1 << 18).fill(1).join() + ']'\n"
        "var arr = Array(1 << 18).fill(1)\n";
    static const char *const stops[] = {
        "for (var i = 0;; i++) if (i == 1000) interrupt()",
        "interrupt(); for (;;) {}",
        "function f() { return 1 } interrupt(); f()",
        "try { for (var i = 0;; i++) if (i == 10) interrupt() } catch (e) { 'caught' } finally { 'finally' }",
        "/(a+)+b/.test(T('a'.repeat(40)))",
        "new RegExp(T('(?:a|b)+c'.repeat(100)))",
        "big.indexOf(T('y'))",
        "big.lastIndexOf(T('yx'))",
        "'ab'.repeat(T(1 << 22))",
        "'a'.repeat(T(1 << 23))",
        "ta.indexOf(1, T(0))",
        "ta.lastIndexOf(1, T(ta.length - 1))",
        "ta.includes(1, T(0))",
        "f64.indexOf(0.5, T(0))",
        "u8.indexOf(1, T(0))",
        "u8.lastIndexOf(1, T(u8.length - 1))",
        "u8.fill(T(7))",
        "ta.fill(T(7))",
        "u8.copyWithin(T(1), 0)",
        "u8.copyWithin(T(0), 1)",
        "JSON.parse(T(json))",
        "JSON.stringify({ toJSON() { interrupt(); return arr } })",
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    char caught[256];
    size_t i;

    add_interrupt(ctx);
    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(stops) / sizeof(stops[0]); i++) {
        CHECK(interrupted(ctx, stops[i]), "%s was not interrupted", stops[i]);
        snprintf(caught, sizeof(caught), "try { %s } catch (e) { 'caught' }",
                 stops[i]);
        CHECK(interrupted(ctx, caught), "%s was not interrupted", caught);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"),
              "the context after %s", stops[i]);
    }
    /* short work reads no flag: the result comes back, and the next call
       stops */
    CHECK(eval_is(ctx, "'ab'.indexOf(T('b'))", "1"), "a short search");
    CHECK(interrupted(ctx, "Math.abs(1)"), "the call after a short search");
    CHECK(eval_is(ctx, "new Int32Array(8).indexOf(1, T(0))", "-1"),
          "a short indexOf");
    *JS_GetInterruptFlag(rt) = 0;
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* now returns the time of the monotonic clock in seconds. */
static double now(void)
{
    struct timespec t;

    clock_gettime(CLOCK_MONOTONIC, &t);
    return t.tv_sec + t.tv_nsec * 1e-9;
}

/* A search for a needle that matches almost everywhere, a long run of one
   character with another at its end or at its start, takes time linear in
   the lengths, forward and back, in strings of 8-bit and 16-bit characters
   and in one of each. Comparing the needle at each index where its first
   character is takes about 2^34 steps for each of these, many seconds. A
   needle of 8 characters, which is compared that way, takes 8 steps for
   each index at most. */
static void test_search_linear(void)
{
    static const char setup[] =
        "var a = 'a'.repeat(1 << 18), m = 'a'.repeat(1 << 17)\n"
        "var w = '\\u4e00'.repeat(1 << 18), wm = '\\u4e00'.repeat(1 << 17)\n"
        "var aw = a + '\\u4e00'\n";
    static const struct {
        const char *src, *want;
    } cases[] = {
        { "a.indexOf(m + 'b')", "-1" },
        { "(a + 'b').indexOf(m + 'b')", "131072" },
        { "a.includes(m + 'b')", "false" },
        { "a.lastIndexOf(m + 'b')", "-1" },
        { "a.lastIndexOf('b' + m)", "-1" },
        { "('b' + a).lastIndexOf('b' + m)", "0" },
        { "w.indexOf(wm + '\\u6587')", "-1" },
        { "(w + '\\u6587').indexOf(wm + '\\u6587')", "131072" },
        { "w.lastIndexOf(wm + '\\u6587')", "-1" },
        { "w.lastIndexOf('\\u6587' + wm)", "-1" },
        { "aw.indexOf(m + 'b')", "-1" },
        { "aw.lastIndexOf('b' + m)", "-1" },
        { "(aw + m).lastIndexOf(m + '\\u4e00')", "131072" },
        { "a.indexOf('aaaaaaab')", "-1" },
        { "a.lastIndexOf('aaaaaaab')", "-1" },
        { "w.indexOf('\\u4e00'.repeat(7) + '\\u6587')", "-1" },
        { "aw.lastIndexOf('aaaaaaab')", "-1" },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    double t;
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        t = now();
        CHECK(eval_is(ctx, cases[i].src, cases[i].want), "%s", cases[i].src);
        t = now() - t;
        CHECK(t < 0.5, "%s took %.3f s", cases[i].src, t);
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A long search reads the interrupt flag while it runs, at least every
   JS_STRING_FIND_STEP units of work, with a match or none, in strings of
   each width and in one of each: with the flag set as the needle becomes a
   string, it ends with the error "interrupted", which try and catch do not
   catch, and the context then runs scripts and searches again. Comparing
   the needle at the one index a search has, as startsWith does, goes
   JS_ROPE_STEP characters at a time with the flag read after each. A needle
   of that kind is made flat with join: one made by + is a rope, and making a
   rope flat reads the flag too (test_rope_polls). */
static void test_search_interrupt(void)
{
    static const char setup[] =
        "var a = 'a'.repeat(1 << 23), m = 'a'.repeat(1000) + 'b'\n"
        "var w = '\\u4e00'.repeat(1 << 22), wm = '\\u4e00'.repeat(1000) + 'b'\n"
        "var aw = a + '\\u4e00', ab = a + 'b', ba = 'b' + a\n"
        "var awm = aw + m, wwm = w + wm, a2 = [a.slice(1), 'a'].join('')\n";
    static const struct {
        const char *hay, *call, *needle, *want;
        int stops;
    } cases[] = {
        { "a", "indexOf", "'b'", "-1", 1 },
        { "a", "lastIndexOf", "'b'", "-1", 1 },
        { "a", "indexOf", "m", "-1", 1 },
        { "a", "lastIndexOf", "'b' + m", "-1", 1 },
        { "w", "indexOf", "wm", "-1", 1 },
        { "w", "lastIndexOf", "'b'", "-1", 1 },
        { "aw", "indexOf", "m", "-1", 1 },
        { "a", "indexOf", "'aaab'", "-1", 1 },
        { "w", "lastIndexOf", "'\\u4e00\\u4e00b'", "-1", 1 },
        { "ab", "indexOf", "'b'", "8388608", 1 },
        { "ab", "indexOf", "m", "8387608", 1 },
        { "ba", "lastIndexOf", "'b'", "0", 1 },
        { "ba", "lastIndexOf", "'baaa'", "0", 1 },
        { "wwm", "indexOf", "wm", "4194304", 1 },
        { "awm", "indexOf", "m", "8388609", 1 },
        { "awm", "indexOf", "'aab'", "8389607", 1 },
        { "ab", "indexOf", "a", "0", 1 },
        { "aw", "includes", "a", "true", 1 },
        { "a", "startsWith", "a2", "true", 1 },
        { "aw", "endsWith", "[a.slice(1), '\\u4e00'].join('')", "true", 1 },
        { "a", "indexOf", "a2", "0", 1 },
        { "a", "startsWith", "[a.slice(1), 'b'].join('')", "false", 1 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    char src[256], trap[256], caught[300];
    size_t i;

    add_interrupt(ctx);
    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        snprintf(src, sizeof(src), "%s.%s(%s)", cases[i].hay, cases[i].call,
                 cases[i].needle);
        snprintf(trap, sizeof(trap), "%s.%s(T(%s))", cases[i].hay,
                 cases[i].call, cases[i].needle);
        snprintf(caught, sizeof(caught), "try { %s } catch (e) { 'caught' }",
                 trap);
        CHECK(eval_is(ctx, src, cases[i].want), "%s", src);
        if (!cases[i].stops) {
            CHECK(eval_is(ctx, trap, cases[i].want), "%s", trap);
            *JS_GetInterruptFlag(rt) = 0;
            continue;
        }
        CHECK(interrupted(ctx, trap), "%s ran to its end", trap);
        CHECK(interrupted(ctx, caught), "%s ran to its end", caught);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"),
              "the context after %s was stopped", trap);
        CHECK(eval_is(ctx, src, cases[i].want), "%s after it was stopped",
              src);
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* stop_later asks the runtime arg to stop after 20 ms, from a thread of its
   own, as a host stops a script from outside: the engine's thread sets the
   flag at its next poll (host_interrupt). */
static void *stop_later(void *arg)
{
    struct timespec d = { 0, 20 * 1000 * 1000 };

    nanosleep(&d, NULL);
    host_interrupt(arg);
    return NULL;
}

/* A sort reads the interrupt flag after each comparison and stops when it
   is set. The sort of a typed array without a comparator runs no
   JavaScript: it stops soon after a thread of the host asks it to, with its
   elements in some order. Array.prototype.sort without a comparator
   runs none after an element became a string: it stops at the comparison
   after the conversion that set the flag and writes nothing back. */
static void test_sort_interrupt(void)
{
    static const char *const sorts[] = {
        "s.sort()",
        "try { s.sort() } catch (e) { 'caught' }",
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    pthread_t th;
    double t;
    size_t i;

    add_interrupt(ctx);
    CHECK(eval(ctx, "var f = new Float64Array(1 << 22), sum = 0\n"
                    "for (var i = 0; i < f.length; i++) sum += f[i] = (i * 7919) % 1000003"),
          "the script threw");
    CHECK(pthread_create(&th, NULL, stop_later, rt) == 0, "pthread_create");
    t = now();
    CHECK(interrupted(ctx, "f.sort()"), "the sort of a typed array was not interrupted");
    t = now() - t;
    pthread_join(th, NULL);
    CHECK(t < 0.2, "the sort of a typed array stopped after %.3f s", t);
    CHECK(eval_is(ctx, "var t = 0; for (var i = 0; i < f.length; i++) t += f[i]; t === sum", "true"),
          "the elements of the typed array after the interrupt");
    for(i = 0; i < sizeof(sorts) / sizeof(sorts[0]); i++) {
        CHECK(eval(ctx, "var s = Array.from({ length: 1000 }, (_, i) => 'k' + i); s[500] = T('m')"),
              "the script threw");
        CHECK(interrupted(ctx, sorts[i]), "%s was not interrupted", sorts[i]);
        CHECK(eval_is(ctx, "s[0] + ' ' + s[999]", "k0 k999"), "%s wrote back", sorts[i]);
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* n_of returns the field n of the async context of ctx, or -1 when it has
   none. */
static int n_of(JSContext *ctx)
{
    JSValue c = JS_GetAsyncContext(ctx), v;
    int32_t n = -1;

    if (JS_IsObject(c)) {
        v = JS_GetPropertyStr(ctx, c, "n");
        if (JS_ToInt32(ctx, &n, v))
            n = -1;
        JS_FreeValue(ctx, v);
    }
    JS_FreeValue(ctx, c);
    return n;
}

/* set_n sets the async context of ctx to an object whose n is n, or to none
   for -1. */
static void set_n(JSContext *ctx, int n)
{
    JSValue c = JS_UNDEFINED;

    if (n >= 0) {
        c = JS_NewObject(ctx);
        JS_SetPropertyStr(ctx, c, "n", JS_NewInt32(ctx, n));
    }
    JS_SetAsyncContext(ctx, c);
    JS_FreeValue(ctx, c);
}

static JSValue js_context_n(JSContext *ctx, JSValueConst this_val, int argc,
                            JSValueConst *argv)
{
    return JS_NewInt32(ctx, n_of(ctx));
}

static int rejected_n;

static void track_rejection(JSContext *ctx, JSValueConst promise,
                            JSValueConst reason, JS_BOOL is_handled,
                            void *opaque)
{
    if (!is_handled)
        rejected_n = n_of(ctx);
}

/* The async context covers the handler of a reaction, not the whole job, as
   the Go API does: the handler runs with the context of its then, and the
   rest of the job, the rejection of the promise then returned and the
   tracker of the host that hears of it, sees the context the runtime has
   when the job runs. A handler registered with no context runs with the
   context of that time too. */
static void test_async_context_job(void)
{
    static const char thrown[] =
        "var inside = -2; Promise.resolve().then(() => { inside = contextN(); throw 7 })\n";
    static const char bare[] =
        "var later = -2; Promise.resolve().then(() => { later = contextN() })\n";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue g = JS_GetGlobalObject(ctx);

    JS_SetPropertyStr(ctx, g, "contextN",
                      JS_NewCFunction(ctx, js_context_n, "contextN", 0));
    JS_FreeValue(ctx, g);
    JS_SetHostPromiseRejectionTracker(rt, track_rejection, NULL);
    rejected_n = -2;
    set_n(ctx, 1);
    CHECK(eval(ctx, thrown), "the script threw");
    set_n(ctx, 2);
    CHECK(run_jobs(rt), "a job threw");
    CHECK(eval_is(ctx, "String(inside)", "1"), "the context of the handler");
    CHECK(rejected_n == 2, "the tracker heard of the rejection in context %d",
          rejected_n);
    set_n(ctx, -1);
    CHECK(eval(ctx, bare), "the script threw");
    set_n(ctx, 3);
    CHECK(run_jobs(rt), "a job threw");
    CHECK(eval_is(ctx, "String(later)", "3"),
          "the context of a handler registered with none");
    set_n(ctx, -1);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* depth_reached runs a recursion until it throws and returns how many levels
   it went down, or -1 when it did not end with a RangeError. The host's
   JS_Eval counts one level and the global code one more, so a plain function
   gets 2 short of the limit. */
static int depth_reached(JSContext *ctx)
{
    static const char src[] =
        "var depth = 0;\n"
        "function down() { depth++; down() }\n"
        "var got; try { down() } catch (e) { got = e instanceof RangeError ? depth : -1 }\n"
        "got";
    JSValue v = JS_Eval(ctx, src, strlen(src), "<depth>", JS_EVAL_TYPE_GLOBAL);
    int32_t d = -1;

    if (JS_IsException(v))
        JS_FreeValue(ctx, JS_GetException(ctx));
    else
        JS_ToInt32(ctx, &d, v);
    JS_FreeValue(ctx, v);
    return d;
}

static JSValue js_set_depth(JSContext *ctx, JSValueConst this_val,
                            int argc, JSValueConst *argv)
{
    int32_t n;

    if (JS_ToInt32(ctx, &n, argv[0]))
        return JS_EXCEPTION;
    return JS_NewInt32(ctx, JS_SetMaxCallDepth(JS_GetRuntime(ctx), n));
}

/* The call depth bounds recursion: a plain function recurses to the limit
   and gets a RangeError, as deep each time (every level that ends is
   counted back), and the limit can be raised to JS_MAX_CALL_DEPTH_LIMIT and
   no further, only while no code runs. */
static int test_call_depth(void *unused)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue global = JS_GetGlobalObject(ctx);
    int d;

    CHECK(JS_GetMaxCallDepth(rt) == JS_DEFAULT_MAX_CALL_DEPTH, "default %d", JS_GetMaxCallDepth(rt));
    d = depth_reached(ctx);
    CHECK(d == JS_DEFAULT_MAX_CALL_DEPTH - 2, "depth %d", d);
    d = depth_reached(ctx);
    CHECK(d == JS_DEFAULT_MAX_CALL_DEPTH - 2, "depth again %d", d);
    CHECK(eval_is(ctx, "1 + 1", "2"), "the runtime runs after a RangeError");

    CHECK(JS_SetMaxCallDepth(rt, JS_MAX_CALL_DEPTH_LIMIT) == 0, "raise to the limit");
    d = depth_reached(ctx);
    CHECK(d == JS_MAX_CALL_DEPTH_LIMIT - 2, "raised depth %d", d);
    CHECK(JS_SetMaxCallDepth(rt, 0) < 0, "0 refused");
    CHECK(JS_SetMaxCallDepth(rt, -1) < 0, "-1 refused");
    CHECK(JS_SetMaxCallDepth(rt, JS_MAX_CALL_DEPTH_LIMIT + 1) < 0, "past the limit refused");
    CHECK(JS_GetMaxCallDepth(rt) == JS_MAX_CALL_DEPTH_LIMIT, "unchanged %d", JS_GetMaxCallDepth(rt));
    CHECK(JS_SetMaxCallDepth(rt, 100) == 0, "lower");
    d = depth_reached(ctx);
    CHECK(d == 98, "lowered depth %d", d);

    JS_SetPropertyStr(ctx, global, "setDepth", JS_NewCFunction(ctx, js_set_depth, "setDepth", 1));
    JS_FreeValue(ctx, global);
    CHECK(eval_is(ctx, "String(setDepth(50))", "-1"), "refused while code runs");
    CHECK(JS_GetMaxCallDepth(rt) == 100, "unchanged while running %d", JS_GetMaxCallDepth(rt));

    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* The first context of a runtime measures the frame of JS_CallInternal for
   the direct calls of C functions. Making it runs no code, so it works with
   the call depth at the limit and with an interrupt asked, and leaves both as
   they were. */
static int test_first_context_any_state(void *unused)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx;
    int d;

    CHECK(JS_SetMaxCallDepth(rt, 100) == 0, "lower");
    CHECK(JS_SetCallDepth(rt, 100) == 0, "depth at the limit");
    ctx = JS_NewContext(rt);
    CHECK(ctx != NULL, "a context at the depth limit");
    if (!ctx) {
        host_free_runtime(rt);
        return 0;
    }
    CHECK(JS_GetCallDepth(rt) == 100, "depth kept %d", JS_GetCallDepth(rt));
    CHECK(JS_SetCallDepth(rt, 0) == 0, "depth back");
    d = depth_reached(ctx);
    CHECK(d == 98, "depth %d", d);
    JS_FreeContext(ctx);
    host_free_runtime(rt);

    rt = host_new_runtime();
    host_interrupt(rt);
    ctx = JS_NewContext(rt);
    CHECK(ctx != NULL, "a context with an interrupt asked");
    if (!ctx) {
        host_free_runtime(rt);
        return 0;
    }
    CHECK(interrupted(ctx, "1 + 1"), "the interrupt still asked");
    CHECK(eval_is(ctx, "1 + 1", "2"), "runs after it");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

static JSValue js_reenter(JSContext *ctx, JSValueConst this_val,
                          int argc, JSValueConst *argv)
{
    return JS_Call(ctx, argv[0], JS_UNDEFINED, 0, NULL);
}

static JSValue js_reeval(JSContext *ctx, JSValueConst this_val,
                         int argc, JSValueConst *argv)
{
    size_t len;
    const char *s = JS_ToCStringLen(ctx, &len, argv[0]);
    JSValue v;

    if (!s)
        return JS_EXCEPTION;
    v = JS_Eval(ctx, s, len, "<reeval>", JS_EVAL_TYPE_GLOBAL);
    JS_FreeCString(ctx, s);
    return v;
}

/* A host function that calls back into the engine adds a level for the
   function and one for the entry: reenter(f) costs f, reenter and JS_Call,
   and reeval("g()") costs g, reeval, JS_Eval and the global code of the
   string. Both end with a RangeError and leave the count as it was. */
static int test_call_depth_reentry(void *unused)
{
    static const char call_src[] =
        "var n = 0; function f() { n++; return reenter(f) }\n"
        "var got; try { f() } catch (e) { got = e instanceof RangeError ? n : -1 }\n"
        "got";
    static const char eval_src[] =
        "var m = 0; function g() { m++; return reeval('g()') }\n"
        "var got2; try { g() } catch (e) { got2 = e instanceof RangeError ? m : -1 }\n"
        "got2";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue global = JS_GetGlobalObject(ctx);
    char want[32];

    JS_SetPropertyStr(ctx, global, "reenter", JS_NewCFunction(ctx, js_reenter, "reenter", 1));
    JS_SetPropertyStr(ctx, global, "reeval", JS_NewCFunction(ctx, js_reeval, "reeval", 1));
    JS_FreeValue(ctx, global);
    /* 2 levels for the entry and the global code, then 3 per f */
    snprintf(want, sizeof(want), "%d", (JS_DEFAULT_MAX_CALL_DEPTH - 2 + 2) / 3);
    CHECK(eval_is(ctx, call_src, want), "JS_Call from a host function");
    /* 2, then 4 per g */
    snprintf(want, sizeof(want), "%d", (JS_DEFAULT_MAX_CALL_DEPTH - 2 + 3) / 4);
    CHECK(eval_is(ctx, eval_src, want), "JS_Eval from a host function");
    CHECK(depth_reached(ctx) == JS_DEFAULT_MAX_CALL_DEPTH - 2, "the count is back");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* ---- reading and writing the current depths ----

   A host that calls from one runtime into another hands the depth over, so
   that the runtimes together are held to one limit: it reads the depth of
   the runtime that calls, sets the other's to it, makes the call, and sets
   the old depth back. */

static JSValue js_call_depth(JSContext *ctx, JSValueConst this_val,
                             int argc, JSValueConst *argv)
{
    return JS_NewInt32(ctx, JS_GetCallDepth(JS_GetRuntime(ctx)));
}

static JSValue js_internal_depth(JSContext *ctx, JSValueConst this_val,
                                 int argc, JSValueConst *argv)
{
    return JS_NewInt32(ctx, JS_GetInternalDepth(JS_GetRuntime(ctx)));
}

/* withDepth(n, f): f() with the call depth set to n, and set back after */
static JSValue js_with_depth(JSContext *ctx, JSValueConst this_val,
                             int argc, JSValueConst *argv)
{
    JSRuntime *rt = JS_GetRuntime(ctx);
    int32_t n;
    int saved = JS_GetCallDepth(rt);
    JSValue ret;

    if (JS_ToInt32(ctx, &n, argv[0]))
        return JS_EXCEPTION;
    if (JS_SetCallDepth(rt, n) < 0)
        return JS_ThrowRangeError(ctx, "depth refused");
    ret = JS_Call(ctx, argv[1], JS_UNDEFINED, 0, NULL);
    CHECK(JS_SetCallDepth(rt, saved) == 0, "set back");
    return ret;
}

/* ends with a RangeError. It reads the name of the error and does not turn the
   error into a string, which is a call, and fails again at the limit. */
static int eval_range_error(JSContext *ctx, const char *src)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    JSValue e, name;
    const char *s;
    int ok;

    if (!JS_IsException(v)) {
        printf("%s: no error\n", src);
        JS_FreeValue(ctx, v);
        return 0;
    }
    e = JS_GetException(ctx);
    name = JS_GetPropertyStr(ctx, e, "name");
    s = JS_ToCString(ctx, name);
    ok = s && strcmp(s, "RangeError") == 0;
    if (!ok)
        printf("%s: %s\n", src, s ? s : "?");
    JS_FreeCString(ctx, s);
    JS_FreeValue(ctx, name);
    JS_FreeValue(ctx, e);
    return ok;
}

/* The completion value of src as an int, or -1 */
static int eval_int(JSContext *ctx, const char *src)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int32_t n = -1;

    if (JS_IsException(v))
        JS_FreeValue(ctx, JS_GetException(ctx));
    else
        JS_ToInt32(ctx, &n, v);
    JS_FreeValue(ctx, v);
    return n;
}

/* The depth a host function sees is the levels that are open: the host's
   JS_Eval, the global code, each function, and the host function itself. The
   limit holds after a write: the next call that does not fit is a RangeError,
   a runtime set to its limit runs nothing, and one set past it (a depth from a
   runtime with a bigger limit) ends the first call with a RangeError. A value
   out of 0 to the limit is refused and changes nothing, and it can be set while
   code runs, which the limit itself cannot. */
static int test_call_depth_access(void *unused)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue global = JS_GetGlobalObject(ctx);
    int max = JS_GetMaxCallDepth(rt);
    char src[128];

    JS_SetPropertyStr(ctx, global, "callDepth", JS_NewCFunction(ctx, js_call_depth, "callDepth", 0));
    JS_SetPropertyStr(ctx, global, "withDepth", JS_NewCFunction(ctx, js_with_depth, "withDepth", 2));
    JS_FreeValue(ctx, global);

    CHECK(JS_GetCallDepth(rt) == 0, "an idle runtime is at %d", JS_GetCallDepth(rt));
    CHECK(eval_is(ctx, "String(callDepth())", "3"), "entry, global code, the host function");
    CHECK(eval_is(ctx, "function f(n) { return n ? f(n - 1) : callDepth() } String(f(10))", "14"), "11 functions");
    CHECK(JS_GetCallDepth(rt) == 0, "back to %d", JS_GetCallDepth(rt));

    /* a write moves the point the limit counts from */
    CHECK(JS_SetCallDepth(rt, 100) == 0, "set 100");
    CHECK(JS_GetCallDepth(rt) == 100, "got %d", JS_GetCallDepth(rt));
    CHECK(eval_is(ctx, "String(callDepth())", "103"), "counts on from 100");
    CHECK(depth_reached(ctx) == max - 100 - 2, "recursion from 100 reaches %d", depth_reached(ctx));
    CHECK(JS_GetCallDepth(rt) == 100, "100 stays %d", JS_GetCallDepth(rt));
    CHECK(JS_SetMaxCallDepth(rt, 200) < 0, "the limit cannot change at depth 100");
    CHECK(JS_SetCallDepth(rt, 0) == 0, "set 0");
    CHECK(depth_reached(ctx) == max - 2, "recursion from 0 reaches %d", depth_reached(ctx));

    /* the edge: the entry, the global code and the call each need a level */
    CHECK(JS_SetCallDepth(rt, max) == 0, "set the limit");
    CHECK(eval_range_error(ctx, "1"), "nothing runs at the limit");
    CHECK(JS_SetCallDepth(rt, max - 1) == 0, "set one below");
    CHECK(eval_range_error(ctx, "1"), "the global code does not fit one below");
    CHECK(JS_SetCallDepth(rt, max - 2) == 0, "set two below");
    CHECK(eval_is(ctx, "1 + 1", "2"), "the global code fits two below");
    CHECK(eval_range_error(ctx, "(function () {})()"), "a call does not fit two below");
    CHECK(JS_SetCallDepth(rt, max - 3) == 0, "set three below");
    CHECK(eval_is(ctx, "(function () {})(), 7", "7"), "one call fits three below");
    CHECK(eval_range_error(ctx, "(function () { (function () {})() })()"), "two do not");
    CHECK(JS_SetCallDepth(rt, 0) == 0, "set 0");
    CHECK(eval_is(ctx, "1 + 1", "2"), "the runtime runs after the RangeError");

    /* a depth past the limit of this runtime is kept, and nothing runs */
    CHECK(JS_SetCallDepth(rt, max + 1000) == 0, "set past the limit");
    CHECK(JS_GetCallDepth(rt) == max + 1000, "kept %d", JS_GetCallDepth(rt));
    CHECK(eval_range_error(ctx, "1"), "nothing runs past the limit");
    CHECK(JS_GetCallDepth(rt) == max + 1000, "nothing counted %d", JS_GetCallDepth(rt));
    CHECK(JS_SetCallDepth(rt, JS_MAX_CALL_DEPTH_LIMIT) == 0, "set the biggest");
    CHECK(JS_SetCallDepth(rt, 0) == 0, "set 0");

    /* out of range: refused, and the depth is as it was */
    CHECK(JS_SetCallDepth(rt, 40) == 0, "set 40");
    CHECK(JS_SetCallDepth(rt, -1) < 0, "-1 refused");
    CHECK(JS_SetCallDepth(rt, INT_MIN) < 0, "INT_MIN refused");
    CHECK(JS_SetCallDepth(rt, JS_MAX_CALL_DEPTH_LIMIT + 1) < 0, "past the biggest refused");
    CHECK(JS_SetCallDepth(rt, INT_MAX) < 0, "INT_MAX refused");
    CHECK(JS_GetCallDepth(rt) == 40, "unchanged %d", JS_GetCallDepth(rt));
    CHECK(JS_SetCallDepth(rt, 0) == 0, "set 0");

    /* while code runs: set for a call and set back */
    CHECK(eval_is(ctx, "String(withDepth(500, () => callDepth()))", "503"), "JS_Call, the function, the host function");
    CHECK(eval_is(ctx, "String(callDepth())", "3"), "back after it");
    snprintf(src, sizeof(src), "let r; try { withDepth(%d, () => 1) } catch (e) { r = e.name } r", max);
    CHECK(eval_is(ctx, src, "RangeError"), "nothing fits at the limit");
    CHECK(eval_is(ctx, "let r2; try { withDepth(-1, () => 1) } catch (e) { r2 = e.name + e.message } r2", "RangeErrordepth refused"),
          "-1 is refused while code runs");
    CHECK(eval_is(ctx, "String(callDepth())", "3"), "back after the RangeError");
    CHECK(JS_GetCallDepth(rt) == 0, "idle again %d", JS_GetCallDepth(rt));
    CHECK(JS_SetMaxCallDepth(rt, 200) == 0, "the limit can change again");

    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* JSON.parse(text) called from C: its result as the name of its type, or the
   name of the error it threw */
static int json_parse_is(JSContext *ctx, JSValueConst parse, const char *text, const char *want)
{
    JSValue arg = JS_NewString(ctx, text), v;
    const char *got = "?", *name = NULL;
    JSValue e = JS_UNDEFINED;
    int ok;

    v = JS_Call(ctx, parse, JS_UNDEFINED, 1, &arg);
    JS_FreeValue(ctx, arg);
    if (JS_IsException(v)) {
        JSRuntime *rt = JS_GetRuntime(ctx);
        int depth = JS_GetInternalDepth(rt);

        e = JS_GetException(ctx);
        /* the text of the error may need a built-in function made, which
           takes a level: read it below the depth of the test */
        JS_SetInternalDepth(rt, 0);
        name = JS_ToCString(ctx, e);
        JS_SetInternalDepth(rt, depth);
        got = name ? name : "?";
    } else {
        got = JS_IsObject(v) ? "object" : JS_IsNumber(v) ? "number" : "other";
    }
    ok = strncmp(got, want, strlen(want)) == 0;
    if (!ok)
        printf("JSON.parse(%s): %s, want %s\n", text, got, want);
    JS_FreeCString(ctx, name);
    JS_FreeValue(ctx, e);
    JS_FreeValue(ctx, v);
    return ok;
}

/* The same for the internal depth: the recursions of C that make no call, held
   to JS_MAX_INTERNAL_DEPTH. A proxy trap runs one level in, and the parser
   counts as well, so a runtime at the limit cannot even read a script. */
static int test_internal_depth_access(void *unused)
{
    static const char json_src[] =
        "(() => {\n"
        "  const nests = n => { try { JSON.parse('['.repeat(n) + ']'.repeat(n)); return true } catch (e) { return false } };\n"
        "  let n = 1; while (nests(n + 1)) n++; return n })()";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue global = JS_GetGlobalObject(ctx), parse;
    char want[16];

    JS_SetPropertyStr(ctx, global, "internalDepth", JS_NewCFunction(ctx, js_internal_depth, "internalDepth", 0));
    JS_FreeValue(ctx, global);

    CHECK(JS_GetInternalDepth(rt) == 0, "an idle runtime is at %d", JS_GetInternalDepth(rt));
    CHECK(eval_is(ctx, "String(internalDepth())", "0"), "a call is not a level of this count");
    CHECK(eval_is(ctx, "String(new Proxy({}, { get: internalDepth }).x)", "1"), "a proxy trap runs inside one level");
    CHECK(eval_is(ctx, "String(new Proxy(new Proxy({}, { get: internalDepth }), {}).x)", "2"), "two proxies, two levels");
    CHECK(JS_GetInternalDepth(rt) == 0, "back to %d", JS_GetInternalDepth(rt));

    snprintf(want, sizeof(want), "%d", JS_MAX_INTERNAL_DEPTH);
    CHECK(eval_is(ctx, json_src, want), "JSON fits the limit from 0");
    CHECK(JS_SetInternalDepth(rt, 30) == 0, "set 30");
    CHECK(JS_GetInternalDepth(rt) == 30, "got %d", JS_GetInternalDepth(rt));
    CHECK(eval_is(ctx, "String(new Proxy({}, { get: internalDepth }).x)", "31"), "counts on from 30");
    snprintf(want, sizeof(want), "%d", JS_MAX_INTERNAL_DEPTH - 30);
    CHECK(eval_is(ctx, json_src, want), "JSON fits what is left of the limit");
    CHECK(JS_GetInternalDepth(rt) == 30, "30 stays %d", JS_GetInternalDepth(rt));
    CHECK(JS_SetMaxCallDepth(rt, 200) < 0, "the call limit cannot change at internal depth 30");

    /* the edge: the parser needs several levels, and JSON.parse one for the
       value and one more inside each array. JSON.parse is called from C, so
       that no script is read at depth */
    parse = JS_Eval(ctx, "JSON.parse", 10, "<unit>", JS_EVAL_TYPE_GLOBAL);
    CHECK(JS_SetInternalDepth(rt, JS_MAX_INTERNAL_DEPTH) == 0, "set the limit");
    CHECK(eval_range_error(ctx, "1"), "the parser has no level left");
    CHECK(json_parse_is(ctx, parse, "1", "RangeError"), "JSON.parse has none either");
    CHECK(JS_SetInternalDepth(rt, JS_MAX_INTERNAL_DEPTH - 1) == 0, "set one below");
    CHECK(json_parse_is(ctx, parse, "1", "number"), "JSON.parse fits a value");
    CHECK(json_parse_is(ctx, parse, "[1]", "RangeError"), "and not an array of one");
    CHECK(JS_SetInternalDepth(rt, JS_MAX_INTERNAL_DEPTH - 2) == 0, "set two below");
    CHECK(json_parse_is(ctx, parse, "[1]", "object"), "and fits it two below");
    CHECK(JS_GetInternalDepth(rt) == JS_MAX_INTERNAL_DEPTH - 2, "kept %d", JS_GetInternalDepth(rt));
    JS_FreeValue(ctx, parse);
    CHECK(JS_SetInternalDepth(rt, 0) == 0, "set 0");

    CHECK(JS_SetInternalDepth(rt, 30) == 0, "set 30");
    CHECK(JS_SetInternalDepth(rt, -1) < 0, "-1 refused");
    CHECK(JS_SetInternalDepth(rt, INT_MIN) < 0, "INT_MIN refused");
    CHECK(JS_SetInternalDepth(rt, JS_MAX_INTERNAL_DEPTH + 1) < 0, "past the limit refused");
    CHECK(JS_SetInternalDepth(rt, INT_MAX) < 0, "INT_MAX refused");
    CHECK(JS_GetInternalDepth(rt) == 30, "unchanged %d", JS_GetInternalDepth(rt));
    CHECK(JS_SetInternalDepth(rt, 0) == 0, "set 0");
    CHECK(JS_SetMaxCallDepth(rt, 200) == 0, "the call limit can change again");

    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* Two runtimes that call each other, one thread. hop() runs step() of the
   other runtime, and step() calls hop() again, until a RangeError. The
   host's hop hands the depth over as above: with share_call set, the call
   depth of the runtime that calls; with share_internal set, the internal depth
   too. The chain is then held to one limit, wherever each level is counted. */
static JSContext *hop_ctx[2];
static int hop_share_call, hop_share_internal;

static JSValue js_hop(JSContext *ctx, JSValueConst this_val,
                      int argc, JSValueConst *argv)
{
    JSContext *octx = hop_ctx[ctx == hop_ctx[0]];
    JSRuntime *rt = JS_GetRuntime(ctx), *ort = JS_GetRuntime(octx);
    int saved_call = JS_GetCallDepth(ort), saved_internal = JS_GetInternalDepth(ort);
    JSValue global, fn, ret;

    if (hop_share_call)
        CHECK(JS_SetCallDepth(ort, JS_GetCallDepth(rt)) == 0, "hand the call depth over");
    if (hop_share_internal)
        CHECK(JS_SetInternalDepth(ort, JS_GetInternalDepth(rt)) == 0, "hand the internal depth over");
    global = JS_GetGlobalObject(octx);
    fn = JS_GetPropertyStr(octx, global, "step");
    JS_FreeValue(octx, global);
    ret = JS_Call(octx, fn, JS_UNDEFINED, 0, NULL);
    JS_FreeValue(octx, fn);
    if (hop_share_call)
        CHECK(JS_SetCallDepth(ort, saved_call) == 0, "hand the call depth back");
    if (hop_share_internal)
        CHECK(JS_SetInternalDepth(ort, saved_internal) == 0, "hand the internal depth back");
    if (JS_IsException(ret)) {
        /* an error does not cross to the other runtime: say it in this one */
        JS_FreeValue(octx, JS_GetException(octx));
        return JS_ThrowRangeError(ctx, "the other runtime ran out of depth");
    }
    JS_FreeValue(octx, ret);
    return JS_UNDEFINED;
}

/* runs the chain from the first runtime and returns how many steps both ran,
   or -1 when it did not end in a RangeError */
static int hop_chain(const char *setup, int share_call, int share_internal)
{
    static const char run[] = "var e; try { step() } catch (x) { e = x instanceof RangeError } String(e)";
    JSRuntime *rt[2];
    int i, total = 0;

    hop_share_call = share_call;
    hop_share_internal = share_internal;
    for (i = 0; i < 2; i++) {
        JSValue global;

        rt[i] = host_new_runtime();
        hop_ctx[i] = JS_NewContext(rt[i]);
        JS_SetMaxCallDepth(rt[i], JS_MAX_CALL_DEPTH_LIMIT);
        global = JS_GetGlobalObject(hop_ctx[i]);
        JS_SetPropertyStr(hop_ctx[i], global, "hop", JS_NewCFunction(hop_ctx[i], js_hop, "hop", 0));
        JS_FreeValue(hop_ctx[i], global);
        CHECK(eval(hop_ctx[i], setup), "setup");
    }
    CHECK(eval_is(hop_ctx[0], run, "true"), "the chain ends with a RangeError");
    for (i = 0; i < 2; i++) {
        int n = eval_int(hop_ctx[i], "steps");

        total += n;
        CHECK(JS_GetCallDepth(rt[i]) == 0 && JS_GetInternalDepth(rt[i]) == 0,
              "runtime %d is back to depths %d and %d", i, JS_GetCallDepth(rt[i]), JS_GetInternalDepth(rt[i]));
        CHECK(depth_reached(hop_ctx[i]) == JS_MAX_CALL_DEPTH_LIMIT - 2, "runtime %d recurses as deep as before", i);
        JS_FreeContext(hop_ctx[i]);
        host_free_runtime(rt[i]);
    }
    return total;
}

static int test_depth_handover(void *unused)
{
    /* every step is 3 levels of the chain: the function, hop, and the entry of
       the call into the other runtime. The first is at 3 after the host's
       JS_Eval and the global code, and step k + 1 fits when 3k + 2 is under the
       limit, so the chain is limit / 3 steps long */
    static const char calls[] = "var steps = 0; function step() { steps++; hop() }";
    /* every step is a proxy get, which is a level of the internal depth: the
       chain is the internal limit long. The call depth is set to its biggest
       so that it is not what ends the chain */
    static const char proxies[] =
        "var steps = 0; var p = new Proxy({}, { get() { steps++; return hop() } });\n"
        "function step() { return p.x }";
    int limit = JS_MAX_CALL_DEPTH_LIMIT, n;

    /* with no hand over, each runtime counts the part of the chain it runs
       alone, so the chain is twice as long (the first runtime ends it) */
    n = hop_chain(calls, 0, 0);
    CHECK(n == 2 * (limit / 3), "separate call depths: %d steps", n);
    n = hop_chain(calls, 1, 0);
    CHECK(n == limit / 3, "shared call depth: %d steps", n);
    n = hop_chain(calls, 1, 1);
    CHECK(n == limit / 3, "both shared: %d steps", n);
    n = hop_chain(proxies, 0, 0);
    CHECK(n == 2 * JS_MAX_INTERNAL_DEPTH, "separate internal depths: %d steps", n);
    n = hop_chain(proxies, 0, 1);
    CHECK(n == JS_MAX_INTERNAL_DEPTH, "shared internal depth: %d steps", n);
    return 0;
}

/* Recursions that go from C to C without JS_CallInternal crashed the native
   host with a segfault: a chain of 300000 functions that Promise.prototype.finally
   makes, and chains of iterator helpers, whose next calls the next of the
   iterator below through the fast path of JS_IteratorNext2. Each now ends
   with a RangeError, the runtime still runs code and recurses as deep as
   before, and it frees with no block left. */
static int test_call_depth_chains(void *unused)
{
    static const char *const srcs[] = {
        "let f = () => 0;\n"
        "for (let i = 0; i < 300000; i++) { let cap; Promise.prototype.finally.call({ then(a) { cap = a } }, f); f = cap }\n"
        "let r; try { f(); r = 'no error' } catch (e) { r = e.name } r",
#define ITER(n, step) \
        "let it = [1, 2, 3].values();\n" \
        "for (let i = 0; i < " #n "; i++) it = it." step ";\n" \
        "let r; try { r = JSON.stringify(it.next()) } catch (e) { r = e.name } r"
        ITER(30000, "take(5)"),
        ITER(300000, "take(5)"),
        ITER(30000, "map(x => x)"),
        ITER(300000, "map(x => x)"),
        ITER(30000, "filter(x => true)"),
        ITER(300000, "filter(x => true)"),
#undef ITER
    };
    size_t i;

    for (i = 0; i < sizeof(srcs) / sizeof(srcs[0]); i++) {
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt);

        CHECK(eval_is(ctx, srcs[i], "RangeError"), "chain %zu", i);
        CHECK(eval_is(ctx, "1 + 1", "2"), "chain %zu: the runtime runs", i);
        CHECK(depth_reached(ctx) == JS_DEFAULT_MAX_CALL_DEPTH - 2, "chain %zu: the count is back", i);
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }
    return 0;
}

/* The recursions of C that make no call count the internal depth, at most
   JS_MAX_INTERNAL_DEPTH levels. instanceof on a chain of 300000 bound
   functions (with no Symbol.hasInstance on the way, nothing but C) and
   Reflect.construct with one as the new target (its realm) recursed once per
   function: clang makes loops of these tail calls, but the generated assembly
   makes no tail call. Each function of a chain has its name deleted, or the
   names, "bound bound ...", take memory by the square of the length. They
   end with a RangeError now, as nesting past the limit does in JSON, the parser,
   regexps, flat and a chain of proxies without traps, and the runtime still
   runs code, nests as deep as before and frees with no block left. */
static int test_internal_depth(void *unused)
{
    static const struct { const char *src, *want; } cases[] = {
        { "const bind = Function.prototype.bind; let f = function () {};\n"
          "for (let i = 0; i < 300000; i++) { f = bind.call(f); delete f.name; Object.setPrototypeOf(f, null) }\n"
          "let r; try { r = String(({}) instanceof f) } catch (e) { r = e.name } r", "RangeError" },
        { "let f = function () {};\n"
          "for (let i = 0; i < 300000; i++) { f = f.bind(); delete f.name }\n"
          "let r; try { r = String(Reflect.construct(Array, [], f).length) } catch (e) { r = e.name } r", "RangeError" },
        { "let r; try { JSON.parse('['.repeat(100000) + ']'.repeat(100000)); r = 'parsed' } catch (e) { r = e.name } r", "RangeError" },
        { "let a = []; for (let i = 0; i < 100000; i++) a = [a];\n"
          "let r; try { r = JSON.stringify(a).length } catch (e) { r = e.name } r", "RangeError" },
        { "let r; try { eval('('.repeat(100000) + '1' + ')'.repeat(100000)) } catch (e) { r = e.name } r", "RangeError" },
        { "let r; try { eval('['.repeat(100000) + ']'.repeat(100000)) } catch (e) { r = e.name } r", "RangeError" },
        { "let r; try { new RegExp('(?:'.repeat(100000) + ')'.repeat(100000)) } catch (e) { r = e.name + ': ' + e.message } r", "SyntaxError: stack overflow" },
        { "let r; try { new RegExp('['.repeat(100000) + ']'.repeat(100000), 'v') } catch (e) { r = e.name + ': ' + e.message } r", "SyntaxError: stack overflow" },
        { "let a = [1]; for (let i = 0; i < 100000; i++) a = [a];\n"
          "let r; try { r = a.flat(Infinity).length } catch (e) { r = e.name } r", "RangeError" },
        { "let p = {}; for (let i = 0; i < 300000; i++) p = new Proxy(p, {});\n"
          "let r = []; for (const f of [() => p.x, () => 'x' in p, () => { p.x = 1 }, () => Object.getPrototypeOf(p),\n"
          "  () => Object.keys(p), () => delete p.x, () => Object.isExtensible(p)])\n"
          "  try { f(); r.push('ok') } catch (e) { r.push(e.name) }\n"
          "r.join()", "RangeError,RangeError,RangeError,RangeError,RangeError,RangeError,RangeError" },
    };
    /* JSON.parse nests one level per array: JS_MAX_INTERNAL_DEPTH of them fit */
    static const char nest_src[] =
        "(() => {\n"
        "  const nests = n => { try { JSON.parse('['.repeat(n) + ']'.repeat(n)); return true } catch (e) { return false } };\n"
        "  let n = 1; while (nests(n + 1)) n++; return n })()";
    /* each proxy without a getPrototypeOf trap is a level: JS_MAX_INTERNAL_DEPTH of them fit */
    static const char proto_src[] =
        "(() => {\n"
        "  const chain = n => { let p = {}; for (let i = 0; i < n; i++) p = new Proxy(p, {}); return p };\n"
        "  const gets = n => { try { return Object.getPrototypeOf(chain(n)) === Object.prototype } catch (e) { return e.name } };\n"
        "  return [gets(%d), gets(%d), gets(%d)].join() })()";
    char want[32], src[512];
    size_t i;

    snprintf(want, sizeof(want), "%d", JS_MAX_INTERNAL_DEPTH);
    {
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt);

        snprintf(src, sizeof(src), proto_src, JS_MAX_INTERNAL_DEPTH - 1, JS_MAX_INTERNAL_DEPTH, JS_MAX_INTERNAL_DEPTH + 1);
        CHECK(eval_is(ctx, src, "true,true,RangeError"), "prototypes of proxies");
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }
    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt);

        CHECK(eval_is(ctx, nest_src, want), "case %zu: JSON nests to the limit before", i);
        CHECK(eval_is(ctx, cases[i].src, cases[i].want), "case %zu", i);
        CHECK(eval_is(ctx, "1 + 1", "2"), "case %zu: the runtime runs", i);
        CHECK(eval_is(ctx, nest_src, want), "case %zu: JSON nests to the limit after", i);
        CHECK(depth_reached(ctx) == JS_DEFAULT_MAX_CALL_DEPTH - 2, "case %zu: the call count is back", i);
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }
    return 0;
}

/* The limit of the internal depth is 2000. It is a constant of the build (not a
   setting like the call depth), and these write it down in numbers: JSON, the
   parser and a chain of proxies take 2000, 665 and 2000 levels, and the next one
   is a RangeError. The other tests take their numbers from the constant. */
static int test_internal_depth_limit(void *unused)
{
    static const struct { const char *src, *want; } cases[] = {
        { "try { JSON.parse('['.repeat(2000) + ']'.repeat(2000)); 'parsed' } catch (e) { e.name }", "parsed" },
        { "try { JSON.parse('['.repeat(2001) + ']'.repeat(2001)); 'parsed' } catch (e) { e.name }", "RangeError" },
        { "try { eval('('.repeat(665) + '1' + ')'.repeat(665)); 'parsed' } catch (e) { e.name }", "parsed" },
        { "try { eval('('.repeat(666) + '1' + ')'.repeat(666)); 'parsed' } catch (e) { e.name }", "RangeError" },
        { "var chain = n => { let p = {}; for (let i = 0; i < n; i++) p = new Proxy(p, {}); return p };\n"
          "try { Object.getPrototypeOf(chain(2000)) === Object.prototype ? 'reached' : 'wrong' } catch (e) { e.name }", "reached" },
        { "try { Object.getPrototypeOf(chain(2001)); 'reached' } catch (e) { e.name }", "RangeError" },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(JS_MAX_INTERNAL_DEPTH == 2000, "the limit of the internal depth is %d", JS_MAX_INTERNAL_DEPTH);
    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        CHECK(eval_is(ctx, cases[i].src, cases[i].want), "case %zu", i);
        CHECK(JS_GetInternalDepth(rt) == 0, "case %zu: the internal depth is back at %d", i, JS_GetInternalDepth(rt));
    }
    CHECK(eval_is(ctx, "1 + 1", "2"), "the runtime runs");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* JSON nested past the limit of the internal depth says so: the error is a
   RangeError that names the JSON, in the parse, in the reviver that walks the
   result and in stringify, and not the stack overflow of a call that recurses
   too deep, which stays as it is. */
static int test_json_nesting_error(void *unused)
{
    static const struct { const char *src, *want; } cases[] = {
        { "try { JSON.parse('['.repeat(2001) + ']'.repeat(2001)) } catch (e) { e.name + ': ' + e.message }",
          "RangeError: JSON nesting too deep" },
        { "try { JSON.parse('{\"a\":'.repeat(2001) + '1' + '}'.repeat(2001)) } catch (e) { e.name + ': ' + e.message }",
          "RangeError: JSON nesting too deep" },
        { "try { JSON.parse('['.repeat(2001) + ']'.repeat(2001), (k, v) => v) } catch (e) { e.name + ': ' + e.message }",
          "RangeError: JSON nesting too deep" },
        { "var a = []; for (let i = 1; i < 2001; i++) a = [a];\n"
          "try { JSON.stringify(a) } catch (e) { e.name + ': ' + e.message }",
          "RangeError: JSON nesting too deep" },
        { "var a = []; for (let i = 1; i < 2001; i++) a = [a];\n"
          "try { JSON.stringify(a, (k, v) => v) } catch (e) { e.name + ': ' + e.message }",
          "RangeError: JSON nesting too deep" },
        { "function f() { return f() }\n"
          "try { f() } catch (e) { e.name + ': ' + e.message }",
          "RangeError: Maximum call stack size exceeded" },
        { "var o = { toJSON() { return JSON.stringify(this) } };\n"
          "try { JSON.stringify(o) } catch (e) { e.name + ': ' + e.message }",
          "RangeError: Maximum call stack size exceeded" },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        CHECK(eval_is(ctx, cases[i].src, cases[i].want), "case %zu", i);
        CHECK(JS_GetInternalDepth(rt) == 0, "case %zu: the internal depth is back at %d", i, JS_GetInternalDepth(rt));
    }
    CHECK(eval_is(ctx, "JSON.stringify(JSON.parse('[[1]]'))", "[[1]]"), "the runtime runs");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* The name of the exception, taken out of ctx, or "" when there is none */
static void take_error_name(JSContext *ctx, char *buf, size_t size)
{
    JSValue e, name;
    const char *s;

    if (!JS_HasException(ctx)) {
        snprintf(buf, size, "%s", "");
        return;
    }
    e = JS_GetException(ctx);
    name = JS_GetPropertyStr(ctx, e, "name");
    s = JS_ToCString(ctx, name);
    snprintf(buf, size, "%s", s ? s : "");
    JS_FreeCString(ctx, s);
    JS_FreeValue(ctx, name);
    JS_FreeValue(ctx, e);
}

/* A built-in function or object, or the prototype of a function, is made at
   the first access to its property, and making it counts a level of the
   internal depth. At the limit the access throws a RangeError and the
   property stays to be made, which the next access below the limit does. */
static int test_internal_depth_autoinit(void *unused)
{
    static const struct { const char *holder, *prop, *type; } cases[] = {
        { "globalThis", "Reflect", "object" },
        { "JSON", "parse", "function" },
        { "(function f() {})", "prototype", "object" },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    char name[32];
    size_t i;

    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        JSValue holder = JS_Eval(ctx, cases[i].holder, strlen(cases[i].holder), "<unit>", JS_EVAL_TYPE_GLOBAL);
        JSValue v;
        const char *type;

        CHECK(JS_SetInternalDepth(rt, JS_MAX_INTERNAL_DEPTH) == 0, "set the limit");
        v = JS_GetPropertyStr(ctx, holder, cases[i].prop);
        CHECK(JS_IsException(v), "%s.%s was made at the limit", cases[i].holder, cases[i].prop);
        JS_FreeValue(ctx, v);
        take_error_name(ctx, name, sizeof(name));
        CHECK(strcmp(name, "RangeError") == 0, "%s.%s at the limit threw %s", cases[i].holder, cases[i].prop, name);
        CHECK(JS_GetInternalDepth(rt) == JS_MAX_INTERNAL_DEPTH, "the depth is %d", JS_GetInternalDepth(rt));
        CHECK(JS_SetInternalDepth(rt, JS_MAX_INTERNAL_DEPTH - 1) == 0, "set one below");
        v = JS_GetPropertyStr(ctx, holder, cases[i].prop);
        CHECK(JS_GetInternalDepth(rt) == JS_MAX_INTERNAL_DEPTH - 1, "the depth is %d", JS_GetInternalDepth(rt));
        CHECK(JS_SetInternalDepth(rt, 0) == 0, "set 0");
        type = JS_IsFunction(ctx, v) ? "function" : JS_IsObject(v) ? "object" : "other";
        CHECK(strcmp(type, cases[i].type) == 0, "%s.%s one below the limit is %s", cases[i].holder, cases[i].prop, type);
        JS_FreeValue(ctx, v);
        JS_FreeValue(ctx, holder);
    }
    CHECK(eval_is(ctx, "typeof Reflect.ownKeys + typeof JSON.parse('1')", "functionnumber"), "the runtime runs");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* A String object and an arguments object define a property through the
   ordinary definition, which counts a level of the internal depth: at the
   limit a definition, and a write that adds a property, throw a RangeError
   and change nothing, and one below the limit they work. */
static int test_internal_depth_define(void *unused)
{
    static const char *const holders[] = {
        "new String('ab')",
        "(function () { 'use strict'; return arguments })(1, 2)",
        "(function (a, b) { return arguments })(1, 2)",
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue global = JS_GetGlobalObject(ctx);
    char name[32];
    size_t i;
    int d;

    for (i = 0; i < sizeof(holders) / sizeof(holders[0]); i++) {
        JSValue o = JS_Eval(ctx, holders[i], strlen(holders[i]), "<unit>", JS_EVAL_TYPE_GLOBAL);

        for (d = JS_MAX_INTERNAL_DEPTH; d >= JS_MAX_INTERNAL_DEPTH - 1; d--) {
            int at = d == JS_MAX_INTERNAL_DEPTH, r1, r2;

            CHECK(JS_SetInternalDepth(rt, d) == 0, "set %d", d);
            r1 = JS_DefinePropertyValueStr(ctx, o, at ? "x" : "y", JS_NewInt32(ctx, 1), JS_PROP_C_W_E | JS_PROP_THROW);
            if (at) {
                take_error_name(ctx, name, sizeof(name));
                CHECK(r1 < 0 && strcmp(name, "RangeError") == 0, "%s: a definition at the limit gave %d, %s", holders[i], r1, name);
            }
            r2 = JS_SetPropertyUint32(ctx, o, at ? 5 : 6, JS_NewInt32(ctx, 2));
            if (at) {
                take_error_name(ctx, name, sizeof(name));
                CHECK(r2 < 0 && strcmp(name, "RangeError") == 0, "%s: a new element at the limit gave %d, %s", holders[i], r2, name);
            } else {
                CHECK(r1 > 0 && r2 > 0, "%s: one below the limit gave %d and %d", holders[i], r1, r2);
            }
            CHECK(JS_GetInternalDepth(rt) == d, "the depth is %d", JS_GetInternalDepth(rt));
        }
        CHECK(JS_SetInternalDepth(rt, 0) == 0, "set 0");
        JS_SetPropertyStr(ctx, global, "o", o);
        CHECK(eval_is(ctx, "Object.keys(o).join()", "0,1,6,y"), "%s: the keys", holders[i]);
    }
    JS_FreeValue(ctx, global);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* Functions nested as deep as the parser goes are made, each with the
   variables it takes from the functions around it, and direct eval in the
   innermost one sees the variables of the outermost: making them counts a
   level per function, and the parser ends with a RangeError first. */
static int test_function_nesting(void *unused)
{
    static const char src[] =
        "(() => {\n"
        "  const nests = n => { try { return eval('let v = 7; ' + '(() => '.repeat(n) + 'eval(\"v\")' + ')()'.repeat(n)) } catch (e) { return e.name } };\n"
        "  let n = 1; while (nests(n + 1) === 7) n++;\n"
        "  return n >= %d ? 'deep ' + nests(n + 1) : 'only ' + n })()";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    char buf[512];

    snprintf(buf, sizeof(buf), src, JS_MAX_INTERNAL_DEPTH / 20);
    CHECK(eval_is(ctx, buf, "deep RangeError"), "nested functions");
    CHECK(eval_is(ctx, "1 + 1", "2"), "the runtime runs");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* The parser counts 3 levels for a parenthesis (an assignment expression,
   a unary one and a postfix one), whatever the operators between them: the
   binary and logical operators are read in loops. The statement and the
   expression around the first parenthesis count 4. */
static int test_paren_depth(void *unused)
{
    static const char src[] =
        "(() => {\n"
        "  const nests = (open, close) => { let n = 1;\n"
        "    for (;;) { try { eval(open.repeat(n + 1) + '1' + close.repeat(n + 1)) } catch (e) { return n } n++ } };\n"
        "  return [nests('(', ')'), nests('(1+', ')'), nests('(1*2|', ')'), nests('(1&&', ')'),\n"
        "          nests('(1||1&&', ')'), nests('(1\?\?', ')'), nests('(1==1<', ')')].join() })()";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    char want[128];
    int n = (JS_MAX_INTERNAL_DEPTH - 4) / 3;

    snprintf(want, sizeof(want), "%d,%d,%d,%d,%d,%d,%d", n, n, n, n, n, n, n);
    CHECK(eval_is(ctx, src, want), "parentheses");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* Ropes are read, hashed and freed by loops: a character at any place, a
   Map key that is the same string as a rope of another shape (the hash goes
   over the strings in order, whatever the tree), ropes that share their
   parts, and long chains of concatenations, at either end and of strings of
   many lengths, that the engine rebalances. Everything is freed with no
   block left. */
static void test_ropes(void)
{
    static const char src[] =
        "(() => {\n"
        "  const piece = n => String.fromCharCode(97 + n % 26).repeat(600) + n;\n"
        "  let s = '', want = [];\n"
        "  for (let i = 0; i < 20000; i++) { s = s + piece(i); want.push(piece(i)) }\n"
        "  let t = '';\n"
        "  for (let i = 19999; i >= 0; i--) t = piece(i) + t;\n"
        "  const flat = want.join('');\n"
        "  if (s.length !== flat.length || t.length !== flat.length) return 'length';\n"
        "  for (let k = 0; k < flat.length; k += 4999) if (s[k] !== flat[k] || t[k] !== flat[k]) return 'char ' + k;\n"
        "  const m = new Map([[s, 1]]);\n"
        "  if (m.get(t) !== 1 || m.get(flat) !== 1) return 'hash';\n"
        "  let u = '', ul = '', ur = '';\n"
        "  for (let i = 0; i < 5000; i++) {\n"
        "    const p = String.fromCharCode(65 + i % 26).repeat(513 + (i * 7919) % 20000);\n"
        "    if (i % 3) { u = u + p; ur = ur + p } else { u = p + u; ul = p + ul }\n"
        "  }\n"
        "  const uflat = ul + ur;\n"
        "  if (u.length !== uflat.length) return 'mixed length';\n"
        "  for (let k = 0; k < uflat.length; k += 9973) if (u[k] !== uflat[k]) return 'mixed char ' + k;\n"
        "  if (new Map([[u, 2]]).get(uflat) !== 2 || u !== uflat) return 'mixed';\n"
        "  u = ul = ur = null;\n"
        "  let a = piece(1) + piece(2), b = a + a, c = b + a + b;\n"
        "  if (c !== a + a + a + a + a) return 'shared';\n"
        "  a = b = c = s = t = null;\n"
        "  return 'ok' })()";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);

    CHECK(eval_is(ctx, src, "ok"), "ropes");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* Freeing runs free_zero_refcount one level deep, whatever is freed: a chain
   of a million objects, and objects of every kind whose finalizers free more,
   dropped at once and by a collection of their cycles. A second level would
   abort. */
static void test_free_nesting(void)
{
    static const char src[] =
        "let o = {}; for (let i = 0; i < 1000000; i++) o = { o };\n"
        "let keep = [];\n"
        "for (let i = 0; i < 1000; i++) {\n"
        "  const t = {}; const g = (function* () { yield t })(); g.next();\n"
        "  const a = (async () => { await null; return t })();\n"
        "  const f = function () { return t }.bind(t);\n"
        "  const m = new Map([[t, f]]); const w = new WeakMap([[t, g]]);\n"
        "  const r = new FinalizationRegistry(() => {}); r.register(t, i);\n"
        "  const p = new Proxy(t, {}); const ta = new Uint8Array(8);\n"
        "  t.self = [t, g, a, f, m, w, r, p, ta, new WeakRef(t), Promise.resolve(t)];\n"
        "  keep.push(t.self);\n"
        "}\n"
        "o = null; keep = null; 'ok'";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);

    CHECK(eval_is(ctx, src, "ok"), "free");
    CHECK(run_jobs(rt), "jobs");
    JS_RunGC(rt);
    CHECK(run_jobs(rt), "jobs after the collection");
    CHECK(eval_is(ctx, "1 + 1", "2"), "the runtime runs");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

static int memory_finalized;

static void memory_finalizer(JSRuntime *rt, JSValue val)
{
    JSMemoryUsage u;

    (void)val;
    JS_ComputeMemoryUsage(rt, &u);
    memory_finalized++;
}

/* A finalizer of the host may count the memory of the runtime while its
   object is freed: JS_ComputeMemoryUsage walks the objects of the
   collector, and the object being freed, which has let go of its shape
   already, is not among them. An object freed while it is young, and one
   freed after a collection made it old. */
static void test_finalizer_memory_usage(void)
{
    static const JSClassDef def = {
        "MemoryUsage", .finalizer = memory_finalizer,
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSClassID class_id = 0;
    JSValue obj;

    JS_NewClassID(rt, &class_id);
    CHECK(JS_NewClass(rt, class_id, &def) == 0, "the class");
    memory_finalized = 0;
    obj = JS_NewObjectClass(ctx, class_id);
    CHECK(JS_IsObject(obj), "the young object");
    JS_FreeValue(ctx, obj);
    CHECK(memory_finalized == 1, "the young object: %d finalizers ran",
          memory_finalized);
    obj = JS_NewObjectClass(ctx, class_id);
    CHECK(JS_IsObject(obj), "the old object");
    JS_RunGC(rt);
    JS_FreeValue(ctx, obj);
    CHECK(memory_finalized == 2, "the old object: %d finalizers ran",
          memory_finalized);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The objects of free_node_class say when they are freed: their finalizer
   writes their number into free_order, then lets go of the value it holds,
   which can free more. */
typedef struct {
    int id;
    JSValue held;
} FreeNode;

static JSClassID free_node_class;
static char free_order[1024];

static void free_node_finalizer(JSRuntime *rt, JSValue val)
{
    FreeNode *n = JS_GetOpaque(val, free_node_class);
    size_t len = strlen(free_order);

    snprintf(free_order + len, sizeof(free_order) - len, "%d ", n->id);
    JS_FreeValueRT(rt, n->held);
    free(n);
}

static void free_node_mark(JSRuntime *rt, JSValueConst val, JS_MarkFunc *mark_func)
{
    FreeNode *n = JS_GetOpaque(val, free_node_class);

    JS_MarkValue(rt, n->held, mark_func);
}

/* node(id, held): an object of free_node_class that holds held */
static JSValue free_node_new(JSContext *ctx, JSValueConst this_val, int argc,
                             JSValueConst *argv)
{
    JSValue obj;
    FreeNode *n;
    int id;

    (void)this_val;
    if (JS_ToInt32(ctx, &id, argv[0]))
        return JS_EXCEPTION;
    obj = JS_NewObjectClass(ctx, free_node_class);
    if (JS_IsException(obj))
        return obj;
    n = malloc(sizeof(*n));
    n->id = id;
    n->held = argc > 1 ? JS_DupValue(ctx, argv[1]) : JS_UNDEFINED;
    JS_SetOpaque(obj, n);
    return obj;
}

/* The order in which the objects that lose their last reference are freed:
   a tree whose properties, arrays, closures, Maps, Sets and finalizers hold
   more objects, some of them watched by a WeakRef or a FinalizationRegistry,
   dropped at once, after which the WeakRef finds its target gone and the
   callback of the FinalizationRegistry gets its value once a collection
   and the jobs ran,
   then a cycle freed by the collection and one freed by the young
   collection. The orders were recorded on the engine before the first
   object to die was freed outside the loop of free_zero_refcount
   (docs/changes/objects-and-shapes.md), and it passes this test. */
static void test_free_order(void)
{
    static const JSClassDef def = {
        "FreeNode", .finalizer = free_node_finalizer, .gc_mark = free_node_mark,
    };
    static const char tree[] =
        "var t = node(1, [node(2), { a: node(3), b: [node(4), node(5)] }]);\n"
        "t.x = node(6, node(7));\n"
        "t.f = (() => { const c = node(8); return () => c })();\n"
        "t.m = new Map([[node(9), node(10)], ['k', node(11)]]);\n"
        "t.s = new Set([node(12), node(13)]);\n"
        "var w = new WeakRef(t.x);\n"
        "var got = [], fr = new FinalizationRegistry((h) => got.push(h)); fr.register(t.s, 's');\n"
        "var chain = node(20); for (let i = 21; i < 25; i++) chain = node(i, { c: chain, n: node(100 + i) });\n"
        "t.chain = chain; chain = null;\n"
        "t.o = { p: { q: node(30) }, r: node(31) };\n"
        "void 0";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue global;

    JS_NewClassID(rt, &free_node_class);
    CHECK(JS_NewClass(rt, free_node_class, &def) == 0, "the class");
    global = JS_GetGlobalObject(ctx);
    CHECK(JS_SetPropertyStr(ctx, global, "node",
                            JS_NewCFunction(ctx, free_node_new, "node", 2)) >= 0, "node");
    JS_FreeValue(ctx, global);

    CHECK(eval(ctx, tree), "the tree");
    /* the job that made the WeakRef ends, and with it the hold of its
       target (docs/changes/weak-collections.md) */
    CHECK(run_jobs(rt), "the end of the job of the tree");
    free_order[0] = '\0';
    CHECK(eval(ctx, "t = null; void 0"), "the drop");
    CHECK(strcmp(free_order, "1 5 4 3 2 31 30 24 124 23 123 22 122 21 121 20 13 12 11 10 9 8 6 7 ") == 0, "the tree: %s", free_order);
    CHECK(eval_is(ctx, "String(w.deref()) + ' ' + got.length", "undefined 0"), "the WeakRef after the drop");
    /* a collection queues the callbacks of the FinalizationRegistry */
    JS_RunGC(rt);
    CHECK(run_jobs(rt), "jobs");
    CHECK(eval_is(ctx, "got.join()", "s"), "the FinalizationRegistry after the drop");

    free_order[0] = '\0';
    CHECK(eval(ctx, "var cy = node(40); cy.self = cy; cy.k = node(41, { a: node(42) }); cy.l = [cy, node(43)]; cy = null; void 0"), "the cycle");
    JS_RunGC(rt);
    CHECK(strcmp(free_order, "42 41 40 43 ") == 0, "the cycle: %s", free_order);

    free_order[0] = '\0';
    CHECK(eval(ctx, "var yc = node(50); yc.self = [yc, node(51), { b: node(52) }]; yc = null; void 0"), "the young cycle");
    JS_RunYoungGC(rt);
    CHECK(strcmp(free_order, "50 51 52 ") == 0, "the young cycle: %s", free_order);
    CHECK(run_jobs(rt), "jobs");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

static uint64_t fnv_word(uint64_t h, uint32_t v)
{
    int i;

    for (i = 0; i < 4; i++) {
        h ^= (v >> (8 * i)) & 0xff;
        h *= 0x100000001b3ULL;
    }
    return h;
}

static const volatile int no_stop = 0;

static void *unicode_realloc(void *opaque, void *ptr, size_t size)
{
    (void)opaque;
    if (size == 0) {
        free(ptr);
        return NULL;
    }
    return realloc(ptr, size);
}

static void sequence_hash(void *opaque, const uint32_t *buf, int len)
{
    uint64_t *h = opaque;
    int i;

    *h = fnv_word(*h, len);
    for (i = 0; i < len; i++)
        *h = fnv_word(*h, buf[i]);
}

/* The modules of the chains of test_module_chains: module n of a chain
   imports module n + 1 or exports from it, up to module chain_length - 1.
   The letter of the name tells what: m imports, s exports all of the next
   (export * from), e exports one name of the next (export { x } from). */
static int chain_length;

static void chain_source(const char *name, char *buf, size_t size)
{
    int n = atoi(name + 1);

    if (n == chain_length - 1)
        snprintf(buf, size, "export const x = 1;");
    else if (name[0] == 'm')
        snprintf(buf, size, "import \"m%d\";", n + 1);
    else if (name[0] == 's')
        snprintf(buf, size, "export * from \"s%d\";", n + 1);
    else
        snprintf(buf, size, "export { x } from \"e%d\";", n + 1);
}

static JSModuleDef *chain_loader(JSContext *ctx, const char *name, void *opaque)
{
    char src[64];
    JSValue v;
    JSModuleDef *m;

    chain_source(name, src, sizeof(src));
    v = JS_Eval(ctx, src, strlen(src), name, JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(v))
        return NULL;
    /* the module is referenced by the context */
    m = JS_VALUE_GET_PTR(v);
    JS_FreeValue(ctx, v);
    return m;
}

/* exception_name puts the name of the exception of ctx in buf. */
static const char *exception_name(JSContext *ctx, char *buf, size_t size)
{
    JSValue e = JS_GetException(ctx);
    JSValue n = JS_GetPropertyStr(ctx, e, "name");
    const char *s = JS_ToCString(ctx, n);

    snprintf(buf, size, "%s", s ? s : "?");
    JS_FreeCString(ctx, s);
    JS_FreeValue(ctx, n);
    JS_FreeValue(ctx, e);
    return buf;
}

/* module_eval runs src as the module name, or only compiles it, and puts
   "ok" or the name of the exception in buf. */
static const char *module_eval(JSContext *ctx, const char *name, const char *src,
                               int flags, char *buf, size_t size)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), name, JS_EVAL_TYPE_MODULE | flags);

    if (!JS_IsException(v)) {
        JS_FreeValue(ctx, v);
        snprintf(buf, size, "ok");
        return buf;
    }
    return exception_name(ctx, buf, size);
}

/* compile_back compiles the modules of a chain from the last to the first,
   or runs them, so that each finds the next one made: none of the
   recursions over the chain goes more than a module down. It returns the
   number of modules made before the first that failed, with its exception
   in buf. */
static int compile_back(JSContext *ctx, char letter, int flags, char *buf, size_t size)
{
    char name[16], src[64];
    int n;

    for (n = chain_length - 1; n >= 0; n--) {
        snprintf(name, sizeof(name), "%c%d", letter, n);
        chain_source(name, src, sizeof(src));
        if (strcmp(module_eval(ctx, name, src, flags, buf, size), "ok") != 0)
            break;
    }
    return chain_length - 1 - n;
}

/* read_chain compiles the modules of the m chain in ctx and reads them from
   their bytecode in ctx2, from the last to the first, which leaves them all
   to resolve. It returns the first, the one ctx2 does not hold alone. */
static JSValue read_chain(JSContext *ctx, JSContext *ctx2)
{
    uint8_t **bufs = calloc(chain_length, sizeof(bufs[0]));
    size_t *sizes = calloc(chain_length, sizeof(sizes[0]));
    char name[16], src[64];
    JSValue v, first = JS_UNDEFINED;
    int n;

    for (n = chain_length - 1; n >= 0; n--) {
        snprintf(name, sizeof(name), "m%d", n);
        chain_source(name, src, sizeof(src));
        v = JS_Eval(ctx, src, strlen(src), name, JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
        bufs[n] = JS_WriteObject(ctx, &sizes[n], v, JS_WRITE_OBJ_BYTECODE);
        JS_FreeValue(ctx, v);
    }
    for (n = chain_length - 1; n >= 0; n--) {
        v = JS_ReadObject(ctx2, bufs[n], sizes[n], JS_READ_OBJ_BYTECODE);
        js_free(ctx, bufs[n]);
        if (n == 0)
            first = v;
        else
            JS_FreeValue(ctx2, v);
    }
    free(bufs);
    free(sizes);
    return first;
}

/* resolve_read resolves the first module of the m chain read from bytecode:
   JS_ResolveModule goes down the chain with no loader between the modules.
   It puts "ok" or the name of the exception in buf. */
static const char *resolve_read(JSRuntime *rt, char *buf, size_t size)
{
    JSContext *ctx = JS_NewContext(rt), *ctx2 = JS_NewContext(rt);
    JSValue first = read_chain(ctx, ctx2);

    if (JS_ResolveModule(ctx2, first) < 0)
        exception_name(ctx2, buf, size);
    else
        snprintf(buf, size, "ok");
    JS_FreeValue(ctx2, first);
    JS_FreeContext(ctx2);
    JS_FreeContext(ctx);
    return buf;
}

/* The recursions over a chain of modules count the internal depth, a
   module a level, and end with a RangeError past JS_MAX_INTERNAL_DEPTH:
   resolving the imports of a module, read from bytecode or through the
   host's loader, making their functions, listing the names a namespace
   exports through a chain of export * from, and finding a name exported
   from module to module. The runtime still runs code and frees with no
   block left. */
static int test_module_chains(void *unused)
{
    static const struct { char letter; int flags; const char *top; } cases[] = {
        /* the loader loads each module from the import of the one before */
        { 0, 0, "import \"m0\";" },
        /* made from the end, only the functions go down the chain */
        { 'm', JS_EVAL_FLAG_COMPILE_ONLY, "import \"m0\";" },
        /* run from the end, only the names of the namespace go down */
        { 's', 0, "import * as ns from \"s0\"; if (Object.keys(ns).join() !== \"x\") throw 1;" },
    };
    char buf[64];
    size_t i;
    int made;

    chain_length = JS_MAX_INTERNAL_DEPTH + 1000;
    {
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx;

        CHECK(strcmp(resolve_read(rt, buf, sizeof(buf)), "RangeError") == 0, "read: %s", buf);
        ctx = JS_NewContext(rt);
        CHECK(eval_is(ctx, "1 + 1", "2"), "read: the runtime runs");
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }
    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt);

        JS_SetModuleLoaderFunc(rt, NULL, chain_loader, NULL);
        if (cases[i].letter) {
            made = compile_back(ctx, cases[i].letter, cases[i].flags, buf, sizeof(buf));
            CHECK(made == chain_length, "case %zu: %d modules made, then %s", i, made, buf);
        }
        module_eval(ctx, "top", cases[i].top, 0, buf, sizeof(buf));
        CHECK(strcmp(buf, "RangeError") == 0, "case %zu: %s", i, buf);
        CHECK(run_jobs(rt), "case %zu: jobs", i);
        CHECK(eval_is(ctx, "1 + 1", "2"), "case %zu: the runtime runs", i);
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }
    {
        /* module n, run, finds x in the last module, chain_length - 1 - n
           modules down */
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt);

        made = compile_back(ctx, 'e', 0, buf, sizeof(buf));
        CHECK(made == JS_MAX_INTERNAL_DEPTH, "export from: %d modules made, then %s", made, buf);
        CHECK(strcmp(buf, "RangeError") == 0, "export from: %s", buf);
        CHECK(run_jobs(rt), "export from: jobs");
        CHECK(eval_is(ctx, "1 + 1", "2"), "export from: the runtime runs");
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }
    return 0;
}

/* A chain of modules that failed past the internal depth leaves no module
   that looks done when the modules it imports are not, or are freed: the
   chain runs again from its middle in the same context. Module mid was
   done, as far as it went, by the run that failed, and the chain from it is
   short enough to run. */
static int test_module_retry(void *unused)
{
    int mid = JS_MAX_INTERNAL_DEPTH * 3 / 4, made;
    char buf[64], src[64];

    chain_length = JS_MAX_INTERNAL_DEPTH + 1000;
    snprintf(src, sizeof(src), "import \"m%d\";", mid);
    {
        /* resolving the imports of the modules read from bytecode fails
           at module JS_MAX_INTERNAL_DEPTH of the chain, which
           js_free_modules frees with the ones after it. The loader loads
           module mid again */
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt), *ctx2 = JS_NewContext(rt);
        JSValue first = read_chain(ctx, ctx2);

        CHECK(JS_ResolveModule(ctx2, first) < 0, "resolve: no error");
        CHECK(strcmp(exception_name(ctx2, buf, sizeof(buf)), "RangeError") == 0, "resolve: %s", buf);
        JS_SetModuleLoaderFunc(rt, NULL, chain_loader, NULL);
        module_eval(ctx2, "again", src, 0, buf, sizeof(buf));
        CHECK(strcmp(buf, "ok") == 0, "resolve again: %s", buf);
        CHECK(run_jobs(rt), "resolve again: jobs");
        JS_FreeValue(ctx2, first);
        JS_FreeContext(ctx2);
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }
    {
        /* making the functions of the modules compiled from the end, all
           resolved, fails at module JS_MAX_INTERNAL_DEPTH - 1 of the chain,
           below the module that imports the first */
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt);

        JS_SetModuleLoaderFunc(rt, NULL, chain_loader, NULL);
        made = compile_back(ctx, 'm', JS_EVAL_FLAG_COMPILE_ONLY, buf, sizeof(buf));
        CHECK(made == chain_length, "functions: %d modules made, then %s", made, buf);
        module_eval(ctx, "top", "import \"m0\";", 0, buf, sizeof(buf));
        CHECK(strcmp(buf, "RangeError") == 0, "functions: %s", buf);
        module_eval(ctx, "again", src, 0, buf, sizeof(buf));
        CHECK(strcmp(buf, "ok") == 0, "functions again: %s", buf);
        CHECK(run_jobs(rt), "functions again: jobs");
        CHECK(eval_is(ctx, "1 + 1", "2"), "functions again: the runtime runs");
        JS_FreeContext(ctx);
        host_free_runtime(rt);
    }
    return 0;
}

/* stack_fail_sweep runs src with the stack short at the first count of the
   engine, then at the second, and so on until it runs whole. Each time it
   fails with a RangeError, or runs to want with no exception left behind. */
static void stack_fail_sweep(const char *what, const char *src, const char *want)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    char buf[64];
    long n;
    int fired;

    for (n = 1;; n++) {
        JSValue v;

        host_stack_fail_at = n;
        v = JS_Eval(ctx, src, strlen(src), "<sweep>", JS_EVAL_TYPE_GLOBAL);
        fired = host_stack_fail_at == 0;
        host_stack_fail_at = 0;
        if (JS_IsException(v)) {
            exception_name(ctx, buf, sizeof(buf));
            CHECK(fired && strcmp(buf, "RangeError") == 0, "%s, short at %ld: %s", what, n, buf);
        } else {
            const char *s;

            if (JS_HasException(ctx)) {
                CHECK(0, "%s, short at %ld: ran, and left %s", what, n,
                      exception_name(ctx, buf, sizeof(buf)));
            }
            s = JS_ToCString(ctx, v);
            CHECK(s && strcmp(s, want) == 0, "%s, short at %ld: got %s", what, n, s ? s : "?");
            JS_FreeCString(ctx, s);
        }
        JS_FreeValue(ctx, v);
        if (!fired)
            break;
    }
    CHECK(n > 1, "%s: no count", what);
    CHECK(eval_is(ctx, "1 + 1", "2"), "%s: the runtime runs", what);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A function that uses a variable of a function around it gets a closure
   variable in each function between, a level of the internal depth each,
   and the lookup fails past it or with too little stack. Code does not get
   there first, as the parser counts more levels for a function than the
   lookups do, so each lookup of the compiler is made to fail in turn: the
   compile throws the RangeError and frees what it made. */
static void test_closure_lookup_fail(void)
{
    static const struct { const char *what, *src, *want; } cases[] = {
        { "argument and local",
          "(function (a) { var b = 2; return function () { return a + b; }; })(1)()", "3" },
        { "with around",
          "(function (o) { with (o) { return function () { return x; }; } })({ x: 4 })()", "4" },
        { "eval in the body",
          "(function () { eval('var x = 5'); return function () { return x; }; })()()", "5" },
        { "eval in the parameters",
          "(function (a = eval('var y = 6')) { return function () { return y; }; })()()", "6" },
        { "function in direct eval",
          "(function () { var y = 7; return eval('(function () { return y + typeof z; })')(); })()",
          "7undefined" },
    };
    size_t i;

    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++)
        stack_fail_sweep(cases[i].what, cases[i].src, cases[i].want);
}

/* JSON.parse with a reviver keeps a record of each value it parsed, for the
   source text of the primitives, and frees the records with no recursion:
   nested as deep as it parses, as wide as it likes, objects with a hash of
   their keys, and records left half made by a syntax error at any depth or
   by data after the end. Everything is freed with no block left. */
static int test_json_records(void *unused)
{
    static const char src[] =
        "(() => {\n"
        "  const n = %d, src = [];\n"
        "  const reviver = function (k, v, c) { if (c && 'source' in c) src.push(c.source); return v };\n"
        "  const a = JSON.parse('['.repeat(n) + '12' + ']'.repeat(n), reviver);\n"
        "  if (src.join() !== '12') return 'deep array ' + src.join();\n"
        "  JSON.parse('{\"a\":'.repeat(n) + '3e1' + '}'.repeat(n), reviver);\n"
        "  if (src.join() !== '12,3e1') return 'deep object';\n"
        "  const keys = []; for (let i = 0; i < 100000; i++) keys.push('\"k' + i + '\":' + i);\n"
        "  const o = JSON.parse('{' + keys.join() + ',\"x\":[' + keys.map((_, i) => i).join() + ']}', (k, v) => v);\n"
        "  if (o.k99999 !== 99999 || o.x.length !== 100000) return 'wide';\n"
        "  const nine = '{\"b\":1,\"c\":2,\"d\":3,\"e\":4,\"f\":5,\"g\":6,\"h\":7,\"i\":8,\"j\":';\n"
        "  for (const bad of ['{\"a\":['.repeat((n >> 1) - 2) + '1,' + nine + '[1,2,x',\n"
        "                    nine.repeat(n - 1) + '}', '[[1],{\"a\":[2]}] x', '[' + keys.join()])\n"
        "    try { JSON.parse(bad, reviver); return 'parsed ' + bad.slice(0, 20) } catch (e) {\n"
        "      if (!(e instanceof SyntaxError)) return e.name }\n"
        "  return 'ok' })()";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    char buf[2048];

    /* the reviver goes down one level more than the parser */
    snprintf(buf, sizeof(buf), src, JS_MAX_INTERNAL_DEPTH - 2);
    CHECK(eval_is(ctx, buf, "ok"), "records");
    CHECK(eval_is(ctx, "1 + 1", "2"), "the runtime runs");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* The case conversions, the case foldings of regexps, the four normal forms
   of every code point and the emoji sequences of regexps are those of the
   recursive code they replace: their hashes are the ones it gave. */
static void test_unicode_tables(void)
{
    static const char *const seqs[] = {
        "Basic_Emoji", "Emoji_Keycap_Sequence", "RGI_Emoji_Modifier_Sequence",
        "RGI_Emoji_Flag_Sequence", "RGI_Emoji_Tag_Sequence",
        "RGI_Emoji_ZWJ_Sequence", "RGI_Emoji",
    };
    uint64_t conv = 0xcbf29ce484222325ULL, norm = conv, seq = conv;
    uint32_t c, res[LRE_CC_RES_LEN_MAX], *dst;
    int t, i, len;
    CharRange cr;

    for (c = 0; c < 0x110000; c++) {
        for (t = 0; t < 3; t++) {
            len = lre_case_conv(res, c, t);
            conv = fnv_word(conv, len);
            for (i = 0; i < len; i++)
                conv = fnv_word(conv, res[i]);
        }
        conv = fnv_word(conv, lre_canonicalize(c, 0));
        conv = fnv_word(conv, lre_canonicalize(c, 1));
        for (t = UNICODE_NFC; t <= UNICODE_NFKD; t++) {
            len = unicode_normalize(&dst, &c, 1, t, NULL, unicode_realloc, &no_stop);
            CHECK(len >= 0, "normalize U+%04X", c);
            if (len < 0)
                return;
            norm = fnv_word(norm, len);
            for (i = 0; i < len; i++)
                norm = fnv_word(norm, dst[i]);
            free(dst);
        }
    }
    for (i = 0; i < countof(seqs); i++) {
        cr_init(&cr, NULL, unicode_realloc);
        CHECK(unicode_sequence_prop(seqs[i], sequence_hash, &seq, &cr) == 0, "%s", seqs[i]);
        cr_free(&cr);
    }
    CHECK(conv == 0x62b5753977fb68f5ULL, "case conversions %016llx", (unsigned long long)conv);
    CHECK(norm == 0x49b42101d280d714ULL, "normal forms %016llx", (unsigned long long)norm);
    CHECK(seq == 0x365073493a2d88c5ULL, "emoji sequences %016llx", (unsigned long long)seq);
}

/* A class of a v-flag regexp reads the strings of \q{...} as class atoms,
   which cannot hold \q again. */
static void test_class_strings(void)
{
    static const char src[] =
        "(() => {\n"
        "  const r = /^[\\q{abc|d\\x41|}x]+$/v;\n"
        "  if (!r.test('abcdAx') || !r.test('x') || r.test('ab')) return 'match';\n"
        "  if (!/^[\\q{\\u{1F600}b}--\\q{b}]$/v.test('\\u{1F600}b')) return 'subtract';\n"
        "  if (!/^[[\\q{ab}c]&&\\q{ab}]$/v.test('ab')) return 'intersect';\n"
        "  if (!/^[\\q{\\x41\\u0042}]$/v.test('AB')) return 'escape';\n"
        "  for (const bad of ['[\\\\q{a\\\\q{b}}]', '[\\\\q{a]', '\\\\q{a}']) {\n"
        "    try { new RegExp(bad, 'v'); return 'accepted ' + bad } catch (e) {\n"
        "      if (!(e instanceof SyntaxError)) return 'error ' + bad }\n"
        "  }\n"
        "  return 'ok' })()";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);

    CHECK(eval_is(ctx, src, "ok"), "class strings");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* rqsort_stop stops soon after its stop is set: cmp sets it at the k-th
   comparison, for every k of sorts of 2 to 64 elements and for k spread over
   sorts of 100 to 5000, and the comparisons after it are at most the 14 that
   cutils.h allows outside the heap sort (which random input does not reach). */
void rqsort_stop(void *base, size_t nmemb, size_t size,
                 int (*cmp)(const void *, const void *, void *),
                 void *arg, const volatile int *stop);

typedef struct {
    long n, k;
    int stop;
} SortStop;

static int sort_stop_cmp(const void *a, const void *b, void *opaque)
{
    SortStop *s = opaque;
    int x = *(const int *)a, y = *(const int *)b;

    if (++s->n == s->k)
        s->stop = 1;
    return (x > y) - (x < y);
}

static void test_sort_stop_bound(void)
{
    static const int big[] = { 100, 1000, 5000 };
    static int a[5000];
    uint32_t seed = 12345;
    long worst = 0, runs = 0;
    int size, i, b;

    for (b = -63; b < 3; b++) {
        long total, k, step;
        SortStop s;

        size = b < 0 ? 65 + b : big[b];
        for (i = 0; i < size; i++) {
            seed = seed * 1103515245 + 12345;
            a[i] = (int)(seed >> 16) % 1000;
        }
        s.n = 0;
        s.k = 0;
        s.stop = 0;
        {
            static int c[5000];
            memcpy(c, a, size * sizeof(int));
            rqsort_stop(c, size, sizeof(int), sort_stop_cmp, &s, &s.stop);
        }
        total = s.n;
        step = size <= 64 ? 1 : total / 300 + 1;
        for (k = 1; k <= total; k += step) {
            static int c[5000];

            memcpy(c, a, size * sizeof(int));
            s.n = 0;
            s.k = k;
            s.stop = 0;
            rqsort_stop(c, size, sizeof(int), sort_stop_cmp, &s, &s.stop);
            runs++;
            if (s.n - k > worst)
                worst = s.n - k;
            CHECK(s.n - k <= 14, "a sort of %d stopped at comparison %ld made %ld more", size, k, s.n - k);
        }
    }
    CHECK(runs > 2000, "only %ld sorts were stopped", runs);
    CHECK(worst > 0, "no sort made a comparison after the stop: the bound is not tested");
}

/* A regexp that looks for where a match can start (lre_skip and lre_run) and
   one whose match runs long read the interrupt flag as the backtracking
   machine does. T(s) sets the flag as the string is converted, so the regexp
   starts with the flag set and must stop in its work. A flag set only at the
   50th reading must stop it too, which shows that the regexp reads the flag
   again and again as it works, not once at the end of it:
   when no position can start a match, when each can but the match fails
   there, and in the loop of a greedy x+, which jumps back by a split op that
   pushes nothing. */
static void test_regexp_stops(void)
{
    static const struct {
        const char *name, *s, *re;
        int log2len;
    } cases[] = {
        { "8bit", "'a'", "/[\"&<>]/", 22 },
        { "16bit", "'\\u4e2d'", "/[\"&<>]/", 22 },
        { "8bit/each", "'a'", "/ab/", 22 },
        { "16bit/each", "'\\u4e2d'", "/\\u4e2db/", 22 },
        { "8bit/set", "'a'", "/(?:a|c)b/", 22 },
        { "16bit/set", "'\\u4e2d'", "/(?:\\u4e2d|c)b/", 22 },
        { "plus", "'a'", "/a+b/", 17 },
        { "plus/unicode", "'a'", "/a+b/u", 17 },
        { "plus/16bit", "'\\u4e2d'", "/\\u4e2d+b/", 17 },
        { "plus/ignore case", "'a'", "/A+b/i", 17 },
        { "class", "'a'", "/\\w+@/", 17 },
        { "capture", "'a'", "/(a)+b/", 17 },
        { "alternatives", "'a'", "/(?:a|c)+b/", 17 },
        /* sticky, a match is tried once: only the split ops that push nothing
           read the flag on the way */
        { "plus/sticky", "'a'", "/a+b/y", 17 },
        { "plus/16bit/sticky", "'\\u4e2d'", "/\\u4e2d+b/y", 17 },
        { "plus/ignore case/sticky", "'a'", "/A+b/iy", 17 },
        { "class/sticky", "'a'", "/\\w+@/y", 17 },
        { "capture/sticky", "'a'", "/(a)+b/y", 17 },
        { "alternatives/sticky", "'a'", "/(?:a|c)+b/y", 17 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    add_interrupt(ctx);
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        char src[256];

        snprintf(src, sizeof(src), "var s = %s.repeat(1 << %d), re = %s; 0",
                 cases[i].s, cases[i].log2len, cases[i].re);
        CHECK(eval(ctx, src), "the script threw");
        CHECK(interrupted(ctx, "re.test(T(s))"), "%s: /%s/ was not interrupted",
              cases[i].name, cases[i].re);
        CHECK(interrupted(ctx, "re.exec(T(s))"), "%s: exec was not interrupted",
              cases[i].name);
        /* and when it is set in the middle of the work, after a few readings */
        host_poll_stop_after(50);
        CHECK(interrupted(ctx, "re.test(s)"), "%s: /%s/ was not stopped in the middle",
              cases[i].name, cases[i].re);
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"),
              "the context after %s", cases[i].name);
    }
    *JS_GetInterruptFlag(rt) = 0;
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* lre_skip reads the interrupt flag after each LRE_SKIP_CHUNK (65536) units
   it looks at and then answers as the backtracking machine does: lre_exec
   returns LRE_RET_TIMEOUT. It reads nothing at the end of the string. The
   readings are counted by the host (host_poll_reads). */
static void test_regexp_skip_polls(void)
{
    enum { CHUNK = 1 << 16 };
    static const struct {
        const char *re, *unit;
        int units, stop, result, reads;
    } cases[] = {
        { "a", "x", CHUNK, 1, 0, 0 },
        { "a", "x", CHUNK + 1, 1, LRE_RET_TIMEOUT, 1 },
        { "a", "x", 3 * CHUNK + 1, 0, 0, 3 },
        { "[ab]", "x", 3 * CHUNK + 1, 1, LRE_RET_TIMEOUT, 1 },
        { "a|b", "x", 3 * CHUNK + 1, 1, LRE_RET_TIMEOUT, 1 },
        { "a|b", "x", 3 * CHUNK + 1, 0, 0, 3 },
        { "a", "\xe4\xb8\xad", 2 * CHUNK + 1, 1, LRE_RET_TIMEOUT, 1 },
        { "a", "x", 2 * CHUNK, 0, 0, 1 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        int wide = cases[i].unit[1] != 0;
        size_t unit_bytes = wide ? 2 : 1;
        uint8_t *buf = malloc((size_t)cases[i].units * unit_bytes);
        uint8_t *capture[2];
        char msg[64];
        uint8_t *bc;
        int len, k, ret;
        long before;

        for(k = 0; k < cases[i].units; k++) {
            if (wide) {
                ((uint16_t *)buf)[k] = 0x4e2d;
            } else {
                buf[k] = 'x';
            }
        }
        *JS_GetInterruptFlag(rt) = 0;
        bc = lre_compile(&len, msg, sizeof(msg), cases[i].re, strlen(cases[i].re), 0, ctx);
        CHECK(bc != NULL, "/%s/: %s", cases[i].re, msg);
        if (bc) {
            *JS_GetInterruptFlag(rt) = cases[i].stop;
            before = host_poll_reads;
            ret = lre_exec(capture, bc, buf, 0, cases[i].units, wide, ctx);
            CHECK(ret == cases[i].result && host_poll_reads - before == cases[i].reads,
                  "/%s/ on %d units%s, flag %d: result %d, %ld reads, want %d, %d",
                  cases[i].re, cases[i].units, wide ? " of 16 bits" : "", cases[i].stop,
                  ret, host_poll_reads - before, cases[i].result, cases[i].reads);
            *JS_GetInterruptFlag(rt) = 0;
            lre_realloc(ctx, bc, 0);
        }
        free(buf);
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The strings of a rope are copied and compared in loops that read the
   interrupt flag after every JS_ROPE_STEP (65536) characters. A rope of 2^27
   characters made by doubling, with a copy of no string in it, is made flat
   by (r1 + 'x').charCodeAt(0) and compared with another by r1 < r2 (an
   equality test cannot report an exception, and reads nothing). The host
   counts the readings of the flag (host_poll_reads), and a flag set at the
   20th reading stops each: a loop that read the flag once or twice would not
   get there. */
static void test_rope_polls(void)
{
    static const struct {
        const char *name, *call;
        long reads;
    } cases[] = {
        { "made flat", "(r1 + 'x').charCodeAt(0)", 1000 },
        { "compared", "r1 < r2", 1000 },
        /* equal ropes are compared leaf by leaf by every form of equality the interpreter has */
        { "compared by ===", "r1 === r2", 1000 },
        { "compared by !==", "r1 !== r2", 1000 },
        { "compared by ==", "r1 == r2", 1000 },
        { "compared by !=", "r1 != r2", 1000 },
        { "compared with a flat string", "r3 === fl", 200 },
        { "a flat string compared with a rope", "fl !== r3", 200 },
        /* two flat strings of more than a step */
        { "flat strings compared by ===", "fl === fl2", 200 },
        { "flat strings compared by !==", "fl !== fl2", 200 },
        { "flat strings compared by ==", "fl == fl2", 200 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, "var f = function () { var r = 'ab'; for (var i = 0; i < 26; i++) r = r + r; return r }; var r1 = f(), r2 = f(); 0"),
          "the script threw");
    CHECK(eval(ctx, "var r3 = 'ab'; for (var i = 0; i < 23; i++) r3 = r3 + r3; var fl = 'ab'.repeat(1 << 23), fl2 = 'ab'.repeat(1 << 23); 0"), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        long before = host_poll_reads;

        CHECK(eval(ctx, cases[i].call), "%s: the script threw", cases[i].name);
        CHECK(host_poll_reads - before >= cases[i].reads, "%s: %ld readings of the flag, want %ld", cases[i].name,
              host_poll_reads - before, cases[i].reads);
        host_poll_stop_after(20);
        CHECK(interrupted(ctx, cases[i].call), "%s was not stopped in the middle", cases[i].name);
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after %s", cases[i].name);
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A Map or Set that holds a long string or a rope finds an equal one after
   hashing it and comparing the two, rope leaf by leaf and flat string in
   steps. Both read the flag. The reading of the hash is measured with an
   empty Set, the rest is the comparison, and a flag set in the middle of
   the comparison stops the call and leaves the collection as it was. */
static void test_rope_key_compare_polls(void)
{
    static const struct {
        const char *call, *make;
    } cases[] = {
        { "c.get(K2)", "c = new Map([[K1, 7], ['x', 8]])" },
        { "c.has(K2)", "c = new Set([K1, 'x'])" },
        { "c.delete(K2)", "c = new Set([K1, 'x'])" },
        { "c.set(K2, 9)", "c = new Map([[K1, 7], ['x', 8]])" },
        { "c.add(K2)", "c = new Set([K1, 'x'])" },
        { "c.getOrInsert(K2, 5)", "c = new Map([[K1, 7], ['x', 8]])" },
    };
    /* the key made twice, as two ropes and as two flat strings, and a rope with a flat string */
    static const char *const keys[][2] = { { "r1", "r2" }, { "s1", "s2" }, { "r1", "s2" }, { "s1", "r2" } };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i, k;

    CHECK(eval(ctx, "var r1 = 'ab'; for (var i = 0; i < 23; i++) r1 = r1 + r1; var r2 = 'ab'; for (var i = 0; i < 23; i++) r2 = r2 + r2; "
                    "var s1 = 'ab'.repeat(1 << 23), s2 = 'ab'.repeat(1 << 23), c, e = new Set(); 0"),
          "the script threw");
    for (k = 0; k < sizeof(keys) / sizeof(keys[0]); k++) {
        long before, hash, all;
        char probe[64];

        snprintf(probe, sizeof(probe), "e.has(%s)", keys[k][1]);
        CHECK(eval(ctx, probe), "the script threw");
        before = host_poll_reads;
        CHECK(eval(ctx, probe), "the script threw");
        hash = host_poll_reads - before;
        CHECK(hash >= 200, "%s: %ld readings of the flag in the hash, want at least 200", probe, hash);
        for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
            char call[96], make[96];
            const char *p;
            size_t o = 0, m = 0;

            for (p = cases[i].call; *p && o < sizeof(call) - 4; p++) {
                if (p[0] == 'K' && (p[1] == '1' || p[1] == '2')) {
                    o += snprintf(call + o, sizeof(call) - o, "%s", keys[k][p[1] - '1']);
                    p++;
                } else {
                    call[o++] = *p;
                }
            }
            call[o] = 0;
            for (p = cases[i].make; *p && m < sizeof(make) - 4; p++) {
                if (p[0] == 'K' && (p[1] == '1' || p[1] == '2')) {
                    m += snprintf(make + m, sizeof(make) - m, "%s", keys[k][p[1] - '1']);
                    p++;
                } else {
                    make[m++] = *p;
                }
            }
            make[m] = 0;
            CHECK(eval(ctx, make) && eval(ctx, call), "%s: the script threw", call);
            CHECK(eval(ctx, make), "%s: the script threw", make);
            before = host_poll_reads;
            CHECK(eval(ctx, call), "%s: the script threw", call);
            all = host_poll_reads - before;
            CHECK(all - hash >= 200, "%s: %ld readings of the flag in the comparison, want at least 200", call, all - hash);
            CHECK(eval(ctx, make), "%s: the script threw", make);
            host_poll_stop_after(hash + (all - hash) / 2);
            CHECK(interrupted(ctx, call), "%s was not stopped in the middle", call);
            host_poll_stop_after(0);
            CHECK(eval_is(ctx, "[c.size, c.has('x'), [...c.keys()].length].join()", "2,true,2"), "the collection after %s", call);
        }
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The next of the iterator of a string, an array, a Map or a Set (and of an
   iterator helper) is called by JS_IteratorNext2 without JS_CallInternal,
   which reads the interrupt flag when a call starts, so JS_IteratorNext2
   reads it for them. A loop of C that walks such an iterator and calls
   nothing else for each item (Object.fromEntries, a spread, toArray,
   Set.prototype.union) is stopped by it. So is the copy of a Set that the
   methods of Set make, which reads the flag every 1024 records. Each call
   below reads the flag at least as often as it says over a Set or a Map of
   65536 items, and is stopped by a flag set at the 20th reading: it throws
   "interrupted", and the context goes on. */
static void test_iterator_polls(void)
{
    static const char setup[] =
        "var s = new Set(), t = new Set(), u = new Set(), m = new Map(), a = [], x, rest\n"
        "for (var i = 0; i < 1 << 16; i++) { s.add(i); t.add(i); u.add(-i - 1); m.set(i, i); a.push(i) }\n"
        "var str = 'ab'.repeat(1 << 15); 0\n";
    static const struct {
        const char *call;
        long reads;
    } cases[] = {
        { "[...s]", 1 << 16 },
        { "[...m]", 1 << 16 },
        { "[...str]", 1 << 16 },
        { "[x, ...rest] = s", 1 << 16 },
        { "Object.fromEntries(m)", 1 << 16 },
        { "Math.sumPrecise(s)", 1 << 16 },
        { "s.values().drop(1 << 16).next()", 1 << 16 },
        { "s.values().toArray()", 1 << 16 },
        { "s.union(t)", 1 << 16 },
        { "s.union(new Set())", 1 << 6 },
        { "s.symmetricDifference(t)", 1 << 16 },
        { "s.isSupersetOf(t)", 1 << 16 },
        { "s.isSubsetOf(t)", 1 << 16 },
        { "s.isDisjointFrom(u)", 1 << 16 },
        { "Array.from(s)", 1 << 16 },
        { "Array.from(a.values())", 1 << 16 },
        { "Array.from(m)", 1 << 16 },
        { "new Set(a.values())", 1 << 16 },
        { "new Map(m)", 1 << 16 },
        { "new Uint8Array(s)", 1 << 16 },
        { "Uint8Array.from(s)", 1 << 16 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *call = cases[i].call;
        long before = host_poll_reads;

        CHECK(eval(ctx, call), "%s: the script threw", call);
        CHECK(host_poll_reads - before >= cases[i].reads, "%s: %ld readings of the flag, want at least %ld", call,
              host_poll_reads - before, cases[i].reads);
        host_poll_stop_after(20);
        CHECK(interrupted(ctx, call), "%s was not stopped in the middle", call);
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after %s", call);
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The loops of the string methods that run for as long as the string is and
   call nothing read the interrupt flag: the search of replace and replaceAll
   for a string (as indexOf does, so a needle that almost matches at each
   index does not take the product of the lengths without a read), the
   matches of replaceAll (every 1024), repeat (every 65536 characters, as
   the filler of padStart), the copy of a long piece of a string
   (trim, slice and substring, every 1048576 characters, after the scan of a
   16-bit string for a character past 0xff) and the filler of padStart and
   padEnd, which is written in steps of 65536 characters whatever its length.
   Each call reads the flag at least as often as it says, and a flag set in the
   middle stops it with the error "interrupted". The strings are made flat by
   join: a rope made by + is made flat by the first call alone, which reads
   the flag for that too (test_rope_flat_polls), and the middle of the first
   call would not be the middle of the next. */
static void test_string_loop_polls(void)
{
    static const char setup[] =
        "var h = 'a'.repeat(1 << 20), nd = 'a'.repeat(100) + 'b', ab = 'ab'.repeat(1 << 17)\n"
        "var sp = [' ', 'x'.repeat(1 << 24), ' '].join('')\n"
        "var wsp = [' ', '\\u4e00'.repeat(1 << 23), ' '].join('')\n"
        "var nsp = ['\\u4e00', ' ', '\\u00e9'.repeat(1 << 23), ' '].join('')\n"
        "var fill = 'ab'.repeat(1 << 22); 0\n";
    static const struct {
        const char *call;
        long reads;
    } cases[] = {
        { "h.replace(nd, 'c')", 15 },
        { "h.replaceAll(nd, 'c')", 15 },
        { "ab.replaceAll('a', 'cc')", 128 },
        { "sp.trim()", 15 },
        { "wsp.trim()", 7 },
        { "sp.slice(1)", 15 },
        { "wsp.substring(1)", 7 },
        { "nsp.slice(1)", 7 },
        { "'x'.repeat(1 << 24)", 255 },
        { "'ab'.repeat(1 << 23)", 255 },
        { "fill.repeat(2)", 255 },
        { "'x'.padStart(1 << 24, fill)", 255 },
        { "'x'.padEnd(1 << 24, fill)", 255 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *call = cases[i].call;
        long before = host_poll_reads, reads;

        CHECK(eval(ctx, call), "%s: the script threw", call);
        reads = host_poll_reads - before;
        CHECK(reads >= cases[i].reads, "%s: %ld readings of the flag, want at least %ld", call, reads, cases[i].reads);
        host_poll_stop_after(reads / 2 + 1);
        CHECK(interrupted(ctx, call), "%s was not stopped in the middle", call);
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after %s", call);
    }
    /* what they make is what they made */
    CHECK(eval_is(ctx, "[sp.trim().length, wsp.trim().length, nsp.slice(1).trim().length, sp.slice(1, -1).length, wsp.substring(1).length].join()",
                  "16777216,8388608,8388608,16777216,8388609"), "the length of the pieces");
    CHECK(eval_is(ctx, "[/^x+$/.test(sp.trim()), wsp.trim() === '\\u4e00'.repeat(1 << 23), nsp.slice(1).trim() === '\\u00e9'.repeat(1 << 23), nsp.slice(1) === ' ' + '\\u00e9'.repeat(1 << 23) + ' '].join()",
                  "true,true,true,true"), "the pieces");
    CHECK(eval_is(ctx, "var r1 = 'ab'.repeat(1 << 23), r2 = fill.repeat(2), r3 = 'x'.repeat((1 << 24) + 3), r4 = '\\u4e2d\\u00e9'.repeat(40000); [r1 === fill + fill, r2 === r1, r3.length, /^x+$/.test(r3), r4.length, r4.slice(79998) === '\\u4e2d\\u00e9', 'abc'.repeat(21846).slice(-4)].join()",
                  "true,true,16777219,true,80000,true,cabc"), "the repeats");
    CHECK(eval_is(ctx, "var pa = 'x'.padStart(100003, 'abc'); [pa.length, pa.slice(0, 7), pa.slice(-7), 'x'.padEnd(10, 'abc')].join()",
                  "100003,abcabca,abcabcx,xabcabcabc"), "the pad");
    /* replace and replaceAll search a string of up to 256 characters with a loop of their own and a longer one with the search of indexOf: both give what indexOf and split give */
    CHECK(eval_is(ctx, "var bad = 0; for (var L of [3, 100, 255, 256, 257, 258, 300, 1000]) { var s = 'xaby'.repeat(300).slice(0, L); "
                  "for (var p of ['a', 'ab', 'y', 'xab', 'zz', 'aby', s.slice(-3), s.slice(1), s]) { var i = s.indexOf(p); "
                  "var r1 = i < 0 ? s : s.slice(0, i) + 'R' + s.slice(i + p.length); if (s.replace(p, 'R') !== r1) bad++; "
                  "if (s.replaceAll(p, 'R') !== s.split(p).join('R')) bad++; "
                  "if (s.replace(p, (m, at) => m.length + ':' + at) !== (i < 0 ? s : s.slice(0, i) + p.length + ':' + i + s.slice(i + p.length))) bad++ } } bad",
                  "0"), "replace of short and long strings");
    CHECK(eval_is(ctx, "var f2 = 'ab'.repeat(1 << 16) + 'c', p2 = 'x'.padStart((1 << 17) + 5, f2); [p2.length, p2.slice(0, 3), p2.slice(131069, 131072), p2.slice(131072, 131076)].join()",
                  "131077,aba,bab,caba"), "the pad with a filler longer than a step");
    CHECK(eval_is(ctx, "var p3 = 'xy'.padEnd((1 << 17) + 5, f2); [p3.length, p3.slice(0, 3), p3.slice(2, 5), p3.slice(131073, 131077), p3.slice(-4)].join()",
                  "131077,xya,aba,bcab,bcab"), "padEnd with a filler longer than a step");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The loops of padStart, padEnd and the long piece of a string read the
   flag whenever the work since the last reading reaches the step (65536
   characters written by a pad, 1048576 characters scanned or copied by
   js_sub_string_long, the two phases counted together), also where the
   filler ends or the scan turns into the copy. A call that has done T units
   of work has read the flag at least T / step times, less those of the same
   call on a short input (the reads of the call and the evaluation). */
static void test_string_loop_gaps(void)
{
    static const char setup[] =
        "var f1 = 'ab'.repeat(65535) + 'c'\n"
        "var wide = ['\\u4e00', '\\u00e9'.repeat(2 << 20)].join(''); 0\n";
    static const struct {
        const char *call, *base;
        long reads;
    } cases[] = {
        /* 399999 characters written in steps of 65536: a filler of 131071 ends in the middle of a step */
        { "'x'.padEnd(400000, f1)", "'x'.padEnd(2, f1)", 6 },
        { "'x'.padStart(400000, f1)", "'x'.padStart(2, f1)", 6 },
        /* 2097152 characters scanned, then as many copied */
        { "wide.slice(1)", "wide.slice(1, 3)", 4 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        long before, base, reads;

        /* the first call of a method makes it, which may read the flag: the second is the one measured */
        CHECK(eval(ctx, cases[i].base), "%s: the script threw", cases[i].base);
        before = host_poll_reads;
        CHECK(eval(ctx, cases[i].base), "%s: the script threw", cases[i].base);
        base = host_poll_reads - before;
        before = host_poll_reads;
        CHECK(eval(ctx, cases[i].call), "%s: the script threw", cases[i].call);
        reads = host_poll_reads - before - base;
        CHECK(reads >= cases[i].reads, "%s: %ld readings of the flag, want at least %ld", cases[i].call, reads,
              cases[i].reads);
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A property key that is a string of millions of characters is hashed, and
   compared with the atom of an equal string if there is one, and a JSON text
   with such a key or string value is scanned in runs. Each of them reads the
   interrupt flag every 65536 characters or bytes. Each call below reads it
   at least as often as it says, and is stopped by a flag set at the 20th
   reading (or at the reading after the hash, for the comparison with an
   atom): it throws "interrupted", the string is a key as before, and the
   context goes on. Each string is used once, as the first use makes it an
   atom. */
static void test_long_key_polls(void)
{
    static const char setup[] =
        "var o = {}, f1 = 'x'.repeat(1 << 23), f2 = 'x'.repeat(1 << 23), f3 = 'x'.repeat(1 << 23)\n"
        "var g1 = 'y'.repeat(1 << 23), g2 = 'y'.repeat(1 << 23), g3 = 'y'.repeat(1 << 23)\n"
        "var j1 = '{\"' + 'z'.repeat(1 << 23) + '\":1}', j2 = '{\"a\":\"' + 'z'.repeat(1 << 23) + '\"}'\n"
        "var j3 = '{\"a\":\"' + '\\u4e2d'.repeat(1 << 22) + '\"}'; 0\n";
    static const struct {
        const char *call;
        long reads;
    } cases[] = {
        { "o[f1] = 1", 100 },
        { "f2 in o", 100 },
        { "o[f3]", 100 },
        { "JSON.parse(j1)", 200 },
        { "JSON.parse(j2)", 100 },
        { "JSON.parse(j3)", 150 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;
    long before, hash, all;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        long reads;

        /* the reads of the call: the string is new, and the stop comes in the middle of them */
        before = host_poll_reads;
        host_poll_stop_after(1 << 30);
        CHECK(eval(ctx, cases[i].call), "%s: the script threw", cases[i].call);
        reads = host_poll_reads - before;
        host_poll_stop_after(0);
        CHECK(reads >= cases[i].reads, "%s: %ld readings of the flag, want at least %ld", cases[i].call, reads, cases[i].reads);
    }
    /* the same calls on strings that are new, stopped at the 20th reading */
    CHECK(eval(ctx, "var h1 = 'x'.repeat(1 << 23), h2 = 'x'.repeat(1 << 23), h3 = 'x'.repeat(1 << 23)\n"
                    "var k1 = '{\"' + 'w'.repeat(1 << 23) + '\":1}', k2 = '{\"a\":\"' + 'w'.repeat(1 << 23) + '\"}'\n"
                    "var k3 = '{\"a\":\"' + '\\u4e2d'.repeat(1 << 22) + '\"}'; 0"), "the script threw");
    {
        static const char *const stopped[] = { "o[h1] = 1", "h2 in o", "o[h3]", "JSON.parse(k1)", "JSON.parse(k2)", "JSON.parse(k3)" };

        for(i = 0; i < sizeof(stopped) / sizeof(stopped[0]); i++) {
            host_poll_stop_after(20);
            CHECK(interrupted(ctx, stopped[i]), "%s was not stopped in the middle", stopped[i]);
            host_poll_stop_after(0);
            CHECK(eval_is(ctx, "[JSON.parse('{\"a\":1}').a, ({ b: 2 }).b, Object.keys(o).length].join()", "1,2,1"),
                  "the context after %s", stopped[i]);
        }
    }
    /* the atom of a string equal to the key is compared with it: stop after the hash */
    CHECK(eval(ctx, "o[g1] = 1; 0"), "the script threw");
    before = host_poll_reads;
    CHECK(eval(ctx, "o[g2] = 2"), "the script threw");
    all = host_poll_reads - before;
    CHECK(eval(ctx, "var hh = 'y'.repeat(1 << 23); 0"), "the script threw");
    before = host_poll_reads;
    CHECK(eval(ctx, "new Set().has(hh)"), "the script threw");
    hash = host_poll_reads - before;
    CHECK(all - hash >= 100, "the comparison with an atom: %ld readings of the flag, want at least 100", all - hash);
    CHECK(eval(ctx, "var g4 = 'y'.repeat(1 << 23); 0"), "the script threw");
    host_poll_stop_after(hash + (all - hash) / 2);
    CHECK(interrupted(ctx, "o[g4] = 3"), "the comparison with an atom was not stopped in the middle");
    host_poll_stop_after(0);
    CHECK(eval_is(ctx, "[o[g1], o[g3], o[g4]].join()", "2,2,2"), "the key after the stop");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* startTag and endTag read the interrupt flag in each loop over a long
   string (test_tag_work_gaps of unit-white.c checks the steps). Each call
   below, made from C with the arguments A so that every reading counted is
   one of the call, reads the flag at least as often as it says, and a flag
   set at its first reading, at the one in the middle and at its last stops
   it: it fails with "interrupted", uncatchable, target holds what it held,
   and the same call then gives what React's functions give. A name or an end
   tag the functions keep is new at each call (prep), so that every call
   makes it. The host checks for a leak when the runtime is freed. */
static JSValue tag_call(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue f = JS_GetPropertyStr(ctx, g, "F"), a = JS_GetPropertyStr(ctx, g, "A");
    JSValue argv[6], len = JS_GetPropertyStr(ctx, a, "length"), r;
    int32_t n = 0;
    int k;

    JS_ToInt32(ctx, &n, len);
    JS_FreeValue(ctx, len);
    for (k = 0; k < n && k < 6; k++)
        argv[k] = JS_GetPropertyUint32(ctx, a, k);
    r = JS_Call(ctx, f, JS_UNDEFINED, k, argv);
    while (k > 0)
        JS_FreeValue(ctx, argv[--k]);
    JS_FreeValue(ctx, a);
    JS_FreeValue(ctx, f);
    JS_FreeValue(ctx, g);
    return r;
}

/* R is what the call r gave: what startTag pushed onto T, or the end tag */
static int tag_result(JSContext *ctx, JSValue r)
{
    JSValue g = JS_GetGlobalObject(ctx);
    int ok = !JS_IsException(r);

    if (ok) {
        JS_SetPropertyStr(ctx, g, "V", r);
        ok = eval(ctx, "R = F === startTag ? (T.length === 2 ? T.pop() : 'declined') : V; 0");
    }
    JS_FreeValue(ctx, g);
    return ok;
}

static void test_tag_polls(void)
{
    static const char setup[] =
        "var hop = Object.prototype.hasOwnProperty, d = {}, fc = { viewTransition: null }, T = ['x'], n = 0, F, A, P, E, V, W, R\n"
        "var tg = 'a'.repeat(1 << 22), kids = '<'.repeat(1 << 20), wide = '\\u0100&'.repeat(1 << 19), q = '\"'.repeat(1 << 20)\n"
        "var sv = [' '.repeat(1 << 20), '<'.repeat(1 << 20)].join(''), ul = [' '.repeat(1 << 20), 'x'].join('')\n"
        "var o = { toString() { return tg } }\n"
        "function key(p) { return Object.keys(p.style)[0] }\n"
        "function start(p, type) { F = startTag; A = [T, p, type, fc, hop, d] }\n"
        "0";
    static const struct {
        const char *prep, *want;
        long reads;
    } cases[] = {
        /* the scan of the tag, its copy and the copy of the start tag: 8, 8 and 9 steps */
        { "start({}, tg)", "'<' + tg + '>'", 25 },
        /* the escape: 256 steps of 4096 */
        { "start({ children: kids }, 'div')", "'<div>' + kids.replaceAll('<', '&lt;')", 256 },
        { "start({ children: wide }, 'div')", "'<div>' + wide.replaceAll('&', '&amp;')", 256 },
        { "start({ title: q }, 'div')", "'<div title=\"' + q.replaceAll('\"', '&quot;') + '\">'", 256 },
        /* the white space, then the escape */
        { "start({ style: { color: sv } }, 'div')", "'<div style=\"color:' + sv.trim().replaceAll('<', '&lt;') + '\">'", 512 },
        { "start({ href: ul }, 'a')", "'<a href=\"' + ul + '\">'", 512 },
        /* a new style name each time: the escape, or the scan and the hyphens */
        { "P = { style: { ['--' + '&'.repeat(1 << 20) + n++ % 10]: 1 } }; start(P, 'div')",
          "'<div style=\"' + key(P).replaceAll('&', '&amp;') + ':1\">'", 256 },
        { "P = { style: { ['aB'.repeat(1 << 21) + 'x' + n++ % 10]: 1 } }; start(P, 'div')",
          "'<div style=\"' + key(P).replace(/([A-Z])/g, '-$1').toLowerCase() + ':1px\">'", 1024 + 8 },
        /* a new end tag each time, and one that is not kept */
        { "E = ['b'.repeat(1 << 22), n++ % 10].join(''); F = endTag; A = [E]", "'</' + E + '>'", 8 },
        { "F = endTag; A = [o]", "'</' + tg + '>'", 8 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue g = JS_GetGlobalObject(ctx), r, e;
    size_t i, j;

    JS_SetPropertyStr(ctx, g, "startTag", JS_NewStartTagFunction(ctx, "startTag", 8));
    JS_SetPropertyStr(ctx, g, "endTag", JS_NewEndTagFunction(ctx, "endTag", 6));
    JS_FreeValue(ctx, g);
    CHECK(eval(ctx, setup), "the setup threw");
    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        char want[512];
        long before, reads, at[3];

        snprintf(want, sizeof(want), "W = %s; 0", cases[i].want);
        CHECK(eval(ctx, cases[i].prep) && eval(ctx, want), "%s: the setup threw", cases[i].prep);
        before = host_poll_reads;
        r = tag_call(ctx);
        reads = host_poll_reads - before;
        CHECK(tag_result(ctx, r) && eval_is(ctx, "R === W", "true"), "%s does not give %s", cases[i].prep, cases[i].want);
        CHECK(reads >= cases[i].reads, "%s: %ld readings of the flag, want at least %ld", cases[i].prep, reads, cases[i].reads);
        at[0] = 1;
        at[1] = reads / 2;
        at[2] = reads;
        for (j = 0; j < 3; j++) {
            const char *m;
            int stopped;

            CHECK(eval(ctx, cases[i].prep) && eval(ctx, want), "%s: the setup threw", cases[i].prep);
            host_poll_stop_after(at[j]);
            r = tag_call(ctx);
            host_poll_stop_after(0);
            *JS_GetInterruptFlag(rt) = 0;
            stopped = JS_IsException(r) && JS_IsUncatchableException(ctx);
            e = JS_GetException(ctx);
            m = JS_ToCString(ctx, e);
            CHECK(stopped && m && strstr(m, "interrupted"), "%s was not stopped at reading %ld of %ld (%s)", cases[i].prep, at[j], reads,
                  m ? m : "no exception");
            JS_FreeCString(ctx, m);
            JS_FreeValue(ctx, e);
            if (!JS_IsException(r))
                JS_FreeValue(ctx, r);
            CHECK(eval_is(ctx, "T.length + T[0]", "1x"), "%s stopped at reading %ld: target changed", cases[i].prep, at[j]);
            r = tag_call(ctx);
            CHECK(tag_result(ctx, r) && eval_is(ctx, "R === W", "true"), "%s after a stop at reading %ld", cases[i].prep, at[j]);
        }
    }
    CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after the stops");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A rope of long strings is made flat in steps of JS_ROPE_STEP (65536)
   characters, with the flag read before the first step and after the last
   (test_poll_work_gaps of unit-white.c checks the steps): by JS_ToString,
   called from C so that every reading counted is one of the flatten, and
   when it is appended to a string buffer (String.raw). Each call reads the
   flag at least as often as it says, and a flag set at its first reading,
   at the one in the middle and at its last stops it: it fails with
   "interrupted", uncatchable, the rope holds what it held and is still a
   rope, and the same call then gives the flat string. The host checks for a
   leak when the runtime is freed. */
static JSValue rope_flat_call(JSContext *ctx, int raw)
{
    JSValue g = JS_GetGlobalObject(ctx), r = JS_GetPropertyStr(ctx, g, "R"), f, a[2], ret;

    if (!raw) {
        ret = JS_ToString(ctx, r);
    } else {
        f = JS_GetPropertyStr(ctx, g, "raw");
        a[0] = JS_GetPropertyStr(ctx, g, "rawArg");
        a[1] = r;
        ret = JS_Call(ctx, f, JS_UNDEFINED, 2, a);
        JS_FreeValue(ctx, a[0]);
        JS_FreeValue(ctx, f);
    }
    JS_FreeValue(ctx, r);
    JS_FreeValue(ctx, g);
    return ret;
}

static int rope_is_rope(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx), r = JS_GetPropertyStr(ctx, g, "R");
    int ok = JS_VALUE_GET_TAG(r) == JS_TAG_STRING_ROPE;

    JS_FreeValue(ctx, r);
    JS_FreeValue(ctx, g);
    return ok;
}

static void test_rope_flat_polls(void)
{
    static const char setup[] =
        "var a = 'a'.repeat(1 << 22), b = 'b'.repeat(1 << 22), w = '\\u4e2d'.repeat(1 << 21), R, W, V\n"
        "var raw = String.raw, rawArg = { raw: ['', ''] }\n"
        "0";
    static const struct {
        const char *rope;
        int raw;
        long reads;
    } cases[] = {
        /* 2^23 characters: 128 steps */
        { "a + b", 0, 128 },
        { "a + w + b", 0, 160 },
        { "w + a", 0, 96 },
        { "a + b", 1, 128 },
        { "a + w", 1, 96 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue g, r, e;
    size_t i, j;

    CHECK(eval(ctx, setup), "the setup threw");
    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *how = cases[i].raw ? "String.raw" : "JS_ToString";
        char prep[128];
        long before, reads, at[3];

        snprintf(prep, sizeof(prep), "R = %s; W = [%s].join(''); 0", cases[i].rope, cases[i].rope);
        CHECK(eval(ctx, prep) && rope_is_rope(ctx), "%s: the setup threw, or R is not a rope", cases[i].rope);
        before = host_poll_reads;
        r = rope_flat_call(ctx, cases[i].raw);
        reads = host_poll_reads - before;
        CHECK(!JS_IsException(r), "%s by %s threw", cases[i].rope, how);
        g = JS_GetGlobalObject(ctx);
        JS_SetPropertyStr(ctx, g, "V", r);
        JS_FreeValue(ctx, g);
        CHECK(eval_is(ctx, "V === W", "true"), "%s by %s does not give the flat string", cases[i].rope, how);
        CHECK(reads >= cases[i].reads, "%s by %s: %ld readings of the flag, want at least %ld", cases[i].rope, how, reads,
              cases[i].reads);
        at[0] = 1;
        at[1] = reads / 2;
        at[2] = reads;
        for (j = 0; j < 3; j++) {
            const char *m;
            int stopped;

            CHECK(eval(ctx, prep) && rope_is_rope(ctx), "%s: the setup threw, or R is not a rope", cases[i].rope);
            host_poll_stop_after(at[j]);
            r = rope_flat_call(ctx, cases[i].raw);
            host_poll_stop_after(0);
            *JS_GetInterruptFlag(rt) = 0;
            stopped = JS_IsException(r) && JS_IsUncatchableException(ctx);
            e = JS_GetException(ctx);
            m = JS_ToCString(ctx, e);
            CHECK(stopped && m && strstr(m, "interrupted"), "%s by %s was not stopped at reading %ld of %ld (%s)", cases[i].rope, how,
                  at[j], reads, m ? m : "no exception");
            JS_FreeCString(ctx, m);
            JS_FreeValue(ctx, e);
            if (!JS_IsException(r))
                JS_FreeValue(ctx, r);
            CHECK(rope_is_rope(ctx) && eval_is(ctx, "R.length === W.length && R === W", "true"),
                  "%s by %s stopped at reading %ld: the rope changed", cases[i].rope, how, at[j]);
            r = rope_flat_call(ctx, cases[i].raw);
            CHECK(!JS_IsException(r), "%s by %s after a stop at reading %ld threw", cases[i].rope, how, at[j]);
            g = JS_GetGlobalObject(ctx);
            JS_SetPropertyStr(ctx, g, "V", r);
            JS_FreeValue(ctx, g);
            CHECK(eval_is(ctx, "V === W", "true"), "%s by %s after a stop at reading %ld", cases[i].rope, how, at[j]);
        }
    }
    CHECK(eval_is(ctx, "R = V = W = null; [1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after the stops");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* Compiling a long regexp reads the interrupt flag in every long loop, not
   only at each term of the parse (test_poll_work_gaps of unit-white.c checks
   the steps): the pattern made a C string, the code copied as it grows,
   scanned for an atom with a quantifier and for its registers, moved for an
   alternative, the pattern scanned for a name used before its group, and
   the code copied into its string, and the loops of a single term: the
   digits of a count, of \u{...} and of a group name, the operands of && and
   --, and the strings of \q{...}, parsed, made lower case and turned into
   code, and the classes of many characters or strings: the characters
   merged into the ranges of a class, in order or not, the ranges of two
   classes intersected, the strings sorted by length, the table of the
   strings grown, and the ranges turned into code. The compile, called from
   C so that every reading counted is one of it, reads the flag at least as
   often as it says, and a flag set at its first reading, at the one in the
   middle and at its last stops it: it throws "interrupted", uncatchable, and
   the same compile then gives a regexp that matches. A case marked late is
   also stopped at readings spread over the end of its compile (the last
   one, and 1, 2, 4, 8, ... before it), where the loops after the parse of
   a class read the flag. The host checks for a leak when the runtime is
   freed. */
static JSValue regexp_compile_call(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx), c = JS_GetPropertyStr(ctx, g, "RegExp"), ret;
    JSValue a[2] = { JS_GetPropertyStr(ctx, g, "P"), JS_GetPropertyStr(ctx, g, "F") };

    ret = JS_CallConstructor(ctx, c, 2, (JSValueConst *)a);
    JS_FreeValue(ctx, a[0]);
    JS_FreeValue(ctx, a[1]);
    JS_FreeValue(ctx, c);
    JS_FreeValue(ctx, g);
    return ret;
}

static void test_regexp_compile_polls(void)
{
    static const char setup[] =
        "var P, F, T, V\n"
        "var Z = '0'.repeat(1 << 21), M = []; for (var i = 0; i < 1 << 17; i++) M.push((i + 1e6).toString(36)); M = M.join('|')\n"
        /* 60,000 characters every other code point above 0xFFFF, upward
           and downward, 330,000 upward, 17,000 below it downward, and 16,384
           strings of 5 to 68 characters in an order of their lengths that is
           not sorted */
        "var U = [], D, Q = [], B = [], L = []; for (var c = 0x10000; c < 0x10000 + 120000; c += 2) U.push(String.fromCodePoint(c))\n"
        "D = U.slice().reverse().join(''); U = U.join('')\n"
        "for (var c = 0x10000; c < 0x10000 + 660000; c += 2) Q.push(String.fromCodePoint(c)); Q = Q.join('')\n"
        "for (var c = 0xd000; c > 0x100; c -= 3) B.push(String.fromCharCode(c)); B = B.join('')\n"
        "for (var i = 0; i < 1 << 14; i++) L.push((i + 1e6).toString(36) + 'x'.repeat(i * 7919 % 64)); L = L.join('|'); 0";
    static const struct {
        const char *pattern, *flags, *text;
        long reads;
        int late;
    } cases[] = {
        /* the parse reads the flag at each term (the first number, all that
           was read before), and the other loops at least the second */
        { "'(?:a|b|c)+d'.repeat(200000)", "undefined", "'ad'.repeat(200000)", 1400001 + 100 },
        { "'(?:' + 'a'.repeat(1 << 21) + ')+'", "undefined", "'a'.repeat(1 << 21)", 2097154 + 120 },
        { "'a'.repeat(1 << 21) + '|b'", "undefined", "'b'", 2097155 + 80 },
        { "'\\\\k<n>' + 'a'.repeat(1 << 21) + '(?<n>x)'", "undefined", "'a'.repeat(1 << 21) + 'x'", 2097156 + 110 },
        { "'\\u4e2d'.repeat(1 << 20)", "undefined", "'\\u4e2d'.repeat(1 << 20)", 1048577 + 38 },
        /* a term of millions of characters: a count, an escape and a group
           name of leading zeros, operands of && and --, and \q{...} with
           one long string and with many, also made lower case */
        { "'a{' + Z + '1}'", "undefined", "'a'", 3 + 120 },
        { "'a{' + Z + '1,' + Z + '2}'", "undefined", "'a'", 3 + 250 },
        { "'\\\\u{' + Z + '61}'", "'u'", "'a'", 2 + 120 },
        { "'(?<\\\\u{' + Z + '61}>b)'", "undefined", "'b'", 3 + 120 },
        { "'[a' + '&&a'.repeat(1 << 19) + ']'", "'v'", "'a'", 4 + (1 << 19) },
        { "'[a' + '--b'.repeat(1 << 19) + ']'", "'v'", "'a'", 4 + (1 << 19) },
        { "'[\\\\q{' + 'a'.repeat(1 << 21) + '}]'", "'v'", "'a'.repeat(1 << 21)", 4 + (2 << 21) },
        { "'[\\\\q{' + 'a'.repeat(1 << 21) + '}]'", "'vi'", "'A'.repeat(1 << 21)", 4 + (3 << 21) },
        { "'[\\\\q{' + M + '}]'", "'v'", "(1e6).toString(36)", 1 << 20 },
        { "'[[\\\\q{' + M + '}]&&[\\\\q{' + M + '}]]'", "'v'", "(1e6).toString(36)", 1 << 21 },
        /* classes of many characters and strings: the parse reads the flag
           at each atom */
        { "'[' + U + ']'", "'u'", "String.fromCodePoint(0x10000 + 119998)", 60000, 1 },
        { "'[' + D + ']'", "'u'", "String.fromCodePoint(0x10000 + 2)", 60000, 1 },
        { "'[^' + D + ']'", "'u'", "String.fromCodePoint(0x10000 + 3)", 60000, 1 },
        { "'[[' + U + ']&&[' + D + ']]'", "'v'", "String.fromCodePoint(0x10000 + 4)", 120000, 1 },
        { "'[[^' + Q + ']&&[\\\\u{10000}-\\\\u{10010}]]'", "'v'", "String.fromCodePoint(0x10001)", 330000, 1 },
        { "'[' + B + ']'", "'i'", "String.fromCharCode(0x103)", 17000, 1 },
        { "'[\\\\q{' + L + '}]'", "'v'", "(1e6 + 1).toString(36) + 'x'.repeat(7919 % 64)", 1 << 14, 1 },
        { "'[\\\\q{' + L + '}]'", "'vi'", "(1e6).toString(36).toUpperCase()", 1 << 14, 1 },
        /* an intersection that leaves no string, and another over the empty
           buckets: the flag at each */
        { "'[[\\\\q{' + M + '}]&&[\\\\q{zz}]&&[\\\\q{yy}]]|a'", "'v'", "'a'", 1 << 19, 1 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue g, r, e;
    size_t i, j;

    /* every call compiles, and no lookup in the cache reads the flag */
    JS_SetRegExpCache(rt, 0, 0);
    CHECK(eval(ctx, setup), "the setup threw");
    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        char prep[256];
        long before, reads, at[40], k;
        size_t n_at;

        snprintf(prep, sizeof(prep), "P = String(%s); F = %s; T = %s; 0", cases[i].pattern, cases[i].flags,
                 cases[i].text);
        CHECK(eval(ctx, prep), "%s: the setup threw", cases[i].pattern);
        before = host_poll_reads;
        r = regexp_compile_call(ctx);
        reads = host_poll_reads - before;
        CHECK(!JS_IsException(r), "%s threw", cases[i].pattern);
        g = JS_GetGlobalObject(ctx);
        JS_SetPropertyStr(ctx, g, "V", r);
        JS_FreeValue(ctx, g);
        CHECK(eval_is(ctx, "V.test(T)", "true"), "%s does not match", cases[i].pattern);
        CHECK(reads >= cases[i].reads, "%s: %ld readings of the flag, want at least %ld", cases[i].pattern, reads,
              cases[i].reads);
        at[0] = 1;
        at[1] = reads / 2;
        at[2] = reads;
        n_at = 3;
        for (k = 1; cases[i].late && k < reads / 2; k *= 2)
            at[n_at++] = reads - k;
        for (j = 0; j < n_at; j++) {
            const char *m;
            int stopped;

            host_poll_stop_after(at[j]);
            r = regexp_compile_call(ctx);
            host_poll_stop_after(0);
            *JS_GetInterruptFlag(rt) = 0;
            stopped = JS_IsException(r) && JS_IsUncatchableException(ctx);
            e = JS_GetException(ctx);
            m = JS_ToCString(ctx, e);
            CHECK(stopped && m && strstr(m, "interrupted"), "%s was not stopped at reading %ld of %ld (%s)", cases[i].pattern,
                  at[j], reads, m ? m : "no exception");
            JS_FreeCString(ctx, m);
            JS_FreeValue(ctx, e);
            if (!JS_IsException(r))
                JS_FreeValue(ctx, r);
            r = regexp_compile_call(ctx);
            CHECK(!JS_IsException(r), "%s after a stop at reading %ld threw", cases[i].pattern, at[j]);
            g = JS_GetGlobalObject(ctx);
            JS_SetPropertyStr(ctx, g, "V", r);
            JS_FreeValue(ctx, g);
            CHECK(eval_is(ctx, "V.test(T)", "true"), "%s after a stop at reading %ld does not match", cases[i].pattern, at[j]);
        }
    }
    CHECK(eval_is(ctx, "P = F = T = V = Z = M = U = D = Q = B = L = null; [1, 2, 3].map(x => x * 2).join()", "2,4,6"),
          "the context after the stops");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A lookup in the regexp cache hashes the pattern and compares it with the
   one of an entry in steps that read the interrupt flag: a pattern of
   900,002 characters (a class, whose code is short enough for the cache to
   keep), another string with the same characters as one the cache keeps,
   is found in 20 to 999 readings (a compile would read the flag for each
   character), and a flag set at the first reading, at the one in the
   middle and at the last stops the lookup with "interrupted", uncatchable.
   The lookup then finds it. */
static void test_regexp_cache_polls(void)
{
    static const char setup[] =
        "var P = String('[' + 'a'.repeat(900000) + ']'), Q = String('[' + 'a'.repeat(900000) + ']'), F, V = new RegExp(P);\n"
        "P = Q; 0";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue r, e;
    long before, reads, at[3];
    size_t j;

    CHECK(eval(ctx, setup), "the setup threw");
    before = host_poll_reads;
    r = regexp_compile_call(ctx);
    reads = host_poll_reads - before;
    CHECK(!JS_IsException(r), "the lookup threw");
    JS_FreeValue(ctx, r);
    CHECK(reads >= 20 && reads < 1000, "the lookup read the flag %ld times, want 20 to 999", reads);
    at[0] = 1;
    at[1] = reads / 2;
    at[2] = reads;
    for (j = 0; j < 3; j++) {
        const char *m;
        int stopped;

        host_poll_stop_after(at[j]);
        r = regexp_compile_call(ctx);
        host_poll_stop_after(0);
        *JS_GetInterruptFlag(rt) = 0;
        stopped = JS_IsException(r) && JS_IsUncatchableException(ctx);
        e = JS_GetException(ctx);
        m = JS_ToCString(ctx, e);
        CHECK(stopped && m && strstr(m, "interrupted"), "the lookup was not stopped at reading %ld of %ld (%s)", at[j], reads,
              m ? m : "no exception");
        JS_FreeCString(ctx, m);
        JS_FreeValue(ctx, e);
        if (!JS_IsException(r))
            JS_FreeValue(ctx, r);
        before = host_poll_reads;
        r = regexp_compile_call(ctx);
        CHECK(!JS_IsException(r) && host_poll_reads - before == reads, "the lookup after a stop at reading %ld", at[j]);
        JS_FreeValue(ctx, r);
    }
    CHECK(eval_is(ctx, "P = Q = V = null; [1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after the stops");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* More loops that took a whole long string in one pass, and now read the
   interrupt flag at least every 65536 characters or 1048576 bytes: a search
   for a string of the length of the one searched, Array.prototype.indexOf,
   includes and lastIndexOf and Object.is on equal strings, JSON.parse of a
   string of escapes, of short runs and escapes, with a wide character after
   a long ASCII run, and of 3 MiB, JSON.stringify, and a Map or an object
   asked for a key that has 64 keys of its hash and length (a chain of
   compares that differ late). Each call reads the flag at least as often as
   it says, and is stopped by a flag set at the 20th reading: it throws
   "interrupted", and the context goes on. */
static void test_long_ops_polls(void)
{
    static const char setup[] =
        "var e1 = 'x'.repeat(1 << 21), e2 = 'x'.repeat(1 << 21), x1 = '\\u4e2d'.repeat(1 << 21), x2 = '\\u4e2d'.repeat(1 << 21)\n"
        "var jesc = '\"' + '\\\\n'.repeat(1 << 20) + '\"'\n"
        "var jmix = '\"' + ('a'.repeat(30000) + '\\\\n').repeat(70) + '\"'\n"
        "var jwid = ['\"', 'a'.repeat(1 << 21), '\\u4e2d\"'].join('')\n"
        "var jbig = '\"' + 'a'.repeat(3 << 20) + '\"'\n"
        "var qesc = '\\n'.repeat(1 << 20), qbig = 'a'.repeat(3 << 20)\n"
        "var A = '\\u0100\\u0207', B = '\\u0101\\u0100', P = 'x'.repeat(65532)\n"
        "function tail(i) { var r = []; for (var j = 0; j < 6; j++) r.push((i >> j & 1) ? A : B); return r.join('') }\n"
        "function mkq() { return [P, A, tail(0)].join('') }\n"
        "var cm = new Map(), co = {}; for (var i = 0; i < 64; i++) { var k = [P, B, tail(i)].join(''); cm.set(k, i); co[k] = i } 0\n";
    static const struct {
        const char *call;
        long reads;
    } cases[] = {
        { "e1.indexOf(e2)", 30 },
        { "e1.includes(e2)", 30 },
        { "e1.startsWith(e2)", 30 },
        { "x1.endsWith(x2)", 30 },
        { "e1.replace(e2, 'y')", 30 },
        { "[1, e1].indexOf(e2)", 30 },
        { "[1, e1].includes(e2)", 30 },
        { "[e1, 1].lastIndexOf(e2)", 30 },
        { "Object.is(e1, e2)", 30 },
        { "JSON.parse(jesc)", 50 },
        { "JSON.parse(jmix)", 30 },
        { "JSON.parse(jwid)", 60 },
        { "JSON.parse(jbig)", 90 },
        { "JSON.stringify(qesc)", 20 },
        { "JSON.stringify(qbig)", 45 },
        { "cm.get(mkq())", 64 },
        { "cm.delete(mkq())", 64 },
        { "co[mkq()] = 1", 64 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *call = cases[i].call;
        long before, reads;

        before = host_poll_reads;
        CHECK(eval(ctx, call), "%s: the script threw", call);
        reads = host_poll_reads - before;
        CHECK(reads >= cases[i].reads, "%s: %ld readings of the flag, want at least %ld", call, reads, cases[i].reads);
        host_poll_stop_after(20);
        CHECK(interrupted(ctx, call), "%s was not stopped in the middle", call);
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after %s", call);
    }
    /* what they make is what they made */
    CHECK(eval_is(ctx, "[JSON.parse(jesc).length, JSON.parse(jmix).length, JSON.parse(jwid).length, JSON.parse(jwid).slice(-2), JSON.parse(jbig).length].join()",
                  "1048576,2100070,2097153,a\xe4\xb8\xad,3145728"), "the strings JSON.parse made");
    CHECK(eval_is(ctx, "[JSON.stringify(qesc).length, JSON.stringify(qbig).length, e1.indexOf(e2), [1, e1].indexOf(e2), Object.is(e1, e2), cm.get(mkq()), cm.has(cm.keys().next().value)].join()",
                  "2097154,3145730,0,1,true,,true"), "the results");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The loops of BigInt whose work grows as the square of the length (a
   product, a division, a power, a conversion to or from a string in a radix
   that is not a power of two) and the loops over each digit of a string in
   a radix that is a power of two read the interrupt flag every 32768 limbs
   of 32 bits or digits (JS_BIGINT_MAX_SIZE). Each call reads the flag at least as often as it says, and a
   flag set in the middle stops it with the error "interrupted": a
   comparison of a BigInt with a string too, which converts the string. The
   runtime is freed with no block left, so a call stopped in the middle frees
   what it made, and so does a power whose product is too large. */
static void test_bigint_polls(void)
{
    static const char setup[] =
        "var x = 10n ** 100000n, nx = -x, xs = '1' + '0'.repeat(100000)\n"
        "var a = 2n ** 170000n - 1n, b = 2n ** 169999n + 12345n\n"
        "var w = 2n ** 340000n - 1n, h = 2n ** 170000n + 12345n\n"
        "var s = '9'.repeat(100000), sb = '0b' + '1'.repeat(1 << 19); 0\n";
    static const struct {
        const char *call;
        long reads;
    } cases[] = {
        { "String(x)", 500 },
        { "String(nx)", 500 },
        { "x.toString(7)", 500 },
        { "x.toString(2)", 8 },
        { "x.toString(16)", 2 },
        { "a * b", 300 },
        { "(-a) * b", 300 },
        { "w / h", 300 },
        { "w % h", 300 },
        { "10n ** 100000n", 300 },
        { "BigInt(s)", 500 },
        { "BigInt(sb)", 25 },
        { "x == xs", 500 },
        { "x < xs", 500 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *call = cases[i].call;
        long before = host_poll_reads, reads;

        CHECK(eval(ctx, call), "%s: the script threw", call);
        reads = host_poll_reads - before;
        CHECK(reads >= cases[i].reads, "%s: %ld readings of the flag, want at least %ld", call, reads, cases[i].reads);
        host_poll_stop_after(reads / 2 + 1);
        CHECK(interrupted(ctx, call), "%s was not stopped in the middle", call);
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after %s", call);
    }
    /* what they make is what they made */
    CHECK(eval_is(ctx, "[String(x) === xs, String(nx) === '-' + xs, x.toString(2).length, BigInt('0b' + x.toString(2)) === x, "
                  "BigInt('0x' + x.toString(16)) === x, x.toString(7).length, x.toString(7).slice(0, 12), x.toString(7).slice(-12)].join()",
                  "true,true,332193,true,true,118330,232254152365,203562641334"), "the strings");
    CHECK(eval_is(ctx, "var p = a * b; [p / b === a, p % b, ((-a) * b) / b === -a, w / h * h + w % h === w, (-w) % h === -(w % h), "
                  "10n ** 100000n === x, BigInt(s) === x - 1n, BigInt(sb) === 2n ** 524288n - 1n, x == xs, x < xs, x - 1n < xs].join()",
                  "true,0,true,true,true,true,true,true,true,false,true"), "the numbers");
    /* the square of 634,000 bits is too large, and the power frees its last product */
    CHECK(eval_is(ctx, "try { (3n ** 400000n) ** 3n } catch (e) { e instanceof RangeError }", "true"),
          "a power too large");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* Turning a string of a million characters into a number or a BigInt reads
   the interrupt flag every 65536 characters in each loop over them: the
   conversion to UTF-8, the spaces around the number, the scan of its digits
   and their copy in js_atof, the digits js_atod reads, the leading zeros of
   a BigInt, and the digits and spaces of a JSON text, spaces of three bytes
   of UTF-8 and \r\n among them. Each call reads it at least 12 times, and stopped in the middle it throws "interrupted" and
   leaves the context good. The strings are flat (a rope is made flat in
   steps that read the flag on their own). */
static void test_number_polls(void)
{
    static const char setup[] =
        "function flat(...a) { return a.join('') }\n"
        "var n = 1 << 20, ones = '1'.repeat(n), zeros = '0'.repeat(n), sp = flat(' '.repeat(n), '1')\n"
        "var fr = flat('0.', ones), ex = flat('1e', zeros), hx = flat('0x', zeros, '1')\n"
        "var oc = flat('0', '7'.repeat(n)), bx = flat('0x', zeros, '1n')\n"
        "var ws = flat('\\u3000'.repeat(n >> 1), '1'), crlf = flat('\\r\\n'.repeat(n >> 1), '1'); 0\n";
    static const char *const cases[] = {
        "+ones", "Number(zeros)", "Number(sp)", "Number(fr)", "Number(ex)", "Number(hx)",
        "isNaN(ones)", "parseInt(ones)", "parseInt(sp, 16)", "parseFloat(fr)",
        "BigInt(zeros)", "BigInt(sp)", "BigInt(hx)", "1n == zeros", "1n < sp",
        "JSON.parse(ones)", "JSON.parse(fr)", "JSON.parse(sp)",
        "(0, eval)(ones)", "(0, eval)(bx)", "(0, eval)(oc)", "Number(ws)", "JSON.parse(crlf)",
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *call = cases[i];
        long before = host_poll_reads, reads;

        CHECK(eval(ctx, call), "%s: the script threw", call);
        reads = host_poll_reads - before;
        CHECK(reads >= 12, "%s: %ld readings of the flag, want at least 12", call, reads);
        host_poll_stop_after(reads / 2 + 1);
        CHECK(interrupted(ctx, call), "%s was not stopped in the middle", call);
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after %s", call);
    }
    CHECK(eval_is(ctx, "[+ones, Number(zeros), Number(sp), Number(fr), Number(ex), Number(hx), parseInt(sp, 16), "
                  "String(BigInt(zeros)), String(BigInt(sp)), String(BigInt(hx)), 1n == zeros, 1n < sp, "
                  "JSON.parse(sp), (0, eval)(bx) === 1n, (0, eval)(oc) === Infinity, Number(ws), JSON.parse(crlf)].join()",
                  "Infinity,0,1,0.1111111111111111,1,1,1,0,1,1,false,false,1,true,true,1,1"), "the results");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A name or the source of a function of 65536 bytes or more is decoded in
   steps that read the interrupt flag, and gives the string the whole decode
   gives (JS_NewStringLen) for every text: a run of continuation bytes, an
   overlong form, a sequence cut short, a form of five bytes, a surrogate and
   a valid character of four bytes, each placed across a step, and the byte
   that the decode of an error swallows after a run. */
static void test_long_utf8_decode(void)
{
    static const struct {
        const char *name;
        const char *bytes;
        int len;
    } pats[] = {
        { "valid 4", "\xf0\x9f\x98\x80", 4 },
        { "overlong 2", "\xc0\x80", 2 },
        { "overlong 3", "\xe0\x80\x80", 3 },
        { "overlong 4", "\xf0\x80\x80\x80", 4 },
        { "cut 3", "\xe2\x82" "a", 3 },
        { "cut 4", "\xf0\x9f\x98" "a", 4 },
        { "form of 5", "\xf8\x88\x80\x80\x80", 5 },
        { "surrogate", "\xed\xa0\x80", 3 },
        { "lead and 6 continuations", "\xe2\x80\x80\x80\x80\x80\x80", 7 },
        { "continuation and ASCII", "\x80" "a", 2 },
        { "continuations and a lead", "\x80\x80\x80\xe2\x82\xac", 6 },
        { "above 10ffff", "\xf4\x90\x80\x80", 4 },
    };
    static const int runs[] = { 0, 3, 5, 65536, 65541 };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t total = 3 << 16, i, j, r, off;
    char *buf = malloc(total + 64);
    int d, bad = 0, cases = 0;

    for(i = 0; i < sizeof(pats) / sizeof(pats[0]); i++) {
        for(r = 0; r < sizeof(runs) / sizeof(runs[0]); r++) {
            for(d = -8; d <= 8; d++) {
                JSValue whole, part;
                JSAtom atom;
                size_t len;

                /* the pattern, then a run of 0x80 of runs[r] bytes, from
                   d bytes before the end of the first step */
                memset(buf, 'a', total);
                off = (1 << 16) + d - pats[i].len / 2;
                memcpy(buf + off, pats[i].bytes, pats[i].len);
                len = off + pats[i].len;
                for(j = 0; j < (size_t)runs[r] && len < total - 8; j++)
                    buf[len++] = (char)0x80;
                len = total;
                whole = JS_NewStringLen(ctx, buf, len);
                atom = JS_NewAtomLen(ctx, buf, len);
                part = atom == JS_ATOM_NULL ? JS_EXCEPTION : JS_AtomToString(ctx, atom);
                cases++;
                if (JS_IsException(whole) || JS_IsException(part) || !JS_StrictEq(ctx, whole, part)) {
                    if (bad++ < 8)
                        printf("long decode of %s with a run of %d at %d differs from the whole\n",
                               pats[i].name, runs[r], d);
                }
                JS_FreeValue(ctx, whole);
                JS_FreeValue(ctx, part);
                if (atom != JS_ATOM_NULL)
                    JS_FreeAtom(ctx, atom);
            }
        }
    }
    CHECK(bad == 0, "%d of %d long decodes differ from the whole", bad, cases);
    /* the source a function keeps, as Function.prototype.toString decodes
       it, with a run of continuation bytes across a step in a comment */
    {
        static const char head[] = "(function f() {/*", tail[] = "*/})";
        size_t n = 3 << 16, hl = sizeof(head) - 1, tl = sizeof(tail) - 1, len = hl + n + tl;
        char *src = malloc(len + 1);
        JSValue f, s, want;

        memcpy(src, head, hl);
        memset(src + hl, 'a', n);
        memset(src + (1 << 16) - 100, 0x80, 200 + (1 << 16));
        memcpy(src + hl + n, tail, tl);
        src[len] = '\0';
        f = JS_Eval(ctx, src, len, "<unit>", JS_EVAL_TYPE_GLOBAL);
        CHECK(!JS_IsException(f), "the function of a comment of continuation bytes threw");
        s = JS_ToString(ctx, f);
        want = JS_NewStringLen(ctx, src + 1, len - 2);
        CHECK(!JS_IsException(s) && JS_StrictEq(ctx, s, want), "toString of a source with a run of continuation bytes differs from the whole decode");
        JS_FreeValue(ctx, f);
        JS_FreeValue(ctx, s);
        JS_FreeValue(ctx, want);
        free(src);
    }
    /* a run of 4 MiB of continuation bytes is one character, and its decode
       reads the flag every 65536 bytes of it: each of the first 64 readings
       stops it with "interrupted" */
    {
        size_t n = 4 << 20;
        char *run = malloc(n);
        long before, reads, k;
        JSAtom atom;

        memset(run, 0x80, n);
        before = host_poll_reads;
        atom = JS_NewAtomLen(ctx, run, n);
        reads = host_poll_reads - before;
        CHECK(atom != JS_ATOM_NULL, "the name of a run of continuation bytes threw");
        JS_FreeAtom(ctx, atom);
        CHECK(reads >= 64, "a run of continuation bytes: %ld readings of the flag, want at least 64", reads);
        for(k = 1; k <= 64; k++) {
            JSValue e;
            const char *m;

            host_poll_stop_after(k);
            atom = JS_NewAtomLen(ctx, run, n);
            *JS_GetInterruptFlag(rt) = 0;
            e = JS_GetException(ctx);
            m = JS_ToCString(ctx, e);
            CHECK(atom == JS_ATOM_NULL && m && strstr(m, "interrupted"),
                  "a run of continuation bytes was not stopped at reading %ld", k);
            JS_FreeCString(ctx, m);
            JS_FreeValue(ctx, e);
            if (atom != JS_ATOM_NULL)
                JS_FreeAtom(ctx, atom);
        }
        host_poll_stop_after(0);
        free(run);
    }
    free(buf);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* Parsing a script or a JSON text of a million spaces, newlines, comment
   characters or identifier characters reads the interrupt flag every 65536
   bytes of them, and so does the look at the token after the current one
   (simple_next_token). Each call reads it at least 12 times, and stopped in
   the middle it throws "interrupted" and leaves the context good. The calls
   catch the SyntaxError or ReferenceError of the parse that is not stopped. */
static void test_parse_polls(void)
{
    static const char setup[] =
        "function flat(...a) { return a.join('') }\n"
        "function run(f, s) { try { return f(s) } catch (e) { if (!(e instanceof SyntaxError || e instanceof ReferenceError)) throw e } }\n"
        "var n = 1 << 20, ev = s => (0, eval)(s)\n"
        "var sp = flat(' '.repeat(n), '1'), nl = flat('\\r\\n'.repeat(n >> 1), '1'), ws = flat('\\u3000'.repeat(n >> 1), '1')\n"
        "var bc = flat('/*', '\\u00e9'.repeat(n), '*/1'), lc = flat('//', 'a'.repeat(n)), id = 'a'.repeat(n)\n"
        "var as = flat('async', ' '.repeat(n), 'function f() {}'), ac = flat('async /*', 'a'.repeat(n), '*/ function f() {}')\n"
        "var st = flat('\"', 'a\\u00e9'.repeat(n >> 1), '\"'), tp = flat('`', 'a'.repeat(n), '`'), rx = flat('/', 'a'.repeat(n), '/')\n"
        "var pn = flat('class A { #', 'a'.repeat(n), ' }'), fs = flat('(function () { /*', 'a'.repeat(n), '*/ })')\n"
        "var se = flat('a'.repeat(n), ' +'), jw = flat('t', 'r'.repeat(n)), id4 = 'b'.repeat(n << 2); 0\n";
    static const char *const cases[] = {
        "run(ev, sp)", "run(ev, nl)", "run(ev, ws)", "run(ev, bc)", "run(ev, lc)", "run(ev, id)",
        "run(ev, as)", "run(ev, ac)", "run(ev, st)", "run(ev, tp)", "run(ev, rx)", "run(ev, pn)",
        "String(ev(fs))", "run(ev, se)", "run(JSON.parse, jw)",
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    /* Each parse of a source of a million bytes reads the flag at least 12
       times, and is stopped at its readings, every one of them or 64 spread
       over them: the scans, the atom of an identifier, the source a function
       keeps, the lines and columns of the code and of the error. The context
       goes on after each, and the runtime keeps no block of them. */
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *call = cases[i];
        long before = host_poll_reads, reads, j, k, stops;

        CHECK(eval(ctx, call), "%s: the script threw", call);
        reads = host_poll_reads - before;
        CHECK(reads >= 12, "%s: %ld readings of the flag, want at least 12", call, reads);
        stops = reads < 64 ? reads : 64;
        for(j = 0; j < stops; j++) {
            k = 1 + j * reads / stops;
            host_poll_stop_after(k);
            CHECK(interrupted(ctx, call), "%s was not stopped at reading %ld of %ld", call, k, reads);
        }
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after %s", call);
    }
    /* the buffer of an identifier of 4 MiB is copied in steps that read the
       flag each time it grows past a MiB: each of the first 96 readings,
       those of the scan and of the copies among them, stops it */
    {
        long before = host_poll_reads, reads, k;

        CHECK(eval(ctx, "run(ev, id4)"), "run(ev, id4): the script threw");
        reads = host_poll_reads - before;
        CHECK(reads >= 96, "run(ev, id4): %ld readings of the flag, want at least 96", reads);
        for(k = 1; k <= 96; k++) {
            host_poll_stop_after(k);
            CHECK(interrupted(ctx, "run(ev, id4)"), "run(ev, id4) was not stopped at reading %ld", k);
        }
        host_poll_stop_after(0);
    }
    CHECK(eval_is(ctx, "[ev(sp), ev(nl), ev(ws), ev(bc), run(ev, lc), typeof ev(as + '; f'), typeof ev(ac + '; f')].join()",
                  "1,1,1,1,,function,function"), "the results");
    CHECK(eval_is(ctx, "[ev(st) === 'a\\u00e9'.repeat(n >> 1), ev(tp).length, ev(rx).source.length, String(ev(fs)) === fs.slice(1, -1), "
                       "typeof ev(pn + '; A')].join()",
                  "true,1048576,1048576,true,function"), "the results of strings and functions");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* Comparing two strings of a million characters and copying one into a
   string buffer read the interrupt flag every 65536 characters, or every MiB
   of a copy of bytes, and so do the hash of the key of Symbol.for and its
   compare with the key of a symbol there is. Each call reads it at least 12
   times, and stopped at its readings (every one, or 64 spread over them) it
   throws "interrupted" and leaves the context good. A sort is left to
   TestStringCompareCopyStops of the root package: this host sets the flag it
   reads, and a sort reads its own stop too. */
static void test_string_copy_polls(void)
{
    static const char setup[] =
        "var n = 1 << 20, s = 'a'.repeat(n), s2 = 'a'.repeat(n), w = '\\u3000'.repeat(n), w2 = '\\u3000'.repeat(n)\n"
        "var b = 'b'.repeat(n << 4), k = 'k'.repeat(n), ks = Symbol.for(k.slice(1)); 0\n";
    static const char *const cases[] = {
        "s < s2", "w < w2", "Symbol.for(k.slice(1))",
        "[b, 'x'].join('')", "[w, 'x'].join('')", "[b].join()", "b.padEnd((n << 4) + 1)",
        "b.replace('b', 'c')", "w.replace('\\u3000', 'c')", "String.raw({raw: [b, b]}, 1)",
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *call = cases[i];
        long before = host_poll_reads, reads, j, k, stops;

        CHECK(eval(ctx, call), "%s: the script threw", call);
        reads = host_poll_reads - before;
        CHECK(reads >= 12, "%s: %ld readings of the flag, want at least 12", call, reads);
        stops = reads < 64 ? reads : 64;
        for(j = 0; j < stops; j++) {
            k = 1 + j * reads / stops;
            host_poll_stop_after(k);
            CHECK(interrupted(ctx, call), "%s was not stopped at reading %ld of %ld", call, k, reads);
        }
        host_poll_stop_after(0);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"), "the context after %s", call);
    }
    CHECK(eval_is(ctx, "[s < s2, s + 'a' < s2 + 'b', w2 + 'b' < w + 'a', [w + 'b', w + 'a'].sort()[0] === w + 'a', "
                       "Symbol.keyFor(ks) === k.slice(1), Symbol.for(k.slice(1)) === ks, "
                       "[b, 'x'].join('') === b + 'x', [w, 'x'].join('') === w + 'x', b.padEnd((n << 4) + 1, 'z').slice(-2), "
                       "b.replace('b', 'c').slice(0, 2), w.replace('\\u3000', 'c').length, "
                       "String.raw({raw: [b, b]}, 1) === b + '1' + b].join()",
                  "false,true,false,true,true,true,true,true,bz,cb,1048576,true"), "the results");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* The export names of a module are sorted by a compare that reads the
   interrupt flag (exported_names_cmp, with rqsort_stop): two names of a
   million characters that differ in the last one and 3,000 short names in a
   shuffled order come out of a namespace import in the order of the strings.
   The stops are left to the Go test (TestModuleExportNamesStop), as for the
   sort of an array: the native host sets the flag it reads, and the sort
   reads its own stop too. */
static void test_module_export_order(void)
{
    static const char gen[] =
        "var p = 'a'.repeat(1 << 20), names = [p + 'b', p + 'a'];\n"
        "for (var i = 0; i < 3000; i++) names.push('n' + (i * 7919) % 3000);\n"
        "'const x = 1; export {' + names.map(k => 'x as ' + JSON.stringify(k)).join(', ') + '}'\n";
    static const char importer[] =
        "import * as ns from 'long.mjs'\n"
        "var k = Object.keys(ns)\n"
        "globalThis.r = [k.length, k[0].length, k[0].slice(-1), k[1].slice(-1), k[2], k[3001],\n"
        "    k.every((x, i) => i == 0 || k[i - 1] < x), ns[k[1]]].join()\n";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue src, v;
    const char *s;
    size_t len;

    src = JS_Eval(ctx, gen, strlen(gen), "<unit>", JS_EVAL_TYPE_GLOBAL);
    CHECK(!JS_IsException(src), "the generator threw");
    s = JS_ToCStringLen(ctx, &len, src);
    CHECK(s, "the source of the module");
    v = JS_Eval(ctx, s, len, "long.mjs", JS_EVAL_TYPE_MODULE);
    CHECK(!JS_IsException(v), "the exporting module threw");
    JS_FreeValue(ctx, v);
    JS_FreeCString(ctx, s);
    JS_FreeValue(ctx, src);
    v = JS_Eval(ctx, importer, strlen(importer), "main.mjs", JS_EVAL_TYPE_MODULE);
    CHECK(!JS_IsException(v), "the importing module threw");
    JS_FreeValue(ctx, v);
    CHECK(run_jobs(rt), "a job threw");
    CHECK(eval_is(ctx, "r", "3002,1048577,a,b,n0,n999,true,1"), "the order of the export names");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* A block that the heap moves is copied a MiB at a time with a read of the
   interrupt flag before each step (heap_copy), and the bytes an ArrayBuffer
   gains are cleared a MiB at a time the same way (js_array_buffer_clear).
   resize and transfer of a buffer of 32 MiB to a quarter and back, which
   moves the block out of its chunk, are stopped at each of their readings,
   every one or 64 spread over them: the call ends with "interrupted", not
   with the out of memory error of a failed resize, the buffer keeps one of
   its two lengths and its bytes, and it is set back to its full length
   before the next stop. A fixed-length view, a length-tracking view and a
   DataView made before each stop follow the block when the buffer is kept,
   with its lengths and maxByteLength, and read and write its bytes, or have
   nothing when a transfer detached the buffer. The bytes a buffer left short
   gains at the reset are zero. The runtime is freed with no block left. */
static void test_heap_move_polls(void)
{
    static const char setup[] =
        "var n = 1 << 25, b = new ArrayBuffer(n, {maxByteLength: n}); new Uint8Array(b).fill(7); 0\n";
    static const char after[] =
        "[b.byteLength === n || b.byteLength === n >> 2, new Uint8Array(b)[0], "
        "new Uint8Array(b)[(n >> 2) - 1], b.detached].join()";
    static const char views[] =
        "var vb = b, fv = new Uint8Array(b, 0, 16), tv = new Uint8Array(b), dv = new DataView(b); 0";
    static const char views_after[] =
        "String(vb === b ? tv.length === b.byteLength && dv.byteLength === b.byteLength && "
        "fv.length === 16 && fv[0] === 7 && tv[(n >> 2) - 1] === 7 && dv.getUint8(2) === 7 && "
        "(tv[1] = 9, new Uint8Array(b)[1] === 9) && (dv.setUint8(1, 7), fv[1] === 7) && "
        "b.maxByteLength === n : vb.detached && tv.length === 0 && fv.length === 0)";
    static const char reset[] =
        "var was = b.byteLength; b.byteLength === n || (b = b.transfer(n)); "
        "String(was === n || [n >> 2, n >> 1, n - 1].every(i => new Uint8Array(b)[i] === 0))";
    static const char *const cases[] = {
        "(b.resize(n >> 2), b.resize(n))",
        "(b = b.transfer(n >> 2), b = b.transfer(n))",
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *call = cases[i];
        long before = host_poll_reads, reads, j, k, stops;

        CHECK(eval(ctx, call), "%s: the script threw", call);
        reads = host_poll_reads - before;
        /* the copy of 8 MiB and the clear of 24 MiB */
        CHECK(reads >= 32, "%s: %ld readings of the flag, want at least 32", call, reads);
        stops = reads < 64 ? reads : 64;
        for(j = 0; j < stops; j++) {
            k = 1 + j * reads / stops;
            CHECK(eval(ctx, views), "%s: the views before reading %ld", call, k);
            host_poll_stop_after(k);
            CHECK(interrupted(ctx, call), "%s was not stopped at reading %ld of %ld", call, k, reads);
            host_poll_stop_after(0);
            CHECK(eval_is(ctx, after, "true,7,7,false"), "the buffer after %s was stopped at reading %ld", call, k);
            CHECK(eval_is(ctx, views_after, "true"), "the views after %s was stopped at reading %ld", call, k);
            /* the next stop starts from the full length, so that it reads as often */
            CHECK(eval_is(ctx, reset, "true"), "%s: the reset after reading %ld", call, k);
        }
        CHECK(eval(ctx, call), "%s after the stops: the script threw", call);
        CHECK(eval_is(ctx, "b.byteLength === n && new Uint8Array(b)[n - 1] === 0", "true"), "the buffer after %s", call);
    }
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* readings of the flag while js_make_error made its error */
static long error_reads;

/* makeError() throws a TypeError made with the interrupt flag set at the
   first reading, as when a stop is asked for while an error is made. */
static JSValue js_make_error(JSContext *ctx, JSValueConst this_val, int argc,
                             JSValueConst *argv)
{
    long before = host_poll_reads;

    host_poll_stop_after(1);
    JS_ThrowTypeError(ctx, "made");
    error_reads = host_poll_reads - before;
    host_poll_stop_after(0);
    *JS_GetInterruptFlag(JS_GetRuntime(ctx)) = 0;
    return JS_EXCEPTION;
}

/* Making an error reads no interrupt flag (JS_ThrowError2), and the heap,
   which reads it while it moves a block of a MiB or more only for the resize
   of an ArrayBuffer (js_array_buffer_realloc), reads none while an error and
   its backtrace are made. A function named with 4 Mi characters makes a
   stack of more than 4 MiB, which grows its buffer past a MiB, and the error
   it throws is made with no reading of the flag and with its whole stack,
   though the flag would be set at the first. */
static void test_error_reads_no_flag(void)
{
    static const char src[] =
        "var name = 'a'.repeat(1 << 22), r;\n"
        "function f() { return makeError(); }\n"
        "Object.defineProperty(f, 'name', {value: name});\n"
        "try { f() } catch (e) { r = [e.message, e.stack.includes(name)].join() }\n"
        "r\n";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    JSValue g = JS_GetGlobalObject(ctx);

    JS_SetPropertyStr(ctx, g, "makeError",
                      JS_NewCFunction(ctx, js_make_error, "makeError", 0));
    JS_FreeValue(ctx, g);
    CHECK(eval_is(ctx, src, "made,true"), "the error and its stack");
    CHECK(error_reads == 0, "%ld readings of the flag while an error was made, want 0", error_reads);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* Hashing the key of a Map or Set walks every character of a string or rope,
   and reads the interrupt flag each time 65536 of them have been hashed since
   the last reading (a string of a rope counts one more than its length, and
   the strings of a rope share the count). A Map or Set operation with such a
   key (has, get, set, add, delete, getOrInsert, getOrInsertComputed, and the
   methods of Set that look keys up or copy the set) stops with "interrupted"
   when the flag is set in the middle, whether the key is flat or a rope. The
   keys of a table being rebuilt are hashed without reading it.

   Each call is measured and stopped on a collection made again, with a key
   that is not in it, so that every one adds, deletes or calls back as it
   does the first time. The flag is set in the hash that comes after the
   callback of getOrInsertComputed, which has made the value to put in, and
   afterwards the collection holds what it held, an iterator made before
   goes on, and no block is left (a value that the failing call made is
   freed). */
static void test_map_key_polls(void)
{
    static const char setup[] =
        "var r = 'ab'; for (var i = 0; i < 23; i++) r = r + r\n"
        "var f = 'ac'.repeat(1 << 23), o1 = new Set([r]), o2 = new Set(['k'])\n"
        "function mk() { var m = new Map([['one', {v: 1}], ['two', {v: 2}], ['three', {v: 3}]]); m.it = m.keys(); return m }\n"
        "function mks() { var s = new Set(['one', 'two', 'three']); s.it = s.values(); return s }\n"
        "var c, cb = 0; 0\n";
    /* where the flag is set: num/den of the reads of the hashes of the long key, after the reads the call makes with a short one */
    static const struct {
        const char *make, *call, *base;
        int num, den;
    } cases[] = {
        { "mk", "c.set(r, 1)", "c.set('k', 1)", 3, 4 },
        { "mk", "c.get(f)", "c.get('k')", 1, 2 },
        { "mk", "c.has(r)", "c.has('k')", 1, 2 },
        { "mk", "c.delete(f)", "c.delete('k')", 1, 2 },
        { "mk", "c.getOrInsert(r, {v: 7})", "c.getOrInsert('k', {v: 7})", 3, 4 },
        /* the key is hashed three times: to find it, to take it out again after the callback, and to add it. The flag is set in the second and in the third */
        { "mk", "c.getOrInsertComputed(f, () => (cb++, {v: 9}))", "c.getOrInsertComputed('k', () => (cb++, {v: 9}))", 1, 2 },
        { "mk", "c.getOrInsertComputed(f, () => (cb++, {v: 9}))", "c.getOrInsertComputed('k', () => (cb++, {v: 9}))", 5, 6 },
        { "mks", "c.add(r)", "c.add('k')", 3, 4 },
        { "mks", "c.has(f)", "c.has('k')", 1, 2 },
        { "mks", "c.delete(r)", "c.delete('k')", 1, 2 },
        { "mks", "c.union(o1)", "c.union(o2)", 3, 4 },
        { "mks", "c.isSupersetOf(o1)", "c.isSupersetOf(o2)", 1, 2 },
        { "mks", "c.symmetricDifference(o1)", "c.symmetricDifference(o2)", 3, 4 },
        { "mks", "c.intersection(o1)", "c.intersection(o2)", 1, 2 },
        { "mks", "c.difference(o1)", "c.difference(o2)", 1, 2 },
        { "mks", "c.isDisjointFrom(o1)", "c.isDisjointFrom(o2)", 1, 2 },
    };
    /* ropes of long strings, 65534 and 65536 characters and the like: the flag is read at every 65536 of the characters (and leaves) of the whole key */
    static const struct {
        const char *key;
        long reads;
    } ropes[] = {
        { "l1 + l2", 2 },
        { "l3 + l2 + l1", 3 },
        { "l2 + l1 + l2 + l1", 4 },
        { "l1 + l1 + l1 + l1 + l1", 4 },
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i;

    CHECK(eval(ctx, setup), "the script threw");
    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        char make[64];
        const char *call = cases[i].call;
        long before, base, all, hashes;

        snprintf(make, sizeof(make), "c = %s()", cases[i].make);
        /* the first call of a method makes it, which may read the flag: the second is the one measured */
        CHECK(eval(ctx, make) && eval(ctx, cases[i].base), "%s: the script threw", cases[i].base);
        CHECK(eval(ctx, make), "%s: the script threw", make);
        before = host_poll_reads;
        CHECK(eval(ctx, cases[i].base), "%s: the script threw", cases[i].base);
        base = host_poll_reads - before;
        CHECK(eval(ctx, make), "%s: the script threw", make);
        before = host_poll_reads;
        CHECK(eval(ctx, call), "%s: the script threw", call);
        all = host_poll_reads - before;
        hashes = all - base;
        CHECK(hashes >= 255, "%s: %ld readings of the flag in the hashes, want at least 255", call, hashes);
        CHECK(eval(ctx, make) && eval(ctx, "cb = 0"), "%s: the script threw", make);
        host_poll_stop_after(base + hashes * cases[i].num / cases[i].den);
        CHECK(interrupted(ctx, call), "%s was not stopped in the middle", call);
        host_poll_stop_after(0);
        /* the callback of getOrInsertComputed had run when the stop came, so that the value it made was freed with the failed call */
        CHECK(eval_is(ctx, "cb", strstr(call, "getOrInsertComputed") ? "1" : "0"), "%s: the callback ran the wrong number of times", call);
        CHECK(eval_is(ctx, cases[i].make[2] == 's' ?
                      "[c.size, [...c.keys()].join('|'), c.it.next().value].join()" :
                      "[c.size, [...c.keys()].join('|'), c.get('one').v, c.get('two').v, c.get('three').v, c.it.next().value].join()",
                      cases[i].make[2] == 's' ? "3,one|two|three,one" : "3,one|two|three,1,2,3,one"),
              "the collection after %s", call);
    }
    CHECK(eval(ctx, "var l1 = 'x'.repeat(65534), l2 = 'y'.repeat(65536), l3 = 'z'.repeat(65535)"), "the leaves");
    for(i = 0; i < sizeof(ropes) / sizeof(ropes[0]); i++) {
        char call[96];
        long before, base, reads;

        CHECK(eval(ctx, "new Set().has('k')"), "the script threw");
        before = host_poll_reads;
        CHECK(eval(ctx, "new Set().has('k')"), "the script threw");
        base = host_poll_reads - before;
        snprintf(call, sizeof(call), "new Set().has(%s)", ropes[i].key);
        before = host_poll_reads;
        CHECK(eval(ctx, call), "%s: the script threw", call);
        reads = host_poll_reads - before - base;
        CHECK(reads >= ropes[i].reads, "%s: %ld readings of the flag, want at least %ld", call, reads, ropes[i].reads);
    }
    /* what they do is what they did: the same keys, found and not found */
    CHECK(eval_is(ctx, "var q = new Set(), k = ['a', 'b'.repeat(70000), 'a' + 'c'.repeat(100000), 'ab'.repeat(40000) + 'x']; "
                  "for (var x of k) q.add(x); q.add(k[1] + ''); q.add('a' + 'c'.repeat(100000)); "
                  "[q.size, q.has('b'.repeat(70000)), q.has('b'.repeat(70001)), q.has('ab'.repeat(40000) + 'x'), q.delete(k[2]), q.size].join()",
                  "4,true,false,true,true,3"), "the keys");
    CHECK(eval_is(ctx, "var w = new Map(); for (var j = 0; j < 100; j++) w.set('k' + j, j); w.set('big'.repeat(30000), 1); "
                  "[w.size, w.get('k42'), w.get('big'.repeat(30000)), w.get('big'.repeat(30001)), w.getOrInsert('k7', 0), w.getOrInsert('new', 5), w.size].join()",
                  "101,42,1,,7,5,102"), "the map");
    CHECK(eval_is(ctx, "var a1 = new Set(['x'.repeat(70000), 'y', 'z']), b1 = new Set(['x'.repeat(70000), 'w']); "
                  "[[...a1.union(b1)].length, [...a1.intersection(b1)].length, [...a1.difference(b1)].length, [...a1.symmetricDifference(b1)].length, "
                  "a1.isSupersetOf(new Set(['y'])), b1.isSubsetOf(a1), a1.isDisjointFrom(new Set(['q']))].join()",
                  "4,1,2,3,true,false,true"), "the Set methods");
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

/* Outside unicode mode, lre_canonicalize gives the upper case of an ASCII
   letter, every other ASCII unit as it is, and an ASCII unit for no other
   unit, as Canonicalize of the specification does: a character from 128
   whose upper case is below 128 stays as it is. lre_first_set relies on it
   for the characters that ignore case. */
static void test_canonicalize_ascii(void)
{
    uint32_t c, got, want;
    int bad = 0;

    for (c = 0; c <= 0xffff; c++) {
        got = lre_canonicalize(c, 0);
        want = c;
        if (c >= 'a' && c <= 'z')
            want = c - 'a' + 'A';
        if ((c < 128 && got != want) || (c >= 128 && got < 128)) {
            if (bad++ < 10)
                printf("lre_canonicalize(%#x) = %#x\n", c, got);
        }
    }
    CHECK(bad == 0, "%d units are canonicalized wrongly", bad);
}

/* Date.now, new Date() and Date() read the wall clock of the host, in whole
   milliseconds since 1970, cut off after the millisecond. A clock before 1970
   gives negative times. */
static void test_date_clock(void)
{
    static const struct {
        int64_t ns;
        const char *want;
    } cases[] = {
        { INT64_C(1700000000123456789), "1700000000123" },
        { 999999, "0" },
        { 1000000, "1" },
        { 0, "0" },
        { -500000000, "-500" },
        { -1000000, "-1" },
    };
    static const char *const reads[] = {
        "String(Date.now())", "String(new Date().getTime())", "String(+new Date())",
    };
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);
    size_t i, j;

    for (i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        char src[128];

        host_set_wall(1, cases[i].ns);
        for (j = 0; j < sizeof(reads) / sizeof(reads[0]); j++)
            CHECK(eval_is(ctx, reads[j], cases[i].want),
                  "%lld ns: %s", (long long)cases[i].ns, reads[j]);
        snprintf(src, sizeof(src), "String(Date() === new Date(%s).toString())",
                 cases[i].want);
        CHECK(eval_is(ctx, src, "true"), "%lld ns: Date()", (long long)cases[i].ns);
    }
    host_set_wall(0, 0);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
}

int main(void)
{
    /* the deep recursions run on a thread with a stack large enough for them */
    host_run_thread(test_call_depth, NULL);
    host_run_thread(test_first_context_any_state, NULL);
    host_run_thread(test_call_depth_reentry, NULL);
    host_run_thread(test_call_depth_access, NULL);
    host_run_thread(test_internal_depth_access, NULL);
    host_run_thread(test_depth_handover, NULL);
    host_run_thread(test_call_depth_chains, NULL);
    host_run_thread(test_internal_depth, NULL);
    host_run_thread(test_internal_depth_limit, NULL);
    host_run_thread(test_json_nesting_error, NULL);
    host_run_thread(test_internal_depth_autoinit, NULL);
    host_run_thread(test_internal_depth_define, NULL);
    host_run_thread(test_function_nesting, NULL);
    host_run_thread(test_paren_depth, NULL);
    host_run_thread(test_json_records, NULL);
    host_run_thread(test_module_chains, NULL);
    host_run_thread(test_module_retry, NULL);
    test_ropes();
    test_free_nesting();
    test_free_order();
    test_finalizer_memory_usage();
    test_unicode_tables();
    test_class_strings();
    test_closure_lookup_fail();
    test_regexp_cache();
    test_regexp_cache_limits();
    test_shape_cache();
    test_shape_cache_lifetime();
    test_func_shapes();
    test_func_shapes_kept();
    test_class_shapes();
    test_class_shape_set_proto();
    test_object_lit_size();
    test_object_lit_one_block();
    test_atom_keys();
    test_own_keys_collected();
    test_interrupt_points();
    test_search_linear();
    test_search_interrupt();
    test_sort_interrupt();
    test_sort_stop_bound();
    test_date_clock();
    test_canonicalize_ascii();
    test_regexp_stops();
    test_regexp_skip_polls();
    test_rope_polls();
    test_rope_key_compare_polls();
    test_iterator_polls();
    test_string_loop_polls();
    test_string_loop_gaps();
    test_map_key_polls();
    test_long_key_polls();
    test_long_ops_polls();
    test_bigint_polls();
    test_number_polls();
    test_parse_polls();
    test_long_utf8_decode();
    test_string_copy_polls();
    test_module_export_order();
    test_heap_move_polls();
    test_error_reads_no_flag();
    test_tag_polls();
    test_rope_flat_polls();
    test_regexp_compile_polls();
    test_regexp_cache_polls();
    test_async_context_job();
    if (failures) {
        printf("unit: %d checks failed\n", failures);
        return 1;
    }
    printf("unit: all checks passed\n");
    return 0;
}
