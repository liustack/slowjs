/*
 * Copyright (c) 2026 The liustack/beastjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE at the
 * root of this repository.
 */

/* unit-api checks through the public API what scripts cannot see: what the
   runtime holds after errors and their stacks, what JS_ParseJSON does with
   bytes that are not UTF-8, and a property a host puts on a proxy. They took
   over the tests that did this on the engine before it (backtraces, JSON,
   error stacks, shared debug information).

     unit-api */
#include <pthread.h>

#include "cutils.h"
#include "testutil.h"

/* usage runs the collector and returns the memory usage of the runtime. */
static JSMemoryUsage usage(VM *v)
{
    JSMemoryUsage u;

    JS_RunGC(v->rt);
    JS_ComputeMemoryUsage(v->rt, &u);
    return u;
}

/* ---- pending stacks and memory ---- */

/* Reading the stacks of errors kept alive must not give each error a shape of
   its own: the formatted stack is a plain data property, as when the stack
   was formatted at once. The second variant gives each error another property
   while its stack is pending. */
static void test_backtrace_shares_shape(void)
{
    static const struct { const char *src; int n; } cases[] = {
        { "var held\n"
          "function make() { held = []; for (var i = 0; i < 10000; i++) held.push(new Error(\"kept\")) }\n"
          "function read() { for (var e of held) void e.stack }\n", 10000 },
        { "var held\n"
          "function make() { held = []; for (var i = 0; i < 1000; i++) { var e = new Error(\"kept\"); e.code = i; held.push(e) } }\n"
          "function read() { for (var e of held) void e.stack }\n", 1000 },
    };
    int i;

    for (i = 0; i < 2; i++) {
        VM v = vm_new();
        JSMemoryUsage pending, read;

        eval_ok(v.ctx, cases[i].src);
        JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
        pending = usage(&v);
        JS_FreeValue(v.ctx, call_global(v.ctx, "read"));
        read = usage(&v);
        CHECK(read.shape_count <= pending.shape_count + 2, "reading %d stacks added %lld shapes", cases[i].n,
              (long long)(read.shape_count - pending.shape_count));
        vm_free(&v);
    }
}

/* Replacing an error's prototype must not keep the old prototype, and the
   object graph behind it, alive while the stack is pending. */
static void test_backtrace_old_prototype(void)
{
    VM v = vm_new();
    JSMemoryUsage u;

    eval_ok(v.ctx,
            "var kept\n"
            "function make() {\n"
            "\tfunction Target() {}\n"
            "\tTarget.prototype = { payload: new ArrayBuffer(4 * 1024 * 1024) }\n"
            "\tvar e = Reflect.construct(Error, [\"retained prototype\"], Target)\n"
            "\tObject.setPrototypeOf(e, null)\n"
            "\tkept = e\n"
            "}\n");
    JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
    u = usage(&v);
    /* the 4 MiB ArrayBuffer of the old prototype must already be gone */
    CHECK(u.memory_used_size <= 2 << 20, "a pending error keeps %lld bytes, the old prototype is retained", (long long)u.memory_used_size);
    vm_free(&v);
}

/* Printing an error reads its stack from C. That read formats the stack once
   and keeps the text, like a read from JS: the second read gives the same
   string. */
static void test_backtrace_native_read_caches(void)
{
    VM v = vm_new();
    JSAtom a;
    JSValue e, s1, s2;

    eval_ok(v.ctx, "function deep(n) { return n ? deep(n - 1) : new Error(\"deep\") }\nfunction make() { return deep(40) }\n");
    e = call_global(v.ctx, "make");
    a = JS_NewAtom(v.ctx, "stack");
    s1 = JS_GetProperty(v.ctx, e, a);
    s2 = JS_GetProperty(v.ctx, e, a);
    CHECK(JS_IsString(s1) && JS_IsString(s2), "the stack is not a string");
    CHECK(JS_VALUE_GET_PTR(s1) == JS_VALUE_GET_PTR(s2), "each read formatted the stack again");
    JS_FreeValue(v.ctx, s1);
    JS_FreeValue(v.ctx, s2);
    JS_FreeAtom(v.ctx, a);
    JS_FreeValue(v.ctx, e);
    vm_free(&v);
}

/* Errors whose stack is never read are freed, with the functions in their
   stack, including when an error and a closure from its own stack reference
   each other. */
static void test_error_stack_memory(void)
{
    VM v = vm_new();
    JSMemoryUsage base, held, freed;

    eval_ok(v.ctx,
            "function deep(n, f) { return n == 0 ? f() : deep(n - 1, f) }\n"
            "var kept\n"
            "function make(n) {\n"
            "\tkept = []\n"
            "\tfor (var i = 0; i < n; i++) {\n"
            "\t\tkept.push(deep(20, function () {\n"
            "\t\t\t// a function whose bytecode only the error keeps\n"
            "\t\t\tvar err = new Function(\"return new Error('e')\")()\n"
            "\t\t\t// the closure holds the error, and the error holds the closure's frame\n"
            "\t\t\terr.self = function () { return err }\n"
            "\t\t\treturn err\n"
            "\t\t}))\n"
            "\t}\n"
            "\treturn kept.length\n"
            "}\n"
            "function drop() { kept = undefined }\n"
            "function make1() { make(1) }\n"
            "function make2000() { make(2000) }\n");
    /* One round first, so that what the first call leaves behind for good
       (shapes, caches) is part of the baseline. */
    JS_FreeValue(v.ctx, call_global(v.ctx, "make1"));
    JS_FreeValue(v.ctx, call_global(v.ctx, "drop"));
    base = usage(&v);
    JS_FreeValue(v.ctx, call_global(v.ctx, "make2000"));
    held = usage(&v);
    JS_FreeValue(v.ctx, call_global(v.ctx, "drop"));
    freed = usage(&v);
    CHECK(held.obj_count - base.obj_count >= 2000, "the errors were not kept: %lld objects", (long long)(held.obj_count - base.obj_count));
    CHECK(freed.obj_count == base.obj_count, "%lld objects left after freeing the errors", (long long)(freed.obj_count - base.obj_count));
    /* the allocator may keep a little memory for reuse */
    CHECK(freed.malloc_size - base.malloc_size <= 64 << 10, "%lld bytes left after freeing the errors", (long long)(freed.malloc_size - base.malloc_size));
    vm_free(&v);
}

/* An error made while a script runs, and kept after it ends, does not keep the
   code of the script alive. Libraries make such errors when they load (React
   keeps one to signal suspense). The size of the functions kept after a
   script is the same whether the script keeps an object or an error. */
static int64_t kept_functions(const char *make)
{
    VM v = vm_new();
    JSMemoryUsage before, after;
    char *script = malloc(200 * 120 + 400);
    size_t len = 0;
    int i;
    int64_t r;

    before = usage(&v);
    len += sprintf(script, "(function load() {\n");
    for (i = 0; i < 200; i++)
        len += sprintf(script + len, "\tfunction f%d(x) { return [x, %d, 'padding to make the code larger'] }\n", i, i);
    len += sprintf(script + len, "\tfunction make() { return %s }\n\tglobalThis.sentinel = make()\n})()\n", make);
    eval_ok(v.ctx, script);
    after = usage(&v);
    r = after.js_func_size + after.js_func_code_size + after.js_func_pc2line_size -
        before.js_func_size - before.js_func_code_size - before.js_func_pc2line_size;
    free(script);
    vm_free(&v);
    return r;
}

static void test_error_stack_top_level(void)
{
    int64_t object = kept_functions("{ message: \"sentinel\" }");
    int64_t err = kept_functions("new Error(\"sentinel\")");

    CHECK(err == object, "keeping an error made by the script keeps %lld bytes of functions, want %lld", (long long)err, (long long)object);
}

/* ---- a property of the host on a proxy ---- */

/* A property the host puts on the proxy itself, with
   JS_SetPropertyFunctionList, is an own property of the proxy: lookups of it,
   full or of the attributes only, find it before the handler and never call a
   trap. Revoking the proxy changes nothing. */
static void test_proxy_own_property(void)
{
    int revoke;

    for (revoke = 0; revoke < 2; revoke++) {
        VM v = vm_new();
        JSValue p;
        static const JSCFunctionListEntry list[] = {
            JS_PROP_STRING_DEF("stack", "host value", JS_PROP_C_W_E),
        };

        eval_ok(v.ctx,
                "var log = []\n"
                "var r = Proxy.revocable({}, { getOwnPropertyDescriptor(t, k) { log.push(k); return undefined } })\n"
                "var p = r.proxy");
        p = JS_Eval(v.ctx, "p", 1, "<unit>", JS_EVAL_TYPE_GLOBAL);
        CHECK(JS_SetPropertyFunctionList(v.ctx, p, list, 1) >= 0, "JS_SetPropertyFunctionList");
        JS_FreeValue(v.ctx, p);
        if (revoke)
            eval_ok(v.ctx, "r.revoke()");
        CHECK(eval_is(v.ctx,
                      "JSON.stringify([\n"
                      "\tObject.getOwnPropertyDescriptor(p, \"stack\").value,\n"
                      "\t(() => { try { return Object.prototype.propertyIsEnumerable.call(p, \"stack\") } catch (e) { return String(e) } })(),\n"
                      "\tlog])",
                      "[\"host value\",true,[]]"),
              "revoked %d", revoke);
        vm_free(&v);
    }
}

/* ---- JSON.parse of bytes that are not UTF-8 ---- */

/* JSON.parse of UTF-8 that is not valid gives the code units of the string, or
   the error and where. A string of javascript always converts to valid UTF-8
   (a lone surrogate to its three bytes), but the C API's JS_ParseJSON may get
   any bytes: each kind of byte a UTF-8 reader treats apart, valid or not. */
static void test_json_bytes(void)
{
    static const struct { const char *text; size_t len; const char *want; } cases[] = {
#define C(s, w) { s, sizeof(s) - 1, w }
        C("\"a" "\xc3\xa9" "b\"", "3: 61 e9 62"),
        C("\"\xc2\x80\xdf\xbf\"", "2: 80 7ff"),
        C("\"\xc0\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xc1\xbf\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe0\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe0\xa0\x80\"", "1: 800"),
        C("\"\xed\xa0\x80" "x\"", "2: d800 78"),
        C("\"\xed\xbf\xbf\"", "1: dfff"),
        C("\"\xef\xbf\xbf\"", "1: ffff"),
        C("\"\xf0\x8f\xbf\xbf\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xf0\x90\x80\x80\"", "2: d800 dc00"),
        C("\"\xf4\x8f\xbf\xbf\"", "2: dbff dfff"),
        C("\"\xf4\x90\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xf5\x80\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xf7\xbf\xbf\xbf\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xf8\x88\x80\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xfc\x84\x80\x80\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xfe\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xff\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xbf\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xc3\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xc3", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe4\xb8\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe4\xb8", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe4\xb8\xad" "q\xe4\"", "SyntaxError: Bad UTF-8 sequence at 1:4"),
        C("\"\xc3\xa9\xc3\"", "SyntaxError: Bad UTF-8 sequence at 1:3"),
        C("\"\xc3\xa9\\n\xe4\xb8\xad\"", "3: e9 a 4e2d"),
#undef C
    };
    VM v = vm_new();
    JSValue g;
    size_t i;
    char *text = malloc(400), want[1024];
    int k;

    eval_ok(v.ctx, "globalThis.hex = function (s) { var o = []; for (var i = 0; i < s.length; i++) o.push(s.charCodeAt(i).toString(16)); return s.length + ': ' + o.join(' '); }; 0");
    g = JS_GetGlobalObject(v.ctx);
    for (i = 0; i < countof(cases) + 3; i++) {
        const char *src;
        size_t len;
        JSValue r;
        char *got;

        want[0] = 0;
        if (i < countof(cases)) {
            src = cases[i].text;
            len = cases[i].len;
            snprintf(want, sizeof(want), "%s", cases[i].want);
        } else if (i == countof(cases)) {
            /* 70 characters of 3 bytes and a lone lead byte */
            len = 0;
            text[len++] = '"';
            for (k = 0; k < 70; k++) { memcpy(text + len, "\xe4\xb8\xad", 3); len += 3; }
            memcpy(text + len, "\xc3\"", 2); len += 2;
            src = text;
            snprintf(want, sizeof(want), "SyntaxError: Bad UTF-8 sequence at 1:72");
        } else if (i == countof(cases) + 1) {
            /* 40 characters of 4 bytes */
            len = 0;
            text[len++] = '"';
            for (k = 0; k < 40; k++) { memcpy(text + len, "\xf0\x9f\x98\x80", 4); len += 4; }
            text[len++] = '"';
            src = text;
            strcpy(want, "80:");
            for (k = 0; k < 40; k++)
                strcat(want, " d83d de00");
        } else {
            /* 50 times a and a two byte character, then a byte that is never UTF-8 */
            len = 0;
            text[len++] = '"';
            for (k = 0; k < 50; k++) { memcpy(text + len, "a\xc3\xa9", 3); len += 3; }
            memcpy(text + len, "\xff\"", 2); len += 2;
            src = text;
            snprintf(want, sizeof(want), "SyntaxError: Bad UTF-8 sequence at 1:102");
        }
        /* JS_ParseJSON wants a zero after the text */
        if (src == text)
            text[len] = 0;
        r = JS_ParseJSON(v.ctx, src, len, "<bytes>");
        if (JS_IsException(r)) {
            JS_FreeValue(v.ctx, r);
            r = JS_GetException(v.ctx);
            JS_SetPropertyStr(v.ctx, g, "v", r);
            got = eval_text(v.ctx, "v.name + ': ' + v.message + ' at ' + v.lineNumber + ':' + v.columnNumber");
        } else {
            JS_SetPropertyStr(v.ctx, g, "v", r);
            got = eval_text(v.ctx, "hex(v)");
        }
        CHECK(got && strcmp(got, want) == 0, "case %zu: got %s, want %s", i, got ? got : "an exception", want);
        free(got);
    }
    JS_FreeValue(v.ctx, g);
    free(text);
    vm_free(&v);
}

/* ---- the C API a Go host calls ---- */

/* what the functions of JS_NewHostFunction were last called with */
static struct {
    uint32_t vm;
    int id, argc, has_this;
    char this_text[64];
} host_last;

static JSContext *host_ctx;

/* The functions of test_host_function: id 1 adds its two arguments, id
   0x7fffffff returns how many it got, id 2 throws, and id 3 runs the jobs. */
static JSValue test_host_call(uint32_t vm, const JSValue *this_val, int argc, JSValueConst *argv, int id)
{
    host_last.vm = vm;
    host_last.id = id;
    host_last.argc = argc;
    host_last.has_this = this_val != NULL;
    host_last.this_text[0] = '\0';
    if (this_val) {
        const char *t = JS_ToCString(host_ctx, *this_val);

        snprintf(host_last.this_text, sizeof(host_last.this_text), "%s", t ? t : "?");
        JS_FreeCString(host_ctx, t);
    }
    switch (id) {
    case 1:
        return JS_NewInt32(host_ctx, JS_VALUE_GET_INT(argv[0]) + JS_VALUE_GET_INT(argv[1]));
    case 0x7fffffff:
        return JS_NewInt32(host_ctx, argc);
    case 2:
        return JS_ThrowErrorKind(host_ctx, JS_ERROR_KIND_RANGE, "from the host", 13);
    case 3: {
        /* runs the jobs from inside javascript (test_weakref_kept) */
        JSContext *jc;
        int n = 0;

        while (JS_ExecutePendingJob(JS_GetRuntime(host_ctx), &jc) > 0)
            n++;
        return JS_NewInt32(host_ctx, n);
    }
    }
    return JS_UNDEFINED;
}

static void define_host(JSContext *ctx, const char *name, int length, int32_t id)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue f = JS_NewHostFunction(ctx, name, strlen(name), length, id);

    CHECK(!JS_IsException(f), "JS_NewHostFunction %s", name);
    JS_SetPropertyStr(ctx, g, name, f);
    JS_FreeValue(ctx, g);
}

/* A function of the host is shaped as a built-in, and a call of it reaches
   host_call with the VM, this, the arguments and its id. */
static void test_host_function(void)
{
    VM v = vm_new();
    JSValue f;
    char *t;

    host_ctx = v.ctx;
    host_function_call = test_host_call;
    define_host(v.ctx, "add", 2, 1);
    define_host(v.ctx, "count", 3, 0x7fffffff);
    define_host(v.ctx, "fail", 0, 2);
    CHECK(eval_is(v.ctx, "add(2, 3)", "5"), "add");
    CHECK(host_last.vm == host_vm_id(v.rt) && host_last.id == 1 && host_last.argc == 2 && host_last.has_this &&
          strcmp(host_last.this_text, "undefined") == 0,
          "the call of add: vm %u id %d argc %d this %s", host_last.vm, host_last.id, host_last.argc, host_last.this_text);
    CHECK(eval_is(v.ctx, "add.call({ toString() { return 'me' } }, 1, 1)", "2"), "add with this");
    CHECK(strcmp(host_last.this_text, "me") == 0, "this %s", host_last.this_text);
    /* argc is what the call passed, and argv has at least length values */
    CHECK(eval_is(v.ctx, "count(1) + ',' + count(1, 2, 3, 4)", "1,4"), "count");
    CHECK(host_last.id == 0x7fffffff, "the id of count: %d", host_last.id);
    CHECK(eval_is(v.ctx,
                  "JSON.stringify([add.name, add.length, count.length, Object.hasOwn(add, 'prototype'),\n"
                  "  String(add), Object.getPrototypeOf(add) === Function.prototype,\n"
                  "  (() => { try { new add(1, 2); return 'made' } catch (e) { return e.name } })()])",
                  "[\"add\",2,3,false,\"function add() {\\n    [native code]\\n}\",true,\"TypeError\"]"),
          "the shape of add");
    CHECK(eval_is(v.ctx, "try { fail() } catch (e) { e.name + ': ' + e.message }", "RangeError: from the host"), "fail");
    f = JS_NewHostFunction(v.ctx, "bad", 3, 0, -1);
    CHECK(JS_IsException(f), "a negative id made a function");
    t = exception_text(v.ctx);
    CHECK(t && strcmp(t, "RangeError: host function id -1") == 0, "the error of a negative id: %s", t ? t : "?");
    free(t);
    host_function_call = NULL;
    vm_free(&v);
}

/* The handles test_host_release saw released, in order. */
static struct {
    uint32_t vm, handles[64];
    int n;
} released;

static void test_release_call(uint32_t vm, uint32_t handle)
{
    released.vm = vm;
    if (released.n < (int)countof(released.handles))
        released.handles[released.n] = handle;
    released.n++;
}

static JSValue new_release(JSContext *ctx, const char *name, int32_t id)
{
    JSValue f = JS_NewHostFunctionRelease(ctx, name, strlen(name), 1, id);

    CHECK(!JS_IsException(f), "JS_NewHostFunctionRelease %s", name);
    return f;
}

/* A function of JS_NewHostFunctionRelease is called as one of
   JS_NewHostFunction, and the engine calls host_release with its id once,
   when it frees the function: at the drop of its last reference, in the
   collection of a cycle, and when the runtime is freed. One of
   JS_NewHostFunction releases nothing. */
static void test_host_release(void)
{
    VM v = vm_new();
    JSValue g, f;

    host_ctx = v.ctx;
    host_function_call = test_host_call;
    host_release_call = test_release_call;
    memset(&released, 0, sizeof(released));
    g = JS_GetGlobalObject(v.ctx);

    f = new_release(v.ctx, "add", 1);
    JS_SetPropertyStr(v.ctx, g, "add", f);
    CHECK(eval_is(v.ctx, "add(2, 3) + ':' + add.name + ':' + add.length", "5:add:1"), "a released function calls");
    CHECK(released.n == 0, "%d released while the function is held", released.n);
    eval_ok(v.ctx, "delete globalThis.add");
    CHECK(released.n == 1 && released.handles[0] == 1 && released.vm == host_vm_id(v.rt),
          "deleting the last reference released %d handles (first %u, vm %u)", released.n, released.handles[0], released.vm);

    /* a cycle goes in a collection */
    JS_SetPropertyStr(v.ctx, g, "cyc", new_release(v.ctx, "cyc", 2));
    eval_ok(v.ctx, "cyc.self = cyc; delete globalThis.cyc");
    CHECK(released.n == 1, "a cycle released before the collection");
    JS_RunGC(v.rt);
    CHECK(released.n == 2 && released.handles[1] == 2, "the collection released %d handles", released.n);

    /* a function of JS_NewHostFunction has nothing to release */
    define_host(v.ctx, "plain", 0, 3);
    eval_ok(v.ctx, "delete globalThis.plain");
    JS_RunGC(v.rt);
    CHECK(released.n == 2, "a function of JS_NewHostFunction released a handle");

    /* the failure of a negative id makes nothing to release */
    f = JS_NewHostFunctionRelease(v.ctx, "bad", 3, 0, -1);
    CHECK(JS_IsException(f), "a negative id made a function");
    free(exception_text(v.ctx));

    /* held when the runtime goes, released then */
    JS_SetPropertyStr(v.ctx, g, "kept", new_release(v.ctx, "kept", 0x7fffffff));
    JS_FreeValue(v.ctx, g);
    vm_free(&v);
    CHECK(released.n == 3 && released.handles[2] == 0x7fffffff, "freeing the runtime released %d handles", released.n);
    host_function_call = NULL;
    host_release_call = NULL;
}


/* Release order at the end of a call. The end of a call frees its locals
   from the first to the last, and skips the ones with no reference count
   (see "The end of a call skips the values with no reference count" in
   docs/internals/changes/c-engine.md). A function of JS_NewHostFunctionRelease calls
   host_release when it is freed, so the handles seen by the time the call has
   returned say which values the end of the call freed, how many times and in
   which order. */

/* order_run makes the globals h1 to h64, functions that release their number,
   runs src, which defines f, and calls f. take(name) in src hands a global
   over to a local: nothing else holds the function then. */
static void order_run(const char *src, const char *what, int want_n, const uint32_t *want, int gc)
{
    VM v = vm_new();
    JSValue g;
    char name[8];
    int i;

    host_ctx = v.ctx;
    host_function_call = test_host_call;
    host_release_call = test_release_call;
    memset(&released, 0, sizeof(released));
    g = JS_GetGlobalObject(v.ctx);
    for (i = 1; i <= 64; i++) {
        snprintf(name, sizeof(name), "h%d", i);
        JS_SetPropertyStr(v.ctx, g, name, new_release(v.ctx, name, i));
    }
    eval_ok(v.ctx, "function take(n) { var v = globalThis[n]; delete globalThis[n]; return v }");
    eval_ok(v.ctx, src);
    CHECK(released.n == 0, "%s: %d released before the call", what, released.n);
    eval_ok(v.ctx, "f()");
    if (gc) {
        CHECK(released.n == 0, "%s: %d released by the call, a cycle waits for the collection", what, released.n);
        JS_RunGC(v.rt);
    }
    CHECK(released.n == want_n, "%s: %d released, want %d", what, released.n, want_n);
    for (i = 0; i < want_n && i < (int)countof(released.handles); i++)
        CHECK(released.handles[i] == want[i], "%s: release %d is handle %u, want %u", what, i, released.handles[i], want[i]);
    JS_FreeValue(v.ctx, g);
    vm_free(&v);
    host_function_call = NULL;
    host_release_call = NULL;
}

/* A function of n locals, in which the local at slots[i] takes the global
   h(i+1), for k slots. */
static void order_slots(int n, const int *slots, int k, const char *what)
{
    char src[4096], *p = src;
    uint32_t want[16];
    int i;

    p += sprintf(p, "function f() {");
    for (i = 0; i < n; i++)
        p += sprintf(p, "var s%d;", i);
    for (i = 0; i < k; i++) {
        p += sprintf(p, "s%d = take('h%d');", slots[i], i + 1);
        want[i] = i + 1;
    }
    sprintf(p, "}");
    order_run(src, what, k, want, 0);
}

