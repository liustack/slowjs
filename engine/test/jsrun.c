/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* jsrun runs scripts on the C tree, so that compare.sh can compare what they
   print with the files kept in the repository.

     jsrun [-raw] [-result] [-call] [-regexp-cache=N,BYTES] [-no-fused-ops] FILE...

   Each file runs as a global script in a runtime and a context of its own,
   then the jobs it queued run. jsrun prints "== " and the name of the file,
   then what the script gave: the string of its completion value, or of what
   that value returns when it is a function (called after the jobs), or the
   exception. A lone surrogate is printed as \u{...}, as Go would turn it into
   U+FFFD, unless -raw is given: then the string is written as the engine
   converts it (three bytes for a lone surrogate), which is how the files in
   engine/test/compare/recorded and testdata hold it.

   A file whose name ends in .mjs runs as a module instead. It has no
   completion value: what it gives is the string of globalThis.result once the
   module and its jobs ran, or the exception that rejected it. Its imports are
   read from the folder of the file, and the file imports itself by its own
   name.

   -result prints globalThis.result once the jobs ran, in place of the
   completion value, for a script that finishes in a job.

   -call takes the file as the body of a function run, evaluates the function,
   and calls it from C, as a host calls a function: nothing is on the stack
   below it. It then prints globalThis.result, as -result does.

   -regexp-cache=N,BYTES sets the cache of compiled regexps (JS_SetRegExpCache)
   before the file runs, N entries and BYTES bytes, 0,0 for none.

   -no-fused-ops compiles the file with the pairs of instructions that the
   compiler joins into one opcode left apart (JS_SetFusedOps).

   The scripts find getAsyncContext and setAsyncContext, the Builtins
   GetAsyncContext and SetAsyncContext of the Go API, as globals, and six
   functions for the scripts that look at the collector: gc() and youngGC()
   run JS_RunGC and JS_RunYoungGC, memorySize() gives JS_GetMemorySize,
   memoryBlocks() the blocks of the heap in use, heapChange(bytes, blocks)
   tells whether the heap holds the same blocks as when those two were read,
   and setGCThreshold(n) sets JS_SetGCThreshold, to its largest value when n is
   negative (the engine then never collects by itself). copyProps is the
   function of JS_NewCopyPropsFunction. Like a VM of the Go API, the context
   keeps SHAPE_CACHE shapes (see JS_SetShapeCache). */
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

static JSValue js_gc(JSContext *ctx, JSValueConst this_val,
                     int argc, JSValueConst *argv)
{
    JS_RunGC(JS_GetRuntime(ctx));
    return JS_UNDEFINED;
}

static JSValue js_young_gc(JSContext *ctx, JSValueConst this_val,
                           int argc, JSValueConst *argv)
{
    JS_RunYoungGC(JS_GetRuntime(ctx));
    return JS_UNDEFINED;
}

static JSValue js_memory_size(JSContext *ctx, JSValueConst this_val,
                              int argc, JSValueConst *argv)
{
    return JS_NewFloat64(ctx, (double)JS_GetMemorySize(JS_GetRuntime(ctx)));
}

/* The blocks of the heap in use (malloc_count of JS_ComputeMemoryUsage, less
   the runtime): with the system malloc (JS_HEAP_SYSTEM_MALLOC) every block of
   the engine, with TLSF the arenas of small blocks and the large blocks. */
static size_t memory_blocks(JSRuntime *rt)
{
    JSMemoryUsage s;

    JS_ComputeMemoryUsage(rt, &s);
    return (size_t)s.malloc_count - 1;
}

static JSValue js_memory_blocks(JSContext *ctx, JSValueConst this_val,
                                int argc, JSValueConst *argv)
{
    return JS_NewFloat64(ctx, (double)memory_blocks(JS_GetRuntime(ctx)));
}

/* How many bytes the same block can count more or less from one time to the
   next, for the same request. With the system malloc, none: the count is the
   usable size, which AddressSanitizer (the builds that use it) gives as the
   size asked for. With TLSF a block counts its TLSF size. TLSF splits a free
   block only when sizeof(block_header_t), 32 bytes, would be left over
   (block_can_split in tlsf.c), so it can give up to 24 bytes more than asked
   (block_size_min, sizes being multiples of 8). A large block of 505 to 4096
   bytes can also come from the cache of large blocks, which files a block by
   its usable size in steps of JS_LARGE_CACHE_STEP, 64 bytes, while a block
   made for the same request gets the size of its class: up to 56 bytes more
   (multiples of 8). The larger of the two is the bound. */
