/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Renders tifa SSR pages on the C tree, the way compare/engines/native/
   ssr-driver.js does on `qjs --std`: the preludes, then the bundle, then for
   each input one render whose HTML can be written out, warm renders, and n
   timed renders. A render is startRender(json) followed by the jobs it
   queued. It prints one JSON line with the time of each page.

     render bundle=B.js inputs=a.json,b.json [prelude=p1.js,p2.js] [warm=50]
            [n=200] [html=DIR] [tag=T] [front=N] [limit=BYTES]
            [footprint=BYTES]

   With soak=N it renders the inputs in turn N times in all, with a young
   collection after each render (and the collections the engine starts by its
   threshold), and every `every` renders (default 100) it runs a full
   collection and prints the memory of the runtime: the bytes in use
   (MemorySize), the blocks, the footprint, the chunks. With release=KEEP it
   then gives memory back as the pool of tifa will, keeping KEEP bytes. */
#include <inttypes.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "host.h"

#ifdef RENDER_OLD_API
/* the C tree before the allocator, for the comparison of timings: the same
   stack limit as host_new_runtime gives, and no time limit */
static JSRuntime *old_runtime(void)
{
    JSRuntime *rt = JS_NewRuntime();

    if (rt)
        JS_SetMaxStackSize(rt, 1024 * 1024);
    return rt;
}
#define NEW_RUNTIME() old_runtime()
#define FREE_RUNTIME(rt) JS_FreeRuntime(rt)
#else
#define NEW_RUNTIME() host_new_runtime()
#define FREE_RUNTIME(rt) host_free_runtime(rt)
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
        if (s)
            fprintf(stderr, "%s%s", i ? " " : "", s);
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
}

static void run_jobs(JSRuntime *rt)
{
    JSContext *c;
    int r;

    while ((r = JS_ExecutePendingJob(rt, &c)) != 0) {
        if (r < 0)
            die(c, "a job");
    }
}

static long render_count; /* renders started */

/* one render: startRender(json), the jobs it queued, and the HTML */
static char *render(JSContext *ctx, JSValueConst start, const char *json, size_t len, size_t *html_len)
{
    JSRuntime *rt = JS_GetRuntime(ctx);
    JSValue g = JS_GetGlobalObject(ctx), arg, r, res;
    const char *s;
    char *html;

    render_count++;
    JS_SetPropertyStr(ctx, g, "renderResult", JS_UNDEFINED);
    arg = JS_NewStringLen(ctx, json, len);
    r = JS_Call(ctx, start, JS_UNDEFINED, 1, &arg);
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
    s = JS_ToCStringLen(ctx, html_len, res);
    html = malloc(*html_len + 1);
    memcpy(html, s, *html_len + 1);
    JS_FreeCString(ctx, s);
    JS_FreeValue(ctx, res);
    JS_SetPropertyStr(ctx, g, "renderResult", JS_UNDEFINED);
    JS_FreeValue(ctx, g);
    return html;
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
    if (arg(argc, argv, "limit", NULL))
        JS_SetMemoryLimit(rt, (size_t)atoll(arg(argc, argv, "limit", "0")));
    ctx = JS_NewContext(rt);
    if (!ctx) {
        fprintf(stderr, "render: no context\n");
        return 1;
    }
    add_globals(ctx);
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
        printf("{\"pages\":[");
        for (int k = 0; k < in.n; k++) {
            size_t len, hl;
            char *html = render(ctx, start, in.json[k], in.len[k], &len);
            double *d = malloc(sizeof(double) * (n > 0 ? n : 1)), sum = 0;

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
            for (int i = 0; i < warm; i++) {
                html = render(ctx, start, in.json[k], in.len[k], &len);
                free(html);
            }
            for (int i = 0; i < n; i++) {
                double t = now_ns();
                html = render(ctx, start, in.json[k], in.len[k], &len);
                d[i] = now_ns() - t;
                sum += d[i];
                if (len != hl) {
                    fprintf(stderr, "render: output changed between renders\n");
                    return 1;
                }
                free(html);
            }
            qsort(d, n, sizeof(double), cmp_double);
            printf("%s{\"input\":\"%s\",\"html_bytes\":%zu,\"n\":%d,\"mean_ns\":%.0f,\"median_ns\":%.0f,\"min_ns\":%.0f,\"max_ns\":%.0f}",
                   k ? "," : "", base_name(in.path[k]), hl, n, n ? sum / n : 0.0, n ? d[n / 2] : 0.0, n ? d[0] : 0.0, n ? d[n - 1] : 0.0);
            free(d);
        }
        printf("]}\n");
    }
    JS_FreeValue(ctx, start);
    JS_FreeContext(ctx);
    FREE_RUNTIME(rt);
    return 0;
}