/* A function g of m arguments and n locals, every one of them set first and
   live to its end (so that no two share a slot), called with h1 to hm: the local at
   slots[i] takes h(m+i+1), for k slots. The return of g frees its arguments,
   then its locals, each from the first to the last, in whatever groups the
   loop takes them. */
static void order_live(int m, int n, const int *slots, int k, const char *what)
{
    char src[8192], *p = src;
    uint32_t want[64];
    int i;

    p += sprintf(p, "function g(");
    for (i = 0; i < m; i++)
        p += sprintf(p, "%sa%d", i ? ", " : "", i);
    p += sprintf(p, ") {");
    for (i = 0; i < n; i++)
        p += sprintf(p, "var s%d = %d;", i, i);
    for (i = 0; i < k; i++)
        p += sprintf(p, "s%d = take('h%d');", slots[i], m + i + 1);
    p += sprintf(p, "if (globalThis.never) {");
    for (i = 0; i < m + n; i++)
        p += i < m ? sprintf(p, " keep(a%d);", i) : sprintf(p, " keep(s%d);", i - m);
    p += sprintf(p, " } }\nfunction f() { g(");
    for (i = 0; i < m; i++)
        p += sprintf(p, "%stake('h%d')", i ? ", " : "", i + 1);
    sprintf(p, ") }");
    for (i = 0; i < m + k; i++)
        want[i] = i + 1;
    order_run(src, what, m + k, want, 0);
}

/* Frames of up to 44 slots, whose return frees them in a loop: every slot
   full, the last one alone, values on each side of the boundaries of the
   groups of 8, and arguments before the locals. */
static void test_release_order_live(void)
{
    static const int edges[] = { 6, 7, 8, 14, 15, 16, 22, 23, 24, 30, 31, 32, 38, 39 };
    int slots[64], n, m, i;
    char what[64];

    for (n = 0; n <= 40; n++) {
        for (i = 0; i < n; i++)
            slots[i] = i;
        snprintf(what, sizeof(what), "all %d live locals", n);
        order_live(0, n, slots, n, what);
        if (n > 0) {
            slots[0] = n - 1;
            snprintf(what, sizeof(what), "the last of %d live locals", n);
            order_live(0, n, slots, 1, what);
        }
    }
    order_live(0, 40, edges, countof(edges), "40 live locals, values at the edges of the groups");
    for (m = 1; m <= 20; m += 3) {
        for (n = 0; n <= 20; n += 5) {
            for (i = 0; i < n; i++)
                slots[i] = i;
            snprintf(what, sizeof(what), "%d arguments and %d live locals", m, n);
            order_live(m, n, slots, n, what);
        }
    }
}

static void test_release_order(void)
{
    static const int five[] = { 0, 3, 4, 7, 8 };
    int slots[16], n, i;
    char what[64];
    static const uint32_t a_then_b[] = { 1, 2 };

    /* values with host release in slots 0, 3, 4, 7 and 8, among undefined ones */
    order_slots(9, five, 5, "slots 0 3 4 7 8 of 9");
    order_slots(16, five, 5, "slots 0 3 4 7 8 of 16");
    order_slots(12, five, 5, "slots 0 3 4 7 8 of 12");

    /* every slot of 0 to 15, and the last one alone */
    for (n = 0; n <= 15; n++) {
        for (i = 0; i < n; i++)
            slots[i] = i;
        snprintf(what, sizeof(what), "all %d slots", n);
        order_slots(n, slots, n, what);
        if (n > 0) {
            slots[0] = n - 1;
            snprintf(what, sizeof(what), "the last of %d slots", n);
            order_slots(n, slots, 1, what);
        }
    }

    /* the same function in slots 0, 3 and 8 is released once, at the end */
    {
        static const uint32_t one[] = { 1 };
        order_run("function f() { var s0, s1, s2, s3, s4, s5, s6, s7, s8; s0 = take('h1'); s3 = s0; s8 = s0 }",
                  "one function in slots 0 3 8", 1, one, 0);
    }

    /* a function that holds another, the first in slot 0 and the second in
       slot 3: the first is released when slot 0 is freed, the second when
       slot 3 is, after the first let go of its hold */
    order_run("function f() { var s0, s1, s2, s3, s4; s0 = take('h1'); s3 = take('h2'); s0.child = s3 }",
              "a holds b, a in slot 0 and b in slot 3", 2, a_then_b, 0);
    /* the holder in the later slot: b is freed from slot 0 first, but a still holds it */
    order_run("function f() { var s0, s1, s2, s3, s4; s0 = take('h2'); s3 = take('h1'); s3.child = s0 }",
              "a holds b, b in slot 0 and a in slot 3", 2, a_then_b, 0);
    /* two that hold each other wait for the collection */
    {
        static const uint32_t both[] = { 1, 2 };
        order_run("function f() { var s0, s1, s2, s3; s0 = take('h1'); s3 = take('h2'); s0.other = s3; s3.other = s0 }",
                  "a cycle of two", 2, both, 1);
    }
}
/* JS_FreeModuleTopLevel frees the top-level function of a module that has
   run, and with it a variable of the top level that no function of the module
   uses, while the variables its functions and exports use stay. It does
   nothing to a module that has not run, or that threw. */
static void test_module_top_level(void)
{
    static const char *srcs[] = {
        "const big = new Uint8Array(4 << 20).fill(1); let kept = big[0] + 1;\n"
        "export let seen = 7;\nglobalThis.get = () => kept++\n",
        "const big = new Uint8Array(4 << 20).fill(1); let kept = big[0] + 1;\nawait null;\n"
        "export let seen = 7;\nglobalThis.get = () => kept++\n",
    };
    int i;

    for (i = 0; i < 2; i++) {
        VM v = vm_new();
        JSValue mod = JS_Eval(v.ctx, srcs[i], strlen(srcs[i]), "<top>", JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
        JSModuleDef *m;
        JSValue p;
        JSMemoryUsage before, after;

        CHECK(JS_VALUE_GET_TAG(mod) == JS_TAG_MODULE, "the module did not compile");
        m = JS_VALUE_GET_PTR(mod);
        /* not run yet: nothing to free */
        JS_FreeModuleTopLevel(v.ctx, m);
        p = JS_EvalFunction(v.ctx, JS_DupValue(v.ctx, mod));
        CHECK(!JS_IsException(p), "the module threw");
        if (i == 1) {
            /* the top level waits at its await: still running */
            JS_FreeModuleTopLevel(v.ctx, m);
        }
        while (JS_ExecutePendingJob(v.rt, NULL) > 0)
            ;
        CHECK(JS_PromiseState(v.ctx, p) == JS_PROMISE_FULFILLED, "module %d did not run to its end", i);
        JS_FreeValue(v.ctx, p);
        before = usage(&v);
        JS_FreeModuleTopLevel(v.ctx, m);
        after = usage(&v);
        CHECK(before.memory_used_size - after.memory_used_size >= 4 << 20,
              "module %d: freeing the top level gave back %lld bytes", i,
              (long long)(before.memory_used_size - after.memory_used_size));
        CHECK(eval_is(v.ctx, "get() + get()", "5"), "module %d: the variable of a function", i);
        /* again does nothing */
        JS_FreeModuleTopLevel(v.ctx, m);
        p = JS_GetModuleNamespace(v.ctx, m);
        CHECK(!JS_IsException(p), "module %d: no namespace", i);
        JS_FreeValue(v.ctx, p);
        CHECK(eval_is(v.ctx, "get()", "4"), "module %d: the variable after the namespace", i);
        JS_FreeValue(v.ctx, mod);
        vm_free(&v);
    }
}

/* JS_NewStringChars gives a string whose characters the host writes where
   JS_HOST_LAYOUT_* says, and the layout is that of the strings the engine
   makes. */
static void test_new_string_chars(void)
{
    extern const int64_t js_host_layout[JS_HOST_LAYOUT_COUNT];
    VM v = vm_new();
    JSValue g = JS_GetGlobalObject(v.ctx);
    JSValue s;
    uint8_t *p;
    uint32_t word;
    char *t;
    int i;

    /* an 8-bit string */
    s = JS_NewStringChars(v.ctx, 5, 0);
    CHECK(JS_VALUE_GET_TAG(s) == JS_TAG_STRING, "an 8-bit string");
    p = JS_VALUE_GET_PTR(s);
    memcpy(&word, p, 4);
    CHECK((word & js_host_layout[JS_HOST_LAYOUT_STRING_LEN_MASK]) == 5 &&
          !(word & js_host_layout[JS_HOST_LAYOUT_STRING_WIDE_BIT]), "the first word %#x", word);
    memcpy(p + js_host_layout[JS_HOST_LAYOUT_STRING_CHARS], "h\xe9llo", 5);
    CHECK(p[js_host_layout[JS_HOST_LAYOUT_STRING_CHARS] + 5] == 0, "the NUL after the characters");
    JS_SetPropertyStr(v.ctx, g, "s8", s);
    CHECK(eval_is(v.ctx, "s8 === 'h\\xe9llo' && s8.length", "5"), "the 8-bit string");

    /* a 16-bit string */
    s = JS_NewStringChars(v.ctx, 3, 1);
    p = JS_VALUE_GET_PTR(s);
    memcpy(&word, p, 4);
    CHECK((word & js_host_layout[JS_HOST_LAYOUT_STRING_LEN_MASK]) == 3 &&
          (word & js_host_layout[JS_HOST_LAYOUT_STRING_WIDE_BIT]), "the first word %#x", word);
    {
        static const uint16_t chars[3] = { 0x4e2d, 0xd83d, 0xde00 };
        memcpy(p + js_host_layout[JS_HOST_LAYOUT_STRING_CHARS], chars, sizeof(chars));
    }
    JS_SetPropertyStr(v.ctx, g, "s16", s);
    CHECK(eval_is(v.ctx, "s16 === '\\u4e2d\\u{1f600}' && s16.length", "3"), "the 16-bit string");

    /* a string JSON.stringify has marked as needing no escape, and an atom:
       the mark is not part of the length */
    for (i = 0; i < 2; i++) {
        static const char *names[] = { "sp", "sa" };
        static const uint32_t lens[] = { 15, 9 };

        CHECK(eval_is(v.ctx, i ? "globalThis.sa = 'plainAtom'; JSON.stringify({ plainAtom: sa }).length"
                               : "globalThis.sp = 'plain'.repeat(3); JSON.stringify([sp, sp]).length",
                      i ? "25" : "37"), "JSON.stringify of %s", names[i]);
        s = JS_GetPropertyStr(v.ctx, g, names[i]);
        CHECK(JS_VALUE_GET_TAG(s) == JS_TAG_STRING, "%s is a string", names[i]);
        p = JS_VALUE_GET_PTR(s);
        memcpy(&word, p, 4);
        CHECK((word & js_host_layout[JS_HOST_LAYOUT_STRING_LEN_MASK]) == lens[i] &&
              !(word & js_host_layout[JS_HOST_LAYOUT_STRING_WIDE_BIT]),
              "the first word of %s after JSON.stringify: %#x", names[i], word);
        JS_FreeValue(v.ctx, s);
    }

    /* the reference count, and the strings the engine makes */
    for (i = 0; i < 2; i++) {
        static const char *texts[] = { "abcdddddddddd", "abc\xe4\xb8\xad" };
        static const uint32_t lens[] = { 13, 4 };
        int32_t rc, rc2;

        s = JS_NewString(v.ctx, texts[i]);
        CHECK(JS_VALUE_GET_TAG(s) == JS_TAG_STRING, "a string of the engine");
        p = JS_VALUE_GET_PTR(s);
        memcpy(&word, p, 4);
        CHECK((word & js_host_layout[JS_HOST_LAYOUT_STRING_LEN_MASK]) == lens[i] &&
              !(word & js_host_layout[JS_HOST_LAYOUT_STRING_WIDE_BIT]) == !i,
              "the first word of %s: %#x", texts[i], word);
        CHECK(memcmp(p + js_host_layout[JS_HOST_LAYOUT_STRING_CHARS], i ? (const void *)"a\0b\0c\0\x2d\x4e" : "abcd", 4 << i) == 0,
              "the characters of %s", texts[i]);
        memcpy(&rc, p + js_host_layout[JS_HOST_LAYOUT_REFCOUNT_OFFSET], 4);
        JS_DupValue(v.ctx, s);
        memcpy(&rc2, p + js_host_layout[JS_HOST_LAYOUT_REFCOUNT_OFFSET], 4);
        CHECK(rc2 == rc + 1, "the reference count went from %d to %d", rc, rc2);
        JS_FreeValue(v.ctx, s);
        JS_FreeValue(v.ctx, s);
    }

    /* an empty string, and one too long */
    s = JS_NewStringChars(v.ctx, 0, 1);
    CHECK(JS_IsString(s), "the empty string");
    JS_SetPropertyStr(v.ctx, g, "s0", s);
    CHECK(eval_is(v.ctx, "s0 === '' ? 'empty' : 'not'", "empty"), "the empty string");
    s = JS_NewStringChars(v.ctx, js_host_layout[JS_HOST_LAYOUT_STRING_LEN_MAX] + 1, 0);
    CHECK(JS_IsException(s), "a string past the most");
    t = exception_text(v.ctx);
    CHECK(t && strcmp(t, "RangeError: invalid string length") == 0, "the error of a string past the most: %s", t ? t : "?");
    free(t);

    /* the atoms */
    for (i = JS_HOST_LAYOUT_ATOM_EMPTY_STRING; i <= JS_HOST_LAYOUT_ATOM_THEN; i++) {
        static const char *names[] = { "", "fileName", "lineNumber", "columnNumber", "message", "stack", "name", "toString", "then" };
        JSAtom a = JS_NewAtom(v.ctx, names[i - JS_HOST_LAYOUT_ATOM_EMPTY_STRING]);

        CHECK(a == (JSAtom)js_host_layout[i], "the atom of %s: %u, table %lld", names[i - JS_HOST_LAYOUT_ATOM_EMPTY_STRING], a, (long long)js_host_layout[i]);
        JS_FreeAtom(v.ctx, a);
    }
    JS_FreeValue(v.ctx, g);

    /* no memory: the out of memory error */
    room(&v, 64);
    s = JS_NewStringChars(v.ctx, 100000, 1);
    CHECK(JS_IsException(s) && JS_IsUncatchableException(v.ctx), "a string with no memory");
    free(exception_text(v.ctx));
    unlimit(&v);
    vm_free(&v);
}

/* JS_ThrowErrorKind throws each kind of error, its message cut as "%s" cuts
   it. */
static void test_throw_error_kind(void)
{
    static const char *names[] = { "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError", "InternalError", "AggregateError" };
    VM v = vm_new();
    char long_msg[400];
    char *t;
    int k;

    for (k = JS_ERROR_KIND_EVAL; k <= JS_ERROR_KIND_AGGREGATE; k++) {
        char want[64];

        JS_ThrowErrorKind(v.ctx, k, "it went wrong", 13);
        t = exception_text(v.ctx);
        snprintf(want, sizeof(want), "%s: it went wrong", names[k]);
        CHECK(t && strcmp(t, want) == 0, "kind %d: %s", k, t ? t : "?");
        free(t);
    }
    memset(long_msg, 'x', sizeof(long_msg));
    JS_ThrowErrorKind(v.ctx, JS_ERROR_KIND_TYPE, long_msg, sizeof(long_msg));
    t = exception_text(v.ctx);
    CHECK(t && strlen(t) == strlen("TypeError: ") + 255, "a long message: %zu bytes", t ? strlen(t) : 0);
    free(t);
    JS_ThrowErrorKind(v.ctx, JS_ERROR_KIND_TYPE, "ab\0cd", 5);
    t = exception_text(v.ctx);
    CHECK(t && strcmp(t, "TypeError: ab") == 0, "a NUL in the message: %s", t ? t : "?");
    free(t);
    JS_ThrowErrorKind(v.ctx, 99, "x", 1);
    t = exception_text(v.ctx);
    CHECK(t && strcmp(t, "InternalError: error kind 99") == 0, "a bad kind: %s", t ? t : "?");
    free(t);
    vm_free(&v);
}

/* ---- ordinary objects ---- */

/* The engine allocates only the part of an ordinary object that comes before
   the union of the data of the classes. Anything that touches that union of
   an ordinary object reads or writes past its block, which a build on the
   system malloc under AddressSanitizer reports (build.sh asan). So this runs
   an ordinary object through what the engine does to objects. */
#define CLASS_OBJECT 1 /* JS_CLASS_OBJECT, which quickjs.h does not name */

static void test_plain_objects(void)
{
    VM v = vm_new();
    JSMemoryUsage u;
    JSClassID id = 99;
    JSValue o, e;

    eval_ok(v.ctx,
            "var keep = [];\n"
            "for (var i = 0; i < 300; i++) {\n"
            "  var o = {};\n"
            "  for (var k = 0; k < 70; k++) o['k' + k] = k;\n"       /* grows the property array and the shape */
            "  for (var k = 0; k < 70; k += 3) delete o['k' + k];\n" /* a hashed shape, with holes */
            "  keep.push(o);\n"
            "}\n"
            "Object.freeze(keep[0]); Object.seal(keep[1]); Object.preventExtensions(keep[2]);\n"
            "Object.defineProperty(keep[3], 'g', { get() { return 1 }, set(x) {}, enumerable: true });\n"
            "var child = Object.create(keep[4]); child.x = 1;\n"        /* as a prototype */
            "Object.setPrototypeOf(keep[5], keep[6]);\n"
            "function F() {} F.prototype = keep[7]; var inst = new F();\n"
            "var wm = new WeakMap(), ws = new WeakSet(), refs = [];\n"  /* as a weak key */
            "var fr = new FinalizationRegistry(function () {});\n"
            "for (var o of keep) { wm.set(o, 1); ws.add(o); refs.push(new WeakRef(o)); fr.register(o, 1); }\n"
            "var cyc = {}; cyc.self = cyc; cyc.list = [cyc, {up: cyc}];\n" /* a cycle for the collector */
            "JSON.stringify(keep[8]); Object.entries(keep[9]); Object.assign({}, keep[10]);\n"
            "void 0\n");
    JS_RunGC(v.rt);
    JS_ComputeMemoryUsage(v.rt, &u);
    CHECK(u.obj_count > 300, "%lld objects", (long long)u.obj_count);
    CHECK(u.obj_size < u.obj_count * 64, "%lld objects take %lld bytes in all, 64 each or more", (long long)u.obj_count,
          (long long)u.obj_size);
    CHECK(eval_is(v.ctx, "[Object.isFrozen(keep[0]), Object.isSealed(keep[1]), child.x, child.k1, inst.k2, wm.has(keep[9])].join()",
                  "true,true,1,1,2,true"), "the objects changed");
    eval_ok(v.ctx, "keep = null; child = null; inst = null; cyc = null; void 0");
    JS_RunGC(v.rt);
    JS_ComputeMemoryUsage(v.rt, &u);

    /* what a host asks of an ordinary object: it has no opaque pointer */
    o = JS_NewObject(v.ctx);
    CHECK(JS_GetOpaque(o, CLASS_OBJECT) == NULL, "JS_GetOpaque of an ordinary object");
    CHECK(JS_GetOpaque(o, CLASS_OBJECT + 1) == NULL, "JS_GetOpaque of another class");
    CHECK(JS_GetAnyOpaque(o, &id) == NULL && id == CLASS_OBJECT, "JS_GetAnyOpaque of an ordinary object: class %d", (int)id);
    CHECK(JS_GetOpaque2(v.ctx, o, CLASS_OBJECT) == NULL && JS_HasException(v.ctx), "JS_GetOpaque2 of an ordinary object");
    e = JS_GetException(v.ctx);
    JS_FreeValue(v.ctx, e);
    JS_FreeValue(v.ctx, o);
    vm_free(&v);
}

/* A for in over an ordinary object whose keys are all strings, and not array
   indexes, keeps no list of its keys: it holds the shape of the object. 1000
   generators stopped in such a loop take as much memory with an object of 40
   keys as with an object of 1 key. */
static int64_t for_in_held(int keys)
{
    VM v = vm_new();
    JSMemoryUsage before, after;
    char src[256];

    snprintf(src, sizeof src,
             "function* walk(o) { for (const k in o) yield k }\n"
             "var o = {}; for (var j = 0; j < %d; j++) o['k' + j] = j\n"
             "var held = []\n"
             "function hold() { for (var i = 0; i < 1000; i++) { var g = walk(o); g.next(); held.push(g) } }\n",
             keys);
    eval_ok(v.ctx, src);
    before = usage(&v);
    JS_FreeValue(v.ctx, call_global(v.ctx, "hold"));
    after = usage(&v);
    CHECK(eval_is(v.ctx, "held[999].next().value", keys > 1 ? "k1" : "undefined"), "the loop of %d keys went wrong", keys);
    vm_free(&v);
    return (after.malloc_size - before.malloc_size) / 1000;
}

static void test_for_in_keeps_shape(void)
{
    int64_t one = for_in_held(1), forty = for_in_held(40);

    CHECK(forty - one < 16, "a for in over 40 keys holds %lld bytes more than over 1 key",
          (long long)(forty - one));
}

/* JS_RunGC sets the threshold of the next collection to 1.5 times what it left,
   as the collection the runtime starts by itself does, so the first object
   after it does not start another. The register stack (JS_RegvmStat 9)
   counts in the memory size but not in what the collection left. A threshold of -1 turns the collections of
   the runtime off, and JS_RunGC leaves it so. */
static void test_run_gc_threshold(void)
{
    size_t i, before[] = { 1, 1 << 20, (size_t)1 << 40 };
    VM v = vm_new();
    size_t live;

    for (i = 0; i < countof(before); i++) {
        JS_SetGCThreshold(v.rt, before[i]);
        JS_RunGC(v.rt);
        live = JS_GetMemorySize(v.rt) - JS_RegvmStat(v.ctx, 9);
        CHECK(JS_GetGCThreshold(v.rt) == live + (live >> 1), "after JS_RunGC with the threshold at %zu: %zu, want 1.5 times the %zu bytes left",
              before[i], JS_GetGCThreshold(v.rt), live);
    }

    /* the first object after it starts no collection */
    JS_SetGCThreshold(v.rt, 1);
    JS_RunGC(v.rt);
    live = JS_GetGCThreshold(v.rt);
    eval_ok(v.ctx, "var o = {}; for (var i = 0; i < 100; i++) o = {o}; void 0");
    CHECK(JS_GetGCThreshold(v.rt) == live, "the script after JS_RunGC started a collection: the threshold went from %zu to %zu", live,
          JS_GetGCThreshold(v.rt));

    JS_SetGCThreshold(v.rt, (size_t)-1);
    JS_RunGC(v.rt);
    CHECK(JS_GetGCThreshold(v.rt) == (size_t)-1, "JS_RunGC changed a threshold of -1 to %zu", JS_GetGCThreshold(v.rt));
    vm_free(&v);
}

/* One large block made by a built-in with no object made before it, of about
   size bytes: setup runs before the collection that measures what is live,
   op after it (see test_gc_block_line). */
typedef struct {
    const char *name, *setup, *op;
    size_t div; /* size / div is the count the scripts take */
} GCBlockKind;

static const GCBlockKind gc_block_kinds[] = {
    { "repeat", NULL, "globalThis.r = 'x'.repeat(%zu); 0", 1 },
    { "padEnd", NULL, "globalThis.r = 'ab'.padEnd(%zu, 'x'); 0", 1 },
    { "ArrayBuffer", NULL, "globalThis.r = new ArrayBuffer(%zu); 0", 1 },
    { "Float64Array", NULL, "globalThis.r = new Float64Array(%zu); 0", 8 },
    { "resize", "globalThis.rb = new ArrayBuffer(16, { maxByteLength: 1 << 30 }); 0", "rb.resize(%zu); 0", 1 },
    { "transfer", "globalThis.tb = new ArrayBuffer(1024); 0", "globalThis.r = tb.transfer(%zu); 0", 1 },
    { "push", NULL, "{ const a = []; for (let i = 0; i < %zu; i++) a.push(i * 0.5); globalThis.r = a } 0", 16 },
    { "a[i] = v", NULL, "{ const a = []; for (let i = 0; i < %zu; i++) a[i] = i * 0.5; globalThis.r = a } 0", 16 },
    { "rope made flat", "{ let p = 'y'.repeat(65536), t = ''; for (let i = 0; i < %zu; i++) t += p; globalThis.t = t } 0",
      "t.indexOf('z')", 65536 },
    { "join", "{ const p = 'j'.repeat(1000); globalThis.ja = []; for (let i = 0; i < %zu; i++) ja.push(p + i) } 0",
      "globalThis.r = ja.join(''); 0", 1004 },
    { "toSorted", "{ globalThis.sa = []; for (let i = 0; i < %zu; i++) sa.push(i) } 0", "globalThis.r = sa.toSorted((a, b) => a - b); 0", 16 },
    { "slice", "{ globalThis.sa = []; for (let i = 0; i < %zu; i++) sa.push(i) } 0", "globalThis.r = sa.slice(); 0", 16 },
};

static int gc_block_run(JSContext *ctx, const char *fmt, size_t n)
{
    char src[512];
    JSValue r;
    int ok;

    snprintf(src, sizeof src, fmt, n);
    r = JS_Eval(ctx, src, strlen(src), "<block>", JS_EVAL_TYPE_GLOBAL);
    ok = !JS_IsException(r);
    JS_FreeValue(ctx, r);
    if (!ok) {
        char *t = exception_text(ctx);
        CHECK(t && (strstr(t, "out of memory") || strcmp(t, "null") == 0), "%s: not out of memory: %s", fmt, t ? t : "(none)");
        free(t);
    }
    return ok;
}

/* Does op of k with size bytes work in a new runtime under limit, with live
   bytes in objects and, with garbage, cyclic garbage made up to just under
   the threshold of the collection that measured them? */
static int gc_block_try(const GCBlockKind *k, size_t limit, size_t live, int garbage, size_t size)
{
    VM v = vm_new();
    int ok = 1;

    JS_SetMemoryLimit(v.rt, limit);
    eval_ok(v.ctx, "globalThis.live = []; globalThis.mk = (n) => { for (let i = 0; i < n; i++) live.push({ a: i, b: [i, i + 1], c: 'k' + i }) }; 0");
    if (k->setup)
        ok = gc_block_run(v.ctx, k->setup, size / k->div);
    while (ok && JS_GetMemorySize(v.rt) < live)
        ok = eval_ok(v.ctx, "mk(1000)");
    if (ok) {
        JS_RunGC(v.rt);
        if (garbage) {
            size_t t = JS_GetGCThreshold(v.rt), top = t < limit ? t : limit;
            static const char g[] = "for (let i = 0; i < 500; i++) { const a = { x: [1, 2, 3] }, b = { a }; a.b = b } 0";

            while (JS_GetMemorySize(v.rt) + (256 << 10) < top && JS_GetGCThreshold(v.rt) == t) {
                JSValue r = JS_Eval(v.ctx, g, strlen(g), "<garbage>", JS_EVAL_TYPE_GLOBAL);

                if (JS_IsException(r)) {
                    free(exception_text(v.ctx));
                    break;
                }
                JS_FreeValue(v.ctx, r);
            }
        }
        ok = gc_block_run(v.ctx, k->op, size / k->div);
    }
    unlimit(&v);
    vm_free(&v);
    return ok;
}

/* the largest size, to 64 KiB, that works */
static size_t gc_block_line(const GCBlockKind *k, size_t limit, size_t live, int garbage)
{
    size_t lo = 0, hi = limit;

    while (hi - lo > (64 << 10)) {
        size_t mid = lo + (hi - lo) / 2;
        if (gc_block_try(k, limit, live, garbage, mid))
            lo = mid;
        else
            hi = mid;
    }
    return lo;
}

/* Garbage in cycles waits for a collection, and only an object made over the
   threshold starts one. A built-in that makes one large block with no object
   before it (js_trigger_gc_block) collects first when the block would pass
   the memory limit, so the garbage left under the threshold does not move
   the size at which the block runs out of memory: the line with the garbage
   is the line without it. Before, it was lower by about the garbage (the
   threshold less what is live), at any factor of the threshold. */
static void test_gc_block_line(void)
{
    size_t i, j, limits[][2] = { { 16 << 20, 4 << 20 }, { 32 << 20, 12 << 20 } };

    for (i = 0; i < countof(gc_block_kinds); i++) {
        for (j = 0; j < countof(limits); j++) {
            const GCBlockKind *k = &gc_block_kinds[i];
            size_t none = gc_block_line(k, limits[j][0], limits[j][1], 0);
            size_t parked = gc_block_line(k, limits[j][0], limits[j][1], 1);
            CHECK(none > (1 << 20), "%s under %zu MiB: works at no size", k->name, limits[j][0] >> 20);
            CHECK(parked + (128 << 10) >= none,
                  "%s under %zu MiB with %zu MiB live: out of memory at %zu KiB with the garbage, %zu KiB without",
                  k->name, limits[j][0] >> 20, limits[j][1] >> 20, parked >> 10, none >> 10);
        }
    }
}

/* The function of JS_NewEscapeTextFunction gives what React DOM's
   escapeTextForBrowser gives, for the strings of every width, length and
   place of the characters it escapes, and for everything that is not a
   string: the same text, or the same error with the same calls made on the
   object. The reference is React 19's function, as its production build has
   it. */
#define ESCAPE_REFERENCE \
    "var matchHtmlRegExp = /[\"'&<>]/;\n" \
    "function reactEscape(text) {\n" \
    "  if (\"boolean\" === typeof text || \"number\" === typeof text || \"bigint\" === typeof text)\n" \
    "    return \"\" + text;\n" \
    "  text = \"\" + text;\n" \
    "  var match = matchHtmlRegExp.exec(text);\n" \
    "  if (match) {\n" \
    "    var html = \"\", index, lastIndex = 0;\n" \
    "    for (index = match.index; index < text.length; index++) {\n" \
    "      switch (text.charCodeAt(index)) {\n" \
    "        case 34: match = \"&quot;\"; break;\n" \
    "        case 38: match = \"&amp;\"; break;\n" \
    "        case 39: match = \"&#x27;\"; break;\n" \
    "        case 60: match = \"&lt;\"; break;\n" \
    "        case 62: match = \"&gt;\"; break;\n" \
    "        default: continue;\n" \
    "      }\n" \
    "      lastIndex !== index && (html += text.slice(lastIndex, index));\n" \
    "      lastIndex = index + 1;\n" \
    "      html += match;\n" \
    "    }\n" \
    "    text = lastIndex !== index ? html + text.slice(lastIndex, index) : html;\n" \
    "  }\n" \
    "  return text;\n" \
    "}\n"

/* same(make) runs reactEscape and esc on fresh values of make() and reports
   whether they agree on the text or the error, and on the calls the values
   saw. */
#define ESCAPE_DRIVER \
    "var bad = 0, count = 0, first = '';\n" \
    "function run1(f, make) {\n" \
    "  var log = [], r;\n" \
    "  try { r = { v: f(make(log)) } } catch (e) { r = { e: e instanceof Error ? e.name + ': ' + e.message : 'thrown ' + String(e) } }\n" \
    "  r.log = log.join();\n" \
    "  return r;\n" \
    "}\n" \
    "function same(make) {\n" \
    "  count++;\n" \
    "  var a = run1(esc, make), b = run1(reactEscape, make);\n" \
    "  var ok = a.log === b.log && a.e === b.e && (a.e !== undefined || (typeof a.v === 'string' && a.v === b.v));\n" \
    "  if (!ok) { bad++; if (!first) first = JSON.stringify(a) + ' against ' + JSON.stringify(b); }\n" \
    "}\n" \
    "function str(s) { same(function () { return s }) }\n" \
    "var alphabet = ['a', '&', '<', '>', '\"', \"'\", '\\u00e9', '\\u0100', '\\ud800', '\\udc00', '\\ud83d\\ude00', ';', '\\u0000'];\n" \
    "function gen(prefix, depth) {\n" \
    "  str(prefix);\n" \
    "  if (depth) for (var c of alphabet) gen(prefix + c, depth - 1);\n" \
    "}\n" \
    "gen('', 4);\n" \
    "var sizes = [1, 7, 4095, 4096, 4097, 8191, 8192, 8193, 12288, 100003];\n" \
    "var marks = ['&', '<', '>', '\"', \"'\"];\n" \
    "for (var size of sizes) {\n" \
    "  for (var wide of ['', '\\u0100']) {\n" \
    "    var filler = 'x'.repeat(size - 1) + wide;\n" \
    "    str(filler);\n" \
    "    for (var at of [0, 1, 4095, 4096, 4097, 8192, size - 2, size - 1]) {\n" \
    "      if (at < 0 || at >= size) continue;\n" \
    "      for (var m of marks) str(filler.slice(0, at) + m + filler.slice(at + 1));\n" \
    "    }\n" \
    "    str(marks.join('').repeat(Math.ceil(size / 5)).slice(0, size) + wide);\n" \
    "    str('&'.repeat(size) + wide);\n" \
    "  }\n" \
    "}\n" \
    "var piece = 'ab<cd' + 'e'.repeat(3000), rope = piece;\n" \
    "for (var i = 0; i < 12; i++) rope = rope + rope + i;\n" \
    "str(rope);\n" \
    "str(rope + '\\u0100');\n" \
    "var o1 = { toString() { return 'x<' } };\n" \
    "var others = [0, -0, 1, -1, 1.5, 1e21, 1e-7, NaN, Infinity, -Infinity, 2 ** 31, 123456789012345680000, 0.1 + 0.2,\n" \
    "  true, false, null, undefined, 10n, -(2n ** 70n), {}, [], [1, 2], ['<', '&'], new Date(0), /a<b/g, function () {},\n" \
    "  new String('a<b'), new Number(4), Object.create(null), Symbol('x'), Symbol.iterator, new Boolean(false), [undefined, null], new Error('<e>')];\n" \
    "for (var x of others) same(function () { return x });\n" \
    "same(function (log) { return { toString() { log.push('t'); return '<b>' } } });\n" \
    "same(function (log) { return { valueOf() { log.push('v'); return '&' } } });\n" \
    "same(function (log) { return { valueOf() { log.push('v'); return 7 }, toString() { log.push('t'); return 'x&y' } } });\n" \
    "same(function (log) { return { valueOf() { log.push('v'); return {} }, toString() { log.push('t'); return 'x\"y' } } });\n" \
    "same(function (log) { return { valueOf() { log.push('v'); return {} }, toString() { log.push('t'); return {} } } });\n" \
    "same(function (log) { return { [Symbol.toPrimitive](hint) { log.push(hint); return hint + '<' } } });\n" \
    "same(function (log) { return { [Symbol.toPrimitive](hint) { log.push(hint); return Symbol('s') } } });\n" \
    "same(function (log) { return { [Symbol.toPrimitive](hint) { log.push(hint); return {} } } });\n" \
    "same(function (log) { return { toString() { log.push('t'); throw new RangeError('boom') } } });\n" \
    "same(function (log) { return { toString() { log.push('t'); throw 'plain' } } });\n" \
    "same(function (log) { return { toString() { log.push('t'); return rope } } });\n" \
    "same(function (log) { return { toString() { log.push('t'); return 'a'.repeat(5000) + '&' } } });\n" \
    "same(function (log) { return { get [Symbol.toPrimitive]() { log.push('g'); return undefined }, toString() { log.push('t'); return \"'\" } } });\n" \
    "same(function (log) { return new Proxy({}, { get(t, k) { log.push(String(k)); return k === 'toString' ? function () { return '>' } : undefined } }) });\n" \
    "var noArgs = [esc(), reactEscape()];\n" \
    "if (noArgs[0] !== noArgs[1] || noArgs[0] !== 'undefined') { bad++; first = first || 'no argument: ' + noArgs; }\n" \
    "var more = [esc('<', 'ignored', '&'), esc.call({ toString() { return 'this' } }, '<'), new Function('esc', 'return esc(\"a&b\")')(esc)];\n" \
    "if (more.join() !== '&lt;,&lt;,a&amp;b') { bad++; first = first || 'arguments and this: ' + more; }\n" \
    "bad === 0 ? 'ok ' + (count > 20000) : bad + ' of ' + count + ' differ, the first: ' + first"

/* define_escape makes the function under the name esc (the bytes given as
   the name stay in name_len). */
static JSValue define_escape(JSContext *ctx, const char *name, size_t name_len)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue f = JS_NewEscapeTextFunction(ctx, name, name_len);

    if (!JS_IsException(f))
        JS_SetPropertyStr(ctx, g, name, JS_DupValue(ctx, f));
    JS_FreeValue(ctx, g);
    return f;
}