#ifdef JS_HEAP_SYSTEM_MALLOC
#define BLOCK_SLACK 0
#else
#define BLOCK_SLACK 56
#endif

/* heapChange(bytes, blocks): 0 when the heap holds the same blocks as when
   memorySize() gave bytes and memoryBlocks() gave blocks, the bytes differing
   by no more than BLOCK_SLACK for each block, or else a string with the
   difference in blocks and in bytes. */
static JSValue js_heap_change(JSContext *ctx, JSValueConst this_val,
                              int argc, JSValueConst *argv)
{
    JSRuntime *rt = JS_GetRuntime(ctx);
    double bytes0, blocks0, bytes, blocks;
    char buf[96];

    if (argc < 2 || JS_ToFloat64(ctx, &bytes0, argv[0]) < 0 ||
        JS_ToFloat64(ctx, &blocks0, argv[1]) < 0)
        return JS_ThrowTypeError(ctx, "heapChange(bytes, blocks)");
    blocks = (double)memory_blocks(rt);
    bytes = (double)JS_GetMemorySize(rt);
    if (blocks == blocks0 &&
        bytes - bytes0 <= BLOCK_SLACK * blocks0 && bytes0 - bytes <= BLOCK_SLACK * blocks0)
        return JS_NewInt32(ctx, 0);
    /* the engine's snprintf, which jsrun links, has no floating point */
    snprintf(buf, sizeof(buf), "%+lld blocks, %+lld bytes", (long long)(blocks - blocks0), (long long)(bytes - bytes0));
    return JS_NewString(ctx, buf);
}

static JSValue js_set_gc_threshold(JSContext *ctx, JSValueConst this_val,
                                   int argc, JSValueConst *argv)
{
    double n = -1;

    if (argc > 0 && JS_ToFloat64(ctx, &n, argv[0]) < 0)
        return JS_EXCEPTION;
    JS_SetGCThreshold(JS_GetRuntime(ctx), n < 0 ? (size_t)-1 : (size_t)n);
    return JS_UNDEFINED;
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
                          JS_NewCFunction(ctx, set_async_context, "setAsyncContext", 1)) < 0 ||
        JS_SetPropertyStr(ctx, global, "gc", JS_NewCFunction(ctx, js_gc, "gc", 0)) < 0 ||
        JS_SetPropertyStr(ctx, global, "youngGC", JS_NewCFunction(ctx, js_young_gc, "youngGC", 0)) < 0 ||
        JS_SetPropertyStr(ctx, global, "memorySize", JS_NewCFunction(ctx, js_memory_size, "memorySize", 0)) < 0 ||
        JS_SetPropertyStr(ctx, global, "memoryBlocks", JS_NewCFunction(ctx, js_memory_blocks, "memoryBlocks", 0)) < 0 ||
        JS_SetPropertyStr(ctx, global, "heapChange", JS_NewCFunction(ctx, js_heap_change, "heapChange", 2)) < 0 ||
        JS_SetPropertyStr(ctx, global, "setGCThreshold", JS_NewCFunction(ctx, js_set_gc_threshold, "setGCThreshold", 1)) < 0 ||
        JS_SetPropertyStr(ctx, global, "copyProps", JS_NewCopyPropsFunction(ctx, "copyProps", 9)) < 0)
        ret = -1;
    JS_FreeValue(ctx, global);
    return ret;
}

/* Options of the command line, for every file. */
static int opt_raw;
static int opt_result;
static int opt_call;
static int opt_regexp_cache = -1;   /* -1 keeps the default of the runtime */
static int opt_no_fused_ops;
static size_t opt_regexp_cache_bytes;

