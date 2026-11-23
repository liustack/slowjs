/*
 * Copyright (c) 2026 The liustack/tifajs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE at the
 * root of this repository.
 */

/* js-fuzz is the libFuzzer target of the engine, built by build.sh fuzz with
   AddressSanitizer (the engine's allocator poisons what is free in it) and
   UndefinedBehaviorSanitizer: it finds the memory errors the Go build cannot
   see. It runs what the fuzz targets of the Go API run (fuzz_test.go,
   fuzzregexp_test.go and fuzzjson_test.go in the root package), with the
   same scripts, in a runtime of its own for each input.

   The first byte of an input says what the rest is:

     bits 0-1  0 a script, 1 a module, 2 a regexp, 3 a JSON text
     bits 2-3  the memory limit: 32 MiB, 4 MiB, 1 MiB or 384 KiB
     bit 4     a script or module is compiled, written with JS_WriteObject,
               read again with JS_ReadObject and run from that
     bit 5     a script or module runs after a stand-in for the harness of
               test262 (prelude_script), so that a test262 file goes on past
               its first assert

   A regexp is a byte of flags (dgimsuvy, one bit each), then the pattern, a
   0 byte and the subject.

   A run stops after HOST_FUZZ_POLLS readings of the interrupt flag, as the
   eval timeout of a VM does. What it checks aborts, which libFuzzer reports
   with the input: a call that returned a value with a catchable exception
   pending, a runtime freed with blocks still in use, a runtime the host still
   knows, or memory still mapped once it is freed. A run that does not stop
   at all is a timeout of libFuzzer (-timeout).

     engine/test/build.sh fuzz
     engine/test/out/fuzz/js-fuzz -timeout=30 -rss_limit_mb=4096 CORPUS */
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "host.h"

/* DefaultShapeCache of the Go API */
#define SHAPE_CACHE 2048
/* readings of the interrupt flag before a run is stopped */
#define HOST_FUZZ_POLLS 200000
/* the most jobs a run drains, as ExecutePendingJobs bounds them */
#define MAX_JOBS 4096

static const size_t limits[] = { 32 << 20, 4 << 20, 1 << 20, 384 << 10 };

/* The scripts of fuzzregexp_test.go and fuzzjson_test.go, with the name of
   the error they throw when a check fails. */
#define CONTRACT "FuzzContract"

static const char regexp_script[] =
    "(() => {\n"
    "  const fail = (what) => { const e = new Error(what); e.name = '" CONTRACT "'; throw e }\n"
    "  const re = new RegExp(P, F)\n"
    "  const out = [re.source, re.flags, String(re)]\n"
    "  const again = new RegExp(re.source, re.flags)\n"
    "  if (again.source !== re.source || again.flags !== re.flags) fail('the source compiles again to another')\n"
    "  const first = re.exec(S)\n"
    "  re.lastIndex = 0\n"
    "  const second = again.exec(S)\n"
    "  if (JSON.stringify(first) !== JSON.stringify(second)) fail('the source compiled again matches otherwise')\n"
    "  if (first) out.push(first.index, first.length, first.groups && Object.keys(first.groups), first.indices && first.indices.length)\n"
    "  re.lastIndex = 0\n"
    "  out.push(re.test(S), re.lastIndex)\n"
    "  re.lastIndex = 0\n"
    "  out.push(S.search(re), S.replace(re, '[$&|$1|$<n>|$`|$\\']'), S.replace(re, (...a) => a.length))\n"
    "  out.push(S.split(re, 32).length, S.match(re))\n"
    "  if (re.global) {\n"
    "    let n = 0\n"
    "    for (const m of S.matchAll(re)) if (++n > 4096) break\n"
    "    out.push(n, S.replaceAll(re, '$$'))\n"
    "  }\n"
    "  re.lastIndex = S.length + 1\n"
    "  out.push(re.exec(S))\n"
    "  return JSON.stringify(out)\n"
    "})()";