static JSValue global_prop(JSContext *ctx, const char *name)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue r = JS_GetPropertyStr(ctx, g, name);

    JS_FreeValue(ctx, g);
    return r;
}

/* escape_reads returns how many times one call of f on the global name reads the flag that stops the engine. */
static long escape_reads(JSContext *ctx, JSValueConst f, const char *name)
{
    JSValue s = global_prop(ctx, name), r;
    long before = host_poll_reads;

    r = JS_Call(ctx, f, JS_UNDEFINED, 1, &s);
    before = host_poll_reads - before;
    JS_FreeValue(ctx, r);
    JS_FreeValue(ctx, s);
    return before;
}

static void test_escape_text(void)
{
    VM v = vm_new();
    JSValue f, s, r, rope;
    char *t;

    f = define_escape(v.ctx, "esc", 3);
    CHECK(!JS_IsException(f), "no function");
    CHECK(eval_ok(v.ctx, ESCAPE_REFERENCE), "the reference of React");
    CHECK(eval_is(v.ctx, ESCAPE_DRIVER, "ok true"), "esc against the function of React");

    /* the shape of a built-in */
    CHECK(eval_is(v.ctx,
                  "JSON.stringify([esc.name, esc.length, Object.hasOwn(esc, 'prototype'),\n"
                  "  String(esc), Object.getPrototypeOf(esc) === Function.prototype,\n"
                  "  (() => { try { new esc('a'); return 'made' } catch (e) { return e.name } })()])",
                  "[\"esc\",1,false,\"function esc() {\\n    [native code]\\n}\",true,\"TypeError\"]"),
          "the shape of esc");
    JS_FreeValue(v.ctx, f);

    /* a string with nothing to escape comes back as it is, not a copy */
    eval_ok(v.ctx, "var clean = 'a plain text, \\u00e9 and more'.repeat(3), dirty = 'a<b'");
    s = global_prop(v.ctx, "clean");
    f = global_prop(v.ctx, "esc");
    r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &s);
    CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_STRING && JS_VALUE_GET_PTR(r) == JS_VALUE_GET_PTR(s),
          "a clean string came back as another string");
    JS_FreeValue(v.ctx, r);
    JS_FreeValue(v.ctx, s);
    /* a rope is linearized and comes back as its string once it has nothing to escape */
    eval_ok(v.ctx, "var cleanRope = ('x'.repeat(9000) + 'y'.repeat(9000)) + ('z'.repeat(9000))");
    rope = global_prop(v.ctx, "cleanRope");
    CHECK(JS_VALUE_GET_TAG(rope) == JS_TAG_STRING_ROPE || JS_VALUE_GET_TAG(rope) == JS_TAG_STRING, "the rope");
    r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &rope);
    CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_STRING, "a rope came back as tag %d", (int)JS_VALUE_GET_TAG(r));
    JS_FreeValue(v.ctx, r);
    JS_FreeValue(v.ctx, rope);
    CHECK(eval_is(v.ctx, "esc(cleanRope).length + ' ' + (esc(cleanRope) === cleanRope)", "27000 true"), "the rope escaped");

    /* A rope that concatenation built and nothing flattened yet is flattened by the first call. An interrupt set
       while it is flattened stops the call there, with the error, and the flat copy is not kept: the rope holds
       what it held before. (Reading 1 is the call's own, 2 and 3 are in the flattening, one at each leaf.) */
    eval_ok(v.ctx, "var ropeLeaf = 'x'.repeat(70000), bigRope = ropeLeaf; "
            "for (var ri = 0; ri < 100; ri++) bigRope = bigRope + ropeLeaf");
    rope = global_prop(v.ctx, "bigRope");
    CHECK(JS_VALUE_GET_TAG(rope) == JS_TAG_STRING_ROPE, "bigRope is tag %d, want a rope", (int)JS_VALUE_GET_TAG(rope));
    JS_RunGC(v.rt);
    {
        size_t held = JS_GetMemorySize(v.rt);

        host_poll_stop_after(3);
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &rope);
        host_poll_stop_after(0);
        *JS_GetInterruptFlag(v.rt) = 0;
        CHECK(JS_IsException(r), "an interrupt in the flattening of a rope did not stop the call");
        t = exception_text(v.ctx);
        CHECK(t && strcmp(t, "InternalError: interrupted") == 0, "the error of an interrupt in a rope: %s", t ? t : "?");
        free(t);
        JS_SetUncatchableException(v.ctx, FALSE);
        JS_RunGC(v.rt);
        CHECK(JS_GetMemorySize(v.rt) < held + (1 << 20), "the call kept a flat copy of the rope: %zu bytes more",
              JS_GetMemorySize(v.rt) - held);
    }
    JS_FreeValue(v.ctx, rope);
    CHECK(eval_is(v.ctx, "esc(bigRope).length", "7070000"), "the rope escaped after the interrupt");

    /* the interrupt stops it on a long string, before and in the middle of the work */
    eval_ok(v.ctx, "var long = '<'.repeat(200000), longClean = 'a'.repeat(200000)");
    s = global_prop(v.ctx, "long");
    *JS_GetInterruptFlag(v.rt) = 1;
    r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &s);
    *JS_GetInterruptFlag(v.rt) = 0;
    CHECK(JS_IsException(r), "an interrupt did not stop it");
    t = exception_text(v.ctx);
    CHECK(t && strcmp(t, "InternalError: interrupted") == 0, "the error of an interrupt: %s", t ? t : "?");
    free(t);
    JS_SetUncatchableException(v.ctx, FALSE);
    JS_FreeValue(v.ctx, s);
    s = global_prop(v.ctx, "longClean");
    *JS_GetInterruptFlag(v.rt) = 1;
    r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &s);
    *JS_GetInterruptFlag(v.rt) = 0;
    CHECK(JS_IsException(r), "an interrupt did not stop the count");
    free(exception_text(v.ctx));
    JS_FreeValue(v.ctx, s);
    s = global_prop(v.ctx, "long");
    host_poll_stop_after(3);
    r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &s);
    host_poll_stop_after(0);
    *JS_GetInterruptFlag(v.rt) = 0;
    CHECK(JS_IsException(r), "an interrupt in the middle did not stop it");
    free(exception_text(v.ctx));
    JS_FreeValue(v.ctx, s);
    CHECK(eval_is(v.ctx, "esc('a<b') + esc(dirty)", "a&lt;ba&lt;b"), "esc after the interrupts");

    /* The flag is read at the start of each block of 4096 units but the first of the count, and before the first
       block of the write when the string has more than one block, so no more than 4096 units pass between two
       readings, also where the count ends and the write begins. The reads of the call itself are taken off. */
    eval_ok(v.ctx, "var none = '', one = '<'.repeat(4096), twoClean = 'a'.repeat(8192), two = '<'.repeat(8192), "
            "three = '<'.repeat(12288)");
    {
        long base = escape_reads(v.ctx, f, "none");

        CHECK(escape_reads(v.ctx, f, "one") - base == 0, "4096 units read the flag %ld times", escape_reads(v.ctx, f, "one") - base);
        CHECK(escape_reads(v.ctx, f, "twoClean") - base == 1, "8192 units with nothing to escape read the flag %ld times",
              escape_reads(v.ctx, f, "twoClean") - base);
        CHECK(escape_reads(v.ctx, f, "two") - base == 3, "8192 units to escape read the flag %ld times, want 3",
              escape_reads(v.ctx, f, "two") - base);
        CHECK(escape_reads(v.ctx, f, "three") - base == 5, "12288 units to escape read the flag %ld times, want 5",
              escape_reads(v.ctx, f, "three") - base);
    }

    /* a string too long once escaped: the RangeError of a string that cannot be built */
    CHECK(eval_ok(v.ctx, "var huge = '&'.repeat(1 << 28)"), "a string of 2**28 units");
    CHECK(eval_is(v.ctx, "try { esc(huge); 'made' } catch (e) { e.name + ': ' + e.message }", "RangeError: invalid string length"),
          "the length of 2**28 ampersands escaped");
    eval_ok(v.ctx, "huge = null");
    JS_FreeValue(v.ctx, f);
    vm_free(&v);

    /* no memory for the escaped string: the error of the engine, nothing left behind */
    v = vm_new();
    f = define_escape(v.ctx, "esc", 3);
    eval_ok(v.ctx, "var big = '&'.repeat(1 << 20)");
    room(&v, 1 << 20);
    s = global_prop(v.ctx, "big");
    r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &s);
    CHECK(JS_IsException(r), "the escape of a string with no memory for it");
    t = exception_text(v.ctx);
    CHECK(t && strcmp(t, "InternalError: out of memory") == 0, "the error with no memory: %s", t ? t : "?");
    free(t);
    JS_SetUncatchableException(v.ctx, FALSE);
    JS_FreeValue(v.ctx, s);
    unlimit(&v);
    CHECK(eval_is(v.ctx, "esc('<')", "&lt;"), "esc after running out of memory");
    JS_FreeValue(v.ctx, f);
    vm_free(&v);

    /* a name with bytes the engine reads as it is, and a name of more than one atom */
    v = vm_new();
    f = define_escape(v.ctx, "__tifa_escape\0ignored", 13);
    CHECK(eval_is(v.ctx, "__tifa_escape.name + ' ' + __tifa_escape('\\'')", "__tifa_escape &#x27;"), "the name");
    JS_FreeValue(v.ctx, f);
    vm_free(&v);
}

/* ---- bytecode cut short ---- */

/* A BigInt written by JS_WriteObject and read back cut short at every length:
   the read fails with an exception, and the BigInt it had allocated before the
   bytes ran out is freed (upstream QuickJS 79f58c1). vm_free finds a block
   left in use. Reading is for trusted bytes, so this checks only the memory,
   not that a cut buffer is always refused. */
static void test_read_truncated_bigint(void)
{
    static const char *const srcs[] = {
        "(1n << 200n) + 12345n", "-((1n << 200n) + 12345n)", "(1n << 64n) - 1n", "-(1n << 128n)",
        "(1n << 1000n) * 3n + 7n", "1n << 63n", "-(1n << 63n)",
    };
    int i;

    for (i = 0; i < (int)countof(srcs); i++) {
        VM v = vm_new();
        JSValue big = JS_Eval(v.ctx, srcs[i], strlen(srcs[i]), "<unit>", JS_EVAL_TYPE_GLOBAL);
        size_t size, n;
        uint8_t *buf;
        int refused = 0;

        CHECK(!JS_IsException(big), "%s", srcs[i]);
        buf = JS_WriteObject(v.ctx, &size, big, 0);
        CHECK(buf != NULL && size > 8, "%s written in %d bytes", srcs[i], (int)size);
        for (n = 0; buf && n < size; n++) {
            JSValue r = JS_ReadObject(v.ctx, buf, n, 0);

            if (JS_IsException(r)) {
                refused++;
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            } else {
                JS_FreeValue(v.ctx, r);
            }
        }
        CHECK(refused > 0, "%s: no cut length was refused", srcs[i]);
        if (buf) {
            JSValue r = JS_ReadObject(v.ctx, buf, size, 0);

            CHECK(!JS_IsException(r), "%s: the whole buffer is read", srcs[i]);
            JS_FreeValue(v.ctx, r);
            js_free(v.ctx, buf);
        }
        JS_FreeValue(v.ctx, big);
        vm_free(&v);
    }
}

/* A script with regexp literals, written as bytecode and read back, gives
   what it gives compiled: a regexp keeps the plan of lre_skip after its
   bytecode and its group names after the plan, and the bytecode of a
   regexp is a string of the constants of the function. Bytecode of another
   version, as of the version before the plan, is refused with an
   exception, not read. */
