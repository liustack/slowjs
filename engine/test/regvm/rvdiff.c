/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* rvdiff runs each script in a process and a runtime of its own and
   compares what the run shows with the golden of the script (-golden,
   below: the two runs, off and on, read the old loop, which is gone):

     out        what the script printed (print, console.log)
     result     the completion value, or what it returns when it is a
                function (called from C with JS_Call, both ways), as jsrun
                gives it
     exception  the exception of the script, its message, and the lines of
     stack      its stack
     job        the exceptions of its jobs, with their stacks
     mem        the objects, atoms, shapes, properties, functions and arrays
                the runtime still holds once the script and its jobs ended
                (mem-end) and after a full collection (mem-gc)
     expect     a function given to rvExpect that has no register code in the
                run with the register loop on (see below)
     status     how the process ended: 0, a leak found when the runtime was
                freed (87), a sanitizer (86 or its report on stderr), a
                signal, or the time limit

     rvdiff [-o DIR] [-v] [-q] [-t SECONDS] [-j N] [-ignore KIND,...]
            [-gc off|auto|BYTES] [-cases FILE] [OPTIONS of jsrun] PATH...

   A PATH is a script (.js, or .mjs for a module) or a folder, read for
   scripts with no recursion into its subfolders. A script names options of
   jsrun on a line "// jsrun: OPTIONS" (-raw, -result, -call, -no-fused-ops,
   -regexp-cache=N,BYTES), and -strict, which puts "use strict" before the
   script, for both runs: the pass converts only strict functions). Options
   given before the paths apply to every script. -cases FILE reads lines "SCRIPT [OPTIONS]" as compare.sh does.

   -o DIR writes NAME.off and NAME.on, the two transcripts, for every script
   that differs (and for every script with -v). -ignore leaves kinds out of
   the comparison (stack, for one). -t is the time a run may take (default
   60 s): past it the run is killed and its status says so. -j runs N scripts
   at a time. -gc off (the default) sets the threshold of the collector so
   that the engine never collects by itself, and both ways collect only where
   the script asks (the memory the register loop takes would move the points
   of an automatic collection, and the mem-end counts with them). auto keeps
   the threshold of the engine, BYTES sets one, a small one to collect often.

   A run that the time limit ended stopped at a point that depends on how
   busy the machine was, so it is not compared: when both runs ran out, or
   the other run took at least half the limit, rvdiff prints a line "SLOW
   PATH: ..." (also with -q) and counts the script as too slow, not as a
   difference. When only one ran out and the other ended in less than half
   the limit, the two are compared as usual: one loop may hang.

   It prints one line per script that differs, with the kinds that differ
   and the first lines that differ, then a summary, and exits with 1 when any
   script differs, 0 when none does, 2 when it cannot run.

   The scripts see the globals jsrun gives them (gc, memorySize, heapChange
   and the rest, see jsrun.c), and print and console.log, which write a line
   to the transcript. In a build with -DRV_STATS the transcript of each run
   ends with the dispatches of each loop (not compared).

   rvExpect(f, g, ...) says which functions the test is for: called after
   the script has called them, it checks with the register loop on that each
   has register code (and says why the pass refused one), so that a test of
   a new op of the register loop cannot pass by running in the old loop. A
   function converts at its first call and runs in the register loop from
   its second. It reads the state of each with JS_RegvmFuncStat, and in a
   build with -DRV_STATS also fails a function that converted but never ran
   in the register loop. rvFusedOps is false in a run with -no-fused-ops,
   for a test whose function the pass takes only with the fused ops.

   Goldens, for when there is one loop only: -record DIR runs both loops as
   above and, for each script where they agree, writes the compared lines
   of the old loop and its status to a file of DIR (one that differs, ran
   out of time or has a report is listed as NORECORD and not written).
   -golden DIR runs each script once, with the register loop on, and
   compares it with its file by the same kinds. A missing file and a run
   the limit ended are differences. -update with -golden writes the file of
   a run that differs from it (read the diff before it goes into a commit).
   The file is DIR/PATH.OPTIONS.txt: PATH under -root ROOT (a script that
   corpus.sh makes into engine/test/out/.../gen/ is gen/NAME), and the
   options of the command line, of the script's "// jsrun:" line and of the
   cases file, each without its dashes. The lines of a kind longer than 32
   KiB in all are kept as one line "hash KIND COUNT FNV", their count and
   the FNV-1a hash of each line with its newline: a difference there says
   the kind, not the line. -full writes every line (a transcript to read,
   as rvgen node needs).

   A line "// rv-expect-op: NAME ..." names register ops (JS_RegvmOpName)
   the script is for: in a build with -DRV_STATS (BIN_DIR of corpus.sh, an
   OUTDIR built with EXTRA_DEFS=-DRV_STATS) the run with the loop on writes
   "expect op NAME ran 0 times" for each that did not run. A build without
   the counts does not check it, and says so in the info of the run.

   The info of each run and the line before the summary say which build ran,
   "engine src HASH tools HASH mode MODE": the hashes of the sources of the
   engine and of these tools that regvm/build.sh passes in RV_ENGINE_STAMP
   (see engine/test/stamp.sh), "engine unknown" for a build without it.
   Such a build refuses to run (status 2) when the tree it was built from,
   RV_ENGINE_DIR, has other sources now: the hashes of the tree come from
   RV_TREE_STAMP when a script has computed them already (stamp_check of
   stamp.sh), else from stamp.sh run once at the start.

   A function the register loop cannot run throws "InternalError: cannot
   run: WHY in NAME (FILE)". The info of the run lists each WHY with how
   often it was thrown and the first function it named ("cannot COUNT WHY
   in NAME (FILE)"). run-test262 fails a test whose harness it cannot run
   with the same lines. */

#include <ctype.h>
#include <dirent.h>
#include <errno.h>
#include <signal.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

#define main jsrun_main
#include "../jsrun.c"
#undef main

#define countof(a) (sizeof(a) / sizeof((a)[0]))

#ifndef RV_ENGINE_STAMP
#define RV_ENGINE_STAMP "unknown"
#endif

/* stamp_fresh: whether the tree this rvdiff was built from has the sources
   it was built from (see the comment at the top) */
static int stamp_fresh(void)
{
#ifdef RV_ENGINE_DIR
    char now[256] = "", cmd[4096];
    const char *e = getenv("RV_TREE_STAMP");
    size_t n;
    if (e) {
        snprintf(now, sizeof(now), "%s", e);
    } else {
        FILE *p;
        snprintf(cmd, sizeof(cmd),
                 ". '%s/test/stamp.sh' && echo \"src $(stamp_src '%s') tools $(stamp_tools '%s')\"",
                 RV_ENGINE_DIR, RV_ENGINE_DIR, RV_ENGINE_DIR);
        p = popen(cmd, "r");
        if (!p || !fgets(now, sizeof(now), p))
            now[0] = 0;
        if (p)
            pclose(p);
        now[strcspn(now, "\n")] = 0;
    }
    n = strlen(now);
    if (n == 0 || strncmp(RV_ENGINE_STAMP, now, n) != 0 ||
        (RV_ENGINE_STAMP[n] != 0 && RV_ENGINE_STAMP[n] != ' ')) {
        fprintf(stderr, "rvdiff: this build is of other sources than %s has now (built from %s, the tree has %s): build it again with engine/test/regvm/build.sh\n",
                RV_ENGINE_DIR, RV_ENGINE_STAMP, n ? now : "no stamp.sh to read");
        return 0;
    }
#endif
    return 1;
}

/* ------------------------------------------------------------ the child */

static FILE *tr; /* the transcript of the run */
/* -gc: -1 off (the default: no collection but the ones the script asks for,
   so that both ways collect at the same points), -2 auto (the threshold of
   the engine, which the memory of the register loop moves), or a threshold
   in bytes */
static long gc_mode = -1;
/* -strict: the script runs as strict code, so that more of it converts */
static int opt_strict;

static void tr_lines(const char *kind, const char *s, size_t len)
{
    size_t i = 0, j;

    if (len == 0) {
        fprintf(tr, "%s \n", kind);
        return;
    }
    while (i < len) {
        for (j = i; j < len && s[j] != '\n'; j++)
            ;
        fprintf(tr, "%s ", kind);
        fwrite(s + i, 1, j - i, tr);
        fputc('\n', tr);
        i = j + 1;
    }
}

static void tr_value(JSContext *ctx, const char *kind, JSValueConst v)
{
    size_t len;
    const char *s = JS_ToCStringLen(ctx, &len, v);

    if (s) {
        tr_lines(kind, s, len);
        JS_FreeCString(ctx, s);
    } else {
        JS_FreeValue(ctx, JS_GetException(ctx));
        fprintf(tr, "%s (no string)\n", kind);
    }
}

/* the pending exception: its string, then its stack. An interrupt
   (stopAfterPolls) is over once the host has taken its exception, as a host
   that stopped a call at its timeout goes on with the runtime: the flag is
   put down, so that the jobs still to run are not stopped by it too. */
static void tr_exception(JSContext *ctx, const char *kind, const char *stack_kind)
{
    JSValue e = JS_GetException(ctx);

    host_poll_stop_after(0);
    *JS_GetInterruptFlag(JS_GetRuntime(ctx)) = 0;
    tr_value(ctx, kind, e);
    if (JS_IsObject(e)) {
        JSValue st = JS_GetPropertyStr(ctx, e, "stack");
        if (JS_IsException(st)) {
            JS_FreeValue(ctx, JS_GetException(ctx));
            fprintf(tr, "%s (throws)\n", stack_kind);
        } else if (!JS_IsUndefined(st)) {
            tr_value(ctx, stack_kind, st);
        }
        JS_FreeValue(ctx, st);
    }
    JS_FreeValue(ctx, e);
}

static JSValue js_print(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    char *buf = NULL;
    size_t size = 0, cap = 0;
    int i;

    (void)this_val;
    for (i = 0; i < argc; i++) {
        size_t len;
        const char *s = JS_ToCStringLen(ctx, &len, argv[i]);
        if (!s) {
            free(buf);
            return JS_EXCEPTION;
        }
        if (size + len + 2 > cap) {
            cap = 2 * (size + len + 2);
            buf = realloc(buf, cap);
        }
        if (i)
            buf[size++] = ' ';
        memcpy(buf + size, s, len);
        size += len;
        JS_FreeCString(ctx, s);
    }
    tr_lines("out", buf ? buf : "", size);
    free(buf);
    return JS_UNDEFINED;
}

static int add_print(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx), console = JS_NewObject(ctx);
    int r = 0;

    if (JS_SetPropertyStr(ctx, g, "print", JS_NewCFunction(ctx, js_print, "print", 1)) < 0 ||
        JS_SetPropertyStr(ctx, console, "log", JS_NewCFunction(ctx, js_print, "log", 1)) < 0 ||
        JS_SetPropertyStr(ctx, g, "console", JS_DupValue(ctx, console)) < 0)
        r = -1;
    JS_FreeValue(ctx, console);
    JS_FreeValue(ctx, g);
    return r;
}

