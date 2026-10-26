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

int main(void)
{
    test_regexp_cache();
    test_regexp_cache_limits();
    test_shape_cache();
    test_shape_cache_lifetime();
    test_func_shapes();
    test_func_shapes_kept();
    test_atom_keys();
    test_own_keys_collected();
    if (failures) {
        printf("unit: %d checks failed\n", failures);
        return 1;
    }
    printf("unit: all checks passed\n");
    return 0;
}