static void test_regexp_bytecode_read(void)
{
    static const char src[] =
        "var m = /(?<a>x)(?<b>[\"&<>])/.exec('yx<'), r = /(?:a|c)b/g;"
        "[m.index, m.groups.a, m.groups.b, 'acbab'.replace(r, '$&!'),"
        " /\\u4e2d+$/.test('a\\u4e2d\\u4e2d'), 'x\\xe9y'.search(/[\\xe0-\\xff]/),"
        " 'k1-2'.split(/[+\\-]?\\d/).length, 'a<b'.replace(/[<>]/g, '$<a>')].join()";
    static const char want[] = "1,x,<,acb!ab!,true,1,3,a$<a>b";
    VM v = vm_new();
    JSValue f = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    JSValue r;
    const char *s;
    uint8_t *buf;
    size_t size;

    CHECK(!JS_IsException(f), "compiled");
    buf = JS_WriteObject(v.ctx, &size, f, JS_WRITE_OBJ_BYTECODE);
    JS_FreeValue(v.ctx, f);
    CHECK(buf != NULL && size > 1, "written");
    if (buf) {
        r = JS_EvalFunction(v.ctx, JS_ReadObject(v.ctx, buf, size, JS_READ_OBJ_BYTECODE));
        s = JS_IsException(r) ? NULL : JS_ToCString(v.ctx, r);
        CHECK(s && !strcmp(s, want), "read back: %s, want %s", s ? s : "(exception)", want);
        JS_FreeCString(v.ctx, s);
        JS_FreeValue(v.ctx, r);
        buf[0]--;
        r = JS_ReadObject(v.ctx, buf, size, JS_READ_OBJ_BYTECODE);
        CHECK(JS_IsException(r), "bytecode of the version before is read");
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, JS_GetException(v.ctx));
        js_free(v.ctx, buf);
    }
    vm_free(&v);
}

/* where(n) is JS_GetScriptOrModuleName(ctx, n) as a string, "(none)" when
   there is none: n counts the frames from the one of where itself. */
static JSValue js_where(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    int32_t n = 0;
    JSAtom name;
    JSValue r;

    (void)this_val;
    if (argc > 0 && JS_ToInt32(ctx, &n, argv[0]) < 0)
        return JS_EXCEPTION;
    name = JS_GetScriptOrModuleName(ctx, n);
    if (name == JS_ATOM_NULL)
        return JS_NewString(ctx, "(none)");
    r = JS_AtomToString(ctx, name);
    JS_FreeAtom(ctx, name);
    return r;
}

/* JS_GetScriptOrModuleName counts the frames of the functions that run in
   the register loop one by one: one C entry of the register loop holds many
   of them (engine/regvm.h). outer of a.js calls mid of b.js, which calls
   inner of c.js, which calls where: frame 1 is inner, 2 mid, 3 outer, and
   there is no frame 4. The first call converts the functions, the calls
   after it run in the register loop. */
static void test_script_name_frames(void)
{
    static const char *const srcs[][2] = {
        { "a.js", "function outer(n) { 'use strict'; var r = mid(n); return r + ''; }" },
        { "b.js", "function mid(n) { 'use strict'; var r = inner(n); return r + ''; }" },
        { "c.js", "function inner(n) { 'use strict'; var r = where(n); return r + ''; }" },
    };
    static const char *const want[] = { "(none)", "c.js", "b.js", "a.js", "(none)" };
    VM v = vm_new();
    JSValue g, f, arg, r;
    char *t;
    int i, k;

    g = JS_GetGlobalObject(v.ctx);
    JS_SetPropertyStr(v.ctx, g, "where", JS_NewCFunction(v.ctx, js_where, "where", 1));
    for (i = 0; i < 3; i++) {
        r = JS_Eval(v.ctx, srcs[i][1], strlen(srcs[i][1]), srcs[i][0], JS_EVAL_TYPE_GLOBAL);
        CHECK(!JS_IsException(r), "%s does not run", srcs[i][0]);
        JS_FreeValue(v.ctx, r);
    }
    f = JS_GetPropertyStr(v.ctx, g, "outer");
    for (k = 0; k < 3; k++) {
        for (i = 0; i < 5; i++) {
            arg = JS_NewInt32(v.ctx, i);
            r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &arg);
            t = value_text(v.ctx, r);
            CHECK(t && strcmp(t, want[i]) == 0, "call %d: frame %d is of %s, want %s",
                  k, i, t ? t : "(exception)", want[i]);
            free(t);
            free(exception_text(v.ctx));
            JS_FreeValue(v.ctx, r);
        }
    }
    /* the code of each script runs once and may convert too: each of the
       three functions is asked */
    for (i = 0; i < 3; i++) {
        static const char *const names[] = { "outer", "mid", "inner" };
        JSValue h = JS_GetPropertyStr(v.ctx, g, names[i]);
        CHECK(JS_RegvmFuncStat(v.ctx, h, 0) == 1, "%s has no register code", names[i]);
        JS_FreeValue(v.ctx, h);
    }
    JS_FreeValue(v.ctx, g);
    JS_FreeValue(v.ctx, f);
    vm_free(&v);
}

/* job_ends ends the job that ran (JS_ClearKeptObjects), as a host does when
   control comes back to it, and runs the jobs that are queued, ending each. */
static void job_ends(VM *v)
{
    JSContext *jc;

    do
        JS_ClearKeptObjects(v->rt);
    while (JS_ExecutePendingJob(v->rt, &jc) > 0);
}

/* The WeakRef constructor and deref keep their target to the end of the job
   (AddToKeptObjects in the specification), and no longer: a target nothing
   else holds is there for deref in the job that made the WeakRef or read it,
   and gone in the next. */
static void test_weakref_kept(void)
{
    VM v = vm_new();
    size_t before;

    CHECK(eval_is(v.ctx, "globalThis.wr = new WeakRef({ a: 1 }); String(wr.deref()?.a)", "1"),
          "the target of a new WeakRef in its job");
    job_ends(&v);
    CHECK(eval_is(v.ctx, "String(wr.deref())", "undefined"), "the target outlived its job");

    CHECK(eval_ok(v.ctx, "globalThis.o = { b: 2 }; globalThis.wr2 = new WeakRef(o)"), "setup");
    job_ends(&v);
    CHECK(eval_is(v.ctx, "wr2.deref(); o = null; String(wr2.deref()?.b)", "2"),
          "the target of deref in its job");
    job_ends(&v);
    CHECK(eval_is(v.ctx, "String(wr2.deref())", "undefined"), "the target of deref outlived its job");

    CHECK(eval_is(v.ctx, "globalThis.ws = new WeakRef(Symbol('s')); String(ws.deref()?.description)", "s"),
          "a symbol target in its job");
    job_ends(&v);
    CHECK(eval_is(v.ctx, "String(ws.deref())", "undefined"), "a symbol target outlived its job");

    /* each promise job is a job of its own */
    CHECK(eval_ok(v.ctx, "globalThis.seen = []; Promise.resolve()"
                  ".then(() => { globalThis.wj = new WeakRef({}); seen.push(wj.deref() !== undefined) })"
                  ".then(() => seen.push(wj.deref() !== undefined))"), "setup");
    job_ends(&v);
    CHECK(eval_is(v.ctx, "seen.join()", "true,false"), "a job kept the target of the job before it");

    /* a function of the host that runs the jobs inside javascript does not
       end the job it is in, with jobs to run and with none */
    host_ctx = v.ctx;
    host_function_call = test_host_call;
    define_host(v.ctx, "drain", 0, 3);
    CHECK(eval_is(v.ctx, "globalThis.wd = new WeakRef({ d: 4 }); Promise.resolve().then(() => {});"
                  "String(drain()) + drain() + wd.deref()?.d", "104"),
          "running the jobs inside javascript ended its job");
    job_ends(&v);
    CHECK(eval_is(v.ctx, "String(wd.deref())", "undefined"), "the target outlived its job");

    /* the collections keep a target of the job that runs: the set holds it */
    CHECK(eval_ok(v.ctx, "globalThis.wg = new WeakRef({ g: 5 })"), "setup");
    JS_RunYoungGC(v.rt);
    JS_RunGC(v.rt);
    CHECK(eval_is(v.ctx, "String(wg.deref()?.g)", "5"), "a collection freed a target of the job");
    job_ends(&v);
    CHECK(eval_is(v.ctx, "String(wg.deref())", "undefined"), "the target outlived its job");

    /* a target read many times is kept once, and the memory of the targets
       of a job comes back after it */
    job_ends(&v);
    JS_RunGC(v.rt);
    before = JS_GetMemorySize(v.rt);
    CHECK(eval_ok(v.ctx, "globalThis.many = []; for (let i = 0; i < 10000; i++) many.push(new WeakRef({ i }));"
                  "for (let k = 0; k < 100; k++) for (const w of many) w.deref()"), "setup");
    CHECK(JS_GetMemorySize(v.rt) < before + (4 << 20),
          "10000 targets read 100 times each take %zu bytes in their job",
          JS_GetMemorySize(v.rt) - before);
    job_ends(&v);
    CHECK(eval_is(v.ctx, "String(many.filter(w => w.deref()).length)", "0"), "targets outlived their job");
    CHECK(eval_ok(v.ctx, "many = null"), "drop");
    job_ends(&v);
    JS_RunGC(v.rt);
    CHECK(JS_GetMemorySize(v.rt) <= before + (64 << 10), "the job left %zu bytes behind",
          JS_GetMemorySize(v.rt) - before);
    vm_free(&v);

    /* the runtime frees what it keeps when the host frees it in a job */
    v = vm_new();
    CHECK(eval_ok(v.ctx, "globalThis.wf = new WeakRef({ big: 'x'.repeat(1000) })"), "setup");
    vm_free(&v);
}

/* MOVW, SETLW and CHKLW name a local, argument, this or func with an
   int16_t: a function of 32700 locals is converted and reads its farthest
   local and this, in a build with RV_SELF_CHECK too. The parser refuses one
   of 40000 with its InternalError "too many local variables", as the
   frame fits RV_FAR_MAX (engine/regvm-func.h, test_rv_frame_limits). */
static void test_rv_far_locals(void)
{
    static const int counts[] = { 32700, 40000 };
    int c, i;

    for (c = 0; c < 2; c++) {
        int n = counts[c];
        size_t cap = (size_t)n * 9 + 256, len;
        char *src = malloc(cap);

        CHECK(src != NULL, "the source of %d locals", n);
        len = snprintf(src, cap, "(function () { 'use strict'; var v0 = 1");
        for (i = 1; i < n; i++)
            len += snprintf(src + len, cap - len, ", v%d", i);
        len += snprintf(src + len, cap - len,
                        "; v%d = 2; return (this === undefined) + ':' + (v0 + v%d) + ':' + v%d; })",
                        n - 1, n - 1, n / 2);
        {
            VM v = vm_new();
            JSValue f, r;
            char *t;
            int k;

            f = JS_Eval(v.ctx, src, len, "far.js", JS_EVAL_TYPE_GLOBAL);
            if (c == 1) {
                char *e = exception_text(v.ctx);
                CHECK(e && strcmp(e, "InternalError: too many local variables") == 0,
                      "%d locals: %s, want the parser's error", n, e ? e : "compiled");
                free(e);
                vm_free(&v);
                free(src);
                continue;
            }
            CHECK(!JS_IsException(f), "the function of %d locals does not compile", n);
            for (k = 0; k < 2; k++) {
                r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
                t = value_text(v.ctx, r);
                CHECK(t && strcmp(t, "true:3:undefined") == 0, "%d locals, call %d: %s",
                      n, k, t ? t : "(exception)");
                free(t);
                free(exception_text(v.ctx));
                JS_FreeValue(v.ctx, r);
            }
            CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 1, "%d locals: no register code (%d, op %d at %d)", n,
                  JS_RegvmFuncStat(v.ctx, f, 0), JS_RegvmFuncStat(v.ctx, f, 2), JS_RegvmFuncStat(v.ctx, f, 3));
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
        }
        free(src);
    }
}

/* C_CALL_ASAN and C_CALL_SAN: built with AddressSanitizer, and with any
   sanitizer that changes the frames (AddressSanitizer or UBSan) */
#if defined(__SANITIZE_ADDRESS__)
#define C_CALL_ASAN 1
#elif defined(__has_feature)
#if __has_feature(address_sanitizer)
#define C_CALL_ASAN 1
#endif
#endif
#ifndef C_CALL_ASAN
#define C_CALL_ASAN 0
#endif
#if C_CALL_ASAN
#define C_CALL_SAN 1
#elif defined(__has_feature)
#if __has_feature(undefined_behavior_sanitizer)
#define C_CALL_SAN 1
#endif
#endif
#ifndef C_CALL_SAN
#define C_CALL_SAN 0
#endif

/* the bytes a call that a direct level skips may save under AddressSanitizer
   beyond the debt the engine counts for it (see test_c_call_recursions) */
#if C_CALL_ASAN
#define C_CALL_SKIP_SLACK 80
#else
#define C_CALL_SKIP_SLACK 0
#endif

/* JS_RegvmStat 5 and 6, the bytes of stack a frame of JS_RegvmRun and of
   JS_CallInternal take before their checks, are measured: neither is 0, which
   is what a measure gives when the compiler inlines the call it measures.
   12, the part of 6 that js_call_c_hop counts in its check, is 0 or more
   and less than 6 while the direct calls are on, or -1 when the engine
   found the frame of js_call_c_hop larger than that of JS_CallInternal
   (Linux's UBSan build) and calls C functions from C through
   JS_CallInternal. 17 and 18, what a direct call and a call through
   js_call_c_hop count as taken while the C function runs (6 and 12 with the
   frame of js_call_c_function less that of js_call_c_plain), are 0 or more
   whenever their path is on: a debt below 0 turns its path off, and with
   17 that would send every call of a C function through JS_CallInternal
   with no test failing for it. With the direct calls off both are -1. The
   same measure gives the same answer each time the direct calls are turned
   on, from wherever it is called: under AddressSanitizer it measures from
   two places 16 bytes apart and keeps the smaller (see
   test_c_call_recursions). */
static void test_call_frames_measured(void)
{
    VM v = vm_new();
    int rv = JS_RegvmStat(v.ctx, 5), entry = JS_RegvmStat(v.ctx, 6);
    int hop = JS_RegvmStat(v.ctx, 12);
    int direct = JS_RegvmStat(v.ctx, 17), hop_run = JS_RegvmStat(v.ctx, 18);

    CHECK(rv > 0, "a frame of JS_RegvmRun takes %d bytes", rv);
    CHECK(entry > 0, "a frame of JS_CallInternal takes %d bytes", entry);
    /* 12: what js_call_c_hop counts as taken of that frame, -1 with the
       direct calls off, when every call goes through JS_CallInternal */
    CHECK(hop == -1 || (hop >= 0 && hop < entry),
          "js_call_c_hop counts %d bytes of %d", hop, entry);
    /* under AddressSanitizer both debts are the least a skipped frame
       saves, multiples of 32 (see test_c_call_recursions) */
    CHECK(!C_CALL_ASAN || (entry % 32 == 0 && (hop == -1 || hop % 32 == 0)),
          "AddressSanitizer: debts of %d and %d bytes, not rounded down to 32", entry, hop);
    /* a build without a sanitizer always has the room */
    CHECK(C_CALL_SAN || hop >= 0, "no sanitizer, and js_call_c_hop is off");
    /* 17 and 18: neither path is turned off by its debt */
    CHECK(direct >= 0, "the direct calls are off: their debt is %d", direct);
    CHECK((hop >= 0) == (hop_run >= 0),
          "js_call_c_hop counts %d bytes in its check and %d while the function runs",
          hop, hop_run);
    CHECK(!C_CALL_ASAN || (direct % 32 == 0 && (hop_run == -1 || hop_run % 32 == 0)),
          "AddressSanitizer: debts of %d and %d bytes while the function runs, not rounded down to 32",
          direct, hop_run);
    CHECK(JS_SetDirectCCalls(v.ctx, 0) == 0 && JS_RegvmStat(v.ctx, 12) == -1,
          "direct calls off: js_call_c_hop counts %d bytes", JS_RegvmStat(v.ctx, 12));
    CHECK(JS_RegvmStat(v.ctx, 17) == -1 && JS_RegvmStat(v.ctx, 18) == -1,
          "direct calls off: the debts are %d and %d",
          JS_RegvmStat(v.ctx, 17), JS_RegvmStat(v.ctx, 18));
    CHECK(JS_SetDirectCCalls(v.ctx, 1) == 0 &&
          JS_RegvmStat(v.ctx, 12) == hop,
          "direct calls on again: js_call_c_hop counts %d bytes, %d before",
          JS_RegvmStat(v.ctx, 12), hop);
    CHECK(JS_RegvmStat(v.ctx, 17) == direct && JS_RegvmStat(v.ctx, 18) == hop_run,
          "direct calls on again: a direct call counts %d bytes, %d before, js_call_c_hop %d, %d before",
          JS_RegvmStat(v.ctx, 17), direct, JS_RegvmStat(v.ctx, 18), hop_run);
    vm_free(&v);
}

/* Recursions that go through a C function the register loop calls itself
   (js_call_c_direct), alone or under a getter or a Proxy trap, or that a C
   function calls (js_call_fwd: call_call, apply_apply, bound_call, which
   start in Reflect.apply so that the loop never makes the call of call or
   of the bound function itself), with frames of 1 to 300 locals.

   A kind with HOP_DEEPER has its C function called from a bound function
   (js_call_bound_function), whose alloca makes a native build push, at each
   call, the arguments that do not fit in registers below it: 16 bytes more
   for the ten words of JS_CallInternal than for the seven of the call that
   stands for it (js_call_c_hop). There a native build may go deeper when
   the calls are direct, by a few levels in a hundred, never less deep. The
   generated engine passes arguments in the frame of the callee, which
   js_measure_call_frame measures, and TestCCallDepth requires the same
   depth for these too.

   Under AddressSanitizer, which aligns its stack objects to 32 bytes,
   JS_RegvmRun, js_call_c_function and js_call_bound_function (and
   JS_CallInternal on x86_64) set their stack pointer down to a multiple of
   32 bytes, so each of their frames is 16 bytes larger or not by where the
   frame above it ends, and they push the arguments of JS_CallInternal that
   do not fit in registers at each call. The engine leaves
   js_measure_call_frame out of the sanitizer, so that it measures the
   frames alone, from two places 16 bytes apart, and counts the smaller of
   each debt rounded down to a multiple of 32, the least a skipped frame
   saves where the next frame is set down (JS_FRAME_SET_DOWN of
   quickjs.c). A direct level with its debts then never takes more than a
   level through JS_CallInternal, and may take less: for each call it
   skips, less than 32 bytes the debt lost to the rounding, 16 by where the
   frame was set down, and 32 for the four words of JS_CallInternal that a
   caller set down pushes and the call that stands for it does not
   (C_CALL_SKIP_SLACK). With the hop on a level of call_call or apply_apply
   skips two calls (the direct call of Reflect.apply and the hop to the C
   function it calls), and a level of bound_call three (the hop from the
   bound function to call, and from that call to the call it makes), one
   with the hop off.
   So a direct run may end deeper, never less deep, by no more than these
   bytes explain against the bytes a level takes through JS_CallInternal,
   which the recursion measures as it runs (c_call_same). Linux's build
   with both sanitizers sets js_function_apply down too, and finds the
   frame of js_call_c_plain 32 bytes larger than that of
   js_call_c_function, so that the hop counts no debt while the function
   runs: with LLVM 22.1.8 a level of call_call there takes 2208 bytes through
   JS_CallInternal and 1856 direct with 224 counted, 128 left out. Without
   a sanitizer no frame is set down so, and the depths are the same.

   With the hop off (JS_RegvmStat 12 is -1: Linux's UBSan build, whose
   frame of js_call_c_hop is the larger) a C function calls a C function
   through JS_CallInternal, as before the hop, and the kinds with a hop end
   at the same depth both ways, HOP_DEEPER too. */
enum { HOP = 1, HOP_DEEPER = 2 };

/* skips: the calls a level skips with the hop on (one with it off) */
static const struct { const char *name, *body; int hop, skips; } c_call_kinds[] = {
    { "call", "return f.call(null) + v0", 0, 1 },
    { "apply", "return f.apply(null, []) + v0", 0, 1 },
    { "reflect_apply", "return Reflect.apply(f, null, []) + v0", 0, 1 },
    { "map", "return [1].map(f)[0] + v0", 0, 1 },
    { "map_tail", "return [0].map(f)", 0, 1 },
    { "sort", "[2, 1].sort(f); return v0", 0, 1 },
    { "getter", "return o.x + v0", 0, 1 },
    { "proxy_get", "return p.x + v0", 0, 1 },
    { "tail_apply", "return apply(f, null, [])", 0, 1 },
    { "call_call", "return apply(call, f, [null]) + v0", HOP, 2 },
    { "apply_apply", "return apply(apply, null, [f, null, []]) + v0", HOP, 2 },
    { "bound_call", "return apply(ucall, null, [f, null]) + v0", HOP_DEEPER, 3 },
};

typedef struct {
    int kind, locals, direct;
    int hop; /* the run had js_call_c_hop on (JS_RegvmStat 12) */
    int rv, entry; /* the frames of JS_RegvmRun and JS_CallInternal (5, 6) */
    int64_t level; /* the bytes of stack a level took, from level 10 to 12 */
    char *out;
} CCallRun;

long __shadow_remaining(void); /* host.c */

/* stackLeft() of c_call_run: the bytes of stack left in this thread */
static JSValue c_call_stack_left(JSContext *ctx, JSValueConst this_val,
                                 int argc, JSValueConst *argv)
{
    (void)this_val;
    (void)argc;
    (void)argv;
    return JS_NewInt64(ctx, __shadow_remaining());
}

/* Runs one recursion of c_call_kinds at the most call depth a runtime
   allows, with the direct calls of C functions on or off, and keeps
   "REACHED ERROR" in r->out, and in r->level the bytes of stack a level
   takes, measured as the recursion passes levels 10 to 12 (a call of
   stackLeft at each, after which its frame is gone). */
static int c_call_run(void *arg)
{
    CCallRun *r = arg;
    VM v = vm_new();
    char vars[8192], src[16384];
    int n = 0;

    n += snprintf(vars + n, sizeof(vars) - n, "var v0 = 0");
    for (int i = 1; i < r->locals; i++)
        n += snprintf(vars + n, sizeof(vars) - n, ", v%d = %d", i, i);
    snprintf(src, sizeof(src),
             "(() => { const apply = Reflect.apply, call = Function.prototype.call;\n"
             "  const ucall = call.bind(call); let reached = 0, l10 = 0, l12 = 0;\n"
             "  const o = { get x() { return apply(f, null, []) } };\n"
             "  const p = new Proxy({}, { get() { return [1].map(f)[0] } });\n"
             "  function f() { %s; reached++;\n"
             "    if (reached === 10) l10 = stackLeft(); else if (reached === 12) l12 = stackLeft();\n"
             "    %s }\n"
             "  let at = ''; try { f() } catch (e) { at = e instanceof RangeError ? 'RangeError' : String(e) }\n"
             "  globalThis.levelBytes = (l10 - l12) / 2;\n"
             "  return reached + ' ' + at })()",
             vars, c_call_kinds[r->kind].body);
    {
        JSValue g = JS_GetGlobalObject(v.ctx);

        JS_SetPropertyStr(v.ctx, g, "stackLeft",
                          JS_NewCFunction(v.ctx, c_call_stack_left, "stackLeft", 0));
        JS_FreeValue(v.ctx, g);
    }
    CHECK(JS_SetMaxCallDepth(v.rt, JS_MAX_CALL_DEPTH_LIMIT) == 0, "the call depth cannot be set");
    CHECK(JS_SetDirectCCalls(v.ctx, r->direct) == 0, "JS_SetDirectCCalls refused");
    r->hop = JS_RegvmStat(v.ctx, 12) >= 0;
    r->rv = JS_RegvmStat(v.ctx, 5);
    r->entry = JS_RegvmStat(v.ctx, 6);
    r->out = eval_text(v.ctx, src);
    free(exception_text(v.ctx));
    {
        JSValue g = JS_GetGlobalObject(v.ctx);
        JSValue b = JS_GetPropertyStr(v.ctx, g, "levelBytes");

        CHECK(JS_ToInt64(v.ctx, &r->level, b) == 0, "levelBytes is not a number");
        JS_FreeValue(v.ctx, b);
        JS_FreeValue(v.ctx, g);
    }
    vm_free(&v);
    return 0;
}

