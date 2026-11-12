/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Renders tifa SSR pages on the C tree, the way compare/engines/native/
   ssr-driver.js does on `qjs --std`: the preludes, then the bundle, then for
   each input one render whose HTML can be written out, warm renders, and n
   timed renders. A render is startRender(json) followed by the jobs it
   queued. It prints one JSON line with the time of each page. The context
   keeps 2048 shapes, as a VM of the Go API does.

     render bundle=B.js inputs=a.json,b.json [prelude=p1.js,p2.js] [warm=50]
            [n=200] [html=DIR] [tag=T] [front=N] [limit=BYTES]
            [footprint=BYTES] [tifa=1 [murmur=0]] [young=1]

   A timed render ends when the HTML is one flat string of the engine: tifa
   makes its UTF-8 in Go, and here it is made after the clock stops. startRender
   is called on the global object, as tifa calls it. With young=1 the VM is
   kept as the pool of tifa keeps it (Pool below): a full collection and the
   GC threshold of tifa once the bundle has run, and after each render,
   outside the time, a young collection, or a full one when the engine
   collected in the render, when the young one left 1.5 times the live size
   or more, or every 256 renders. The collections the engine starts by its
   threshold then come seldom. Each page gives gc_renders, the timed renders
   in which such a collection ran, and with young=1 young_mean_ns, the time
   of the collection after a timed render, and full_between, the full ones
   among them.

   tifa=1 sets the VM up as tifa sets each VM of its pool: a memory limit of
   128 MiB (ssr.vm_memory, limit= sets another), calls 10000 deep
   (ssr.vm_stack_depth), the engine's __tifa_escape, __tifa_start_tag,
   __tifa_end_tag and __tifa_copy_props, __tifa_async_context__ for the
   AsyncLocalStorage of the bundle, __tifa_random_bytes__ (from a fixed
   sequence here), and __tifa_murmur2__, the hash of emotion that tifa
   computes in Go, here in C (murmur=0 leaves it out, and the bundle hashes
   in JavaScript).

   With soak=N it renders the inputs in turn N times in all, with a young
   collection after each render (and the collections the engine starts by its
   threshold), and every `every` renders (default 100) it runs a full
   collection and prints the memory of the runtime: the bytes in use
   (MemorySize), the blocks, the footprint, the chunks. With release=KEEP it
   then gives memory back as the pool of tifa will, keeping KEEP bytes.

   With RV_STATS set in the environment, a build with -DRV_STATS prints the
   dispatches of the register loop and the calls into it from C and out of
   it, a timed render, and with RV_OPS also set the dispatches of each
   register op. */
#include <inttypes.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "host.h"

/* With -DRENDER_CALLGRIND (and valgrind's headers), callgrind counts the
   timed renders only, run with --instr-atstart=no. */
#ifdef RENDER_CALLGRIND
#include <valgrind/callgrind.h>
#define RV_START(rt) CALLGRIND_START_INSTRUMENTATION
#define RV_STOP(rt) CALLGRIND_STOP_INSTRUMENTATION
#else
#define RV_START(rt) ((void)0)
#define RV_STOP(rt) ((void)0)
#endif

#ifdef RENDER_OLD_API
/* the C tree before the allocator, for the comparison of timings: the stack
   limit of QuickJS, and no time limit */
static JSRuntime *old_runtime(void)
{
    JSRuntime *rt = JS_NewRuntime();

    if (rt)
        JS_SetMaxStackSize(rt, 1024 * 1024);
    return rt;
}
#define NEW_RUNTIME() old_runtime()
#define FREE_RUNTIME(rt) JS_FreeRuntime(rt)
#define END_JOB(rt) ((void)(rt)) /* it kept no target of WeakRef */
#else
#define NEW_RUNTIME() host_new_runtime()
#define FREE_RUNTIME(rt) host_free_runtime(rt)
/* the end of a script or a job, as the Go API ends it */
#define END_JOB(rt) JS_ClearKeptObjects(rt)
#endif

static const char *arg(int argc, char **argv, const char *name, const char *def)
{
    size_t n = strlen(name);

    for (int i = 1; i < argc; i++)
        if (!strncmp(argv[i], name, n) && argv[i][n] == '=')
            return argv[i] + n + 1;
    return def;
}

