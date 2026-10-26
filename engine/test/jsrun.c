/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* jsrun runs scripts on the C tree as internal/jsrun runs them on the Go
   engine, so that compare.sh can compare what they print with the files kept
   in the repository (and with the Go engine, while it is there).

     jsrun [-raw] [-regexp-cache=N,BYTES] FILE...

   Each file runs as a global script in a runtime and a context of its own,
   then the jobs it queued run. jsrun prints "== " and the name of the file,
   then what the script gave: the string of its completion value, or of what
   that value returns when it is a function (called after the jobs), or the
   exception. A lone surrogate is printed as \u{...}, as Go would turn it into
   U+FFFD, unless -raw is given: then the string is written as the engine
   converts it (three bytes for a lone surrogate), which is how the files in
   lib/testdata hold it.

   -regexp-cache=N,BYTES sets the cache of compiled regexps (JS_SetRegExpCache)
   before the file runs, N entries and BYTES bytes, 0,0 for none.

   The scripts find getAsyncContext and setAsyncContext, the Builtins
   GetAsyncContext and SetAsyncContext of the Go API, as globals. Like a VM
   of the Go API, the context keeps SHAPE_CACHE shapes (see
   JS_SetShapeCache). */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "host.h"

/* DefaultShapeCache of the Go API */
#define SHAPE_CACHE 2048

/* What both runners call with the completion value. */
static const char format_src_raw[] =
    "(v) => typeof v === 'function' ? String(v()) : String(v)";

static const char format_src[] =
    "(v) => {\n"
    "  const s = typeof v === 'function' ? String(v()) : String(v)\n"
    "  if (s.isWellFormed()) return s\n"
    "  let r = ''\n"
    "  for (const c of s) {\n"
    "    const u = c.charCodeAt(0)\n"
    "    r += c.length === 1 && u >= 0xd800 && u <= 0xdfff ? '\\\\u{' + u.toString(16) + '}' : c\n"
    "  }\n"
    "  return r\n"
    "}";

static char *read_file(const char *path, size_t *plen)
{
    FILE *f = fopen(path, "rb");
    char *buf;
    long n;

    if (!f)
        return NULL;
    if (fseek(f, 0, SEEK_END) < 0 || (n = ftell(f)) < 0 || fseek(f, 0, SEEK_SET) < 0) {
        fclose(f);
        return NULL;
    }
    buf = malloc(n + 1);
    if (buf && fread(buf, 1, n, f) != (size_t)n) {
        free(buf);
        buf = NULL;
    }
    fclose(f);
    if (buf) {
        buf[n] = '\0';
        *plen = n;
    }
    return buf;
}

/* print_exception prints the pending exception of ctx as <what=...>. */
static void print_exception(JSContext *ctx, const char *what)
{
    JSValue e = JS_GetException(ctx);
    size_t len;
    const char *s = JS_ToCStringLen(ctx, &len, e);

    printf("<%s=", what);
    if (s) {
        fwrite(s, 1, len, stdout);
        JS_FreeCString(ctx, s);
    } else {
        JS_FreeValue(ctx, JS_GetException(ctx));
        printf("(no string)");
    }
    printf(">\n");
    JS_FreeValue(ctx, e);
}

static JSValue get_async_context(JSContext *ctx, JSValueConst this_val,
                                 int argc, JSValueConst *argv)
{
    return JS_GetAsyncContext(ctx);
}

static JSValue set_async_context(JSContext *ctx, JSValueConst this_val,
                                 int argc, JSValueConst *argv)
{
    JS_SetAsyncContext(ctx, argc > 0 ? argv[0] : JS_UNDEFINED);
    return JS_UNDEFINED;
}

/* define_globals gives the global object of ctx what the Go runner gives
   it. */
static int define_globals(JSContext *ctx)
{
    JSValue global = JS_GetGlobalObject(ctx);
    int ret = 0;

    if (JS_SetPropertyStr(ctx, global, "getAsyncContext",
                          JS_NewCFunction(ctx, get_async_context, "getAsyncContext", 0)) < 0 ||
        JS_SetPropertyStr(ctx, global, "setAsyncContext",
                          JS_NewCFunction(ctx, set_async_context, "setAsyncContext", 1)) < 0)
        ret = -1;
    JS_FreeValue(ctx, global);
    return ret;
}

/* Options of the command line, for every file. */
static int opt_raw;
static int opt_regexp_cache = -1;   /* -1 keeps the default of the runtime */
static size_t opt_regexp_cache_bytes;