static void *c_call_thread(void *arg)
{
    c_call_run(arg);
    return NULL;
}

/* c_call_slack: the bytes a level of kind k may take through
   JS_CallInternal beyond what a direct level takes with its debts, which no
   measure sees (c_call_kinds): C_CALL_SKIP_SLACK for each call the level
   skips, and with the hop on, 16 for the arguments pushed below a bound
   function of a kind with HOP_DEEPER */
static int c_call_slack(int k, int hop)
{
    return (hop ? c_call_kinds[k].skips : 1) * C_CALL_SKIP_SLACK +
        (hop && c_call_kinds[k].hop == HOP_DEEPER ? 16 : 0);
}

/* c_call_same: TRUE when the run d directly ended as the run t through
   JS_CallInternal, with the same error, never less deep, and deeper by no
   more than the slack explains: a level of t takes t->level bytes, and one
   of d with its debts at least t->level - s with s bytes of slack a level,
   so the m levels of d take no more stack than the n levels of t and the
   one t stopped in when m * (t->level - s) <= (n + 1) * t->level */
static int c_call_same(int k, const CCallRun *t, const CCallRun *d)
{
    const char *a, *b;
    long n, m, s, level;

    if (!t->out || !d->out)
        return 0;
    if (strcmp(t->out, d->out) == 0)
        return 1;
    a = strchr(t->out, ' ');
    b = strchr(d->out, ' ');
    n = atol(t->out);
    m = atol(d->out);
    if (!a || !b || strcmp(a, b) != 0 || m < n)
        return 0;
    s = c_call_slack(k, d->hop);
    level = (long)t->level;
    return s > 0 && level > s && m * (level - s) <= (n + 1) * level;
}

/* Every recursion of c_call_kinds ends in a RangeError, never past the end
   of the stack, on the main thread (8 MiB, the smallest stack the tests run
   the engine on) and on a thread of 1 MiB, where the stack ends it before
   the call depth. And it ends at the same depth whether the register loop
   calls the C functions itself or through JS_CallInternal: a direct call
   counts the frame of JS_CallInternal it skips as taken (stack_debt), so a
   measure of that frame of 0 would let it go deeper. Under a sanitizer, or
   for a kind with HOP_DEEPER, a direct run may end a little deeper, as
   c_call_kinds says. */
static void test_c_call_recursions(void)
{
    static const int locals[] = { 1, 100, 300 };

    for (int stack = 0; stack < 2; stack++) {
        for (int k = 0; k < (int)countof(c_call_kinds); k++) {
            for (int l = 0; l < (int)countof(locals); l++) {
                CCallRun r[2];

                for (int d = 0; d < 2; d++) {
                    r[d] = (CCallRun){ .kind = k, .locals = locals[l], .direct = d };
                    if (stack == 0) {
                        c_call_run(&r[d]);
                    } else {
                        pthread_attr_t attr;
                        pthread_t tid;

                        pthread_attr_init(&attr);
                        pthread_attr_setstacksize(&attr, (size_t)1 << 20);
                        CHECK(pthread_create(&tid, &attr, c_call_thread, &r[d]) == 0, "no thread");
                        pthread_attr_destroy(&attr);
                        pthread_join(tid, NULL);
                    }
                    CHECK(r[d].out && strstr(r[d].out, " RangeError"), "%s, %s, %d locals, direct %d: %s",
                          stack ? "1 MiB" : "main thread", c_call_kinds[k].name, locals[l], d,
                          r[d].out ? r[d].out : "(exception)");
                }
                CHECK(c_call_same(k, &r[0], &r[1]),
                      "%s, %s, %d locals: through JS_CallInternal %s, direct %s (slack %d, a level %lld and %lld bytes, frames %d and %d, hop %d)",
                      stack ? "1 MiB" : "main thread", c_call_kinds[k].name, locals[l],
                      r[0].out ? r[0].out : "?", r[1].out ? r[1].out : "?",
                      c_call_slack(k, r[1].hop), (long long)r[0].level, (long long)r[1].level,
                      r[1].rv, r[1].entry, r[1].hop);
                free(r[0].out);
                free(r[1].out);
            }
        }
    }
}

/* hop_reads returns how many times a call of fn from C (JS_Call) reads the
   flag that stops the engine, and its result in *ok (FALSE on an exception,
   which it takes). */
static long hop_reads(JSContext *ctx, JSValueConst fn, JSValueConst this_obj,
                      int argc, JSValueConst *argv, BOOL *ok)
{
    long before = host_poll_reads;
    JSValue r = JS_Call(ctx, fn, this_obj, argc, argv);

    before = host_poll_reads - before;
    *ok = !JS_IsException(r);
    if (!*ok)
        free(exception_text(ctx));
    JS_FreeValue(ctx, r);
    return before;
}

/* hold_uncatchable leaves an uncatchable exception pending and returns, as
   a host function should not: the next call must not run. */
static JSValue hold_uncatchable(JSContext *ctx, JSValueConst this_val,
                                int argc, JSValueConst *argv)
{
    JS_ThrowInternalError(ctx, "held");
    JS_SetUncatchableException(ctx, TRUE);
    return JS_UNDEFINED;
}

/* A call of a plain C function from C (JS_Call here, through
   JS_CallInternal, then Reflect.apply's own call of push, in js_call_c_hop
   when the calls are direct and the hop is on) reads the interrupt flag once and stops on an
   interrupt or a pending uncatchable exception before the function runs:
   one read for one hop, two for two, and push runs only when nothing stops
   it. */
static void test_c_call_hop_checks(void)
{
    for (int d = 0; d < 2; d++) {
        VM v = vm_new();
        JSValue push, apply, a, argv[3];
        BOOL ok;
        long n;

        CHECK(JS_SetDirectCCalls(v.ctx, d) == 0, "JS_SetDirectCCalls refused");
        /* the hop is off with the direct calls off, and on with them unless
           the engine found its frame larger (test_call_frames_measured):
           the reads below are the same either way */
        CHECK(d || JS_RegvmStat(v.ctx, 12) == -1, "direct 0: the hop is on");
        eval_ok(v.ctx, "var a = [], push = Array.prototype.push, apply = Reflect.apply, one = [1]");
        push = global_prop(v.ctx, "push");
        apply = global_prop(v.ctx, "apply");
        a = global_prop(v.ctx, "a");
        argv[0] = push;
        argv[1] = a;
        argv[2] = global_prop(v.ctx, "one");

        n = hop_reads(v.ctx, push, a, 1, (JSValueConst *)&argv[2], &ok);
        CHECK(ok && n == 1, "direct %d: one hop reads the flag %ld times (ok %d), want 1", d, n, ok);
        n = hop_reads(v.ctx, apply, JS_UNDEFINED, 3, (JSValueConst *)argv, &ok);
        CHECK(ok && n == 2, "direct %d: two hops read the flag %ld times (ok %d), want 2", d, n, ok);
        CHECK(eval_is(v.ctx, "a.length", "2"), "direct %d: push did not run twice", d);

        /* stopped at the first hop, then at the second: push does not run */
        *JS_GetInterruptFlag(v.rt) = 1;
        hop_reads(v.ctx, push, a, 1, (JSValueConst *)&argv[2], &ok);
        *JS_GetInterruptFlag(v.rt) = 0;
        JS_SetUncatchableException(v.ctx, FALSE);
        CHECK(!ok, "direct %d: an interrupt did not stop the hop", d);
        host_poll_stop_after(2);
        hop_reads(v.ctx, apply, JS_UNDEFINED, 3, (JSValueConst *)argv, &ok);
        host_poll_stop_after(0);
        *JS_GetInterruptFlag(v.rt) = 0;
        JS_SetUncatchableException(v.ctx, FALSE);
        CHECK(!ok, "direct %d: an interrupt at the second hop did not stop it", d);
        CHECK(eval_is(v.ctx, "a.length", "2"), "direct %d: push ran after an interrupt", d);

        /* an uncatchable exception pending stops the hop too */
        JS_ThrowInternalError(v.ctx, "held");
        JS_SetUncatchableException(v.ctx, TRUE);
        n = host_poll_reads;
        {
            JSValue r = JS_Call(v.ctx, push, a, 1, (JSValueConst *)&argv[2]);
            CHECK(JS_IsException(r), "direct %d: a hop ran with an uncatchable exception pending", d);
            JS_FreeValue(v.ctx, r);
        }
        CHECK(host_poll_reads - n == 1, "direct %d: %ld reads with an uncatchable exception pending, want 1",
              d, host_poll_reads - n);
        free(exception_text(v.ctx));
        JS_SetUncatchableException(v.ctx, FALSE);
        CHECK(eval_is(v.ctx, "a.length", "2"), "direct %d: push ran with an uncatchable exception pending", d);

        /* and at the second hop: the getter of the list Reflect.apply reads
           leaves one pending, before Reflect.apply calls push */
        {
            JSValue g = JS_GetGlobalObject(v.ctx);
            JS_SetPropertyStr(v.ctx, g, "hold", JS_NewCFunction(v.ctx, hold_uncatchable, "hold", 0));
            JS_FreeValue(v.ctx, g);
        }
        {
            char *t = eval_text(v.ctx, "apply(push, a, { length: 1, get 0() { hold(); return 3 } })");
            CHECK(!t, "direct %d: the second hop ran with an uncatchable exception pending", d);
            free(t);
        }
        free(exception_text(v.ctx));
        JS_SetUncatchableException(v.ctx, FALSE);
        CHECK(eval_is(v.ctx, "a.length", "2"), "direct %d: push ran after the getter left an uncatchable exception", d);

        JS_FreeValue(v.ctx, push);
        JS_FreeValue(v.ctx, apply);
        JS_FreeValue(v.ctx, a);
        JS_FreeValue(v.ctx, argv[2]);
        vm_free(&v);
    }
}

/* new in the register loop (engine/regvm-ops-ctor.h): a constructor given
   more arguments than it has parameters runs in the loop of the function
   that calls new, as a call with extra arguments does, and starts no run of
   JS_RegvmRun of its own (JS_RegvmStat 2 counts those runs). */
static void test_rv_new_in_loop(void)
{
    VM v = vm_new();
    JSValue f, arg, r;
    int32_t got;
    int runs;

    CHECK(eval_ok(v.ctx,
                  "function K(a) { this.a = a; this.n = arguments.length; this.x = arguments[2]; }\n"
                  "class D extends K { constructor(a) { super(a, 1, 2, 3); } }\n"
                  "function extra(n) { var s = 0; for (var i = 0; i < n; i++)"
                  " s += new K(i, 1, 2).n + new K(i, 1, i).x + new D(i, 5).n; return s; }"),
          "the scripts of new");
    f = global_prop(v.ctx, "extra");
    for (int k = 0; k < 2; k++) {
        arg = JS_NewInt32(v.ctx, k ? 100 : 2);
        runs = JS_RegvmStat(v.ctx, 2);
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, (JSValueConst *)&arg);
        runs = JS_RegvmStat(v.ctx, 2) - runs;
        CHECK(JS_ToInt32(v.ctx, &got, r) == 0 && got == (k ? 7 * 100 + 4950 : 15),
              "extra(%d) gave %d", k ? 100 : 2, got);
        JS_FreeValue(v.ctx, r);
        /* the first call converts K and D in runs of their own */
        if (k)
            CHECK(runs == 1, "extra(100) made %d runs of the register loop, want 1", runs);
    }
    JS_FreeValue(v.ctx, f);

    /* a new whose function holds its prototype as a plain data property
       reads the interrupt flag once, as a call does: no JavaScript runs
       between the call's read and the object made. One whose prototype is
       not an object, or is made at the first read, reads it again after,
       as QuickJS's interpreter does after the getter of a prototype. */
    CHECK(eval_ok(v.ctx,
                  "function P(a) { this.a = a; }\n"
                  "function Q(a) { this.a = a; } Q.prototype = 1;\n"
                  "function c(a) { return a; }\n"
                  "function news(n) { var s = 0; for (var i = 0; i < n; i++) s += new P(i).a; return s; }\n"
                  "function prims(n) { var s = 0; for (var i = 0; i < n; i++) s += new Q(i).a; return s; }\n"
                  "function extras(n) { var s = 0; for (var i = 0; i < n; i++) s += new P(i, i).a; return s; }\n"
                  "function calls(n) { var s = 0; for (var i = 0; i < n; i++) s += c(i); return s; }\n"
                  "news(2); prims(2); extras(2); calls(2);"),
          "the scripts of the reads");
    {
        /* the reads of 100 more turns of the loop of each function, and
           how many more than those of calls they must be */
        static const struct { const char *fn; int more; } cases[] = {
            { "calls", 0 }, { "news", 0 }, { "extras", 0 }, { "prims", 100 },
        };
        long calls = 0;
        for (size_t i = 0; i < countof(cases); i++) {
            JSValue g = global_prop(v.ctx, cases[i].fn);
            long reads[2];
            BOOL ok;
            for (int k = 0; k < 2; k++) {
                arg = JS_NewInt32(v.ctx, k ? 200 : 100);
                reads[k] = hop_reads(v.ctx, g, JS_UNDEFINED, 1, (JSValueConst *)&arg, &ok);
                CHECK(ok, "%s threw", cases[i].fn);
            }
            JS_FreeValue(v.ctx, g);
            if (!i)
                calls = reads[1] - reads[0];
            CHECK(reads[1] - reads[0] == calls + cases[i].more,
                  "100 more turns of %s read the flag %ld more times, those of calls %ld, want %ld more",
                  cases[i].fn, reads[1] - reads[0], calls, calls + cases[i].more);
        }
    }
    vm_free(&v);
}

/* The parameters and locals of a function, with this, the function and
   new.target, fit RV_FAR_MAX (32768, engine/regvm-func.h): 32765 of them
   together compile and run, one more fails to compile with the parser's
   InternalError "too many local variables" or "too many arguments",
   whichever it adds last, and so do 40000 parameters. */
static void test_rv_frame_limits(void)
{
    static const struct { int params, locals; const char *err; } cases[] = {
        { 0, 32765, NULL }, { 0, 32766, "InternalError: too many local variables" },
        { 32764, 1, NULL }, { 32765, 1, "InternalError: too many local variables" },
        { 32766, 1, "InternalError: too many arguments" },
        { 16383, 16382, NULL }, { 16383, 16383, "InternalError: too many local variables" },
        { 40000, 1, "InternalError: too many arguments" },
    };
    int c, i;

    for (c = 0; c < (int)countof(cases); c++) {
        int np = cases[c].params, nl = cases[c].locals;
        size_t cap = (size_t)(np + nl) * 9 + 256, len;
        char *src = malloc(cap);
        const char *want;

        CHECK(src != NULL, "the source of %d parameters", np);
        /* the locals are r and v1 up */
        len = snprintf(src, cap, "(function (");
        for (i = 0; i < np; i++)
            len += snprintf(src + len, cap - len, "%sp%d", i ? ", " : "", i);
        len += snprintf(src + len, cap - len, ") { 'use strict'; var r = 5");
        for (i = 1; i < nl; i++)
            len += snprintf(src + len, cap - len, ", v%d", i);
        if (nl > 1)
            len += snprintf(src + len, cap - len, "; return r + ':' + (v%d === undefined); })", nl - 1);
        else
            len += snprintf(src + len, cap - len, "; return r + ':' + (r === undefined); })");
        want = cases[c].err;
        {
            VM v = vm_new();
            JSValue f, r, arg;
            char *t, *e;

            f = JS_Eval(v.ctx, src, len, "frame.js", JS_EVAL_TYPE_GLOBAL);
            e = exception_text(v.ctx);
            if (want) {
                CHECK(e && strcmp(e, want) == 0, "%d parameters, %d locals: %s, want %s", np, nl,
                      e ? e : "compiled", want);
            } else {
                CHECK(!e, "%d parameters, %d locals: %s", np, nl, e);
                arg = JS_NewInt32(v.ctx, 5);
                for (i = 0; i < 2 && !e; i++) {
                    r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &arg);
                    t = value_text(v.ctx, r);
                    CHECK(t && strcmp(t, nl > 1 ? "5:true" : "5:false") == 0, "%d parameters, %d locals, call %d: %s",
                          np, nl, i, t ? t : "(exception)");
                    free(t);
                    free(exception_text(v.ctx));
                    JS_FreeValue(v.ctx, r);
                }
                if (!cases[c].err)
                    CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 1, "%d parameters, %d locals: no register code (%d)",
                          np, nl, JS_RegvmFuncStat(v.ctx, f, 0));
            }
            free(e);
            JS_FreeValue(v.ctx, f);
            vm_free(&v);
        }
        free(src);
    }
}

/* A function whose stack is deeper than RV_STACK_FIT (118) folds it into a
   frame that keeps RV_FOLD_PINS_MAX (48) temps for its catch offsets
   (engine/regvm-func.h): a try takes one, a for-of three. The parser
   refuses a function that needs more with its InternalError "stack
   overflow", at compile time: 118 nested try or 39 nested for-of around
   r += a (their own catch offsets make the stack too deep), 49 nested try
   or 17 nested for-of around an expression 130 deep. One fewer compiles
   and runs, in sloppy and strict code and in a generator, and a plain
   function of it converts to register code. */
static const char nest_js[] =
    "function rep(s, n) { var r = '', i; for (i = 0; i < n; i++) r += s; return r; }\n"
    "function tries(n, body) { return rep('try { ', n) + body + rep(' } catch (e) { r = -1; }', n); }\n"
    "function forofs(n, body) { var r = '', i; for (i = 0; i < n; i++) r += 'for (const x' + i + ' of A) ';\n"
    "  return r + '{ ' + body + ' }'; }\n"
    "function deep(n) { return 'r += ' + rep('a + (', n) + 'a' + rep(')', n) + ';'; }\n"
    "var wrap = {\n"
    "  sloppy: function (s) { return 'var r = 0; ' + s + ' return r;'; },\n"
    "  strict: function (s) { return \"'use strict'; var r = 0; \" + s + ' return r;'; },\n"
    "  generator: function (s) { return 'var g = function* () { var r = 0; ' + s + ' yield r; };\\n' +\n"
    "    'return g().next().value;'; }\n"
    "};\n"
    "function make(kind, what, n, d) {\n"
    "  var body = d ? deep(d) : 'r += a;';\n"
    "  return Function('A, a', wrap[kind](what === 'try' ? tries(n, body) : forofs(n, body)));\n"
    "}\n";

static void test_rv_nesting_limits(void)
{
    static const char *const kinds[] = { "sloppy", "strict", "generator" };
    static const struct { const char *what; int deep, limit; } cases[] = {
        { "try", 0, 118 }, { "for-of", 0, 39 }, { "try", 130, 49 }, { "for-of", 130, 17 },
    };
    char src[256], *t, *e;
    int k, c, over;

    for (k = 0; k < 3; k++) {
        for (c = 0; c < 4; c++) {
            for (over = 0; over < 2; over++) {
                int n = cases[c].limit - 1 + over;
                {
                    VM v = vm_new();
                    JSValue g, f, r, args[2];
                    int i, refused;

                    CHECK(eval_ok(v.ctx, nest_js), "the setup");
                    snprintf(src, sizeof(src), "make('%s', '%s', %d, %d)", kinds[k], cases[c].what, n,
                             cases[c].deep);
                    f = JS_Eval(v.ctx, src, strlen(src), "nest.js", JS_EVAL_TYPE_GLOBAL);
                    t = exception_text(v.ctx);
                    refused = over;
                    if (refused)
                        CHECK(t && strncmp(t, "InternalError: stack overflow", 29) == 0,
                              "%s, %d %s around %d: %s, want the parser's error", kinds[k], n, cases[c].what,
                              cases[c].deep, t ? t : "compiled");
                    else
                        CHECK(!t, "%s, %d %s around %d: %s", kinds[k], n, cases[c].what, cases[c].deep, t);
                    free(t);
                    if (!refused && !JS_IsException(f)) {
                        g = JS_GetGlobalObject(v.ctx);
                        args[0] = JS_NewArray(v.ctx);
                        JS_SetPropertyUint32(v.ctx, args[0], 0, JS_NewInt32(v.ctx, 1));
                        args[1] = JS_NewInt32(v.ctx, 1);
                        for (i = 0; i < 2; i++) {
                            r = JS_Call(v.ctx, f, JS_UNDEFINED, 2, args);
                            t = value_text(v.ctx, r);
                            e = t ? NULL : exception_text(v.ctx);
                            CHECK(t && atoi(t) == cases[c].deep + 1, "%s, %d %s around %d, call %d: %s", kinds[k], n,
                                  cases[c].what, cases[c].deep, i, t ? t : e ? e : "(exception)");
                            free(t);
                            free(e);
                            JS_FreeValue(v.ctx, r);
                        }
                        if (k < 2 && !over)
                            CHECK(JS_RegvmFuncStat(v.ctx, f, 0) == 1, "%s, %d %s around %d: no register code (%d, %d)",
                                  kinds[k], n, cases[c].what, cases[c].deep, JS_RegvmFuncStat(v.ctx, f, 0),
                                  JS_RegvmFuncStat(v.ctx, f, 3));
                        JS_FreeValue(v.ctx, args[0]);
                        JS_FreeValue(v.ctx, g);
                    }
                    JS_FreeValue(v.ctx, f);
                    vm_free(&v);
                }
            }
        }
    }
}

/* names_src writes in buf a source of n names declared by decl ("var" or
   "const"), ten to a statement, then an array of n / 10 functions that each
   refer to 100 of them from a function inside, around body:
   "(function(){%s})" for a function, "%s" for a script or a module. The
   functions come last, so that the pass that captures by value
   (capture_by_value in engine/quickjs.c), which goes over the code after
   each function made, has little to go over. */
static void names_src(char *buf, size_t size, const char *body, const char *decl, int n)
{
    static char s[1 << 22];
    size_t k = 0;
    int i, j, r = 1;

#define ADD(...) (k += (size_t)snprintf(s + k, sizeof(s) - k, __VA_ARGS__))
    for (i = 0; i < n; i++)
        ADD("%s%sn%d=%d%s", i % 10 ? "" : decl, i % 10 ? "" : " ", i, i, i % 10 == 9 ? ";" : ",");
    ADD("[");
    for (i = 0; i < n / 10; i++) {
        ADD("function(a){return function(){return 0");
        for (j = 0; j < 100; j++) {
            r = r * 1103515245 + 12345;
            ADD("+n%d", (int)((unsigned)r >> 8) % n);
        }
        ADD("}},");
    }
    ADD("];");
#undef ADD
    snprintf(buf, size, body, s);
}

/* names_reads returns how many times the compile of src reads the interrupt
   flag, which it does after a count of its work (rt->compile_left): one unit
   for each entry a look for a name goes over. */
static long names_reads(const char *src, int flags)
{
    VM v = vm_new();
    long reads = host_poll_reads;
    JSValue r = JS_Eval(v.ctx, src, strlen(src), "names.js", flags | JS_EVAL_FLAG_COMPILE_ONLY);

    reads = host_poll_reads - reads;
    CHECK(!JS_IsException(r), "the source of names does not compile");
    JS_FreeValue(v.ctx, r);
    vm_free(&v);
    return reads;
}

/* A bundle puts thousands of names in one function, and the functions in it
   refer to them: the compile of each reference must not look through all the
   names around it, or the compile grows as the square of the bundle. Eight
   times the names and the references must cost at most twelve times the
   work, where the square makes it sixty-four. */