static double now_ns(void)
{
    struct timespec t;

    clock_gettime(CLOCK_MONOTONIC, &t);
    return t.tv_sec * 1e9 + t.tv_nsec;
}

static char *read_file(const char *path, size_t *len)
{
    FILE *f = fopen(path, "rb");
    char *buf;
    long n;

    if (!f) {
        fprintf(stderr, "render: cannot read %s\n", path);
        exit(2);
    }
    fseek(f, 0, SEEK_END);
    n = ftell(f);
    fseek(f, 0, SEEK_SET);
    buf = malloc(n + 1);
    if (fread(buf, 1, n, f) != (size_t)n) {
        fprintf(stderr, "render: cannot read %s\n", path);
        exit(2);
    }
    buf[n] = 0;
    fclose(f);
    *len = n;
    return buf;
}

static void die(JSContext *ctx, const char *what)
{
    JSValue e = JS_GetException(ctx);
    const char *s = JS_ToCString(ctx, e);

    fprintf(stderr, "render: %s: %s\n", what, s ? s : "?");
    if (s) {
        JSValue st = JS_GetPropertyStr(ctx, e, "stack");
        const char *ss = JS_IsUndefined(st) ? NULL : JS_ToCString(ctx, st);
        if (ss)
            fprintf(stderr, "%s\n", ss);
    }
    exit(1);
}

static JSValue js_print(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    (void)this_val;
    for (int i = 0; i < argc; i++) {
        const char *s = JS_ToCString(ctx, argv[i]);
        if (!s)
            return JS_EXCEPTION;
        fprintf(stderr, "%s%s", i ? " " : "", s);
        JS_FreeCString(ctx, s);
    }
    fputc('\n', stderr);
    return JS_UNDEFINED;
}

static JSValue js_nothing(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    (void)ctx; (void)this_val; (void)argc; (void)argv;
    return JS_UNDEFINED;
}

static JSValue js_perf_now(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    (void)this_val; (void)argc; (void)argv;
    return JS_NewFloat64(ctx, now_ns() / 1e6);
}

/* murmur2 of @emotion/hash 0.9 over the low bytes b of the UTF-16 code units
   of a string, in base 36 into out (at least 8 bytes). The JavaScript
   multiplies in two halves that stay exact doubles, and its bit operators cut
   each product to 32 bits, so each step is a product of uint32_t. */
static void murmur2_low(const uint8_t *b, size_t n, char *out)
{
    const uint32_t m = 0x5bd1e995;
    uint32_t h = 0, k;
    char d[8];
    int i = 0;

    for (; n >= 4; b += 4, n -= 4) {
        k = (uint32_t)b[0] | (uint32_t)b[1] << 8 | (uint32_t)b[2] << 16 | (uint32_t)b[3] << 24;
        k *= m;
        k ^= k >> 24;
        h = k * m ^ h * m;
    }
    switch (n) {
    case 3:
        h ^= (uint32_t)b[2] << 16;
        /* fall through */
    case 2:
        h ^= (uint32_t)b[1] << 8;
        /* fall through */
    case 1:
        h ^= b[0];
        h *= m;
    }
    h ^= h >> 13;
    h *= m;
    h ^= h >> 15;
    do {
        d[i++] = "0123456789abcdefghijklmnopqrstuvwxyz"[h % 36];
        h /= 36;
    } while (h);
    while (i)
        *out++ = d[--i];
    *out = 0;
}

/* tifa's murmur2 builtin (internal/ssr/hash.go of tifa): the hash of a
   string argument, undefined for anything else. The string comes as WTF-8
   (a lone surrogate as the three bytes of its code point), as tifa reads a
   rope, and each code unit gives its low byte. */