static void tr_mem(JSRuntime *rt, const char *kind)
{
    JSMemoryUsage u;

    JS_ComputeMemoryUsage(rt, &u);
    fprintf(tr, "%s obj=%lld atom=%lld shape=%lld prop=%lld jsfunc=%lld cfunc=%lld array=%lld fast_array=%lld elements=%lld binary=%lld\n",
            kind, (long long)u.obj_count, (long long)u.atom_count, (long long)u.shape_count,
            (long long)u.prop_count, (long long)u.js_func_count, (long long)u.c_func_count,
            (long long)u.array_count, (long long)u.fast_array_count,
            (long long)u.fast_array_elements, (long long)u.binary_object_count);
}

typedef struct Job {
    const char *path;
    int regvm;
} Job;

/* the opcodes of the old loop, for the reason of a refusal */
#ifndef SHORT_OPCODES
#define SHORT_OPCODES 1
#endif
static const char *const rvexpect_names[] = {
#define FMT(f)
#define DEF(id, size, n_pop, n_push, f) #id,
#define def(id, size, n_pop, n_push, f)
#include "quickjs-opcode.h"
#undef def
#undef DEF
#undef FMT
};

/* The state of f in the register loop (JS_RegvmFuncStat 0), and for a
   refused function what the pass stops at, in why */
static int rvexpect_state(JSContext *ctx, JSValueConst f, char *why, int why_size)
{
    int st = JS_RegvmFuncStat(ctx, f, 0), op, pos;

    why[0] = 0;
    if (st == 1 && JS_RegvmStat(ctx, 10) > 0 && JS_RegvmFuncStat(ctx, f, 1) == 0) {
        /* a build with -DRV_STATS counts the frames: converted, never run */
        snprintf(why, why_size, "converted, but no frame of it ran in the register loop");
        return -4;
    }
    if (st != -1)
        return st;
    op = JS_RegvmFuncStat(ctx, f, 2);
    pos = JS_RegvmFuncStat(ctx, f, 3);
    if (op == -1)
        snprintf(why, why_size, "the whole function:%s%s%s%s",
                 pos & 8 ? " stack over 124" : "", pos & 16 ? " frame over 127 slots" : "",
                 pos & 32 ? " over 32 K locals and arguments" : "",
                 pos & 64 ? " stack too deep to fold" : "");
    else if (op == -4)
        snprintf(why, why_size, "an index string past the near constants");
    else if (op == -99)
        snprintf(why, why_size, "its jumps, its frame or the memory of the pass");
    else if (op == -3)
        snprintf(why, why_size, "code no compiler makes, at %d", pos);
    else if (op >= 0 && op < (int)(sizeof(rvexpect_names) / sizeof(rvexpect_names[0])))
        snprintf(why, why_size, "%s at offset %d", rvexpect_names[op], pos);
    else
        snprintf(why, why_size, "op %d at offset %d", op, pos);
    return -1;
}