static void test_compile_names_linear(void)
{
    static const char *decls[] = { "var", "const" };
    static char src[1 << 22];
    long small, large;
    size_t i;

    for (i = 0; i < countof(decls); i++) {
        names_src(src, sizeof(src), "(function(){%s})", decls[i], 500);
        small = names_reads(src, JS_EVAL_TYPE_GLOBAL);
        names_src(src, sizeof(src), "(function(){%s})", decls[i], 4000);
        large = names_reads(src, JS_EVAL_TYPE_GLOBAL);
        CHECK(large <= 12 * small, "%ss in a function: the compile of 4,000 names read the flag %ld times, of 500 %ld",
              decls[i], large, small);
    }
}

/* The blocks a function has left are not on the chain of its names: 8,000
   names declared after 2,000 blocks, or before them, must not cost each
   declaration (or each block) a look through all the names before it, which
   grows as the square, to millions of entries and thousands of readings of
   the flag. closed_blocks_reads counts the readings of the compile of the
   names with the blocks before (where 1), after (2) or none (0). Each is a
   const, as a let statement reads the flag once to see what follows it. */
static long closed_blocks_reads(int where)
{
    static char src[1 << 18];
    size_t k = 0;
    int i;

#define ADD(...) (k += (size_t)snprintf(src + k, sizeof(src) - k, __VA_ARGS__))
    ADD("(function(){");
    for (i = 0; where == 1 && i < 2000; i++)
        ADD("{const t=0;}");
    for (i = 0; i < 8000; i++)
        ADD(i ? ",v%d=0" : "const v%d=0", i);
    ADD(";");
    for (i = 0; where == 2 && i < 2000; i++)
        ADD("{const t=0;}");
    ADD("})");
#undef ADD
    return names_reads(src, JS_EVAL_TYPE_GLOBAL);
}

static void test_compile_names_closed_blocks(void)
{
    long none = closed_blocks_reads(0), before = closed_blocks_reads(1),
        after = closed_blocks_reads(2);

    CHECK(before <= 2 * none && after <= 2 * none,
          "8,000 names with 2,000 closed blocks: the compile read the flag %ld times with the blocks before them, %ld after, %ld with none",
          before, after, none);
}

/* A compile stopped at a reading of the interrupt flag frees what it made,
   the code of each pass included, by its opcodes: compile_stops compiles src
   in a VM of its own once to count its readings, then stops it at each of
   them. Each stop ends with the error interrupted, or, at a reading of the
   conversion to the register code, which leaves the function to convert at
   its call, with the compile done and the flag still set, and the word after
   the flag is 0 again (the mark of a stopped compile, JS_GetInterruptFlag in
   engine/quickjs.h). Run under AddressSanitizer, it shows that no stop leaves
   code that the freeing reads past its opcodes, and the VM, freed at the end,
   that no stop leaks. The count of the work goes on from one compile to the
   next (rt->compile_left in engine/quickjs.c), which moves the readings of a
   compile by one either way: each compile starts where a stop at the first
   reading leaves the count (compile_stop_first), so that each reads the flag
   where the one that counted did. */
static void compile_stop_first(VM *v, const char *src, int flags)
{
    volatile int *flag = JS_GetInterruptFlag(v->rt);
    JSValue r;

    host_poll_stop_after(1);
    r = JS_Eval(v->ctx, src, strlen(src), "stops.js", flags | JS_EVAL_FLAG_COMPILE_ONLY);
    host_poll_stop_after(0);
    *flag = 0;
    if (JS_IsException(r))
        r = JS_GetException(v->ctx);
    JS_FreeValue(v->ctx, r);
}

static void compile_stops(const char *what, const char *src, int flags)
{
    VM v = vm_new();
    volatile int *flag = JS_GetInterruptFlag(v.rt);
    long reads, k;
    JSValue r;
    char *t;
    int set;

    compile_stop_first(&v, src, flags);
    reads = host_poll_reads;
    r = JS_Eval(v.ctx, src, strlen(src), "stops.js", flags | JS_EVAL_FLAG_COMPILE_ONLY);
    reads = host_poll_reads - reads;
    CHECK(!JS_IsException(r), "%s does not compile", what);
    JS_FreeValue(v.ctx, r);
    for (k = 1; k <= reads && !failures; k++) {
        compile_stop_first(&v, src, flags);
        host_poll_stop_after(k);
        r = JS_Eval(v.ctx, src, strlen(src), "stops.js", flags | JS_EVAL_FLAG_COMPILE_ONLY);
        host_poll_stop_after(0);
        set = *flag;
        *flag = 0;
        if (JS_IsException(r)) {
            t = exception_text(v.ctx);
            CHECK(t && strcmp(t, "InternalError: interrupted") == 0, "%s: the stop at reading %ld of %ld: %s", what,
                  k, reads, t ? t : "no exception");
            free(t);
        } else {
            CHECK(set, "%s: the stop at reading %ld of %ld did not stop the compile", what, k, reads);
            JS_FreeValue(v.ctx, r);
        }
        CHECK(flag[1] == 0, "%s: the stop at reading %ld of %ld left the mark at %d", what, k, reads, flag[1]);
    }
    vm_free(&v);
}

/* Loops whose jumps back resolve_labels makes shorter after the jump out of
   each is: it did so by changing the opcode, then taking the bytes it no
   longer needs out, and a jump back keeps a negative offset in them, whose
   0xff a free in between read as an opcode past the table. Each loop is one
   statement longer than the one before, so that some of them end where the
   jump back fits in a byte only after the jump out is short. The 8,192 ifs
   after them make the work of each shrink more than a step of the count, so
   that the flag is read at each. Their statements start with no name, which
   the parse would look past (peek_token), reading the flag each time.
   Then the closure variables of an eval in five functions around the locals
   of a sixth, which the compile collects (add_eval_variables), and a module
   with a table of exports. */
static void test_compile_stops(void)
{
    static char src[1 << 20];
    size_t n = 0;
    int i, k;

#define ADD(...) (n += (size_t)snprintf(src + n, sizeof(src) - n, __VA_ARGS__))
    ADD("(function(){var a = 0, b = 0;");
    for (k = 40; k <= 80; k++) {
        ADD("while(!a){");
        for (i = 0; i < k; i++)
            ADD("++a;");
        ADD("}");
    }
    for (i = 0; i < 8192; i++)
        ADD("if(!a)++b;");
    ADD("})");
    compile_stops("loops with jumps back", src, JS_EVAL_TYPE_GLOBAL);

    n = 0;
    ADD("(function(){var x0");
    for (i = 1; i < 300; i++)
        ADD(",x%d", i);
    ADD(";return function(){return function(){return function(){return function(){"
        "return function(){return eval('x0')}}}}}})");
    compile_stops("an eval in five functions", src, JS_EVAL_TYPE_GLOBAL);

    n = 0;
    ADD("var a0");
    for (i = 1; i < 2000; i++)
        ADD(",a%d", i);
    ADD(";export {a0 as e0");
    for (i = 1; i < 2000; i++)
        ADD(",a%d as e%d", i, i);
    ADD("}");
    compile_stops("a module of 2,000 exports", src, JS_EVAL_TYPE_MODULE);
#undef ADD
    /* the indexes of the names (find_var in engine/quickjs.c) */
    names_src(src, sizeof(src), "(function(){%s})", "var", 400);
    compile_stops("400 vars in a function", src, JS_EVAL_TYPE_GLOBAL);
    names_src(src, sizeof(src), "%s", "const", 400);
    compile_stops("400 consts of a script", src, JS_EVAL_TYPE_GLOBAL);
}

/* A module that imports each of 300 names of another one, then all of them
   as a namespace, then the module 300 times more, stopped at each reading of
   its eval, in a VM of its own each time, the other module evaluated first:
   the resolution of the imports, the making of the namespace and the walks
   of the link and of the evaluation read the flag. Each stop ends the eval
   with the error interrupted, or the promise of the module rejected with it
   (a stop in the evaluation), or the eval done with the flag still set.
   Run under AddressSanitizer, it shows that what each failure frees is
   whole, and the VM, freed at the end, that no stop leaks. */
static void test_link_stops(void)
{
    static char a[1 << 14], b[1 << 15];
    size_t n;
    long reads = 0, k, before;
    JSValue r, e;
    char *t;
    int i, set;

#define ADD(s, ...) (n += (size_t)snprintf(s + n, sizeof(s) - n, __VA_ARGS__))
    n = 0;
    ADD(a, "export var e0");
    for (i = 1; i < 300; i++)
        ADD(a, ",e%d", i);
    ADD(a, ";");
    n = 0;
    ADD(b, "import {e0");
    for (i = 1; i < 300; i++)
        ADD(b, ",e%d", i);
    ADD(b, "} from \"a.js\"; import * as ns from \"a.js\";");
    for (i = 0; i < 300; i++)
        ADD(b, "import \"a.js\";");
    ADD(b, "globalThis.r = Object.keys(ns).length;");
#undef ADD
    for (k = 0; k <= reads && !failures; k++) {
        VM v = vm_new();
        volatile int *flag = JS_GetInterruptFlag(v.rt);

        r = JS_Eval(v.ctx, a, strlen(a), "a.js", JS_EVAL_TYPE_MODULE);
        CHECK(!JS_IsException(r), "the module of the exports does not run");
        JS_FreeValue(v.ctx, r);
        before = host_poll_reads;
        host_poll_stop_after(k);
        r = JS_Eval(v.ctx, b, strlen(b), "b.js", JS_EVAL_TYPE_MODULE);
        host_poll_stop_after(0);
        set = *flag;
        *flag = 0;
        if (k == 0) {
            reads = host_poll_reads - before;
            CHECK(!JS_IsException(r) && JS_PromiseState(v.ctx, r) == JS_PROMISE_FULFILLED,
                  "the module of the imports does not run");
        } else if (JS_IsException(r)) {
            t = exception_text(v.ctx);
            CHECK(t && strcmp(t, "InternalError: interrupted") == 0, "the stop at reading %ld of %ld: %s", k, reads,
                  t ? t : "no exception");
            free(t);
        } else if (JS_PromiseState(v.ctx, r) == JS_PROMISE_REJECTED) {
            e = JS_PromiseResult(v.ctx, r);
            JS_Throw(v.ctx, e);
            t = exception_text(v.ctx);
            CHECK(t && strcmp(t, "InternalError: interrupted") == 0,
                  "the stop at reading %ld of %ld rejected the module with %s", k, reads, t ? t : "nothing");
            free(t);
        } else {
            CHECK(set, "the stop at reading %ld of %ld did not stop the module", k, reads);
        }
        JS_FreeValue(v.ctx, r);
        vm_free(&v);
    }
    CHECK(reads > 10, "the module of the imports read the flag %ld times", reads);
}

/* stop_end ends a run that a stop may have cut: it runs the jobs the run
   left until one fails, then clears the flag, takes the exception and runs
   the rest with no stop, so that nothing of the run is left to act later. */
static void stop_end(VM *v)
{
    while (!JS_HasException(v->ctx) && JS_ExecutePendingJob(v->rt, NULL) > 0)
        ;
    host_poll_stop_after(0);
    *JS_GetInterruptFlag(v->rt) = 0;
    JS_FreeValue(v->ctx, JS_GetException(v->ctx));
    drain_jobs(v);
}

/* A stop at each reading of the flag in the run of module b.js, which
   imports a.js, which imports c.js, both made but not run, then an import of
   b.js with no stop: the import settles, and the runtime is freed with no
   block left. A stop in the evaluation leaves each module it ran recorded
   with the error, and the promise of b.js was left pending for good, as the
   call that rejects it fails while the interrupt is pending: the import
   after it now rejects with the error recorded. A stop in the link leaked
   the bindings the link made (test_link_retry). */
static void test_module_eval_stops(void)
{
    static const char c[] = "export var z = 1;";
    static const char a[] =
        "import {z} from \"c.js\"; export var y = z;"
        "for (let i = 0; i < 50; i++) y += 0;";
    static const char b[] =
        "import {y} from \"a.js\"; globalThis.x = y;"
        "for (let i = 0; i < 50; i++) globalThis.x += 0;";
    long k, reads = 0, before, rejected = 0;
    char buf[128];
    int leaked;

    for (k = 0; k <= reads; k++) {
        VM v = vm_new();

        compile_module(&v, "c.js", c);
        compile_module(&v, "a.js", a);
        before = host_poll_reads;
        host_poll_stop_after(k);
        JS_FreeValue(v.ctx, JS_Eval(v.ctx, b, strlen(b), "b.js", JS_EVAL_TYPE_MODULE));
        if (k == 0)
            reads = host_poll_reads - before;
        stop_end(&v);
        import_state(&v, "b.js", buf, sizeof(buf));
        if (strcmp(buf, "ok") == 0) {
            CHECK(eval_is(v.ctx, "x", "1"), "the stop at reading %ld of %ld: x is not 1", k, reads);
        } else {
            CHECK(strcmp(buf, "InternalError: interrupted") == 0 || strncmp(buf, "ReferenceError", 14) == 0,
                  "the stop at reading %ld of %ld: the import after it gave %s", k, reads, buf);
            rejected += strcmp(buf, "InternalError: interrupted") == 0;
        }
        leaked = host_leaked_runtimes();
        vm_free(&v);
        CHECK(host_leaked_runtimes() == leaked, "the stop at reading %ld of %ld left blocks in the runtime", k, reads);
    }
    CHECK(reads > 10, "the module read the flag %ld times", reads);
    CHECK(rejected > 10, "%ld imports after a stop rejected with the error the module recorded", rejected);
}

/* A stop at each reading of the flag in an import of module b.js, which
   binds 300 names of a.js and a namespace a.js exports from c.js, then an
   import of b.js again with no stop, once the jobs of the first are done.
   A link that stopped had bound some of the names, and the link again took
   a second reference to each and lost the first, and made the namespace
   binding again: the runtime was freed with blocks left (js_unbind_module).
   A stop in the making of the promise of b.js left the exception in its
   place, and the import again rejected with no error. The import again
   links b.js and runs it, with the objects, atoms and blocks of an import
   that never stopped, or rejects with the error a stopped evaluation
   recorded, and the runtime is freed with no block left. */
static void test_link_retry(void)
{
    static const char c[] = "export var z = 1;";
    static const char first[] = "import(\"b.js\"); 0";
    char a[4096], b[4096], buf[128];
    size_t n;
    long k, reads = 0, before, relinked = 0;
    int i, leaked;
    int64_t objs = 0, atoms = 0, blocks = 0;
    JSMemoryUsage u;
    JSValue r;

#define ADD(s, ...) (n += snprintf(s + n, sizeof(s) - n, __VA_ARGS__))
    n = 0;
    ADD(a, "export * as nsx from \"c.js\"; export var e0 = 0");
    for (i = 1; i < 300; i++)
        ADD(a, ",e%d = 0", i);
    ADD(a, ";");
    CHECK(n < sizeof(a), "the source of a.js does not fit");
    n = 0;
    ADD(b, "import {nsx, e0");
    for (i = 1; i < 300; i++)
        ADD(b, ",e%d", i);
    ADD(b, "} from \"a.js\"; globalThis.x = e0 + e299 + nsx.z;");
    CHECK(n < sizeof(b), "the source of b.js does not fit");
#undef ADD
    for (k = 0; k <= reads; k++) {
        VM v = vm_new();

        r = JS_Eval(v.ctx, c, strlen(c), "c.js", JS_EVAL_TYPE_MODULE);
        CHECK(!JS_IsException(r), "c.js does not run");
        JS_FreeValue(v.ctx, r);
        r = JS_Eval(v.ctx, a, strlen(a), "a.js", JS_EVAL_TYPE_MODULE);
        CHECK(!JS_IsException(r), "a.js does not run");
        JS_FreeValue(v.ctx, r);
        compile_module(&v, "b.js", b);
        drain_jobs(&v);
        before = host_poll_reads;
        host_poll_stop_after(k);
        JS_FreeValue(v.ctx, JS_Eval(v.ctx, first, strlen(first), "<first>", JS_EVAL_TYPE_GLOBAL));
        stop_end(&v);
        if (k == 0)
            reads = host_poll_reads - before;
        import_state(&v, "b.js", buf, sizeof(buf));
        if (strcmp(buf, "ok") == 0) {
            CHECK(eval_is(v.ctx, "x", "1"), "the stop at reading %ld of %ld: x is not 1", k, reads);
            JS_RunGC(v.rt);
            JS_ComputeMemoryUsage(v.rt, &u);
            if (k == 0) {
                objs = u.obj_count;
                atoms = u.atom_count;
                blocks = u.malloc_count;
            } else {
                relinked++;
                CHECK(u.obj_count == objs && u.atom_count == atoms && u.malloc_count == blocks,
                      "the stop at reading %ld of %ld: %lld objects, %lld atoms and %lld blocks, against %lld, %lld and %lld",
                      k, reads, (long long)u.obj_count, (long long)u.atom_count, (long long)u.malloc_count,
                      (long long)objs, (long long)atoms, (long long)blocks);
            }
        } else {
            CHECK(k > 0 && strcmp(buf, "InternalError: interrupted") == 0,
                  "the stop at reading %ld of %ld: the import after it gave %s", k, reads, buf);
        }
        leaked = host_leaked_runtimes();
        vm_free(&v);
        CHECK(host_leaked_runtimes() == leaked, "the stop at reading %ld of %ld left blocks in the runtime", k, reads);
    }
    CHECK(reads > 10 && relinked > 10, "the import read the flag %ld times, and %ld stops linked again", reads, relinked);
}

/* A stop at each reading of the flag in the run of module u.js, compiled
   before, and in the jobs that go on with it: u.js imports t.js, whose
   top-level await waits for c.js, whose top-level await ends in a job, and
   t.js runs on after its own await in a job too, in its body or in an
   async function it awaits (inner). A stop in such a job left the promise
   that would go on with t.js pending for good: t.js stayed evaluating, and
   so did u.js, and an import of either did not settle. Now the host taking
   the error records every module still evaluating as failed with it
   (js_fail_async_modules): an import of u.js after the stop gives the
   result or the error interrupted, and so does one of t.js, which fails
   only when u.js does. done.js, evaluated before, and idle.js, made but
   not evaluated, are left as they were, and the runtime is freed with no
   block left. Each runs also with an async context set
   (JS_SetAsyncContext), which wraps each handler of a promise in a
   function that sets it. */