static JSValue js_murmur2(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    const uint8_t *s;
    uint8_t *low;
    size_t len, n = 0;
    char out[8];

    (void)this_val;
    if (argc < 1 || !JS_IsString(argv[0]))
        return JS_UNDEFINED;
    s = (const uint8_t *)JS_ToCStringLen(ctx, &len, argv[0]);
    if (!s)
        return JS_EXCEPTION;
    low = malloc(len ? len : 1);
    if (!low) {
        JS_FreeCString(ctx, (const char *)s);
        return JS_ThrowOutOfMemory(ctx);
    }
    for (size_t i = 0; i < len;) {
        uint8_t c = s[i];
        if (c < 0x80) {
            low[n++] = c;
            i++;
        } else if (c < 0xe0) {
            low[n++] = (uint8_t)(c << 6 | (s[i + 1] & 0x3f));
            i += 2;
        } else if (c < 0xf0) {
            low[n++] = (uint8_t)(s[i + 1] << 6 | (s[i + 2] & 0x3f));
            i += 3;
        } else {
            uint32_t r = ((uint32_t)(c & 7) << 18 | (uint32_t)(s[i + 1] & 0x3f) << 12 |
                          (uint32_t)(s[i + 2] & 0x3f) << 6 | (uint32_t)(s[i + 3] & 0x3f)) - 0x10000;
            low[n++] = (uint8_t)(0xd800 + (r >> 10));
            low[n++] = (uint8_t)(0xdc00 + (r & 0x3ff));
            i += 4;
        }
    }
    JS_FreeCString(ctx, (const char *)s);
    murmur2_low(low, n, out);
    free(low);
    return JS_NewString(ctx, out);
}

/* __tifa_murmur2__ as tifa defines it on each VM (murmur2Hook of
   internal/ssr/hash.go): the hash of a string while the page has not replaced
   String.prototype.charCodeAt or Number.prototype.toString, which the
   JavaScript murmur2 calls, and undefined otherwise, when the bundle hashes
   in JavaScript. */
static void add_murmur2(JSContext *ctx)
{
    static const char hook[] =
        "(hash) => {\n"
        "  const S = String.prototype, N = Number.prototype, charCodeAt = S.charCodeAt, toString = N.toString\n"
        "  Object.defineProperty(globalThis, '__tifa_murmur2__', {\n"
        "    value: (s) => ('string' === typeof s && S.charCodeAt === charCodeAt && N.toString === toString ? hash(s) : void 0),\n"
        "  })\n"
        "}";
    JSValue define = JS_Eval(ctx, hook, sizeof hook - 1, "<murmur2>", JS_EVAL_TYPE_GLOBAL), b, r;

    if (JS_IsException(define))
        die(ctx, "murmur2");
    b = JS_NewCFunction(ctx, js_murmur2, "murmur2", 1);
    r = JS_Call(ctx, define, JS_UNDEFINED, 1, &b);
    if (JS_IsException(r))
        die(ctx, "murmur2");
    JS_FreeValue(ctx, r);
    JS_FreeValue(ctx, b);
    JS_FreeValue(ctx, define);
}

/* what qjs --std gives the bundles: print, a console that is not heard,
   performance.now */
static void add_globals(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx), console = JS_NewObject(ctx), perf = JS_NewObject(ctx);
    static const char *const names[] = { "log", "info", "warn", "error", "debug" };

    JS_SetPropertyStr(ctx, g, "print", JS_NewCFunction(ctx, js_print, "print", 1));
    for (size_t i = 0; i < sizeof names / sizeof names[0]; i++)
        JS_SetPropertyStr(ctx, console, names[i], JS_NewCFunction(ctx, js_nothing, names[i], 0));
    JS_SetPropertyStr(ctx, g, "console", console);
    JS_SetPropertyStr(ctx, perf, "now", JS_NewCFunction(ctx, js_perf_now, "now", 0));
    JS_SetPropertyStr(ctx, g, "performance", perf);
    JS_FreeValue(ctx, g);
}

/* a promise that is rejected and nobody handles: say why, as qjs does */
static void js_unhandled(JSContext *ctx, JSValueConst promise, JSValueConst reason, int is_handled, void *opaque)
{
    (void)promise;
    (void)opaque;
    if (!is_handled) {
        const char *s = JS_ToCString(ctx, reason);
        JSValue st = JS_GetPropertyStr(ctx, reason, "stack");
        const char *ss = JS_IsString(st) ? JS_ToCString(ctx, st) : NULL;

        fprintf(stderr, "render: unhandled rejection: %s\n%s\n", s ? s : "?", ss ? ss : "");
    }
}

static void run_script(JSContext *ctx, const char *path)
{
    size_t len;
    char *src = read_file(path, &len);
    JSValue v = JS_Eval(ctx, src, len, path, JS_EVAL_TYPE_GLOBAL);

    if (JS_IsException(v))
        die(ctx, path);
    JS_FreeValue(ctx, v);
    free(src);
    END_JOB(JS_GetRuntime(ctx));
}