/* whether the run has the register loop on, for rvExpect */
static int child_regvm;

/* rvExpect(f, ...): with the register loop on, each f must have register
   code by now (the pass converted it), or the run writes a line "expect
   NAME ..." that the other run does not have. With the loop off it does
   nothing. It returns undefined. */
static JSValue js_rv_expect(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    static const char *const state[] = { "never entered", "not a function of bytecode", "made with the register loop off", "refused", "not converted yet: called only once or not at all" };
    int i;

    (void)this_val;
    if (!child_regvm)
        return JS_UNDEFINED;
    for (i = 0; i < argc; i++) {
        char why[160];
        int st = rvexpect_state(ctx, argv[i], why, sizeof(why));
        const char *name = NULL;
        JSValue n;

        if (st == 1)
            continue;
        n = JS_IsObject(argv[i]) ? JS_GetPropertyStr(ctx, argv[i], "name") : JS_UNDEFINED;
        if (JS_IsString(n))
            name = JS_ToCString(ctx, n);
        fprintf(tr, "expect %s (argument %d) has no register code: %s%s%s\n", name && *name ? name : "?", i + 1,
                state[st + 4], *why ? ": " : "", why);
        if (name)
            JS_FreeCString(ctx, name);
        JS_FreeValue(ctx, n);
    }
    return JS_UNDEFINED;
}

static int add_rv_expect(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx);
    int r = JS_SetPropertyStr(ctx, g, "rvExpect", JS_NewCFunction(ctx, js_rv_expect, "rvExpect", 1));

    if (r >= 0)
        r = JS_SetPropertyStr(ctx, g, "rvFusedOps", JS_NewBool(ctx, !opt_no_fused_ops));

    JS_FreeValue(ctx, g);
    return r < 0 ? -1 : 0;
}

static int tr_expect_ops(JSContext *ctx, const char *path);