static void module_await_stops(int inner, int with_context)
{
    static const char c[] = "await 0; globalThis.c = 1;";
    static const char t_body[] =
        "import \"c.js\"; globalThis.n = 0;"
        "for (let i = 0; i < 20; i++) n++;"
        "await 0; for (let i = 0; i < 20; i++) n++;";
    static const char t_inner[] =
        "import \"c.js\"; globalThis.n = 0;"
        "for (let i = 0; i < 20; i++) n++;"
        "await (async function () { await 0; for (let i = 0; i < 20; i++) n++; })();";
    static const char u[] = "import \"t.js\"; globalThis.x = n;";
    static const char done[] = "export var d = 7; await 0; globalThis.done = d;";
    static const char idle[] = "globalThis.idle = 1;";
    static const char stopped[] = "InternalError: interrupted";
    long k, reads = 0, sync_reads = 0, before, rejected = 0, in_jobs = 0;
    char buf[128], buf2[128], buf3[128], buf4[128];
    int leaked;
    JSValue m;

    for (k = 0; k <= reads; k++) {
        VM v = vm_new();

        if (with_context) {
            JSValue o = JS_NewObject(v.ctx);

            JS_SetAsyncContext(v.ctx, o);
            JS_FreeValue(v.ctx, o);
        }
        compile_module(&v, "done.js", done);
        CHECK(strcmp(import_state(&v, "done.js", buf3, sizeof(buf3)), "ok") == 0, "done.js gave %s", buf3);
        compile_module(&v, "idle.js", idle);
        compile_module(&v, "c.js", c);
        compile_module(&v, "t.js", inner ? t_inner : t_body);
        m = JS_Eval(v.ctx, u, strlen(u), "u.js", JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
        CHECK(!JS_IsException(m), "u.js does not compile");
        before = host_poll_reads;
        host_poll_stop_after(k);
        JS_FreeValue(v.ctx, JS_EvalFunction(v.ctx, m));
        if (k == 0)
            sync_reads = host_poll_reads - before;
        stop_end(&v);
        if (k == 0)
            reads = host_poll_reads - before;
        import_state(&v, "u.js", buf, sizeof(buf));
        import_state(&v, "t.js", buf2, sizeof(buf2));
        if (strcmp(buf, "ok") == 0) {
            CHECK(strcmp(buf2, "ok") == 0, "inner %d, context %d, the stop at reading %ld of %ld: u.js ran, and the import of t.js gave %s",
                  inner, with_context, k, reads, buf2);
            CHECK(eval_is(v.ctx, "x", "40"), "inner %d, context %d, the stop at reading %ld of %ld: x is not 40",
                  inner, with_context, k, reads);
        } else {
            CHECK(k > 0 && strcmp(buf, stopped) == 0 && (strcmp(buf2, "ok") == 0 || strcmp(buf2, stopped) == 0),
                  "inner %d, context %d, the stop at reading %ld of %ld: the imports after it gave %s and %s",
                  inner, with_context, k, reads, buf, buf2);
            rejected++;
            in_jobs += k > sync_reads;
        }
        import_state(&v, "done.js", buf3, sizeof(buf3));
        import_state(&v, "idle.js", buf4, sizeof(buf4));
        CHECK(strcmp(buf3, "ok") == 0 && strcmp(buf4, "ok") == 0 && eval_is(v.ctx, "done + idle", "8"),
              "inner %d, context %d, the stop at reading %ld of %ld: done.js and idle.js gave %s and %s",
              inner, with_context, k, reads, buf3, buf4);
        leaked = host_leaked_runtimes();
        vm_free(&v);
        CHECK(host_leaked_runtimes() == leaked, "inner %d, context %d, the stop at reading %ld of %ld left blocks in the runtime",
              inner, with_context, k, reads);
    }
    CHECK(reads > 20 && rejected > 10 && in_jobs > 5,
          "inner %d, context %d: the modules read the flag %ld times, and %ld imports rejected with the error recorded, %ld after a stop in a job",
          inner, with_context, reads, rejected, in_jobs);
}

static void test_module_await_stops(void)
{
    int inner, with_context;

    for (inner = 0; inner < 2; inner++)
        for (with_context = 0; with_context < 2; with_context++)
            module_await_stops(inner, with_context);
}

/* A module with a top-level await that changes Promise.prototype.constructor
   before its first await: the handlers of the module were put on the
   promise of its function by Promise.prototype.then, whose look for the
   species constructor threw, and the module stayed evaluating for good,
   with the TypeError left pending after the eval returned its promise. The
   module now runs to its end (js_execute_async_module). An import() whose
   module is evaluated while the constructor is changed never settled, its
   job ending with the TypeError pending, for the same look: it now gives
   the module (JS_LoadModuleInternal). */
static void test_module_await_species(void)
{
    static const char s[] = "Promise.prototype.constructor = 1; await 0; globalThis.s = 1;";
    static const char imp[] = "import(\"q.js\")";
    VM v = vm_new();
    JSValue r = JS_Eval(v.ctx, s, strlen(s), "s.js", JS_EVAL_TYPE_MODULE);
    int failed = 0;
    JSContext *ctx;

    CHECK(!JS_IsException(r) && !JS_HasException(v.ctx), "the eval of s.js threw");
    drain_jobs(&v);
    CHECK(JS_PromiseState(v.ctx, r) == JS_PROMISE_FULFILLED, "s.js is in state %d", JS_PromiseState(v.ctx, r));
    CHECK(eval_is(v.ctx, "s", "1"), "s.js did not run to its end");
    JS_FreeValue(v.ctx, r);
    compile_module(&v, "q.js", "globalThis.q = 1;");
    r = JS_Eval(v.ctx, imp, strlen(imp), "<import>", JS_EVAL_TYPE_GLOBAL);
    CHECK(!JS_IsException(r), "the import threw");
    while (JS_ExecutePendingJob(v.rt, &ctx) != 0 || JS_HasException(v.ctx)) {
        if (JS_HasException(v.ctx)) {
            JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            failed++;
        }
    }
    CHECK(failed == 0 && JS_PromiseState(v.ctx, r) == JS_PROMISE_FULFILLED,
          "the import is in state %d, with %d exceptions left by its jobs", JS_PromiseState(v.ctx, r), failed);
    JS_FreeValue(v.ctx, r);
    vm_free(&v);
}

/* gc_tracker runs the collector, as a host's tracker of rejections may. */
static int gc_tracker_calls;

static void gc_tracker(JSContext *ctx, JSValueConst promise, JSValueConst reason,
                       JS_BOOL is_handled, void *opaque)
{
    (void)promise;
    (void)reason;
    (void)is_handled;
    (void)opaque;
    gc_tracker_calls++;
    JS_RunGC(JS_GetRuntime(ctx));
}

/* The modules a stop fails (js_fail_async_modules) are those of every
   context of the runtime. Context b waits in b.js on a promise nothing
   settles, its eval's promise and the host's reference to b freed, so that
   b, its module and its promise are a cycle only the collector frees,
   which does not run on its own. A stop of context a fails b.js with it:
   its rejection called the host's tracker while the walk of the modules
   was on b.js, with no reference to b, its module or its promise, and a
   tracker that ran the collector freed them all under it. The tracker is
   not told of these rejections, and the walk calls nothing. */
static void test_module_fail_collect(void)
{
    static const char wait[] = "await new Promise(() => {});";
    static const char spin[] = "for (;;) {}";
    JSRuntime *rt = host_new_runtime();
    JSContext *a = JS_NewContext(rt), *b = JS_NewContext(rt);
    long leaked = host_leaked_runtimes();
    JSValue r, e;
    const char *s;

    JS_SetGCThreshold(rt, (size_t)-1);
    JS_SetHostPromiseRejectionTracker(rt, gc_tracker, NULL);
    gc_tracker_calls = 0;
    r = JS_Eval(b, wait, strlen(wait), "b.js", JS_EVAL_TYPE_MODULE);
    CHECK(!JS_IsException(r), "the eval of b.js threw");
    JS_FreeValue(b, r);
    JS_FreeContext(b);
    host_poll_stop_after(1);
    r = JS_Eval(a, spin, strlen(spin), "<spin>", JS_EVAL_TYPE_GLOBAL);
    host_poll_stop_after(0);
    CHECK(JS_IsException(r), "the loop of a ran to its end");
    *JS_GetInterruptFlag(rt) = 0;
    e = JS_GetException(a);
    s = JS_ToCString(a, e);
    CHECK(s && strcmp(s, "InternalError: interrupted") == 0, "a ended with %s", s ? s : "?");
    JS_FreeCString(a, s);
    JS_FreeValue(a, e);
    CHECK(gc_tracker_calls == 0, "the tracker was called %d times", gc_tracker_calls);
    JS_RunGC(rt);
    JS_FreeContext(a);
    host_free_runtime(rt);
    CHECK(host_leaked_runtimes() == leaked, "blocks left in the runtime");
}

/* A job that a stop ends fails in the return of JS_ExecutePendingJob, with
   its error pending, whatever its reaction had to settle. The reaction of
   an await settles no promise of its own (its capability is undefined), and
   its job returned 1 with the uncatchable error pending: a host that takes
   the exception only on a negative return, as the API says, never took it,
   and a module with a top-level await stayed evaluating for good, as the
   failure of the modules runs where the host takes the error
   (js_fail_async_modules). Each case is stopped at each reading of the flag
   in its jobs, and only the return value decides what the host does. The
   promise of the module is then rejected, also after a stop in the job
   that ended it, which refused the call of its resolving function
   (js_module_settle). */
static void test_job_stop_returns(void)
{
    static const struct {
        const char *what, *src;
        int module;
    } cases[] = {
        { "module", "await 0; for (let i = 0; i < 50; i++) globalThis.n = i;", 1 },
        { "async function",
          "(async () => { await 0; for (let i = 0; i < 50; i++) globalThis.n = i; })()", 0 },
        { "then", "Promise.resolve().then(() => { for (let i = 0; i < 50; i++) globalThis.n = i; })", 0 },
    };
    size_t i;

    for (i = 0; i < countof(cases); i++) {
        long k;

        for (k = 1;; k++) {
            VM v = vm_new();
            JSValue p = JS_Eval(v.ctx, cases[i].src, strlen(cases[i].src), "<job>",
                                cases[i].module ? JS_EVAL_TYPE_MODULE : JS_EVAL_TYPE_GLOBAL);
            JSContext *c;
            int ret, failed = 0, swallowed = 0;

            CHECK(!JS_IsException(p), "%s: the eval threw", cases[i].what);
            host_poll_reads = 0;
            host_poll_stop_after(k);
            while ((ret = JS_ExecutePendingJob(v.rt, &c)) != 0) {
                if (ret < 0) {
                    JSValue e;
                    const char *s;

                    *JS_GetInterruptFlag(v.rt) = 0;
                    e = JS_GetException(v.ctx);
                    s = JS_ToCString(v.ctx, e);
                    CHECK(s && strcmp(s, "InternalError: interrupted") == 0, "%s, stop at %ld: %s",
                          cases[i].what, k, s ? s : "?");
                    JS_FreeCString(v.ctx, s);
                    JS_FreeValue(v.ctx, e);
                    failed++;
                } else if (JS_HasException(v.ctx)) {
                    swallowed++;
                    break;
                }
            }
            host_poll_stop_after(0);
            CHECK(swallowed == 0, "%s, stop at %ld: a job returned 1 with the error pending",
                  cases[i].what, k);
            if (swallowed) {
                *JS_GetInterruptFlag(v.rt) = 0;
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            }
            if (cases[i].module && failed) {
                CHECK(JS_PromiseState(v.ctx, p) == JS_PROMISE_REJECTED, "module, stop at %ld: in state %d",
                      k, JS_PromiseState(v.ctx, p));
            }
            JS_FreeValue(v.ctx, p);
            drain_jobs(&v);
            vm_free(&v);
            if (!failed && !swallowed)
                break;
        }
        CHECK(k > 3, "%s: %ld readings", cases[i].what, k);
    }
}

/* A call of the host that a stop ends returns the exception, whatever the
   code it ran returned. An async function stopped before its first await
   cannot reject its promise, as the call of its reject function is refused
   while the error is pending, and returned that promise, still pending:
   JS_Eval of a module with a top-level await after a busy loop returned the
   pending promise of the module, no job was left to run, and a host that
   takes the exception only when a call fails never took it, so the module
   stayed evaluating for good. A module with no await returned its promise
   rejected, a script that calls an async function and JS_Call of one their
   pending promise. Each is stopped at each reading of the flag up to
   ENTRY_STOPS, in the compile and in the loop. */
#define ENTRY_STOPS 40
static void test_entry_stop_returns(void)
{
    static const struct {
        const char *what, *src;
        int module, call;
    } cases[] = {
        { "module with an await", "for (;;) {} await 0;", 1, 0 },
        { "module", "for (;;) {}", 1, 0 },
        { "script", "(async () => { for (;;) {} })()", 0, 0 },
        { "JS_Call", "globalThis.f = async () => { for (;;) {} };", 0, 1 },
    };
    size_t i;
    long k;

    for (i = 0; i < countof(cases); i++) {
        for (k = 1; k <= ENTRY_STOPS; k++) {
            VM v = vm_new();
            JSValue r, f = JS_UNDEFINED, e;
            const char *s;

            if (cases[i].call) {
                r = JS_Eval(v.ctx, cases[i].src, strlen(cases[i].src), "<entry>", JS_EVAL_TYPE_GLOBAL);
                CHECK(!JS_IsException(r), "%s: the eval threw", cases[i].what);
                JS_FreeValue(v.ctx, r);
                JSValue g = JS_GetGlobalObject(v.ctx);

                f = JS_GetPropertyStr(v.ctx, g, "f");
                JS_FreeValue(v.ctx, g);
            }
            host_poll_reads = 0;
            host_poll_stop_after(k);
            if (cases[i].call)
                r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
            else
                r = JS_Eval(v.ctx, cases[i].src, strlen(cases[i].src), "<entry>",
                            cases[i].module ? JS_EVAL_TYPE_MODULE : JS_EVAL_TYPE_GLOBAL);
            host_poll_stop_after(0);
            CHECK(JS_IsException(r), "%s, stop at %ld: returned %s with %s pending", cases[i].what, k,
                  JS_IsException(r) ? "the exception" : "a value",
                  JS_IsUncatchableException(v.ctx) ? "an uncatchable error" : "no error");
            JS_FreeValue(v.ctx, r);
            *JS_GetInterruptFlag(v.rt) = 0;
            e = JS_GetException(v.ctx);
            s = JS_ToCString(v.ctx, e);
            CHECK(s && strcmp(s, "InternalError: interrupted") == 0, "%s, stop at %ld: %s",
                  cases[i].what, k, s ? s : "?");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, e);
            JS_FreeValue(v.ctx, f);
            drain_jobs(&v);
            vm_free(&v);
        }
    }
}

/* the module loader of test_api_stop_returns: every module is api_module_src */
static const char api_module_src[] = "for (;;) {} await 0;";

static JSModuleDef *api_module_loader(JSContext *ctx, const char *name, void *opaque)
{
    JSValue f = JS_Eval(ctx, api_module_src, strlen(api_module_src), name,
                        JS_EVAL_TYPE_MODULE | JS_EVAL_FLAG_COMPILE_ONLY);
    JSModuleDef *m;

    (void)opaque;
    if (JS_IsException(f))
        return NULL;
    m = JS_VALUE_GET_PTR(f);
    JS_FreeValue(ctx, f);
    return m;
}

enum {
    API_GET, API_SET, API_INSTANCEOF, API_TYPED_ARRAY, API_TO_FLOAT64,
    API_JSON, API_LOAD_MODULE,
};

/* Every function of the API that runs code fails when a stop ends it,
   whatever that code returned: a place that catches an exception to settle
   a promise or to keep it as a result ends with an uncatchable one instead.
   An async function, an async generator, a for await over a sync iterator,
   a module with a top-level await and the close of an iterator by Set
   methods each turned a stop into a value: the getter returned the pending
   promise of an async function stopped before its first await, the setter
   succeeded, and so on, with the interrupt left pending. Each case is
   stopped at each reading of the flag up to ENTRY_STOPS. */
static void test_api_stop_returns(void)
{
    static const char setup[] =
        "globalThis.spin = async function () { for (;;) {} };\n"
        "globalThis.o = {};\n"
        "Object.defineProperty(o, 'get', { get: async function () { for (;;) {} } });\n"
        "Object.defineProperty(o, 'set', { set: async function (v) { for (;;) {} } });\n"
        "Object.defineProperty(o, 'gen', { get: function () {\n"
        "  return (async function* () { for (;;) {} })().next(); } });\n"
        "Object.defineProperty(o, 'syncIter', { get: function () {\n"
        "  return (async function () {\n"
        "    for await (const x of { [Symbol.iterator]() { return { next() { for (;;) {} } }; } }) {}\n"
        "  })(); } });\n"
        "Object.defineProperty(o, 'setClose', { get: function () {\n"
        "  return new Set([1, 2]).isDisjointFrom({ size: 1, has() { return true; }, keys() {\n"
        "    return { next() { return { value: 1, done: false }; }, return() { for (;;) {} } }; } });\n"
        "  } });\n"
        "globalThis.C = { [Symbol.hasInstance]: async function () { for (;;) {} } };\n"
        "globalThis.iterable = { [Symbol.iterator]() { return { next() {\n"
        "  const r = { value: 1 };\n"
        "  Object.defineProperty(r, 'done', { get: async function () { for (;;) {} } });\n"
        "  return r; } }; } };\n"
        "globalThis.num = { valueOf() { spin(); return 7; } };\n"
        "globalThis.j = {};\n"
        "Object.defineProperty(j, 'toJSON', { get: async function () { for (;;) {} } });\n"
        "globalThis.j2 = {};\n"
        "Object.defineProperty(j2, 'a', { get: async function () { for (;;) {} }, enumerable: true });\n";
    static const struct {
        const char *what, *prop;
        int op;
    } cases[] = {
        { "async getter", "get", API_GET },
        { "async generator", "gen", API_GET },
        { "for await of a sync iterator", "syncIter", API_GET },
        { "close of Set methods", "setClose", API_GET },
        { "async setter", "set", API_SET },
        { "async Symbol.hasInstance", NULL, API_INSTANCEOF },
        { "async done of an iterator", NULL, API_TYPED_ARRAY },
        { "valueOf calling an async function", NULL, API_TO_FLOAT64 },
        { "async toJSON getter", NULL, API_JSON },
        { "async getter of a property", "j2", API_JSON },
        { "JS_LoadModule", NULL, API_LOAD_MODULE },
    };
    size_t i;
    long k;

    for (i = 0; i < countof(cases); i++) {
        for (k = 1; k <= ENTRY_STOPS; k++) {
            VM v = vm_new();
            JSValue g, arg, r = JS_UNDEFINED, e;
            const char *s;
            int failed = 0;
            double d;

            JS_SetModuleLoaderFunc(v.rt, NULL, api_module_loader, NULL);
            r = JS_Eval(v.ctx, setup, strlen(setup), "<setup>", JS_EVAL_TYPE_GLOBAL);
            CHECK(!JS_IsException(r), "%s: the setup threw", cases[i].what);
            JS_FreeValue(v.ctx, r);
            r = JS_UNDEFINED;
            g = JS_GetGlobalObject(v.ctx);
            switch (cases[i].op) {
            case API_GET: case API_SET: arg = JS_GetPropertyStr(v.ctx, g, "o"); break;
            case API_INSTANCEOF: arg = JS_GetPropertyStr(v.ctx, g, "C"); break;
            case API_TYPED_ARRAY: arg = JS_GetPropertyStr(v.ctx, g, "iterable"); break;
            case API_TO_FLOAT64: arg = JS_GetPropertyStr(v.ctx, g, "num"); break;
            case API_JSON: arg = JS_GetPropertyStr(v.ctx, g, cases[i].prop ? cases[i].prop : "j"); break;
            default: arg = JS_UNDEFINED; break;
            }
            host_poll_reads = 0;
            host_poll_stop_after(k);
            switch (cases[i].op) {
            case API_GET:
                r = JS_GetPropertyStr(v.ctx, arg, cases[i].prop);
                failed = JS_IsException(r);
                break;
            case API_SET:
                failed = JS_SetPropertyStr(v.ctx, arg, cases[i].prop, JS_NewInt32(v.ctx, 1)) < 0;
                break;
            case API_INSTANCEOF:
                failed = JS_IsInstanceOf(v.ctx, g, arg) < 0;
                break;
            case API_TYPED_ARRAY:
                r = JS_NewTypedArray(v.ctx, 1, (JSValueConst *)&arg, JS_TYPED_ARRAY_UINT8);
                failed = JS_IsException(r);
                break;
            case API_TO_FLOAT64:
                failed = JS_ToFloat64(v.ctx, &d, arg) < 0;
                break;
            case API_JSON:
                r = JS_JSONStringify(v.ctx, arg, JS_UNDEFINED, JS_UNDEFINED);
                failed = JS_IsException(r);
                break;
            case API_LOAD_MODULE:
                r = JS_LoadModule(v.ctx, "", "stop.js");
                failed = JS_IsException(r);
                break;
            }
            host_poll_stop_after(0);
            CHECK(failed, "%s, stop at %ld: succeeded with %s pending", cases[i].what, k,
                  JS_IsUncatchableException(v.ctx) ? "an uncatchable error" : "no error");
            JS_FreeValue(v.ctx, r);
            *JS_GetInterruptFlag(v.rt) = 0;
            e = JS_GetException(v.ctx);
            s = JS_ToCString(v.ctx, e);
            CHECK(s && strcmp(s, "InternalError: interrupted") == 0, "%s, stop at %ld: %s",
                  cases[i].what, k, s ? s : "?");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, e);
            JS_FreeValue(v.ctx, arg);
            JS_FreeValue(v.ctx, g);
            drain_jobs(&v);
            vm_free(&v);
        }
    }
}

/* take_stop clears the flag and frees the exception a stop left, which must
   be the interrupt. */
static void take_stop(VM *v, const char *what, long k)
{
    JSValue e;
    const char *s;

    *JS_GetInterruptFlag(v->rt) = 0;
    e = JS_GetException(v->ctx);
    s = JS_ToCString(v->ctx, e);
    CHECK(s && strcmp(s, "InternalError: interrupted") == 0, "%s, stop at %ld: %s",
          what, k, s ? s : "?");
    JS_FreeCString(v->ctx, s);
    JS_FreeValue(v->ctx, e);
}

/* call_next calls next of the generator g from C */
static JSValue call_next(JSContext *ctx, JSValueConst g)
{
    JSValue f = JS_GetPropertyStr(ctx, g, "next");
    JSValue r = JS_Call(ctx, f, g, 0, NULL);

    JS_FreeValue(ctx, f);
    return r;
}

/* The requests queued on an async generator that finished all settle, as
   the drain of the spec settles them (AsyncGeneratorDrainQueue): after a
   failure of its body, and behind a return. Only the first after the end
   settled, and those after it stayed pending for good. */
static void test_async_gen_drain(void)
{
    static const struct {
        const char *what, *src, *want;
    } cases[] = {
        { "after a throw",
          "var r = [];\n"
          "var it = (async function* () { await 0; throw 1; })();\n"
          "it.next().then(v => r.push('1ok'), e => r.push('1err'));\n"
          "it.next().then(v => r.push('2 ' + v.done));\n"
          "it.next().then(v => r.push('3 ' + v.done));\n"
          "it.throw(4).catch(e => r.push('4 ' + e));\n"
          "it.next().then(v => r.push('5 ' + v.done));\n",
          "1err,2 true,3 true,4 4,5 true" },
        { "behind a return",
          "var r = [];\n"
          "var it = (async function* () {})();\n"
          "it.next();\n"
          "it.return(5).then(v => r.push('ret ' + v.value));\n"
          "it.next().then(v => r.push('n1 ' + v.done));\n"
          "it.next().then(v => r.push('n2 ' + v.done));\n",
          "ret 5,n1 true,n2 true" },
    };
    size_t i;

    for (i = 0; i < countof(cases); i++) {
        VM v = vm_new();

        eval_ok(v.ctx, cases[i].src);
        drain_jobs(&v);
        CHECK(eval_is(v.ctx, "r.join()", cases[i].want), "%s", cases[i].what);
        vm_free(&v);
    }
}

/* A stop in the run of an async generator ends it, and the request it ran
   for leaves the queue unsettled (the stop cannot become its rejection):
   every later request settles. The stopped request was left at the head of
   the queue, where the next call of next settled it in place of its own,
   whose promise stayed pending for good. A request queued behind the
   stopped one (from a job) settles at the next call too, as all those
   queued on a finished generator do. Each case is stopped at each reading
   of the flag until a run ends with no stop. A stop in the job of the
   await before it reached the generator loses the rest of the await, as it
   does for an async function, and the generator waits for good. */
static void test_async_gen_stop_queue(void)
{
    static const char setup[] =
        "globalThis.make = () => (async function* () {\n"
        "  for (let i = 0; i < 50; i++) globalThis.n = i;\n"
        "})();\n"
        "globalThis.makeAwait = () => (async function* () {\n"
        "  await 0;\n"
        "  for (let i = 0; i < 50; i++) globalThis.n = i;\n"
        "})();\n";
    int job;
    long k;

    for (job = 0; job < 2; job++) {
        const char *what = job ? "stop in a job" : "stop in next";

        for (k = 1;; k++) {
            VM v = vm_new();
            JSValue g, p1, p2 = JS_UNDEFINED, p3;
            int stopped = 0, ret;
            char *t;

            eval_ok(v.ctx, setup);
            g = call_global(v.ctx, job ? "makeAwait" : "make");
            CHECK(JS_IsObject(g), "%s: no generator", what);
            if (job) {
                p1 = call_next(v.ctx, g);
                p2 = call_next(v.ctx, g);
                host_poll_reads = 0;
                host_poll_stop_after(k);
                while ((ret = JS_ExecutePendingJob(v.rt, NULL)) > 0)
                    ;
                stopped = ret < 0;
            } else {
                host_poll_reads = 0;
                host_poll_stop_after(k);
                p1 = call_next(v.ctx, g);
                stopped = JS_IsException(p1);
            }
            host_poll_stop_after(0);
            if (stopped)
                take_stop(&v, what, k);
            t = eval_text(v.ctx, "String(globalThis.n)");
            p3 = call_next(v.ctx, g);
            drain_jobs(&v);
            /* a stop in the job before it called the generator lost the
               end of the await, as for an async function: the generator
               waits for good. A stop in the body leaves both requests to
               settle, and one in the settle of a request at the end (n is
               49) may leave that request pending */
            if (!(job && t && strcmp(t, "undefined") == 0)) {
                if (job && !(t && strcmp(t, "49") == 0))
                    CHECK(JS_PromiseState(v.ctx, p2) == JS_PROMISE_FULFILLED,
                          "%s, stop at %ld: the second request is in state %d", what, k,
                          JS_PromiseState(v.ctx, p2));
                CHECK(JS_PromiseState(v.ctx, p3) == JS_PROMISE_FULFILLED,
                      "%s, stop at %ld: the request after the stop is in state %d", what, k,
                      JS_PromiseState(v.ctx, p3));
            }
            free(t);
            JS_FreeValue(v.ctx, p1);
            JS_FreeValue(v.ctx, p2);
            JS_FreeValue(v.ctx, p3);
            JS_FreeValue(v.ctx, g);
            vm_free(&v);
            if (!stopped)
                break;
        }
        CHECK(k > 3, "%s: %ld readings", what, k);
    }
}

/* A stop in a step of an await (a getter of then that loops) ends an async
   generator or an async function as a failure of its body ends it: its
   variables closed and its frame freed, so that a closure it made keeps
   only what it captured. They were left with the frame suspended and the
   variables open: keep held the frame, and the buffer of 16 MiB in it, for
   as long as it lived. Each case is stopped at each reading of the flag up
   to ENTRY_STOPS. A stop in the getter of constructor that an await reads
   (PromiseResolve) ends the body the same way: unlike an error a script can
   catch, it is not thrown at the await, so no catch or finally of the body
   runs and ran stays undefined. */
static void test_await_stop_frees_frame(void)
{
    static const struct {
        const char *what, *src;
    } cases[] = {
        { "async generator",
          "globalThis.run = () => (async function* () {\n"
          "  let buf = new ArrayBuffer(16 << 20), x = 0;\n"
          "  globalThis.keep = () => ++x;\n"
          "  await { get then() { for (;;) {} } };\n"
          "  return buf.byteLength;\n"
          "})().next();\n" },
        { "async function",
          "globalThis.run = () => (async function () {\n"
          "  let buf = new ArrayBuffer(16 << 20), x = 0;\n"
          "  globalThis.keep = () => ++x;\n"
          "  await { get then() { for (;;) {} } };\n"
          "  return buf.byteLength;\n"
          "})();\n" },
        { "async function, constructor getter",
          "globalThis.run = () => (async function () {\n"
          "  let buf = new ArrayBuffer(16 << 20), x = 0;\n"
          "  globalThis.keep = () => ++x;\n"
          "  const p = Promise.resolve(1);\n"
          "  Object.defineProperty(p, 'constructor', { get() { for (;;) {} } });\n"
          "  try { await p; } catch (e) { globalThis.ran = 'catch'; }\n"
          "  finally { globalThis.ran = (globalThis.ran || '') + 'finally'; }\n"
          "  return buf.byteLength;\n"
          "})();\n" },
    };
    size_t i;
    long k;

    for (i = 0; i < countof(cases); i++) {
        for (k = 1; k <= ENTRY_STOPS; k++) {
            VM v = vm_new();
            JSMemoryUsage base, after;
            JSValue r;

            eval_ok(v.ctx, cases[i].src);
            base = usage(&v);
            host_poll_reads = 0;
            host_poll_stop_after(k);
            r = call_global(v.ctx, "run");
            host_poll_stop_after(0);
            CHECK(JS_IsException(r), "%s, stop at %ld: the call returned a value", cases[i].what, k);
            JS_FreeValue(v.ctx, r);
            take_stop(&v, cases[i].what, k);
            drain_jobs(&v);
            r = global_prop(v.ctx, "ran");
            CHECK(JS_IsUndefined(r), "%s, stop at %ld: a catch or finally ran", cases[i].what, k);
            JS_FreeValue(v.ctx, r);
            after = usage(&v);
            CHECK(after.malloc_size < base.malloc_size + (1 << 20),
                  "%s, stop at %ld: %lld bytes kept by the closure", cases[i].what, k,
                  (long long)(after.malloc_size - base.malloc_size));
            vm_free(&v);
        }
    }
}

