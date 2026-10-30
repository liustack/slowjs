/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* unit-api checks through the public API what scripts cannot see: what the
   runtime holds after errors and their stacks, what JS_ParseJSON does with
   bytes that are not UTF-8, and a property a host puts on a proxy. They took
   over the tests that did this on the engine before it (backtraces, JSON,
   error stacks, shared debug information).

     unit-api */
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
   0x7fffffff returns how many it got, and id 2 throws. */
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
    uint32_t vm, handles[8];
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
   after it does not start another. A threshold of -1 turns the collections of
   the runtime off, and JS_RunGC leaves it so. */
static void test_run_gc_threshold(void)
{
    size_t i, before[] = { 1, 1 << 20, (size_t)1 << 40 };
    VM v = vm_new();
    size_t live;

    for (i = 0; i < countof(before); i++) {
        JS_SetGCThreshold(v.rt, before[i]);
        JS_RunGC(v.rt);
        live = JS_GetMemorySize(v.rt);
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

int main(void)
{
    test_for_in_keeps_shape();
    test_plain_objects();
    test_run_gc_threshold();
    test_backtrace_shares_shape();
    test_backtrace_old_prototype();
    test_backtrace_native_read_caches();
    test_error_stack_memory();
    test_error_stack_top_level();
    test_proxy_own_property();
    test_json_bytes();
    test_host_function();
    test_host_release();
    test_module_top_level();
    test_new_string_chars();
    test_throw_error_kind();
    test_escape_text();
    if (failures) {
        printf("unit-api: %d of %ld checks failed\n", failures, checks);
        return 1;
    }
    printf("unit-api: %ld checks passed\n", checks);
    return 0;
}