static int child_run(void *p)
{
    int ops_counted;
    Job *job = p;
    const char *path = job->path;
    JSRuntime *rt;
    JSContext *ctx, *job_ctx;
    JSValue v, format, s;
    char *src;
    size_t len;
    int r, module = is_module(path);
    char dir[4096];
    const char *slash;

    src = read_file(path, &len);
    if (!src) {
        fprintf(stderr, "rvdiff: cannot read %s\n", path);
        return 2;
    }
    if (opt_strict && !module) {
        /* the directive on the first line, so that the lines stay where
           they are */
        static const char d[] = "\"use strict\";";
        char *s2 = malloc(sizeof(d) + len);
        memcpy(s2, d, sizeof(d) - 1);
        memcpy(s2 + sizeof(d) - 1, src, len + 1);
        free(src);
        src = s2;
        len += sizeof(d) - 1;
    }
    rt = host_new_runtime();
    ctx = rt ? JS_NewContext(rt) : NULL;
    if (!ctx || JS_SetShapeCache(ctx, SHAPE_CACHE) < 0 || define_globals(ctx) < 0 || add_print(ctx) < 0 ||
        add_rv_expect(ctx) < 0) {
        fprintf(stderr, "rvdiff: no runtime\n");
        return 2;
    }
    if (opt_regexp_cache >= 0)
        JS_SetRegExpCache(rt, opt_regexp_cache, opt_regexp_cache_bytes);
    if (opt_no_fused_ops)
        JS_SetFusedOps(ctx, 0);
    if (gc_mode == -1)
        JS_SetGCThreshold(rt, (size_t)-1);
    else if (gc_mode >= 0)
        JS_SetGCThreshold(rt, (size_t)gc_mode);
    child_regvm = job->regvm;
    if (module) {
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
    /* the script was a job: it ends here, and with it the hold of the
       targets of its WeakRefs (KeptObjects), as jsrun ends each job */
    JS_ClearKeptObjects(rt);
    if (JS_IsException(v)) {
        tr_exception(ctx, "exception", "stack");
        v = JS_UNDEFINED;
        goto end;
    }
    for (;;) {
        r = JS_ExecutePendingJob(rt, &job_ctx);
        if (r == 0)
            break;
        if (r < 0)
            tr_exception(job_ctx, "job", "job-stack");
        JS_ClearKeptObjects(rt);
    }
    if (module) {
        JSPromiseStateEnum state = JS_PromiseState(ctx, v);

        if (state == JS_PROMISE_REJECTED) {
            JS_Throw(ctx, JS_PromiseResult(ctx, v));
            JS_FreeValue(ctx, v);
            v = JS_UNDEFINED;
            tr_exception(ctx, "exception", "stack");
            goto end;
        }
        if (state == JS_PROMISE_PENDING) {
            JS_FreeValue(ctx, v);
            v = JS_UNDEFINED;
            fprintf(tr, "result <module never finished>\n");
            goto end;
        }
    }
    if (JS_IsFunction(ctx, v)) {
        /* called from C both ways, so that the paths into the function
           are the same */
        JSValue rv = JS_Call(ctx, v, JS_UNDEFINED, 0, NULL);
        JS_ClearKeptObjects(rt);
        JS_FreeValue(ctx, v);
        v = rv;
        if (JS_IsException(v)) {
            tr_exception(ctx, "exception", "stack");
            v = JS_UNDEFINED;
            goto end;
        }
        /* the jobs the function queued */
        while ((r = JS_ExecutePendingJob(rt, &job_ctx)) != 0) {
            if (r < 0)
                tr_exception(job_ctx, "job", "job-stack");
            JS_ClearKeptObjects(rt);
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
        tr_exception(ctx, "exception", "stack");
        goto end;
    }
    s = JS_Call(ctx, format, JS_UNDEFINED, 1, (JSValueConst *)&v);
    JS_FreeValue(ctx, format);
    if (JS_IsException(s)) {
        tr_exception(ctx, "exception", "stack");
    } else {
        tr_value(ctx, "result", s);
        JS_FreeValue(ctx, s);
    }
 end:
    JS_FreeValue(ctx, v);
    /* what the run left: the jobs a format queued too */
    while ((r = JS_ExecutePendingJob(rt, &job_ctx)) != 0) {
        if (r < 0)
            tr_exception(job_ctx, "job", "job-stack");
        JS_ClearKeptObjects(rt);
    }
    /* the format and the calls that end the run were a job too */
    JS_ClearKeptObjects(rt);
    tr_mem(rt, "mem-end");
    JS_RunGC(rt);
    tr_mem(rt, "mem-gc");
    ops_counted = tr_expect_ops(ctx, path);
    fprintf(tr, "--- info\n");
    fprintf(tr, "engine %s\n", RV_ENGINE_STAMP);
    if (!ops_counted)
        fprintf(tr, "rv-expect-op not checked: a build without -DRV_STATS\n");
    {
        JSMemoryUsage u;
        JS_ComputeMemoryUsage(rt, &u);
        fprintf(tr, "bytes %lld blocks %lld\n", (long long)u.malloc_size, (long long)u.malloc_count - 1);
    }
    fprintf(tr, "regvm converted %d refused %d entries %d last-refusal op %d at %d\n",
            JS_RegvmStat(ctx, 0), JS_RegvmStat(ctx, 1), JS_RegvmStat(ctx, 2),
            JS_RegvmStat(ctx, 3), JS_RegvmStat(ctx, 4));
    fprintf(tr, "dispatches new %d old %d calls old>new %d C>new %d new>old %d new>C %d\n",
            JS_RegvmStat(ctx, 10), JS_RegvmStat(ctx, 11), JS_RegvmStat(ctx, 12),
            JS_RegvmStat(ctx, 13), JS_RegvmStat(ctx, 14), JS_RegvmStat(ctx, 15));
    /* what the register loop could not run */
    {
        const char *m;
        int i, n;
        for (i = 0; (m = JS_RegvmCannotRun(ctx, i, &n)) != NULL; i++)
            fprintf(tr, "cannot %d %s\n", n, m);
    }
    fflush(tr);
    JS_SetShapeCache(ctx, 0);
    JS_SetAsyncContext(ctx, JS_UNDEFINED);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    return 0;
}

/* ------------------------------------------------------------ the parent */

typedef struct Script {
    char *path;
    char *opts; /* options of the cases file, or NULL */
} Script;

static Script *scripts;
static int nscripts, cap_scripts;
static const char *out_dir;
static int verbose, quiet, timeout_sec = 60, jobs = 1;
static char ignore[256];
static char global_opts[1024];
/* -record DIR: the goldens of the old loop are written to DIR. -golden DIR:
   one run with the register loop on, compared with DIR. -update: with
   -golden, a golden that differs or is missing is written from the run.
   -root ROOT: the names of the goldens are the paths under ROOT. */
static const char *record_dir, *golden_dir, *golden_root;
static int golden_update;
static int golden_full; /* -full: no kind is kept as its hash */
static int stat_norecord, stat_recorded, stat_nogolden, stat_updated;

static void add_script(const char *path, const char *opts)
{
    if (nscripts == cap_scripts) {
        cap_scripts = cap_scripts ? 2 * cap_scripts : 64;
        scripts = realloc(scripts, sizeof(*scripts) * cap_scripts);
    }
    scripts[nscripts].path = strdup(path);
    scripts[nscripts].opts = opts ? strdup(opts) : NULL;
    nscripts++;
}

static int is_script(const char *name)
{
    size_t n = strlen(name);
    return (n > 3 && !strcmp(name + n - 3, ".js")) || (n > 4 && !strcmp(name + n - 4, ".mjs"));
}

static int cmp_str(const void *a, const void *b)
{
    return strcmp(*(char *const *)a, *(char *const *)b);
}

static int add_path(const char *path)
{
    struct stat st;
    DIR *d;
    struct dirent *e;
    char **names = NULL;
    int n = 0, cap = 0, i;

    if (stat(path, &st) < 0) {
        fprintf(stderr, "rvdiff: %s: %s\n", path, strerror(errno));
        return -1;
    }
    if (!S_ISDIR(st.st_mode)) {
        add_script(path, NULL);
        return 0;
    }
    d = opendir(path);
    if (!d)
        return -1;
    while ((e = readdir(d)) != NULL) {
        if (e->d_name[0] == '.' || !is_script(e->d_name))
            continue;
        if (n == cap) {
            cap = cap ? 2 * cap : 64;
            names = realloc(names, sizeof(*names) * cap);
        }
        names[n] = malloc(strlen(path) + strlen(e->d_name) + 2);
        sprintf(names[n], "%s/%s", path, e->d_name);
        n++;
    }
    closedir(d);
    qsort(names, n, sizeof(*names), cmp_str);
    for (i = 0; i < n; i++) {
        add_script(names[i], NULL);
        free(names[i]);
    }
    free(names);
    return 0;
}

static int read_cases(const char *file)
{
    FILE *f = fopen(file, "r");
    char line[8192];

    if (!f) {
        fprintf(stderr, "rvdiff: cannot read %s\n", file);
        return -1;
    }
    while (fgets(line, sizeof(line), f)) {
        char *p = line, *path, *opts;
        size_t n = strlen(p);
        while (n && (p[n - 1] == '\n' || p[n - 1] == '\r'))
            p[--n] = 0;
        while (*p == ' ' || *p == '\t')
            p++;
        if (!*p || *p == '#')
            continue;
        path = p;
        while (*p && *p != ' ' && *p != '\t')
            p++;
        if (*p)
            *p++ = 0;
        opts = p;
        add_script(path, opts);
    }
    fclose(f);
    return 0;
}

/* the options of jsrun in s, a list split by spaces */
static int set_opts(const char *s)
{
    char buf[1024], *p, *tok;

    snprintf(buf, sizeof(buf), "%s", s);
    for (p = buf; (tok = strsep(&p, " \t")) != NULL;) {
        if (!*tok)
            continue;
        if (!strcmp(tok, "-raw"))
            opt_raw = 1;
        else if (!strcmp(tok, "-result"))
            opt_result = 1;
        else if (!strcmp(tok, "-call"))
            opt_call = 1;
        else if (!strcmp(tok, "-no-fused-ops"))
            opt_no_fused_ops = 1;
        else if (!strcmp(tok, "-strict"))
            opt_strict = 1;
        else if (!strcmp(tok, "-regvm"))
            ; /* the runner sets it */
        else if (!strncmp(tok, "-regexp-cache=", 14)) {
            char *end;
            long n = strtol(tok + 14, &end, 10);
            if (*end != ',' || n < 0)
                return -1;
            opt_regexp_cache = (int)n;
            opt_regexp_cache_bytes = (size_t)strtoll(end + 1, NULL, 10);
        } else
            return -1;
    }
    return 0;
}

/* the rest of the first line of a script that starts with prefix, such as
   "// jsrun: OPTIONS" */
static void script_line(const char *path, const char *prefix, char *buf, size_t size)
{
    FILE *f = fopen(path, "r");
    char line[4096];
    size_t k = strlen(prefix);

    buf[0] = 0;
    if (!f)
        return;
    while (fgets(line, sizeof(line), f)) {
        if (!strncmp(line, prefix, k)) {
            size_t n = strlen(line + k);
            while (n && (line[k + n - 1] == '\n' || line[k + n - 1] == '\r'))
                n--;
            snprintf(buf, size, "%.*s", (int)n, line + k);
            break;
        }
    }
    fclose(f);
}

static void script_opts(const char *path, char *buf, size_t size)
{
    script_line(path, "// jsrun: ", buf, size);
}

/* "// rv-expect-op: NAME ...": with the register loop on, in a build with
   -DRV_STATS, each register op named (JS_RegvmOpName) must have run, or the
   run writes a line "expect op NAME ...". It returns 0 when the build does
   not count the ops, so the line was not checked. */
static int tr_expect_ops(JSContext *ctx, const char *path)
{
    char want[1024], *p, *tok;

    script_line(path, "// rv-expect-op: ", want, sizeof(want));
    if (!*want || !child_regvm)
        return 1;
    /* a counting build counts the dispatches of the old loop of every run */
    if (JS_RegvmStat(ctx, 11) == 0)
        return 0;
    for (p = want; (tok = strsep(&p, " \t,")) != NULL;) {
        int op;

        if (!*tok)
            continue;
        for (op = 0; JS_RegvmOpName(op) && strcmp(JS_RegvmOpName(op), tok); op++)
            ;
        if (!JS_RegvmOpName(op))
            fprintf(tr, "expect op %s: not a register op (JS_RegvmOpName)\n", tok);
        else if (JS_RegvmStat(ctx, 1000 + op) == 0)
            fprintf(tr, "expect op %s ran 0 times in the register loop\n", tok);
    }
    return 1;
}

typedef struct Run {
    pid_t pid;
    FILE *out, *err;
    int status;
    long long t0, ms; /* when it started, and how long it ran */
} Run;

static long long now_ms(void)
{
    struct timespec t;

    clock_gettime(CLOCK_MONOTONIC, &t);
    return (long long)t.tv_sec * 1000 + t.tv_nsec / 1000000;
}

/* the time after which the host or the alarm ends a run, in ms (0: none) */
static long long limit_ms;

static int ran_out(const Run *r)
{
    return (WIFSIGNALED(r->status) && WTERMSIG(r->status) == SIGALRM) ||
        (limit_ms > 0 && r->ms >= limit_ms);
}

static int start_run(Run *r, Script *sc, int regvm)
{
    fflush(stdout);
    fflush(stderr);
    r->out = tmpfile();
    r->err = tmpfile();
    if (!r->out || !r->err)
        return -1;
    r->t0 = now_ms();
    r->pid = fork();
    if (r->pid < 0)
        return -1;
    if (r->pid == 0) {
        char fopts[1024];
        Job job = { sc->path, regvm };
        int st;

        dup2(fileno(r->err), 2);
        tr = r->out;
        signal(SIGALRM, SIG_DFL);
        if (timeout_sec > 0)
            alarm(timeout_sec);
        script_opts(sc->path, fopts, sizeof(fopts));
        if (set_opts(global_opts) < 0 || set_opts(fopts) < 0 ||
            (sc->opts && set_opts(sc->opts) < 0)) {
            fprintf(stderr, "rvdiff: %s: unknown option\n", sc->path);
            _exit(2);
        }
        st = host_run_thread(child_run, &job);
        fflush(tr);
        fflush(stderr);
        /* exit, not _exit: the check of the host for runtimes freed with
           blocks in use (87) and LeakSanitizer run at exit */
        exit(st);
    }
    return 0;
}

static char *slurp(FILE *f, size_t *len)
{
    long n;
    char *buf;

    fflush(f);
    fseek(f, 0, SEEK_END);
    n = ftell(f);
    fseek(f, 0, SEEK_SET);
    buf = malloc(n + 1);
    if (n > 0 && fread(buf, 1, n, f) != (size_t)n)
        n = 0;
    buf[n] = 0;
    *len = n;
    fclose(f);
    return buf;
}

typedef struct Side {
    char *tr, *err;
    size_t tr_len, err_len;
    char status[128];
    char **line;  /* the compared lines of tr */
    int nline;
    char *info;   /* the lines after "--- info", or NULL */
} Side;

static int has_report(const char *err)
{
    return strstr(err, "Sanitizer") != NULL || strstr(err, "runtime error:") != NULL ||
        strstr(err, "host: ") != NULL;
}

static void finish_side(Side *s, Run *r)
{
    char *p;
    int cap = 0;

    s->tr = slurp(r->out, &s->tr_len);
    s->err = slurp(r->err, &s->err_len);
    if (WIFEXITED(r->status))
        snprintf(s->status, sizeof(s->status), "exit %d", WEXITSTATUS(r->status));
    else if (WIFSIGNALED(r->status))
        snprintf(s->status, sizeof(s->status), "signal %d%s", WTERMSIG(r->status),
                 WTERMSIG(r->status) == SIGALRM ? " (time limit)" : "");
    else
        snprintf(s->status, sizeof(s->status), "status %d", r->status);
    if (has_report(s->err))
        strncat(s->status, " with a report on stderr", sizeof(s->status) - strlen(s->status) - 1);
    s->line = NULL;
    s->nline = 0;
    s->info = NULL;
    for (p = s->tr; *p;) {
        char *e = strchr(p, '\n');
        if (e)
            *e = 0;
        if (!strcmp(p, "--- info")) {
            if (e)
                s->info = e + 1;
            break;
        }
        if (s->nline == cap) {
            cap = cap ? 2 * cap : 64;
            s->line = realloc(s->line, sizeof(char *) * cap);
        }
        s->line[s->nline++] = p;
        if (!e)
            break;
        p = e + 1;
    }
}

/* the kind of a line: the word before its first space */
static void line_kind(const char *l, char *kind, size_t size)
{
    size_t n = strcspn(l, " ");
    snprintf(kind, size, "%.*s", (int)(n < size ? n : size - 1), l);
}

/* the kind as reported: mem-end and mem-gc, job and job-stack, out, ... */
static int ignored(const char *kind)
{
    char buf[300], *p, *tok;
    const char *k = kind;

    if (!strcmp(kind, "job-stack"))
        k = "stack";
    snprintf(buf, sizeof(buf), "%s", ignore);
    for (p = buf; (tok = strsep(&p, ",")) != NULL;)
        if (*tok && (!strcmp(tok, kind) || !strcmp(tok, k) ||
                     (!strcmp(tok, "mem") && !strncmp(kind, "mem-", 4))))
            return 1;
    return 0;
}

static const char *const kinds[] = { "out", "result", "exception", "stack", "job", "job-stack", "mem-end", "mem-gc", "expect" };

/* the lines of side s of the kind, in order */
static int kind_lines(Side *s, const char *kind, char ***out)
{
    int i, n = 0;
    char k[64];

    *out = malloc(sizeof(char *) * (s->nline + 1));
    for (i = 0; i < s->nline; i++) {
        line_kind(s->line[i], k, sizeof(k));
        if (!strcmp(k, kind))
            (*out)[n++] = s->line[i];
    }
    return n;
}

static void write_file(const char *dir, const char *name, const char *ext, const char *a, size_t alen,
                       const char *b, size_t blen, const char *status)
{
    char path[4096];
    FILE *f;

    snprintf(path, sizeof(path), "%s/%s.%s", dir, name, ext);
    f = fopen(path, "wb");
    if (!f)
        return;
    fwrite(a, 1, alen, f);
    fprintf(f, "--- status %s\n--- stderr\n", status);
    fwrite(b, 1, blen, f);
    fclose(f);
}

static const char *base_of(const char *path)
{
    const char *s = strrchr(path, '/');
    return s ? s + 1 : path;
}

/* ------------------------------------------------------------ goldens */

/* adds the options of s to the name: each without its dashes, with what is
   not a letter, a digit, '-' or '_' made '_' */
static void name_opts(char *buf, size_t size, const char *s)
{
    char tok[256];
    size_t n;

    while (s && *s) {
        while (*s == ' ')
            s++;
        n = strcspn(s, " ");
        if (n > 0) {
            size_t i, j = 0;
            const char *t = s;
            while (*t == '-' && t < s + n)
                t++;
            for (i = 0; t + i < s + n && j < sizeof(tok) - 1; i++) {
                char c = t[i];
                tok[j++] = (isalnum((unsigned char)c) || c == '-' || c == '_') ? c : '_';
            }
            tok[j] = 0;
            snprintf(buf + strlen(buf), size - strlen(buf), ".%s", tok);
        }
        s += n;
    }
}

/* the file of the golden of sc: the path under -root (a script made into
   engine/test/out/.../gen/ by corpus.sh is gen/NAME), the options, .txt */
static void golden_name(const Script *sc, const char *dir, char *buf, size_t size)
{
    const char *p = sc->path, *g;
    char fopts[1024];
    size_t n = golden_root ? strlen(golden_root) : 0;

    if (n && !strncmp(p, golden_root, n) && p[n] == '/')
        p += n + 1;
    if (!strncmp(p, "engine/test/out/", 16) && (g = strstr(p, "/gen/")) != NULL)
        snprintf(buf, size, "%s/gen/%s", dir, base_of(g));
    else
        snprintf(buf, size, "%s/%s", dir, p[0] == '/' ? base_of(p) : p);
    name_opts(buf, size, global_opts);
    script_opts(sc->path, fopts, sizeof(fopts));
    name_opts(buf, size, fopts);
    name_opts(buf, size, sc->opts);
    snprintf(buf + strlen(buf), size - strlen(buf), ".txt");
}

/* makes the folders of the file path */
static void mkdirs(const char *path)
{
    char buf[4096], *p;

    snprintf(buf, sizeof(buf), "%s", path);
    for (p = buf + 1; (p = strchr(p, '/')) != NULL; p++) {
        *p = 0;
        mkdir(buf, 0777);
        *p = '/';
    }
}

/* a kind whose lines in a golden are more than this many bytes is kept as
   its count and hash */
#define GOLDEN_FULL_BYTES 32768

/* the FNV-1a hash of the lines of the kind (each with its newline), their
   count and their size */
static uint64_t kind_hash(Side *s, const char *kind, int *count, size_t *bytes)
{
    uint64_t h = 0xcbf29ce484222325ULL;
    char **l;
    int n = kind_lines(s, kind, &l), i;
    const unsigned char *p;

    *bytes = 0;
    for (i = 0; i < n; i++) {
        for (p = (const unsigned char *)l[i]; *p; p++)
            h = (h ^ *p) * 0x100000001b3ULL;
        h = (h ^ '\n') * 0x100000001b3ULL;
        *bytes += strlen(l[i]) + 1;
    }
    free(l);
    *count = n;
    return h;
}

/* the line "hash KIND COUNT FNV" of a golden, or NULL */
static const char *golden_hash_line(const Side *s, const char *kind)
{
    size_t n = strlen(kind);
    int i;

    for (i = 0; i < s->nline; i++)
        if (!strncmp(s->line[i], "hash ", 5) && !strncmp(s->line[i] + 5, kind, n) && s->line[i][5 + n] == ' ')
            return s->line[i];
    return NULL;
}

/* writes the compared lines of a side and its status, the status without
   " with a report on stderr" (a run with a report is never written). A
   kind with more than GOLDEN_FULL_BYTES is written as its hash line. */
static int write_golden(const char *path, Side *s)
{
    FILE *f;
    int i, k, big[countof(kinds)], count[countof(kinds)];
    uint64_t h[countof(kinds)];
    char kind[64];

    for (k = 0; k < (int)countof(kinds); k++) {
        size_t bytes;
        h[k] = kind_hash(s, kinds[k], &count[k], &bytes);
        big[k] = !golden_full && bytes > GOLDEN_FULL_BYTES;
    }
    mkdirs(path);
    f = fopen(path, "wb");
    if (!f)
        return -1;
    for (i = 0; i < s->nline; i++) {
        line_kind(s->line[i], kind, sizeof(kind));
        for (k = 0; k < (int)countof(kinds) && strcmp(kind, kinds[k]); k++)
            ;
        if (k == (int)countof(kinds) || !big[k])
            fprintf(f, "%s\n", s->line[i]);
    }
    for (k = 0; k < (int)countof(kinds); k++)
        if (big[k])
            fprintf(f, "hash %s %d %016llx\n", kinds[k], count[k], (unsigned long long)h[k]);
    fprintf(f, "--- status %s\n", s->status);
    return fclose(f) ? -1 : 0;
}

/* a side read from a golden: its lines up to "--- status STATUS" */
static int load_golden(const char *path, Side *s)
{
    FILE *f = fopen(path, "rb");
    char *p;
    int cap = 0;

    memset(s, 0, sizeof(*s));
    if (!f)
        return -1;
    s->tr = slurp(f, &s->tr_len);
    s->err = strdup("");
    snprintf(s->status, sizeof(s->status), "(no status in the golden)");
    for (p = s->tr; *p;) {
        char *e = strchr(p, '\n');
        if (e)
            *e = 0;
        if (!strncmp(p, "--- status ", 11)) {
            snprintf(s->status, sizeof(s->status), "%s", p + 11);
            break;
        }
        if (s->nline == cap) {
            cap = cap ? 2 * cap : 64;
            s->line = realloc(s->line, sizeof(char *) * cap);
        }
        s->line[s->nline++] = p;
        if (!e)
            break;
        p = e + 1;
    }
    return 0;
}

static int stat_mismatch, stat_kind[16], stat_status, stat_lines, stat_slow;
static long long stat_new, stat_old; /* dispatches of the runs with the register loop on */

/* compares the two sides of a script and reports it: 1 when they differ.
   With -golden, off is NULL and the first side is the golden. With -record,
   the side of the old loop is written as the golden when the two agree. */
static int compare(Script *sc, Run *off, Run *on, int index)
{
    Side a, b;
    char diffk[256] = "";
    int differ = 0, i, k, slow = 0;
    char name[512], gname[4096];
    const char *an = golden_dir ? "gold" : "off";
    long long off_ms = off ? off->ms : 0;

    if (golden_dir || record_dir)
        golden_name(sc, golden_dir ? golden_dir : record_dir, gname, sizeof(gname));
    finish_side(&b, on);
    if (golden_dir) {
        if (load_golden(gname, &a) < 0) {
            stat_nogolden++;
            stat_mismatch++;
            printf("DIFF %s%s%s: no golden %s\n", sc->path, sc->opts && *sc->opts ? " " : "",
                   sc->opts ? sc->opts : "", gname);
            if (golden_update && !ran_out(on) && !has_report(b.err) && write_golden(gname, &b) == 0) {
                stat_updated++;
                printf("UPDATE %s\n", gname);
            } else if (golden_update) {
                printf("NOUPDATE %s: %s%s\n", gname, b.status, ran_out(on) ? " (time limit)" : "");
            }
            fflush(stdout);
            free(b.tr);
            free(b.err);
            free(b.line);
            return 1;
        }
    } else {
        finish_side(&a, off);
    }
    if (golden_dir && ran_out(on)) {
        /* no second run to blame: a run the limit ended is a difference */
        printf("SLOW %s%s%s: the time limit ended the run (%lld ms), a difference with no second run\n",
               sc->path, sc->opts && *sc->opts ? " " : "", sc->opts ? sc->opts : "", on->ms);
        slow = 1;
    } else if (!golden_dir && (ran_out(off) || ran_out(on))) {
        const Run *other = ran_out(off) ? on : off;

        if ((ran_out(off) && ran_out(on)) || other->ms * 2 >= limit_ms) {
            stat_slow++;
            printf("SLOW %s%s%s: the time limit ended the run %s (off %lld ms, on %lld ms), not compared\n",
                   sc->path, sc->opts && *sc->opts ? " " : "", sc->opts ? sc->opts : "",
                   ran_out(off) && ran_out(on) ? "both ways" : ran_out(off) ? "with the loop off" : "with the loop on",
                   off->ms, on->ms);
            if (record_dir) {
                stat_norecord++;
                printf("NORECORD %s: too slow\n", gname);
            }
            fflush(stdout);
            free(a.tr);
            free(a.err);
            free(a.line);
            free(b.tr);
            free(b.err);
            free(b.line);
            return 0;
        }
    }
    for (k = 0; k < (int)countof(kinds); k++) {
        char **la, **lb;
        int na, nb, j;

        if (ignored(kinds[k]))
            continue;
        if (golden_dir && golden_hash_line(&a, kinds[k])) {
            /* a kind the golden keeps as its count and hash */
            int ga, cb;
            unsigned long long ha;
            size_t bytes;
            uint64_t hb = kind_hash(&b, kinds[k], &cb, &bytes);

            if (sscanf(golden_hash_line(&a, kinds[k]) + 5 + strlen(kinds[k]), "%d %llx", &ga, &ha) != 2)
                ga = -1;
            if (ga != cb || ha != hb) {
                if (!differ && !quiet)
                    printf("DIFF %s%s%s\n", sc->path, sc->opts && *sc->opts ? " " : "", sc->opts ? sc->opts : "");
                differ = 1;
                stat_kind[k]++;
                snprintf(diffk + strlen(diffk), sizeof(diffk) - strlen(diffk), "%s%s", *diffk ? "," : "", kinds[k]);
                if (!quiet)
                    printf("  %s: %d lines gold, %d on, not the hash of the golden (it keeps the hash only)\n",
                           kinds[k], ga, cb);
            }
            continue;
        }
        na = kind_lines(&a, kinds[k], &la);
        nb = kind_lines(&b, kinds[k], &lb);
        for (j = 0; j < na && j < nb && !strcmp(la[j], lb[j]); j++)
            ;
        if (j < na || j < nb) {
            if (!differ && !quiet)
                printf("DIFF %s%s%s\n", sc->path, sc->opts && *sc->opts ? " " : "", sc->opts ? sc->opts : "");
            differ = 1;
            stat_kind[k]++;
            snprintf(diffk + strlen(diffk), sizeof(diffk) - strlen(diffk), "%s%s", *diffk ? "," : "", kinds[k]);
            if (!quiet) {
                int shown;
                printf("  %s: %d lines %s, %d on, first difference at line %d of the kind\n",
                       kinds[k], na, an, nb, j + 1);
                for (shown = 0; shown < 3 && (j + shown < na || j + shown < nb); shown++) {
                    if (j + shown < na)
                        printf("    %s| %.300s\n", an, la[j + shown]);
                    if (j + shown < nb)
                        printf("    on  | %.300s\n", lb[j + shown]);
                }
            }
        }
        free(la);
        free(lb);
    }
    if (!ignored("status") && (strcmp(a.status, b.status) || slow)) {
        if (!differ && !quiet)
            printf("DIFF %s%s%s\n", sc->path, sc->opts && *sc->opts ? " " : "", sc->opts ? sc->opts : "");
        differ = 1;
        stat_status++;
        snprintf(diffk + strlen(diffk), sizeof(diffk) - strlen(diffk), "%sstatus", *diffk ? "," : "");
        if (!quiet) {
            printf("  status: %s %s, on %s (%s %lld ms, on %lld ms)\n", an, a.status, b.status, an, off_ms, on->ms);
            if (b.err_len)
                printf("    on stderr| %.600s\n", b.err);
            if (a.err_len)
                printf("    off stderr| %.600s\n", a.err);
        }
    } else if (!quiet && (has_report(a.err) || has_report(b.err))) {
        printf("NOTE %s: both runs %s\n", sc->path, a.status);
    }
    if (differ) {
        stat_mismatch++;
        if (quiet)
            printf("DIFF %s%s%s: %s\n", sc->path, sc->opts && *sc->opts ? " " : "", sc->opts ? sc->opts : "", diffk);
    } else if (verbose) {
        const char *info = b.info ? strstr(b.info, "regvm ") : NULL;
        printf("ok %s", sc->path);
        if (info) {
            const char *e = strchr(info, '\n');
            printf(" (%.*s)", e ? (int)(e - info) : (int)strlen(info), info);
        }
        printf("\n");
    }
    if (record_dir) {
        if (differ || has_report(a.err) || has_report(b.err)) {
            stat_norecord++;
            printf("NORECORD %s: %s\n", gname, differ ? "the loops differ" : "a sanitizer or host report");
        } else if (write_golden(gname, &a) < 0) {
            stat_norecord++;
            printf("NORECORD %s: cannot write it: %s\n", gname, strerror(errno));
        } else {
            stat_recorded++;
        }
    }
    if (golden_dir && golden_update && differ && !slow && !has_report(b.err)) {
        if (write_golden(gname, &b) == 0) {
            stat_updated++;
            printf("UPDATE %s\n", gname);
        }
    }
    if (out_dir && (differ || verbose)) {
        snprintf(name, sizeof(name), "%03d-%s", index, base_of(sc->path));
        /* the lines were cut at their ends: put them back */
        for (i = 0; i < (int)a.tr_len; i++)
            if (!a.tr[i])
                a.tr[i] = '\n';
        for (i = 0; i < (int)b.tr_len; i++)
            if (!b.tr[i])
                b.tr[i] = '\n';
        write_file(out_dir, name, an, a.tr, a.tr_len, a.err, a.err_len, a.status);
        write_file(out_dir, name, "on", b.tr, b.tr_len, b.err, b.err_len, b.status);
    }
    stat_lines += a.nline;
    if (b.info) {
        const char *dsp = strstr(b.info, "dispatches new ");
        long long nn, oo;
        if (dsp && sscanf(dsp, "dispatches new %lld old %lld", &nn, &oo) == 2) {
            stat_new += nn;
            stat_old += oo;
        }
    }
    fflush(stdout);
    free(a.tr);
    free(a.err);
    free(a.line);
    free(b.tr);
    free(b.err);
    free(b.line);
    return differ;
}

typedef struct Slot {
    int script;
    Run run[2];
    int left; /* runs not yet waited for */
} Slot;

int main(int argc, char **argv)
{
    int i, next = 0, done = 0, nslots;
    Slot *slots;

    for (i = 1; i < argc && argv[i][0] == '-' && argv[i][1]; i++) {
        const char *a = argv[i];
        if (!strcmp(a, "-o") && i + 1 < argc) {
            out_dir = argv[++i];
            mkdir(out_dir, 0777);
        } else if (!strcmp(a, "-v")) {
            verbose = 1;
        } else if (!strcmp(a, "-q")) {
            quiet = 1;
        } else if (!strcmp(a, "-t") && i + 1 < argc) {
            timeout_sec = atoi(argv[++i]);
        } else if (!strcmp(a, "-j") && i + 1 < argc) {
            jobs = atoi(argv[++i]);
            if (jobs < 1)
                jobs = 1;
        } else if (!strcmp(a, "-gc") && i + 1 < argc) {
            i++;
            gc_mode = !strcmp(argv[i], "off") ? -1 : !strcmp(argv[i], "auto") ? -2 : atol(argv[i]);
        } else if (!strcmp(a, "-ignore") && i + 1 < argc) {
            snprintf(ignore, sizeof(ignore), "%s", argv[++i]);
        } else if (!strcmp(a, "-cases") && i + 1 < argc) {
            if (read_cases(argv[++i]) < 0)
                return 2;
        } else if (!strcmp(a, "-record") && i + 1 < argc) {
            record_dir = argv[++i];
        } else if (!strcmp(a, "-golden") && i + 1 < argc) {
            golden_dir = argv[++i];
        } else if (!strcmp(a, "-update")) {
            golden_update = 1;
        } else if (!strcmp(a, "-full")) {
            golden_full = 1;
        } else if (!strcmp(a, "-root") && i + 1 < argc) {
            golden_root = argv[++i];
        } else {
            /* an option of jsrun, for every script */
            size_t n = strlen(global_opts);
            snprintf(global_opts + n, sizeof(global_opts) - n, "%s%s", n ? " " : "", a);
            if (set_opts(a) < 0) {
                fprintf(stderr, "rvdiff: unknown option %s\n", a);
                return 2;
            }
            opt_raw = opt_result = opt_call = opt_no_fused_ops = 0;
            opt_strict = 0;
            opt_regexp_cache = -1;
        }
    }
    for (; i < argc; i++)
        if (add_path(argv[i]) < 0)
            return 2;
    if (nscripts == 0 || (record_dir && golden_dir) || (golden_update && !golden_dir)) {
        fprintf(stderr, "usage: rvdiff [-o DIR] [-v] [-q] [-t SECONDS] [-j N] [-ignore KIND,...] [-gc off|auto|BYTES] [-cases FILE] [-record DIR | -golden DIR [-update]] [-full] [-root ROOT] [OPTIONS of jsrun] PATH...\n");
        return 2;
    }
    if (!stamp_fresh())
        return 2;
    if (!golden_dir) {
        /* the two runs compared and -record read the old loop, which is
           gone: only the run against the goldens is left */
        fprintf(stderr, "rvdiff: one loop only: give -golden DIR\n");
        return 2;
    }
    /* the time limit of the host, so that a script that runs too long ends
       with "interrupted" where the engine polls, before the alarm */
    if (!getenv("HOST_TIMEOUT_SEC")) {
        char t[32];
        snprintf(t, sizeof(t), "%d", timeout_sec > 5 ? timeout_sec - 5 : timeout_sec);
        setenv("HOST_TIMEOUT_SEC", t, 1);
    }
    limit_ms = atol(getenv("HOST_TIMEOUT_SEC")) * 1000LL;
    if (limit_ms <= 0 || (timeout_sec > 0 && timeout_sec * 1000LL < limit_ms))
        limit_ms = timeout_sec * 1000LL;
    nslots = jobs;
    slots = calloc(nslots, sizeof(*slots));
    for (i = 0; i < nslots; i++)
        slots[i].script = -1;
    while (done < nscripts) {
        int status;
        pid_t pid;

        for (i = 0; i < nslots && next < nscripts; i++) {
            if (slots[i].script >= 0)
                continue;
            slots[i].script = next++;
            /* with -golden only the run with the register loop on */
            slots[i].left = golden_dir ? 1 : 2;
            slots[i].run[0].pid = 0;
            if ((!golden_dir && start_run(&slots[i].run[0], &scripts[slots[i].script], 0) < 0) ||
                start_run(&slots[i].run[1], &scripts[slots[i].script], 1) < 0) {
                fprintf(stderr, "rvdiff: cannot start a run: %s\n", strerror(errno));
                free(slots);
                return 2;
            }
        }
        pid = wait(&status);
        if (pid < 0) {
            fprintf(stderr, "rvdiff: wait: %s\n", strerror(errno));
            free(slots);
            return 2;
        }
        for (i = 0; i < nslots; i++) {
            int m;
            if (slots[i].script < 0)
                continue;
            for (m = 0; m < 2; m++)
                if (slots[i].run[m].pid == pid && pid > 0) {
                    slots[i].run[m].status = status;
                    slots[i].run[m].ms = now_ms() - slots[i].run[m].t0;
                    slots[i].run[m].pid = 0;
                    slots[i].left--;
                }
            if (slots[i].left == 0) {
                compare(&scripts[slots[i].script], golden_dir ? NULL : &slots[i].run[0], &slots[i].run[1],
                        slots[i].script);
                slots[i].script = -1;
                done++;
            }
        }
    }
    printf("rvdiff: engine %s\n", RV_ENGINE_STAMP);
    printf("rvdiff: %d scripts, %d differ", nscripts, stat_mismatch);
    if (stat_mismatch) {
        int k, first = 1;
        printf(" (");
        for (k = 0; k < (int)countof(kinds); k++)
            if (stat_kind[k]) {
                printf("%s%s %d", first ? "" : ", ", kinds[k], stat_kind[k]);
                first = 0;
            }
        if (stat_status)
            printf("%sstatus %d", first ? "" : ", ", stat_status);
        printf(")");
    }
    if (stat_slow)
        printf(", %d too slow to compare", stat_slow);
    if (golden_dir)
        printf(", against the goldens of %s (%d missing, %d written)", golden_dir, stat_nogolden, stat_updated);
    if (record_dir)
        printf(", %d goldens recorded in %s, %d not recorded", stat_recorded, record_dir, stat_norecord);
    printf("\n");
    if (stat_new + stat_old > 0)
        printf("rvdiff: with the register loop on, %lld dispatches ran in the register loop and %lld in the old loop (%.2f%% in the register loop)\n",
               stat_new, stat_old, 100.0 * stat_new / (stat_new + stat_old));
    free(slots);
    return stat_mismatch ? 1 : 0;
}