/* call_method calls the method m of the global object o from C, with the
   argument 1 */
static JSValue call_method(JSContext *ctx, const char *o, const char *m)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue obj = JS_GetPropertyStr(ctx, g, o);
    JSValue f = JS_GetPropertyStr(ctx, obj, m);
    JSValue one = JS_NewInt32(ctx, 1);
    JSValue r = JS_Call(ctx, f, obj, 1, (JSValueConst *)&one);

    JS_FreeValue(ctx, f);
    JS_FreeValue(ctx, obj);
    JS_FreeValue(ctx, g);
    return r;
}

/* A settle of an async function or an async generator that fails on an
   error a script can catch, the call depth, fails the call that ran it, as
   no call may return a value with an exception left pending. The settle
   that failed kept its error pending and the call returned its promise:
   return(1) on a finished generator (the promise of the value to await
   failed, and the rejected one made in its place too), next on it, and an
   async function or a generator whose resolve of its own promise failed.
   The jobs of an await of return settle on the same steps. Each case runs
   at each call depth up to 12, and the runtime is closed with no block in
   use (vm_free). */
static void test_settle_depth_fails_call(void)
{
    static const struct {
        const char *what, *setup, *o, *m;
    } cases[] = {
        { "return on a finished generator",
          "globalThis.g = (async function* () {})(); g.next();", "g", "return" },
        { "next on a finished generator",
          "globalThis.g = (async function* () {})(); g.next();", "g", "next" },
        { "next that yields",
          "globalThis.g = (async function* () { yield 1; })();", "g", "next" },
        { "next that throws",
          "globalThis.g = (async function* () { throw 1; })();", "g", "next" },
        { "async function that returns",
          "globalThis.o = { f: async function (x) { return x; } };", "o", "f" },
        { "async function that throws",
          "globalThis.o = { f: async function (x) { throw x; } };", "o", "f" },
        { "jobs of an await of return",
          "globalThis.g = (async function* () {})(); g.next();\n"
          "g.return(Promise.resolve(2)); g.next(); g.next();\n"
          "globalThis.o = { f() {} };", "o", "f" },
        { "jobs of an await of a generator",
          "globalThis.g = (async function* () { await 0; return 1; })();\n"
          "g.next(); g.next();\n"
          "globalThis.o = { f() {} };", "o", "f" },
        { "jobs of an await of an async function",
          "(async function () { await 0; return 1; })();\n"
          "globalThis.o = { f() {} };", "o", "f" },
    };
    size_t i;
    int d;

    for (i = 0; i < countof(cases); i++) {
        for (d = 1; d <= 12; d++) {
            VM v = vm_new();
            int max = JS_GetMaxCallDepth(v.rt), ret;
            JSContext *jctx;
            JSValue r;

            eval_ok(v.ctx, cases[i].setup);
            CHECK(JS_SetMaxCallDepth(v.rt, d) == 0, "%s: depth %d not set", cases[i].what, d);
            r = call_method(v.ctx, cases[i].o, cases[i].m);
            CHECK(JS_IsException(r) == JS_HasException(v.ctx),
                  "%s, depth %d: the call returned %s with %s pending", cases[i].what, d,
                  JS_IsException(r) ? "an exception" : "a value",
                  JS_HasException(v.ctx) ? "an exception" : "nothing");
            if (JS_IsException(r))
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            JS_FreeValue(v.ctx, r);
            while ((ret = JS_ExecutePendingJob(v.rt, &jctx)) != 0) {
                CHECK((ret < 0) == JS_HasException(v.ctx),
                      "%s, depth %d: a job returned %d with %s pending", cases[i].what, d, ret,
                      JS_HasException(v.ctx) ? "an exception" : "nothing");
                if (JS_HasException(v.ctx))
                    JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            }
            JS_SetMaxCallDepth(v.rt, max);
            drain_jobs(&v);
            vm_free(&v);
        }
    }
}

/* After a stop, async work that was running may never settle, and the VM
   is kept only to be read and closed: what must hold is that nothing is
   used after it was freed (the sanitizers), that no call or job ends with
   a value and an error left pending, and that closing the runtime frees
   every block (vm_free). Each case sets up its async work, then go runs
   with a stop at each reading of the flag, in the call and in the jobs
   that follow it, until a run ends with no stop. Each run is done twice:
   once the VM is closed as soon as the stop is taken, as a host that drops
   it does, and once after a call of after and the rest of the jobs. Some run with an async context set, so
   that each handler of an await is wrapped in a function of its own. No
   promise is checked: these are the paths where one stays pending. */
static void test_stop_async_frees(void)
{
    static const struct {
        const char *what, *setup;
        int actx;
    } cases[] = {
        { "a stop in the jobs of an await of a generator",
          "globalThis.g = (async function* () {\n"
          "  await 0;\n"
          "  for (let i = 0; i < 50; i++) globalThis.n = i;\n"
          "})();\n"
          "globalThis.go = () => { g.next(); g.next(); };\n"
          "globalThis.after = () => g.next();\n", 0 },
        { "the same, each handler wrapped for its async context",
          "globalThis.g = (async function* () {\n"
          "  await 0;\n"
          "  for (let i = 0; i < 50; i++) globalThis.n = i;\n"
          "})();\n"
          "globalThis.go = () => { g.next(); g.next(); };\n"
          "globalThis.after = () => g.next();\n", 1 },
        { "a stop in the jobs of an await of an async function",
          "globalThis.go = () => (async function () {\n"
          "  let buf = new ArrayBuffer(1 << 20);\n"
          "  globalThis.keep = () => buf;\n"
          "  await 0;\n"
          "  for (let i = 0; i < 50; i++) globalThis.n = i;\n"
          "})();\n"
          "globalThis.after = () => 0;\n", 1 },
        { "a stop in the then of the result a return settles with",
          "globalThis.g = (async function* () {})(); g.next();\n"
          "globalThis.go = () => {\n"
          "  Object.defineProperty(Object.prototype, 'then', { configurable: true,\n"
          "    get() { for (let i = 0; i < 50; i++) globalThis.n = i; } });\n"
          "  g.return(Promise.resolve(1)); g.next(); g.next();\n"
          "};\n"
          "globalThis.after = () => { delete Object.prototype.then; return g.next(); };\n", 0 },
        { "a stop in the then of a thenable an await gets",
          "var res, p = new Promise(r => { res = r; });\n"
          "globalThis.g = (async function* () { await p; return 1; })();\n"
          "g.next(); g.next();\n"
          "globalThis.go = () => res({ then(f) {\n"
          "  for (let i = 0; i < 50; i++) globalThis.n = i;\n"
          "  f(1);\n"
          "} });\n"
          "globalThis.after = () => g.next();\n", 0 },
        { "an await of an async function a stop left pending",
          "async function inner() {\n"
          "  await { get then() { for (let i = 0; i < 50; i++) globalThis.n = i; } };\n"
          "  return 1;\n"
          "}\n"
          "globalThis.g = (async function* () { await inner(); return 2; })();\n"
          "globalThis.go = () => { g.next(); g.next(); };\n"
          "globalThis.after = () => g.next();\n", 1 },
    };
    size_t i;
    long k;
    int drop;

    for (i = 0; i < countof(cases); i++) {
      for (drop = 0; drop < 2; drop++) {
        for (k = 1; k < 4000; k++) {
            VM v = vm_new();
            JSContext *jctx;
            JSValue r;
            int stopped = 0, ret;

            if (cases[i].actx) {
                JSValue ac = JS_NewObject(v.ctx);

                JS_SetAsyncContext(v.ctx, ac);
                JS_FreeValue(v.ctx, ac);
            }
            eval_ok(v.ctx, cases[i].setup);
            host_poll_reads = 0;
            host_poll_stop_after(k);
            r = call_global(v.ctx, "go");
            CHECK(JS_IsException(r) == JS_HasException(v.ctx),
                  "%s, stop at %ld: the call returned %s with %s pending", cases[i].what, k,
                  JS_IsException(r) ? "an exception" : "a value",
                  JS_HasException(v.ctx) ? "an exception" : "nothing");
            if (JS_IsException(r)) {
                stopped = 1;
                take_stop(&v, cases[i].what, k);
            }
            JS_FreeValue(v.ctx, r);
            while (!(drop && stopped) && (ret = JS_ExecutePendingJob(v.rt, &jctx)) != 0) {
                CHECK((ret < 0) == JS_HasException(v.ctx),
                      "%s, stop at %ld: a job returned %d with %s pending", cases[i].what, k,
                      ret, JS_HasException(v.ctx) ? "an exception" : "nothing");
                if (ret < 0) {
                    stopped = 1;
                    take_stop(&v, cases[i].what, k);
                }
            }
            host_poll_stop_after(0);
            if (*JS_GetInterruptFlag(v.rt)) {
                /* a stop no step read */
                stopped = 1;
                *JS_GetInterruptFlag(v.rt) = 0;
            }
            if (!(drop && stopped)) {
                JS_FreeValue(v.ctx, call_global(v.ctx, "after"));
                CHECK(!JS_HasException(v.ctx), "%s, stop at %ld: after threw", cases[i].what, k);
                drain_jobs(&v);
            }
            vm_free(&v);
            if (!stopped)
                break;
        }
        CHECK(k > 3 && k < 4000, "%s, drop %d: %ld readings", cases[i].what, drop, k);
      }
    }
}

/* A template object read back from bytecode is sealed as a parsed one is: the
   strings array and its raw array are frozen, their length is neither
   writable nor configurable (upstream QuickJS 88130c0). */
static void test_template_object_sealed(void)
{
    static const char src[] =
        "function tag(s) { return s; }"
        "var s = tag`a${1}b`;"
        "[Object.isFrozen(s), Object.isFrozen(s.raw),"
        " Object.getOwnPropertyDescriptor(s, 'length').writable,"
        " Object.getOwnPropertyDescriptor(s, 'length').configurable,"
        " Object.getOwnPropertyDescriptor(s.raw, 'length').writable].join()";
    static const char want[] = "true,true,false,false,false";
    VM v = vm_new();
    JSValue f = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    JSValue r;
    const char *s;
    uint8_t *buf;
    size_t size;

    CHECK(!JS_IsException(f), "compiled");
    buf = JS_WriteObject(v.ctx, &size, f, JS_WRITE_OBJ_BYTECODE);
    JS_FreeValue(v.ctx, f);
    CHECK(buf != NULL, "written");
    if (buf) {
        r = JS_EvalFunction(v.ctx, JS_ReadObject(v.ctx, buf, size, JS_READ_OBJ_BYTECODE));
        s = JS_IsException(r) ? NULL : JS_ToCString(v.ctx, r);
        CHECK(s && !strcmp(s, want), "read back: %s, want %s", s ? s : "(exception)", want);
        JS_FreeCString(v.ctx, s);
        JS_FreeValue(v.ctx, r);
        js_free(v.ctx, buf);
    }
    vm_free(&v);
}

/* An object reference to the slot a typed array holds while its buffer is
   read is refused with a SyntaxError, not followed into a NULL object
   (upstream QuickJS f68db0e). The bytes are a typed array written with
   references, whose buffer is replaced by a reference to the typed array
   itself. */
static void test_read_null_reference(void)
{
    VM v = vm_new();
    JSValue ta, pair, r;
    uint8_t *a, *b, bytes[16];
    size_t asize, bsize;
    const char *s;

    ta = JS_Eval(v.ctx, "new Uint8Array(1)", 17, "<unit>", JS_EVAL_TYPE_GLOBAL);
    pair = JS_Eval(v.ctx, "var o = {}; [o, o]", 18, "<unit>", JS_EVAL_TYPE_GLOBAL);
    a = JS_WriteObject(v.ctx, &asize, ta, JS_WRITE_OBJ_REFERENCE);
    b = JS_WriteObject(v.ctx, &bsize, pair, JS_WRITE_OBJ_REFERENCE);
    CHECK(a && b && asize > 8 && bsize > 3, "written");
    if (a && b && asize > 8 && bsize > 3) {
        /* header (version, no atoms), tag, element type, length, offset */
        memcpy(bytes, a, 6);
        /* the last two bytes of the pair: the tag of a reference and index 0 */
        bytes[6] = b[bsize - 2];
        bytes[7] = 0;
        r = JS_ReadObject(v.ctx, bytes, 8, JS_READ_OBJ_REFERENCE);
        CHECK(JS_IsException(r), "the reference to the typed array is refused");
        if (JS_IsException(r)) {
            JSValue e = JS_GetException(v.ctx);
            s = JS_ToCString(v.ctx, e);
            CHECK(s && strstr(s, "invalid object reference"), "error: %s", s ? s : "(none)");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, e);
        } else {
            JS_FreeValue(v.ctx, r);
        }
    }
    js_free(v.ctx, a);
    js_free(v.ctx, b);
    JS_FreeValue(v.ctx, ta);
    JS_FreeValue(v.ctx, pair);
    vm_free(&v);
}

/* The release callback of an ArrayBuffer made by JS_NewArrayBuffer runs once
   whichever way the buffer ends: detached by JS_DetachArrayBuffer, or
   transferred by script to a buffer of the same size (the new buffer keeps the
   callback) or of another size (the data is copied and released at once)
   (upstream QuickJS 813636e). */
static int release_count;
static void *release_ptr;

static void count_release(JSRuntime *rt, void *opaque, void *ptr)
{
    (void)rt;
    CHECK(opaque == (void *)&release_count, "opaque of the callback");
    release_count++;
    release_ptr = ptr;
    free(ptr);
}

static void test_array_buffer_release_once(void)
{
    static const char *const scripts[] = {
        NULL,                              /* JS_DetachArrayBuffer */
        "globalThis.t = ab.transfer()",    /* same size */
        "globalThis.t = ab.transfer(16)",  /* longer */
        "globalThis.t = ab.transfer(2)",   /* shorter */
    };
    int i;

    for (i = 0; i < (int)countof(scripts); i++) {
        VM v = vm_new();
        uint8_t *data = malloc(8);
        JSValue ab, g;

        memset(data, 7, 8);
        release_count = 0;
        ab = JS_NewArrayBuffer(v.ctx, data, 8, count_release, &release_count, FALSE);
        CHECK(!JS_IsException(ab), "buffer %d", i);
        g = JS_GetGlobalObject(v.ctx);
        CHECK(JS_SetPropertyStr(v.ctx, g, "ab", JS_DupValue(v.ctx, ab)) >= 0, "global ab");
        JS_FreeValue(v.ctx, g);
        if (scripts[i]) {
            JSValue r = JS_Eval(v.ctx, scripts[i], strlen(scripts[i]), "<unit>", JS_EVAL_TYPE_GLOBAL);

            CHECK(!JS_IsException(r), "script %d", i);
            if (JS_IsException(r))
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            JS_FreeValue(v.ctx, r);
        } else {
            JS_DetachArrayBuffer(v.ctx, ab);
            CHECK(release_count == 1, "detached: released %d times", release_count);
        }
        g = JS_GetGlobalObject(v.ctx);
        eval_ok(v.ctx, "delete globalThis.ab; delete globalThis.t");
        JS_FreeValue(v.ctx, g);
        JS_FreeValue(v.ctx, ab);
        JS_RunGC(v.rt);
        CHECK(release_count == 1, "case %d: released %d times", i, release_count);
        vm_free(&v);
    }
}

/* An ArrayBuffer of JS_NewArrayBuffer with no release callback keeps memory
   the host owns. Transferring it copies the bytes, or for the same size keeps
   the memory, and never calls the missing callback. */
static void test_array_buffer_no_release(void)
{
    static const char *const scripts[] = {
        "var t = ab.transfer(); [t.byteLength, new Uint8Array(t)[7]].join()",
        "var t = ab.transfer(16); [t.byteLength, new Uint8Array(t)[7], new Uint8Array(t)[15]].join()",
        "var t = ab.transfer(2); [t.byteLength, new Uint8Array(t)[1]].join()",
        "var t = ab.transfer(0); [t.byteLength, ab.detached].join()",
        "var t = ab.transferToFixedLength(4); [t.byteLength, new Uint8Array(t)[3]].join()",
    };
    static const char *const want[] = { "8,7", "16,7,0", "2,7", "0,true", "4,7" };
    static uint8_t data[8];
    int i;

    for (i = 0; i < (int)countof(scripts); i++) {
        VM v = vm_new();
        JSValue ab, g;

        memset(data, 7, sizeof(data));
        ab = JS_NewArrayBuffer(v.ctx, data, sizeof(data), NULL, NULL, FALSE);
        CHECK(!JS_IsException(ab), "buffer %d", i);
        g = JS_GetGlobalObject(v.ctx);
        CHECK(JS_SetPropertyStr(v.ctx, g, "ab", ab) >= 0, "global ab");
        JS_FreeValue(v.ctx, g);
        CHECK(eval_is(v.ctx, scripts[i], want[i]), "%s: want %s", scripts[i], want[i]);
        CHECK(eval_is(v.ctx, "ab.detached", "true"), "%s: the buffer is not detached", scripts[i]);
        CHECK(data[0] == 7 && data[7] == 7, "%s: the memory of the host changed", scripts[i]);
        vm_free(&v);
    }
}

/* JS_WRITE_OBJ_BYTECODE writes a frozen array as a template object, which the
   reader seals again, and any other array as an array. An array that was only
   made not extensible, or sealed, keeps a length and elements that can be
   written: before, the writer took every array that was not extensible for a
   template object, and it came back with its elements and its length read
   only. */
static void test_bytecode_array_round_trip(void)
{
    static const struct { const char *make, *check, *want; } cases[] = {
        { "Object.preventExtensions([])",
          "(function () { 'use strict'; a.length = 2; return a.length; })()", "2" },
        { "Object.preventExtensions([1, 2])",
          "(function () { 'use strict'; a[0] = 5; a.length = 1; return a.join(); })()", "5" },
        { "Object.seal([1, 2])",
          "(function () { 'use strict'; a[0] = 5; return a.join(); })()", "5,2" },
        { "Object.freeze([1, 2])",
          "[Object.isFrozen(a), a.join(), Object.getOwnPropertyDescriptor(a, 'length').writable].join()",
          "true,1,2,false" },
        { "Object.freeze([])", "[Object.isFrozen(a), a.length].join()", "true,0" },
    };
    int i;

    for (i = 0; i < (int)countof(cases); i++) {
        VM v = vm_new();
        JSValue val = JS_Eval(v.ctx, cases[i].make, strlen(cases[i].make), "<unit>", JS_EVAL_TYPE_GLOBAL);
        JSValue r, g;
        uint8_t *buf;
        size_t size;

        CHECK(!JS_IsException(val), "%s: the eval", cases[i].make);
        buf = JS_WriteObject(v.ctx, &size, val, JS_WRITE_OBJ_BYTECODE);
        JS_FreeValue(v.ctx, val);
        CHECK(buf != NULL, "%s: written", cases[i].make);
        if (!buf) {
            vm_free(&v);
            continue;
        }
        r = JS_ReadObject(v.ctx, buf, size, JS_READ_OBJ_BYTECODE);
        js_free(v.ctx, buf);
        CHECK(!JS_IsException(r), "%s: read", cases[i].make);
        g = JS_GetGlobalObject(v.ctx);
        CHECK(JS_SetPropertyStr(v.ctx, g, "a", r) >= 0, "global a");
        JS_FreeValue(v.ctx, g);
        CHECK(eval_is(v.ctx, cases[i].check, cases[i].want), "%s read back: %s, want %s",
              cases[i].make, cases[i].check, cases[i].want);
        vm_free(&v);
    }
}

/* JS_GetTypedArrayBuffer gives the byte length the typed array has now: a
   typed array that follows a resizable ArrayBuffer is as long as the buffer
   lets it be (upstream QuickJS 56d38b8). */
static void test_typed_array_buffer_length(void)
{
    static const struct {
        const char *make;
        size_t offset, length_before, length_after;
    } cases[] = {
        { "new Uint8Array(ab)", 0, 8, 12 },
        { "new Uint16Array(ab, 2)", 2, 6, 10 },
        { "new Uint8Array(ab, 0, 4)", 0, 4, 4 },
    };
    int i;

    for (i = 0; i < (int)countof(cases); i++) {
        VM v = vm_new();
        char src[200];
        JSValue r, buffer;
        size_t off = 99, len = 99, per = 99;

        snprintf(src, sizeof(src),
                 "var ab = new ArrayBuffer(8, { maxByteLength: 16 }); var ta = %s; ta", cases[i].make);
        r = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
        CHECK(!JS_IsException(r), "case %d", i);
        buffer = JS_GetTypedArrayBuffer(v.ctx, r, &off, &len, &per);
        CHECK(!JS_IsException(buffer) && off == cases[i].offset && len == cases[i].length_before,
              "case %d before: offset %d length %d", i, (int)off, (int)len);
        JS_FreeValue(v.ctx, buffer);
        JS_FreeValue(v.ctx, JS_Eval(v.ctx, "ab.resize(12)", 13, "<unit>", JS_EVAL_TYPE_GLOBAL));
        buffer = JS_GetTypedArrayBuffer(v.ctx, r, &off, &len, &per);
        CHECK(!JS_IsException(buffer) && len == cases[i].length_after,
              "case %d after resize: length %d, want %d", i, (int)len, (int)cases[i].length_after);
        JS_FreeValue(v.ctx, buffer);
        JS_FreeValue(v.ctx, r);
        vm_free(&v);
    }
}

int main(void)
{
    test_compile_stops();
    test_compile_names_linear();
    test_compile_names_closed_blocks();
    test_link_stops();
    test_module_eval_stops();
    test_link_retry();
    test_module_await_stops();
    test_module_await_species();
    test_module_fail_collect();
    test_job_stop_returns();
    test_entry_stop_returns();
    test_api_stop_returns();
    test_async_gen_drain();
    test_async_gen_stop_queue();
    test_await_stop_frees_frame();
    test_settle_depth_fails_call();
    test_stop_async_frees();
    test_for_in_keeps_shape();
    test_plain_objects();
    test_run_gc_threshold();
    test_gc_block_line();
    test_backtrace_shares_shape();
    test_backtrace_old_prototype();
    test_backtrace_native_read_caches();
    test_error_stack_memory();
    test_error_stack_top_level();
    test_proxy_own_property();
    test_json_bytes();
    test_host_function();
    test_host_release();
    test_release_order();
    test_release_order_live();
    test_module_top_level();
    test_new_string_chars();
    test_throw_error_kind();
    test_escape_text();
    test_read_truncated_bigint();
    test_regexp_bytecode_read();
    test_script_name_frames();
    test_weakref_kept();
    test_rv_far_locals();
    test_call_frames_measured();
    test_c_call_recursions();
    test_c_call_hop_checks();
    test_rv_new_in_loop();
    test_rv_frame_limits();
    test_rv_nesting_limits();
    test_template_object_sealed();
    test_read_null_reference();
    test_array_buffer_release_once();
    test_typed_array_buffer_length();
    test_array_buffer_no_release();
    test_bytecode_array_round_trip();
    if (failures) {
        printf("unit-api: %d of %ld checks failed\n", failures, checks);
        return 1;
    }
    printf("unit-api: %ld checks passed\n", checks);
    return 0;
}