static int run(const char *path)
{
    JSRuntime *rt;
    JSContext *ctx, *job_ctx;
    JSValue v, format, s;
    char *src;
    size_t len;
    int failed_job = 0, r;

    src = read_file(path, &len);
    if (!src) {
        fprintf(stderr, "jsrun: cannot read %s\n", path);
        return -1;
    }
    rt = host_new_runtime();
    ctx = rt ? JS_NewContext(rt) : NULL;
    if (!ctx || JS_SetShapeCache(ctx, SHAPE_CACHE) < 0 || define_globals(ctx) < 0) {
        fprintf(stderr, "jsrun: no runtime\n");
        return -1;
    }
    if (opt_regexp_cache >= 0)
        JS_SetRegExpCache(rt, opt_regexp_cache, opt_regexp_cache_bytes);
    v = JS_Eval(ctx, src, len, "<eval>", JS_EVAL_TYPE_GLOBAL);
    free(src);
    if (JS_IsException(v)) {
        print_exception(ctx, "exception");
        goto done;
    }
    for (;;) {
        r = JS_ExecutePendingJob(rt, &job_ctx);
        if (r == 0)
            break;
        /* Like ExecutePendingJobs of the Go API: report the first job
           that threw, and run the others. */
        if (r < 0) {
            if (failed_job)
                JS_FreeValue(ctx, JS_GetException(ctx));
            else
                print_exception(ctx, "job exception");
            failed_job = 1;
        }
    }
    if (opt_raw)
        format = JS_Eval(ctx, format_src_raw, sizeof(format_src_raw) - 1, "<eval>", JS_EVAL_TYPE_GLOBAL);
    else
        format = JS_Eval(ctx, format_src, sizeof(format_src) - 1, "<eval>", JS_EVAL_TYPE_GLOBAL);
    if (JS_IsException(format)) {
        print_exception(ctx, "format");
        JS_FreeValue(ctx, v);
        goto done;
    }
    s = JS_Call(ctx, format, JS_UNDEFINED, 1, (JSValueConst *)&v);
    JS_FreeValue(ctx, format);
    JS_FreeValue(ctx, v);
    if (JS_IsException(s)) {
        print_exception(ctx, "exception");
    } else {
        const char *str = JS_ToCStringLen(ctx, &len, s);
        if (str) {
            fwrite(str, 1, len, stdout);
            printf("\n");
            JS_FreeCString(ctx, str);
        } else {
            print_exception(ctx, "exception");
        }
        JS_FreeValue(ctx, s);
    }
 done:
    /* as VM.Close does: the kept shapes and the async context can hold the
       context */
    JS_SetShapeCache(ctx, 0);
    JS_SetAsyncContext(ctx, JS_UNDEFINED);
    JS_FreeContext(ctx);
    JS_FreeRuntime(rt);
    return 0;
}

typedef struct {
    int argc;
    char **argv;
} Args;

static int run_all(void *p)
{
    Args *a = p;
    int i;
    const char *name;

    for (i = 1; i < a->argc; i++) {
        if (a->argv[i][0] == '-' && a->argv[i][1])
            continue;
        name = strrchr(a->argv[i], '/');
        printf("== %s\n", name ? name + 1 : a->argv[i]);
        if (run(a->argv[i]) < 0)
            return 1;
        fflush(stdout);
    }
    return 0;
}

int main(int argc, char **argv)
{
    Args a = { argc, argv };
    int i;

    for (i = 1; i < argc && argv[i][0] == '-' && argv[i][1]; i++) {
        if (strcmp(argv[i], "-raw") == 0) {
            opt_raw = 1;
        } else if (strncmp(argv[i], "-regexp-cache=", 14) == 0) {
            char *end;
            long n = strtol(argv[i] + 14, &end, 10);
            long long bytes;

            if (*end != ',' || n < 0) {
                fprintf(stderr, "jsrun: %s: want N,BYTES\n", argv[i]);
                return 2;
            }
            bytes = strtoll(end + 1, &end, 10);
            if (*end || bytes < 0) {
                fprintf(stderr, "jsrun: %s: want N,BYTES\n", argv[i]);
                return 2;
            }
            opt_regexp_cache = (int)n;
            opt_regexp_cache_bytes = (size_t)bytes;
        } else {
            fprintf(stderr, "jsrun: unknown option %s\n", argv[i]);
            return 2;
        }
    }
    /* a script may recurse as deep as the engine allows */
    return host_run_thread(run_all, &a);
}