/* The folder of the module that runs: imports are read from it. */
static JSModuleDef *module_loader(JSContext *ctx, const char *name, void *opaque,
                                  JSValueConst attributes)
{
    const char *dir = opaque;
    char path[4096];
    size_t len;
    uint8_t *buf;
    JSValue f;
    JSModuleDef *m;

    snprintf(path, sizeof(path), "%s/%s", dir, name);
    buf = js_load_file(ctx, &len, path);
    if (!buf) {
        JS_ThrowReferenceError(ctx, "could not load module '%s'", name);
        return NULL;
    }
    f = JS_Eval(ctx, (char *)buf, len, name, JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    js_free(ctx, buf);
    if (JS_IsException(f))
        return NULL;
    /* the module is already referenced, so f is freed here */
    m = JS_VALUE_GET_PTR(f);
    JS_FreeValue(ctx, f);
    return m;
}

static int is_module(const char *path)
{
    size_t n = strlen(path);

    return n > 4 && strcmp(path + n - 4, ".mjs") == 0;
}

static int run(const char *path)
{
    JSRuntime *rt;
    JSContext *ctx, *job_ctx;
    JSValue v, format, s;
    char *src;
    size_t len;
    int failed_job = 0, r, module = is_module(path);
    char dir[4096];
    const char *slash;

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
    if (opt_no_fused_ops)
        JS_SetFusedOps(ctx, 0);
    if (module) {
        /* The file is the module "NAME", read from its folder, and a module
           of "<eval>" imports it, as a host does with its main module. So
           the module has the name its own imports use for it, and what it
           prints does not depend on where it is. */
        static char stub[4200];

        slash = strrchr(path, '/');
        snprintf(dir, sizeof(dir), "%.*s", slash ? (int)(slash - path) : 1, slash ? path : ".");
        snprintf(stub, sizeof(stub), "import \"%s\";", slash ? slash + 1 : path);
        JS_SetModuleLoaderFunc2(rt, NULL, module_loader, NULL, dir);
        free(src);
        src = strdup(stub);
        len = strlen(stub);
    }
    if (opt_call) {
        /* The body of a function, called from C: what the Go API does with
           VM.Call. */
        static const char head[] = "function run() {\n", tail[] = "\n}";
        char *wrapped = malloc(sizeof(head) + len + sizeof(tail));
        JSValue global, fn;

        memcpy(wrapped, head, sizeof(head) - 1);
        memcpy(wrapped + sizeof(head) - 1, src, len);
        memcpy(wrapped + sizeof(head) - 1 + len, tail, sizeof(tail));
        v = JS_Eval(ctx, wrapped, sizeof(head) - 1 + len + sizeof(tail) - 1, "<eval>", JS_EVAL_TYPE_GLOBAL);
        free(wrapped);
        free(src);
        if (!JS_IsException(v)) {
            JS_FreeValue(ctx, v);
            global = JS_GetGlobalObject(ctx);
            fn = JS_GetPropertyStr(ctx, global, "run");
            JS_FreeValue(ctx, global);
            v = JS_Call(ctx, fn, JS_UNDEFINED, 0, NULL);
            JS_FreeValue(ctx, fn);
        }
    } else {
        v = JS_Eval(ctx, src, len, "<eval>", module ? JS_EVAL_TYPE_MODULE : JS_EVAL_TYPE_GLOBAL);
        free(src);
    }
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
    if (module) {
        /* v is the promise of the module: it is rejected with what the
           module threw, or pending when it waits for ever. The module gives
           globalThis.result. */
        JSPromiseStateEnum state = JS_PromiseState(ctx, v);

        if (state == JS_PROMISE_REJECTED) {
            JS_Throw(ctx, JS_PromiseResult(ctx, v));
            JS_FreeValue(ctx, v);
            print_exception(ctx, "exception");
            goto done;
        }
        if (state == JS_PROMISE_PENDING) {
            JS_FreeValue(ctx, v);
            printf("<module never finished>\n");
            goto done;
        }
    }
    if (module || opt_result || opt_call) {
        JSValue global = JS_GetGlobalObject(ctx);

        JS_FreeValue(ctx, v);
        v = JS_GetPropertyStr(ctx, global, "result");
        JS_FreeValue(ctx, global);
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
        } else if (strcmp(argv[i], "-result") == 0) {
            opt_result = 1;
        } else if (strcmp(argv[i], "-call") == 0) {
            opt_call = 1;
        } else if (strcmp(argv[i], "-no-fused-ops") == 0) {
            opt_no_fused_ops = 1;
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