static const char json_script[] =
    "(() => {\n"
    "  const fail = (what) => { const e = new Error(what); e.name = '" CONTRACT "'; throw e }\n"
    "  let v\n"
    "  try { v = JSON.parse(T) } catch (e) { if (e instanceof SyntaxError) return undefined; throw e }\n"
    "  const s = JSON.stringify(v)\n"
    "  if (JSON.stringify(JSON.parse(s)) !== s) fail('the text JSON.stringify wrote parses to another value')\n"
    "  const pretty = JSON.stringify(v, null, '\\t')\n"
    "  if (JSON.stringify(JSON.parse(pretty)) !== s) fail('the indented text parses to another value')\n"
    "  const keys = []\n"
    "  const revived = JSON.parse(T, function (k, x) { if (keys.length < 4096) keys.push(k); return x })\n"
    "  if (JSON.stringify(revived) !== s) fail('a reviver that changes nothing changed the value')\n"
    "  const filtered = JSON.stringify(v, (k, x) => typeof x === 'number' ? String(x) : x)\n"
    "  JSON.parse(filtered)\n"
    "  return s\n"
    "})()";

/* What the files of test262 call from their harness, in a few lines: the
   asserts throw when they fail, the helpers that check properties do
   nothing. */
static const char prelude_script[] =
    "function Test262Error(m) { this.message = m }\n"
    "Test262Error.thrower = (m) => { throw new Test262Error(m) }\n"
    "var assert = (c, m) => { if (c !== true) throw new Test262Error(m) }\n"
    "assert.sameValue = (a, b, m) => { if (!Object.is(a, b)) throw new Test262Error(m) }\n"
    "assert.notSameValue = (a, b, m) => { if (Object.is(a, b)) throw new Test262Error(m) }\n"
    "assert.throws = (C, f, m) => { try { f() } catch (e) { return } throw new Test262Error(m) }\n"
    "assert.compareArray = (a, b, m) => { if (String(a) !== String(b)) throw new Test262Error(m) }\n"
    "assert.deepEqual = assert.compareArray\n"
    "var compareArray = (a, b) => String(a) === String(b)\n"
    "var verifyProperty = () => true, verifyNotEnumerable = () => {}, verifyNotWritable = () => {},\n"
    "  verifyNotConfigurable = () => {}, verifyEnumerable = () => {}, verifyWritable = () => {},\n"
    "  verifyConfigurable = () => {}, verifyPrimordialProperty = () => true, verifyCallableProperty = () => true\n"
    "var $DONE = (e) => { if (e) throw e }, $DONOTEVALUATE = () => { throw 'Test262: This statement should not be evaluated.' }\n"
    "var asyncTest = (f) => f().then(() => $DONE(), $DONE)\n"
    "var print = () => {}, $262 = { global: globalThis, gc() {}, detachArrayBuffer() {}, evalScript: (s) => (0, eval)(s), createRealm() { return $262 }, agent: {} }\n"
    "var testWithTypedArrayConstructors = (f) => [Int8Array, Uint8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array, Uint32Array, Float32Array, Float64Array].forEach((C) => f(C))\n"
    "var isConstructor = (f) => { try { Reflect.construct(String, [], f) } catch { return false } return true }\n";

static void die(const char *what)
{
    fprintf(stderr, "js-fuzz: %s\n", what);
    abort();
}

/* settle checks the result of a call and frees it. A value comes with no
   catchable exception pending: an uncatchable one may be, which the Go API
   turns into the error of the call (settled). An exception thrown by a check
   of the scripts aborts. It returns 0 when the runtime can take more calls. */
