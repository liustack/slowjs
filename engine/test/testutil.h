/*
 * Copyright (c) 2026 The liustack/tifajs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE at the
 * root of this repository.
 */
#ifndef TIFAJS_TEST_TESTUTIL_H
#define TIFAJS_TEST_TESTUTIL_H

/* What the unit programs that came from the Go tests share (unit-oom.c,
   unit-api.c): a runtime with a context, a way to give it a little memory
   and no more, and the small helpers to evaluate and to read an exception.
   Every function is static, so a program takes what it uses. */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "host.h"

#ifndef countof
#define countof(x) (sizeof(x) / sizeof((x)[0]))
#endif

static int failures;
static long checks;

#define CHECK(cond, ...)                                                \
    do {                                                                \
        checks++;                                                       \
        if (!(cond)) {                                                  \
            if (failures++ < 40) {                                      \
                printf("FAIL %s:%d: %s: ", __FILE__, __LINE__, #cond);  \
                printf(__VA_ARGS__);                                    \
                printf("\n");                                           \
            }                                                           \
        }                                                               \
    } while (0)

typedef struct {
    JSRuntime *rt;
    JSContext *ctx;
} VM;

static VM vm_new(void)
{
    VM v;

    v.rt = host_new_runtime();
    v.ctx = v.rt ? JS_NewContext(v.rt) : NULL;
    if (!v.ctx) {
        fprintf(stderr, "no runtime\n");
        exit(2);
    }
    return v;
}

/* The runtime goes back to the host, which says so and exits with status 87
   when a block is still in use: that is the leak check of these tests. */
static void vm_free(VM *v)
{
    JS_FreeContext(v->ctx);
    host_free_runtime(v->rt);
}

/* room lets the runtime allocate n more bytes than it holds now, after a
   collection, and nothing else. unlimit takes the limit away. */
static void room(VM *v, size_t n)
{
    JS_RunGC(v->rt);
    JS_SetMemoryLimit(v->rt, JS_GetMemorySize(v->rt) + n);
}

static void unlimit(VM *v)
{
    JS_SetMemoryLimit(v->rt, (size_t)-1);
}

/* exception_text takes the pending exception out of the context, whatever
   kind it is, and returns its text (malloc'd), or NULL when there is none. An
   exception that cannot be converted to text is "(no text)". */
static char *exception_text(JSContext *ctx)
{
    JSValue e;
    const char *s;
    char *r;

    if (!JS_HasException(ctx))
        return NULL;
    JS_SetUncatchableException(ctx, 0);
    e = JS_GetException(ctx);
    s = JS_ToCString(ctx, e);
    if (s) {
        r = strdup(s);
        JS_FreeCString(ctx, s);
    } else {
        JS_FreeValue(ctx, JS_GetException(ctx));
        r = strdup("(no text)");
    }
    JS_FreeValue(ctx, e);
    return r;
}

/* eval_ok runs src as a global script, frees the result and reports whether
   it ran. The exception, if any, is printed and taken. */
static int eval_ok(JSContext *ctx, const char *src)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int ok = !JS_IsException(v);

    if (!ok) {
        char *t = exception_text(ctx);

        printf("exception: %s\n", t ? t : "?");
        free(t);
    }
    JS_FreeValue(ctx, v);
    return ok;
}

/* eval_text runs src as a global script and returns the string of its value
   (malloc'd), or NULL when it threw. The exception is left in the context. */
static char *eval_text(JSContext *ctx, const char *src)
{
    JSValue v = JS_Eval(ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    const char *s;
    char *r = NULL;

    if (JS_IsException(v))
        return NULL;
    s = JS_ToCString(ctx, v);
    if (s) {
        r = strdup(s);
        JS_FreeCString(ctx, s);
    }
    JS_FreeValue(ctx, v);
    return r;
}

/* eval_is runs src and reports whether its value has the string want. */
static int eval_is(JSContext *ctx, const char *src, const char *want)
{
    char *t = eval_text(ctx, src);
    int ok = t && strcmp(t, want) == 0;

    if (!ok) {
        char *e = t ? NULL : exception_text(ctx);

        printf("%s: got %s, want %s\n", src, t ? t : (e ? e : "?"), want);
        free(e);
    }
    free(t);
    return ok;
}

/* call_global calls the global function name with no argument and no this,
   from C, as a host does, so that no top-level code is on the stack. */
static JSValue call_global(JSContext *ctx, const char *name)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue f = JS_GetPropertyStr(ctx, g, name);
    JSValue r = JS_Call(ctx, f, JS_UNDEFINED, 0, NULL);

    JS_FreeValue(ctx, f);
    JS_FreeValue(ctx, g);
    return r;
}

/* value_text returns the string of v (malloc'd), or NULL for an exception. v
   is not freed. */
static char *value_text(JSContext *ctx, JSValueConst v)
{
    const char *s;
    char *r;

    if (JS_IsException(v))
        return NULL;
    s = JS_ToCString(ctx, v);
    if (!s) {
        JS_FreeValue(ctx, JS_GetException(ctx));
        return NULL;
    }
    r = strdup(s);
    JS_FreeCString(ctx, s);
    return r;
}

/* stack_text reads the stack of the error e and returns its text (malloc'd),
   or NULL when the read threw (the exception stays pending). */
static char *stack_text(JSContext *ctx, JSValueConst e)
{
    JSAtom a = JS_NewAtom(ctx, "stack");
    JSValue s = JS_GetProperty(ctx, e, a);
    char *r = value_text(ctx, s);

    JS_FreeAtom(ctx, a);
    JS_FreeValue(ctx, s);
    return r;
}

/* drain_jobs runs the jobs of v until none is left, each failure its own,
   with its exception taken. */
static void drain_jobs(VM *v)
{
    JSContext *ctx;
    int r;

    while ((r = JS_ExecutePendingJob(v->rt, &ctx)) != 0) {
        if (r < 0)
            JS_FreeValue(ctx, JS_GetException(ctx));
    }
}

/* import_state imports name with no stop, runs the jobs, and puts in buf
   what the promise of that import became: "ok", the text of its error, or
   "pending". */
static const char *import_state(VM *v, const char *name, char *buf, size_t size)
{
    char src[64];
    JSValue p, e;
    const char *s;

    snprintf(src, sizeof(src), "import(\"%s\")", name);
    p = JS_Eval(v->ctx, src, strlen(src), "<again>", JS_EVAL_TYPE_GLOBAL);
    if (JS_IsException(p)) {
        JS_FreeValue(v->ctx, JS_GetException(v->ctx));
        snprintf(buf, size, "threw");
        return buf;
    }
    drain_jobs(v);
    switch (JS_PromiseState(v->ctx, p)) {
    case JS_PROMISE_FULFILLED:
        snprintf(buf, size, "ok");
        break;
    case JS_PROMISE_REJECTED:
        e = JS_PromiseResult(v->ctx, p);
        s = JS_ToCString(v->ctx, e);
        snprintf(buf, size, "%s", s ? s : "?");
        JS_FreeCString(v->ctx, s);
        JS_FreeValue(v->ctx, e);
        break;
    default:
        snprintf(buf, size, "pending");
        break;
    }
    JS_FreeValue(v->ctx, p);
    return buf;
}

/* compile_module compiles src as the module name, not run. */
static void compile_module(VM *v, const char *name, const char *src)
{
    JSValue r = JS_Eval(v->ctx, src, strlen(src), name, JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);

    CHECK(!JS_IsException(r), "%s does not compile", name);
    JS_FreeValue(v->ctx, r);
}

#endif
