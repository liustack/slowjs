/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* unit checks through the public API what the scripts of compare.sh cannot
   see. It exits 0 when every check passes.

     unit */
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "host.h"
#include "cutils.h"
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

/* run_jobs runs the jobs of rt until none is left and reports whether none
   threw. */
static int run_jobs(JSRuntime *rt)
{
    JSContext *ctx;
    int r, ok = 1;

    while ((r = JS_ExecutePendingJob(rt, &ctx)) != 0) {
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
    JS_FreeRuntime(rt);
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
    JS_FreeRuntime(rt);

    /* freed with shapes in the ring, which hold the context */
    rt = host_new_runtime();
    ctx = JS_NewContext(rt);
    CHECK(JS_SetShapeCache(ctx, 2048) == 0, "no ring");
    CHECK(eval(ctx, ten), "the script threw");
    JS_FreeContext(ctx);
    JS_FreeRuntime(rt);
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
    JS_FreeRuntime(rt);
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
        JS_FreeRuntime(rt);
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
    JS_FreeRuntime(rt);
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
    JS_FreeRuntime(rt);
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
    JS_FreeRuntime(rt);
}

static int interrupts;

static int count_interrupts(JSRuntime *rt, void *opaque)
{
    interrupts++;
    return 0;
}

/* A search through a long string counts a step for each 8 indexes toward
   the interrupt counter, so that the interrupt handler runs after each of
   these searches, where each would count as one call without that. */
static void test_search_steps(void)
{
    static const char src[] =
        "var s = 'a'.repeat(1 << 20);\n"
        "for (var i = 0; i < 25; i++) { s.indexOf('b'); s.lastIndexOf('b'); s.includes('b') }\n";
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt);

    interrupts = 0;
    JS_SetInterruptHandler(rt, count_interrupts, NULL);
    CHECK(eval(ctx, src), "the script threw");
    CHECK(interrupts >= 75, "the interrupt handler ran %d times", interrupts);
    JS_FreeContext(ctx);
    JS_FreeRuntime(rt);
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
    JS_FreeRuntime(rt);
}

static int polls, stop_at;

static int stop_interrupts(JSRuntime *rt, void *opaque)
{
    return ++polls == stop_at;
}

/* A long search checks the interrupt counter while it runs and calls the
   interrupt handler when the counter runs out, as the interpreter does: one
   search calls it many times, with a match or none, in strings of each
   width and in one of each, and a handler that asks to stop ends the search
   with the error "interrupted", which try and catch do not catch. Once the
   handler is gone and the error taken, the context runs scripts and
   searches again. Comparing the needle at the one index a search has, as
   startsWith does, is linear in its length and does not call the handler,
   as === does not. */
