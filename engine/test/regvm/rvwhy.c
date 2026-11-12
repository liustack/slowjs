/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* rvwhy runs one script with the register loop on, as rvdiff runs it, and
   prints why what still runs in the old loop does: the board of score.sh
   (rvscore-engine.c) for one script, with the refused functions that run
   the most and what blocks each.

     rvwhy FILE

   Built by score.sh into engine/test/out/score/rvwhy, on the counting
   engine. The scripts see the globals of jsrun, and print and console.log
   write nothing. */

#define main jsrun_main
#include "../jsrun.c"
#undef main

void JS_RegvmCount(JSRuntime *rt, int on);
void rvscore_dump(JSRuntime *rt);

void *js_rv_alloc(size_t size)
{
    void *p = calloc(1, size);
    if (!p)
        abort();
    return p;
}

/* the counting build also writes out each function it freed: only the
   board is wanted */
static int emit_on;

void js_rv_emit(const char *s, size_t len)
{
    if (emit_on)
        fwrite(s, 1, len, stdout);
}

static JSValue js_quiet(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    (void)ctx; (void)this_val; (void)argc; (void)argv;
    return JS_UNDEFINED;
}

static int why(void *p)
{
    const char *path = p;
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = JS_NewContext(rt), *jc;
    JSValue g, console, v;
    size_t len;
    char *src = read_file(path, &len);

    if (!src) {
        fprintf(stderr, "rvwhy: cannot read %s\n", path);
        return 2;
    }
    define_globals(ctx);
    g = JS_GetGlobalObject(ctx);
    console = JS_NewObject(ctx);
    JS_SetPropertyStr(ctx, g, "print", JS_NewCFunction(ctx, js_quiet, "print", 1));
    JS_SetPropertyStr(ctx, console, "log", JS_NewCFunction(ctx, js_quiet, "log", 1));
    JS_SetPropertyStr(ctx, g, "console", console);
    JS_FreeValue(ctx, g);
    JS_SetRegvm(ctx, 1);
    JS_RegvmCount(rt, 1);
    v = JS_Eval(ctx, src, len, "<eval>", JS_EVAL_TYPE_GLOBAL);
    free(src);
    if (JS_IsFunction(ctx, v)) {
        JSValue r = JS_Call(ctx, v, JS_UNDEFINED, 0, NULL);
        JS_FreeValue(ctx, v);
        v = r;
    }
    if (JS_IsException(v))
        JS_FreeValue(ctx, JS_GetException(ctx));
    JS_FreeValue(ctx, v);
    while (JS_ExecutePendingJob(rt, &jc) != 0)
        ;
    JS_RegvmCount(rt, 0);
    printf("dispatches: register loop %d, old loop %d\n", JS_RegvmStat(ctx, 10), JS_RegvmStat(ctx, 11));
    emit_on = 1;
    rvscore_dump(rt);
    fflush(stdout);
    /* the counting build keeps what it counted: the runtime is left as it is */
    return 0;
}

int main(int argc, char **argv)
{
    if (argc != 2) {
        fprintf(stderr, "usage: rvwhy FILE\n");
        return 2;
    }
    return host_run_thread(why, argv[1]);
}