static void run_jobs(JSRuntime *rt)
{
    JSContext *c;
    int r;

    for (;;) {
        END_JOB(rt);
        r = JS_ExecutePendingJob(rt, &c);
        if (r == 0)
            break;
        if (r < 0)
            die(c, "a job");
    }
}

static long render_count; /* renders started */

/* A collection is seen by a sentinel: an object of a class of the host that
   holds itself, made before a timed render and dropped, which only a
   collection frees. Its finalizer counts. */
static JSClassID sentinel_class;
static long sentinels_freed;

static void sentinel_finalizer(JSRuntime *rt, JSValue val)
{
    (void)rt; (void)val;
    sentinels_freed++;
}

static void add_sentinel(JSContext *ctx)
{
    JSValue o = JS_NewObjectClass(ctx, (int)sentinel_class);

    if (JS_IsException(o) || JS_SetPropertyStr(ctx, o, "self", JS_DupValue(ctx, o)) < 0)
        die(ctx, "sentinel");
    JS_FreeValue(ctx, o);
}

/* one render: startRender(json), the jobs it queued, and the HTML as one
   string of the engine, a rope made flat, as tifa reads it */
static JSValue render_value(JSContext *ctx, JSValueConst start, const char *json, size_t len)
{
    JSRuntime *rt = JS_GetRuntime(ctx);
    JSValue g = JS_GetGlobalObject(ctx), arg, r, res, flat;

    render_count++;
    JS_SetPropertyStr(ctx, g, "renderResult", JS_UNDEFINED);
    arg = JS_NewStringLen(ctx, json, len);
    r = JS_Call(ctx, start, g, 1, &arg);
    JS_FreeValue(ctx, arg);
    if (JS_IsException(r))
        die(ctx, "startRender");
    JS_FreeValue(ctx, r);
    run_jobs(rt);
    res = JS_GetPropertyStr(ctx, g, "renderResult");
    if (!JS_IsString(res)) {
        fprintf(stderr, "render: no renderResult after the jobs ran (render %ld)\n", render_count);
        exit(1);
    }
    flat = JS_ToString(ctx, res);
    JS_FreeValue(ctx, res);
    if (JS_IsException(flat))
        die(ctx, "renderResult");
    JS_SetPropertyStr(ctx, g, "renderResult", JS_UNDEFINED);
    JS_FreeValue(ctx, g);
    return flat;
}

/* the HTML of render_value as UTF-8, which it frees. tifa makes it in Go,
   so a timed render ends before it. */
static char *html_of(JSContext *ctx, JSValue res, size_t *html_len)
{
    const char *s = JS_ToCStringLen(ctx, html_len, res);
    char *html;

    if (!s)
        die(ctx, "renderResult");
    html = malloc(*html_len + 1);
    memcpy(html, s, *html_len + 1);
    JS_FreeCString(ctx, s);
    JS_FreeValue(ctx, res);
    return html;
}

static char *render(JSContext *ctx, JSValueConst start, const char *json, size_t len, size_t *html_len)
{
    return html_of(ctx, render_value(ctx, start, json, len), html_len);
}

static int cmp_double(const void *a, const void *b)
{
    double x = *(const double *)a, y = *(const double *)b;

    return x < y ? -1 : x > y;
}

typedef struct Inputs {
    char *path[64];
    char *json[64];
    size_t len[64];
    int n;
} Inputs;

static void load_inputs(Inputs *in, const char *list)
{
    char *copy = strdup(list), *p = copy, *tok;

    while ((tok = strsep(&p, ",")) != NULL) {
        if (!*tok)
            continue;
        in->path[in->n] = strdup(tok);
        in->json[in->n] = read_file(tok, &in->len[in->n]);
        in->n++;
    }
    free(copy);
}

static const char *base_name(const char *p)
{
    const char *s = strrchr(p, '/');

    return s ? s + 1 : p;
}

static void report(JSRuntime *rt, long renders, const char *what)
{
    JSMemoryUsage u;

    JS_ComputeMemoryUsage(rt, &u);
#ifdef RENDER_OLD_API
    printf("%s renders=%ld size=%" PRId64 " blocks=%" PRId64 "\n", what, renders, u.malloc_size, u.malloc_count - 1);
#else
    printf("%s renders=%ld size=%" PRId64 " blocks=%" PRId64 " footprint=%" PRId64 " chunks=%" PRId64
           " threshold=%zu\n",
           what, renders, u.malloc_size, u.malloc_count - 1, u.footprint, u.chunk_count, JS_GetGCThreshold(rt));
#endif
    fflush(stdout);
}