static int settle(JSContext *ctx, JSValue v)
{
    JSValue e;
    const char *name;
    int contract;

    if (!JS_IsException(v)) {
        JS_FreeValue(ctx, v);
        if (JS_HasException(ctx) && !JS_IsUncatchableException(ctx))
            die("a call returned a value with an exception pending");
        return JS_IsUncatchableException(ctx) ? -1 : 0;
    }
    if (JS_IsUncatchableException(ctx)) {
        JS_FreeValue(ctx, JS_GetException(ctx));
        return -1;
    }
    e = JS_GetException(ctx);
    contract = 0;
    if (JS_IsObject(e)) {
        JSValue n = JS_GetPropertyStr(ctx, e, "name");
        name = JS_ToCString(ctx, n);
        if (name) {
            contract = strcmp(name, CONTRACT) == 0;
            JS_FreeCString(ctx, name);
        }
        JS_FreeValue(ctx, n);
        if (contract) {
            JSValue m = JS_GetPropertyStr(ctx, e, "message");
            const char *s = JS_ToCString(ctx, m);
            fprintf(stderr, "js-fuzz: %s\n", s ? s : "(no message)");
            die("a check of the script failed");
        }
    }
    JS_FreeValue(ctx, e);
    /* what reading the error threw */
    if (JS_HasException(ctx)) {
        int u = JS_IsUncatchableException(ctx);
        JS_FreeValue(ctx, JS_GetException(ctx));
        if (u)
            return -1;
    }
    return 0;
}

/* run_jobs drains the jobs as ExecutePendingJobs does: a job that throws does
   not stop it, an uncatchable exception does. */
static void run_jobs(JSRuntime *rt, JSContext *ctx)
{
    JSContext *job_ctx;
    int i, r;

    for (i = 0; i < MAX_JOBS; i++) {
        JS_ClearKeptObjects(rt);
        r = JS_ExecutePendingJob(rt, &job_ctx);
        if (r == 0)
            return;
        if (r > 0 && JS_IsUncatchableException(ctx))
            r = -1;
        if (r < 0) {
            int u = JS_IsUncatchableException(ctx);
            JS_FreeValue(ctx, JS_GetException(ctx));
            if (u)
                return;
        }
    }
}

/* set_string sets the global name to the UTF-8 text s of n bytes. */
static int set_string(JSContext *ctx, const char *name, const char *s, size_t n)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue v = JS_NewStringLen(ctx, s, n);
    int r;

    if (JS_IsException(v)) {
        JS_FreeValue(ctx, g);
        JS_FreeValue(ctx, JS_GetException(ctx));
        return -1;
    }
    r = JS_SetPropertyStr(ctx, g, name, v);
    JS_FreeValue(ctx, g);
    if (r < 0) {
        JS_FreeValue(ctx, JS_GetException(ctx));
        return -1;
    }
    return 0;
}

/* run_source runs src as a script or a module, then its jobs. */
static void run_source(JSRuntime *rt, JSContext *ctx, const char *src, size_t n, int module, int bytecode)
{
    int flags = module ? JS_EVAL_TYPE_MODULE : JS_EVAL_TYPE_GLOBAL;
    JSValue v;

    if (!bytecode) {
        v = JS_Eval(ctx, src, n, "<eval>", flags);
    } else {
        JSValue obj = JS_Eval(ctx, src, n, "<eval>", flags | JS_EVAL_FLAG_COMPILE_ONLY);
        uint8_t *buf;
        size_t len;

        if (JS_IsException(obj)) {
            settle(ctx, obj);
            return;
        }
        buf = JS_WriteObject(ctx, &len, obj, JS_WRITE_OBJ_BYTECODE);
        JS_FreeValue(ctx, obj);
        if (!buf) {
            settle(ctx, JS_EXCEPTION);
            return;
        }
        obj = JS_ReadObject(ctx, buf, len, JS_READ_OBJ_BYTECODE);
        js_free(ctx, buf);
        if (JS_IsException(obj)) {
            settle(ctx, obj);
            return;
        }
        if (module && JS_ResolveModule(ctx, obj) < 0) {
            JS_FreeValue(ctx, obj);
            settle(ctx, JS_EXCEPTION);
            return;
        }
        v = JS_EvalFunction(ctx, obj);
    }
    if (settle(ctx, v) == 0)
        run_jobs(rt, ctx);
}