static void test_search_interrupt(void)
{
    static const char setup[] =
        "var a = 'a'.repeat(1 << 23), m = 'a'.repeat(1000) + 'b'\n"
        "var w = '\\u4e00'.repeat(1 << 22), wm = '\\u4e00'.repeat(1000) + 'b'\n"
        "var aw = a + '\\u4e00', ab = a + 'b', ba = 'b' + a\n"
        "var awm = aw + m, wwm = w + wm, a2 = a.slice(1) + 'a'\n";
    static const struct {
        const char *src, *want;
        int stops;
    } cases[] = {
        { "a.indexOf('b')", "-1", 1 },
        { "a.lastIndexOf('b')", "-1", 1 },
        { "a.indexOf(m)", "-1", 1 },
        { "a.lastIndexOf('b' + m)", "-1", 1 },
        { "w.indexOf(wm)", "-1", 1 },
        { "w.lastIndexOf('b')", "-1", 1 },
        { "aw.indexOf(m)", "-1", 1 },
        { "a.indexOf('aaab')", "-1", 1 },
        { "w.lastIndexOf('\\u4e00\\u4e00b')", "-1", 1 },
        { "ab.indexOf('b')", "8388608", 1 },
        { "ab.indexOf(m)", "8387608", 1 },
        { "ba.lastIndexOf('b')", "0", 1 },
        { "ba.lastIndexOf('baaa')", "0", 1 },
        { "wwm.indexOf(wm)", "4194304", 1 },
        { "awm.indexOf(m)", "8388609", 1 },
        { "awm.indexOf('aab')", "8389607", 1 },
        { "ab.indexOf(a)", "0", 1 },
        { "aw.includes(a)", "true", 1 },
        { "a.startsWith(a2)", "true", 0 },
        { "aw.endsWith(a.slice(1) + '\\u4e00')", "true", 0 },
        { "a.indexOf(a2)", "0", 0 },
        { "a.startsWith(a.slice(1) + 'b')", "false", 0 },
    };
    size_t i;

    for(i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        const char *src = cases[i].src, *want = cases[i].want;
        JSRuntime *rt = host_new_runtime();
        JSContext *ctx = JS_NewContext(rt);
        char caught[256];
        const char *runs[2];
        JSValue v, e;
        const char *s;
        int j;

        CHECK(eval(ctx, setup), "the script threw");
        JS_SetInterruptHandler(rt, stop_interrupts, NULL);
        polls = 0;
        stop_at = -1;
        CHECK(eval_is(ctx, src, want), "%s", src);
        if (!cases[i].stops) {
            CHECK(polls <= 1, "%s called the interrupt handler %d times", src,
                  polls);
            JS_FreeContext(ctx);
            JS_FreeRuntime(rt);
            continue;
        }
        CHECK(polls >= 20, "%s called the interrupt handler %d times", src,
              polls);
        snprintf(caught, sizeof(caught), "try { %s } catch (e) { 'caught' }",
                 src);
        runs[0] = src;
        runs[1] = caught;
        for(j = 0; j < 2; j++) {
            polls = 0;
            stop_at = 3;
            v = JS_Eval(ctx, runs[j], strlen(runs[j]), "<unit>",
                        JS_EVAL_TYPE_GLOBAL);
            CHECK(JS_IsException(v), "%s ran to its end", runs[j]);
            e = JS_GetException(ctx);
            s = JS_ToCString(ctx, e);
            CHECK(s && strstr(s, "interrupted"), "%s threw %s", runs[j],
                  s ? s : "?");
            CHECK(polls == 3, "%s called the handler %d times", runs[j],
                  polls);
            JS_FreeCString(ctx, s);
            JS_FreeValue(ctx, e);
            JS_FreeValue(ctx, v);
        }
        JS_SetInterruptHandler(rt, NULL, NULL);
        CHECK(eval_is(ctx, "[1, 2, 3].map(x => x * 2).join()", "2,4,6"),
              "the context after %s was stopped", src);
        CHECK(eval_is(ctx, src, want), "%s after it was stopped", src);
        JS_FreeContext(ctx);
        JS_FreeRuntime(rt);
    }
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
   in the Go engine: the handler runs with the context of its then, and the
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
    JS_FreeRuntime(rt);
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
        e = JS_GetException(ctx);
        name = JS_ToCString(ctx, e);
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

static uint64_t fnv_word(uint64_t h, uint32_t v)
{
    int i;

    for (i = 0; i < 4; i++) {
        h ^= (v >> (8 * i)) & 0xff;
        h *= 0x100000001b3ULL;
    }
    return h;
}

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
            len = unicode_normalize(&dst, &c, 1, t, NULL, unicode_realloc);
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

int main(void)
{
    /* the deep recursions run on a thread with a stack large enough for them */
    host_run_thread(test_call_depth, NULL);
    host_run_thread(test_call_depth_reentry, NULL);
    host_run_thread(test_call_depth_access, NULL);
    host_run_thread(test_internal_depth_access, NULL);
    host_run_thread(test_depth_handover, NULL);
    host_run_thread(test_call_depth_chains, NULL);
    host_run_thread(test_internal_depth, NULL);
    host_run_thread(test_function_nesting, NULL);
    host_run_thread(test_paren_depth, NULL);
    host_run_thread(test_json_records, NULL);
    host_run_thread(test_module_chains, NULL);
    host_run_thread(test_module_retry, NULL);
    test_ropes();
    test_free_nesting();
    test_unicode_tables();
    test_class_strings();
    test_closure_lookup_fail();
    test_regexp_cache();
    test_regexp_cache_limits();
    test_shape_cache();
    test_shape_cache_lifetime();
    test_func_shapes();
    test_func_shapes_kept();
    test_atom_keys();
    test_own_keys_collected();
    test_search_steps();
    test_search_linear();
    test_search_interrupt();
    test_async_context_job();
    if (failures) {
        printf("unit: %d checks failed\n", failures);
        return 1;
    }
    printf("unit: all checks passed\n");
    return 0;
}