#ifndef RENDER_OLD_API
/* A VM as the pool of tifa keeps it (internal/ssr/pool.go of tifa), with
   young=1: after the bundle loads, a full collection, the GC threshold of
   tifa and the memory given back (Load). After each render, the threshold
   set again from the live size (release), then a young collection, or a full
   one when the engine collected in the render, when the young one left 1.5
   times the live size or more, or every 256 renders (runGC). */
typedef struct Pool {
    size_t memory;    /* the memory limit of a VM, ssr.vm_memory */
    size_t live;      /* what the last full collection left */
    size_t need;      /* the threshold renders that collected asked for */
    size_t threshold; /* the threshold the pool set */
    int young;        /* renders since the last full collection */
    long fulls;       /* the full collections between renders */
} Pool;

/* gcThreshold of tifa */
static size_t pool_threshold(Pool *p)
{
    size_t t = 2 * p->live > p->need ? 2 * p->live : p->need, cap = p->live / 2 + p->memory / 2;

    return t < cap ? t : cap;
}

static void pool_set_threshold(JSRuntime *rt, Pool *p)
{
    p->threshold = pool_threshold(p);
    JS_SetGCThreshold(rt, p->threshold);
}

static void pool_full(JSRuntime *rt, Pool *p)
{
    size_t cand, rel;

    JS_RunGC(rt);
    p->young = 0;
    p->live = JS_GetMemorySize(rt);
    pool_set_threshold(rt, p);
    host_release_memory(rt, 0, &cand, &rel);
}

/* Load of tifa, after the bundle ran */
static void pool_loaded(JSRuntime *rt, Pool *p)
{
    pool_full(rt, p);
}

/* release and collect of tifa, after a render */
static void pool_between(JSRuntime *rt, Pool *p)
{
    int engine_gc = JS_GetGCThreshold(rt) != p->threshold;

    if (engine_gc) {
        size_t t = JS_GetGCThreshold(rt), most = 4 * p->live;

        if (t > p->need)
            p->need = t;
        if (p->need > most)
            p->need = most;
    } else if (p->need > 2 * p->live) {
        p->need -= (p->need - 2 * p->live) / 1024;
    }
    pool_set_threshold(rt, p);
    p->young++;
    if (!engine_gc && p->young < 256) {
        JS_RunYoungGC(rt);
        if (JS_GetMemorySize(rt) < p->live + p->live / 2)
            return;
    }
    pool_full(rt, p);
    p->fulls++;
}
#else
typedef struct Pool {
    size_t memory;
    long fulls;
} Pool;
#define pool_loaded(rt, p) JS_RunGC(rt)
#define pool_between(rt, p) JS_RunYoungGC(rt)
#endif

static long random_calls, async_sets; /* for the stderr of a run */

/* __tifa_async_context__ as tifa defines it on each VM (asyncContextHook of
   internal/ssr/asynccontext.go): get and set the async context of the
   runtime, which promise jobs keep, for the AsyncLocalStorage of the bundle */
static JSValue js_get_async_context(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    (void)this_val; (void)argc; (void)argv;
    return JS_GetAsyncContext(ctx);
}

static JSValue js_set_async_context(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    async_sets++;
    (void)this_val;
    JS_SetAsyncContext(ctx, argc ? argv[0] : JS_UNDEFINED);
    return JS_UNDEFINED;
}

static void add_async_context(JSContext *ctx)
{
    static const char hook[] =
        "(get, set) => {\n"
        "  Object.defineProperty(globalThis, '__tifa_async_context__', { value: { get, set }, configurable: true })\n"
        "}";
    JSValue define = JS_Eval(ctx, hook, sizeof hook - 1, "<asynccontext>", JS_EVAL_TYPE_GLOBAL), f[2], r;

    if (JS_IsException(define))
        die(ctx, "async context");
    f[0] = JS_NewCFunction(ctx, js_get_async_context, "getAsyncContext", 0);
    f[1] = JS_NewCFunction(ctx, js_set_async_context, "setAsyncContext", 1);
    r = JS_Call(ctx, define, JS_UNDEFINED, 2, f);
    if (JS_IsException(r))
        die(ctx, "async context");
    JS_FreeValue(ctx, r);
    JS_FreeValue(ctx, f[0]);
    JS_FreeValue(ctx, f[1]);
    JS_FreeValue(ctx, define);
}