static void run_one(const uint8_t *data, size_t size)
{
    JSRuntime *rt;
    JSContext *ctx;
    int kind, leaks;
    const char *text;
    size_t n;

    if (size < 1)
        return;
    kind = data[0] & 3;
    text = (const char *)data + 1;
    n = size - 1;
    leaks = host_leaked_runtimes();
    rt = host_new_runtime();
    if (!rt)
        die("no runtime");
    JS_SetMemoryLimit(rt, limits[(data[0] >> 2) & 3]);
    ctx = JS_NewContext(rt);
    if (!ctx) {
        /* the limit is below what a context needs */
        host_free_runtime(rt);
        return;
    }
    if (JS_SetShapeCache(ctx, SHAPE_CACHE) < 0) {
        JS_FreeValue(ctx, JS_GetException(ctx));
        goto done;
    }
    host_poll_stop_after(HOST_FUZZ_POLLS);
    switch (kind) {
    case 0:
    case 1: {
        /* JS_Eval wants a 0 after the source */
        char *src = malloc(n + 1);
        if (!src)
            die("no memory");
        memcpy(src, text, n);
        src[n] = 0;
        if ((data[0] >> 5) & 1)
            settle(ctx, JS_Eval(ctx, prelude_script, sizeof(prelude_script) - 1, "<harness>", JS_EVAL_TYPE_GLOBAL));
        run_source(rt, ctx, src, n, kind == 1, (data[0] >> 4) & 1);
        free(src);
        break;
    }
    case 2: {
        static const char letters[] = "dgimsuvy";
        char flags[9];
        const char *zero;
        size_t i, k = 0;

        if (n < 1)
            break;
        for (i = 0; i < 8; i++) {
            if (text[0] & (1 << i))
                flags[k++] = letters[i];
        }
        text++, n--;
        zero = memchr(text, 0, n);
        if (!zero)
            zero = text + n;
        if (set_string(ctx, "P", text, zero - text) < 0 ||
            set_string(ctx, "F", flags, k) < 0 ||
            set_string(ctx, "S", zero < text + n ? zero + 1 : zero, zero < text + n ? text + n - zero - 1 : 0) < 0)
            break;
        settle(ctx, JS_Eval(ctx, regexp_script, sizeof(regexp_script) - 1, "<eval>", JS_EVAL_TYPE_GLOBAL));
        break;
    }
    case 3:
        if (set_string(ctx, "T", text, n) < 0)
            break;
        settle(ctx, JS_Eval(ctx, json_script, sizeof(json_script) - 1, "<eval>", JS_EVAL_TYPE_GLOBAL));
        break;
    }
 done:
    host_poll_stop_after(0);
    /* as VM.Close does: the kept shapes and the async context can hold the
       context */
    (void)JS_SetShapeCache(ctx, 0);
    JS_SetAsyncContext(ctx, JS_UNDEFINED);
    JS_FreeContext(ctx);
    host_free_runtime(rt);
    if (host_leaked_runtimes() != leaks)
        die("the runtime was freed with blocks still in use");
    if (host_live_vms() != 0)
        die("the host still knows a runtime that was freed");
    if (host_mapped_bytes() != 0)
        die("the host still maps memory of a runtime that was freed");
}

typedef struct {
    const uint8_t *data;
    size_t size;
} Input;

static int run_thread(void *p)
{
    Input *in = p;

    run_one(in->data, in->size);
    return 0;
}

int LLVMFuzzerTestOneInput(const uint8_t *data, size_t size)
{
    Input in = { data, size };

    /* a script may recurse as deep as the engine allows, which the main
       thread's 8 MiB do not hold under the sanitizers */
    host_run_thread(run_thread, &in);
    return 0;
}