/* __tifa_random_bytes__(n) of tifa (internal/ssr/random.go): n bytes, 0 to
   65536, in base64. tifa reads them from crypto/rand, here they come from a
   fixed sequence, so that a run gives the same HTML each time. */
static uint64_t random_state = 0x9e3779b97f4a7c15u;

static JSValue js_random_bytes(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    static const char b64[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    double d;
    size_t n, o = 0;
    uint8_t *b;
    char *s;
    JSValue r;

    (void)this_val;
    if (argc != 1)
        return JS_ThrowTypeError(ctx, "random bytes requires a length");
    if (!JS_IsNumber(argv[0]) || JS_ToFloat64(ctx, &d, argv[0]) < 0 || d < 0 || d > 65536 || d != (double)(long)d)
        return JS_ThrowRangeError(ctx, "random byte length must be an integer from 0 to 65536");
    random_calls++;
    n = (size_t)d;
    b = malloc(n + 3);
    s = malloc((n + 2) / 3 * 4 + 1);
    for (size_t i = 0; i < n; i++) {
        random_state ^= random_state << 13;
        random_state ^= random_state >> 7;
        random_state ^= random_state << 17;
        b[i] = (uint8_t)random_state;
    }
    for (size_t i = 0; i < n; i += 3) {
        uint32_t v = (uint32_t)b[i] << 16 | (i + 1 < n ? (uint32_t)b[i + 1] << 8 : 0) | (i + 2 < n ? b[i + 2] : 0);

        s[o++] = b64[v >> 18 & 63];
        s[o++] = b64[v >> 12 & 63];
        s[o++] = i + 1 < n ? b64[v >> 6 & 63] : '=';
        s[o++] = i + 2 < n ? b64[v & 63] : '=';
    }
    r = JS_NewStringLen(ctx, s, o);
    free(b);
    free(s);
    return r;
}

int main(int argc, char **argv)
{
    const char *bundle = arg(argc, argv, "bundle", NULL), *inputs = arg(argc, argv, "inputs", NULL);
    const char *prelude = arg(argc, argv, "prelude", "");
    const char *html_dir = arg(argc, argv, "html", "");
    const char *tag = arg(argc, argv, "tag", "");
    int warm = atoi(arg(argc, argv, "warm", "50")), n = atoi(arg(argc, argv, "n", "200"));
    long soak = atol(arg(argc, argv, "soak", "0")), every = atol(arg(argc, argv, "every", "100"));
    const char *release = arg(argc, argv, "release", NULL);
    int young = atoi(arg(argc, argv, "young", soak ? "1" : "0"));
    int tifa = atoi(arg(argc, argv, "tifa", "0"));
    Pool pool = { 0 };
    JSRuntime *rt;
    JSContext *ctx;
    JSValue g, start;
    Inputs in = { { 0 }, { 0 }, { 0 }, 0 };
    char *copy, *p, *tok;

    if (!bundle || !inputs) {
        fprintf(stderr, "usage: render bundle=B inputs=A,B [prelude=P,Q] [warm=N] [n=N] [html=DIR] [tag=T] [soak=N every=N release=KEEP young=0|1] [front=N] [limit=B] [footprint=B]\n");
        return 2;
    }
    /* a soak or a timing run takes longer than a test: no time limit unless asked */
    setenv("HOST_TIMEOUT_SEC", "0", 0);
    rt = NEW_RUNTIME();
    if (!rt) {
        fprintf(stderr, "render: no runtime\n");
        return 1;
    }
#ifndef RENDER_OLD_API
    if (arg(argc, argv, "front", NULL))
        JS_SetFrontCache(rt, atoi(arg(argc, argv, "front", "256")));
    if (arg(argc, argv, "footprint", NULL))
        JS_SetFootprintLimit(rt, (size_t)atoll(arg(argc, argv, "footprint", "0")));
#endif
    /* the memory limit, with tifa=1 by default the 128 MiB of ssr.vm_memory,
       which the threshold of young=1 counts with too */
    pool.memory = (size_t)128 << 20;
    if (arg(argc, argv, "limit", NULL))
        pool.memory = (size_t)atoll(arg(argc, argv, "limit", "0"));
    if (arg(argc, argv, "limit", NULL) || tifa)
        JS_SetMemoryLimit(rt, pool.memory);
    ctx = JS_NewContext(rt);
    if (!ctx) {
        fprintf(stderr, "render: no context\n");
        return 1;
    }
#ifndef RENDER_OLD_API
    /* the shapes a context keeps, as NewVM of the Go API keeps them
       (DefaultShapeCache of vm.go) */
    if (JS_SetShapeCache(ctx, 2048) < 0) {
        fprintf(stderr, "render: no shape cache\n");
        return 1;
    }
#endif
    {
        JSClassDef def = { "Sentinel", sentinel_finalizer, NULL, NULL, NULL };
        JS_NewClassID(rt, &sentinel_class);
        if (JS_NewClass(rt, sentinel_class, &def) < 0) {
            fprintf(stderr, "render: no class\n");
            return 1;
        }
    }
    add_globals(ctx);
    if (tifa) {
        /* the engine functions tifa defines on each VM of its pool, the depth
           of its calls (ssr.vm_stack_depth), its random bytes and its async
           context */
        JSValue g = JS_GetGlobalObject(ctx);
#ifndef RENDER_OLD_API
        JS_SetMaxCallDepth(rt, 10000);
#endif
        JS_SetPropertyStr(ctx, g, "__tifa_random_bytes__", JS_NewCFunction(ctx, js_random_bytes, "__tifa_random_bytes__", 1));
        add_async_context(ctx);
        JS_SetPropertyStr(ctx, g, "__tifa_escape", JS_NewEscapeTextFunction(ctx, "__tifa_escape", 13));
        JS_SetPropertyStr(ctx, g, "__tifa_start_tag", JS_NewStartTagFunction(ctx, "__tifa_start_tag", 16));
        JS_SetPropertyStr(ctx, g, "__tifa_end_tag", JS_NewEndTagFunction(ctx, "__tifa_end_tag", 14));
        JS_SetPropertyStr(ctx, g, "__tifa_copy_props", JS_NewCopyPropsFunction(ctx, "__tifa_copy_props", 17));
        JS_FreeValue(ctx, g);
        /* and tifa's Go hash for emotion, unless murmur=0 leaves the hash to
           the JavaScript of the bundle */
        if (atoi(arg(argc, argv, "murmur", "1")))
            add_murmur2(ctx);
    }
    JS_SetHostPromiseRejectionTracker(rt, js_unhandled, NULL);
    copy = strdup(prelude);
    p = copy;
    while ((tok = strsep(&p, ",")) != NULL)
        if (*tok)
            run_script(ctx, tok);
    free(copy);
    {
        double t0 = now_ns();
        run_script(ctx, bundle);
        fprintf(stderr, "render: bundle loaded in %.1f ms\n", (now_ns() - t0) / 1e6);
    }
    load_inputs(&in, inputs);
    g = JS_GetGlobalObject(ctx);
    start = JS_GetPropertyStr(ctx, g, "startRender");
    JS_FreeValue(ctx, g);
    if (!JS_IsFunction(ctx, start)) {
        fprintf(stderr, "render: the bundle has no startRender\n");
        return 1;
    }

    if (soak) {
        JS_RunGC(rt);
        report(rt, 0, "loaded");
        for (long i = 0; i < soak; i++) {
            int k = (int)(i % in.n);
            size_t len;
            char *html = render(ctx, start, in.json[k], in.len[k], &len);
            free(html);
            if (young)
                JS_RunYoungGC(rt);
            if (every && (i + 1) % every == 0) {
                JS_RunGC(rt);
#ifndef RENDER_OLD_API
                if (release) {
                    size_t cand, rel;
                    host_release_memory(rt, (size_t)atoll(release), &cand, &rel);
                    report(rt, i + 1, "released");
                    printf("release candidate=%zu released=%zu\n", cand, rel);
                } else
#endif
                    report(rt, i + 1, "full-gc");
            }
        }
    } else {
        if (young)
            pool_loaded(rt, &pool);
        printf("{\"pages\":[");
        for (int k = 0; k < in.n; k++) {
            size_t len, hl;
            char *html = render(ctx, start, in.json[k], in.len[k], &len);
            long rv_before[4];
            static long rv_ops_before[256];
            double *d = malloc(sizeof(double) * (n > 0 ? n : 1)), sum = 0, young_sum = 0;
            int gc_renders = 0;
            long fulls0;

            hl = len;
            if (*html_dir) {
                char path[1024];
                FILE *f;

                snprintf(path, sizeof path, "%s/%s-%s.html", html_dir, tag, base_name(in.path[k]));
                f = fopen(path, "wb");
                if (f) {
                    fwrite(html, 1, len, f);
                    fclose(f);
                }
            }
            free(html);
            if (young)
                pool_between(rt, &pool);
            for (int i = 0; i < warm; i++) {
                html = render(ctx, start, in.json[k], in.len[k], &len);
                free(html);
                if (young)
                    pool_between(rt, &pool);
            }
            for (int c = 0; c < 4; c++)
                rv_before[c] = JS_RegvmStat(ctx, c ? 12 + c : 10);
            for (int c = 0; c < 256; c++)
                rv_ops_before[c] = JS_RegvmStat(ctx, 1000 + c);
            fulls0 = pool.fulls;
            for (int i = 0; i < n; i++) {
                long freed;
                double t;
                JSValue res;
                add_sentinel(ctx);
                freed = sentinels_freed;
                t = now_ns();
                RV_START(rt);
                res = render_value(ctx, start, in.json[k], in.len[k]);
                RV_STOP(rt);
                d[i] = now_ns() - t;
                sum += d[i];
                if (sentinels_freed != freed)
                    gc_renders++;
                html = html_of(ctx, res, &len);
                if (len != hl) {
                    fprintf(stderr, "render: output changed between renders\n");
                    return 1;
                }
                free(html);
                if (young) {
                    t = now_ns();
                    pool_between(rt, &pool);
                    young_sum += now_ns() - t;
                }
            }
            if (n > 0 && getenv("RV_STATS"))
                /* the counts of a native build with -DRV_STATS, a render */
                fprintf(stderr, "render: %s a render: dispatches %.0f, calls C>rv %.1f rv>bytecode %.1f rv>C %.1f\n",
                        base_name(in.path[k]), (double)(JS_RegvmStat(ctx, 10) - rv_before[0]) / n,
                        (double)(JS_RegvmStat(ctx, 13) - rv_before[1]) / n,
                        (double)(JS_RegvmStat(ctx, 14) - rv_before[2]) / n,
                        (double)(JS_RegvmStat(ctx, 15) - rv_before[3]) / n);
            if (getenv("RV_STATS"))
                /* the most of a segment of register stack in use so far, and
                   the segments made so far */
                fprintf(stderr, "render: %s register stack peak %d bytes, segments made %d\n",
                        base_name(in.path[k]), JS_RegvmStat(ctx, 16), JS_RegvmStat(ctx, 7));
            if (n > 0 && getenv("RV_STATS") && getenv("RV_OPS")) {
                /* and with RV_OPS, the dispatches of each register op, a render */
                for (int c = 0; c < 256 && JS_RegvmOpName(c); c++) {
                    long v = JS_RegvmStat(ctx, 1000 + c) - rv_ops_before[c];
                    if (v)
                        fprintf(stderr, "render: %s op %s %.1f\n", base_name(in.path[k]), JS_RegvmOpName(c), (double)v / n);
                }
            }
            qsort(d, n, sizeof(double), cmp_double);
            printf("%s{\"input\":\"%s\",\"html_bytes\":%zu,\"n\":%d,\"mean_ns\":%.0f,\"median_ns\":%.0f,\"min_ns\":%.0f,\"max_ns\":%.0f,"
                   "\"gc_renders\":%d,\"young\":%d,\"young_mean_ns\":%.0f,\"full_between\":%ld}",
                   k ? "," : "", base_name(in.path[k]), hl, n, n ? sum / n : 0.0, n ? d[n / 2] : 0.0, n ? d[0] : 0.0, n ? d[n - 1] : 0.0,
                   gc_renders, young, n && young ? young_sum / n : 0.0, pool.fulls - fulls0);
            free(d);
        }
        printf("]}\n");
    }
    if (tifa)
        fprintf(stderr, "render: the bundle set the async context %ld times and asked for random bytes %ld times\n", async_sets, random_calls);
    JS_FreeValue(ctx, start);
    JS_FreeContext(ctx);
    FREE_RUNTIME(rt);
    return 0;
}
